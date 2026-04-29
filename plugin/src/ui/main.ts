// UI iframe — vanilla TS forwarder. Replaces the Svelte runtime so the bundle
// is small and the message hot path doesn't pay reactivity overhead.
//
// Responsibilities:
//   1. Hold the WebSocket to the Go server (the plugin sandbox can't open one
//      from main.ts, only from the UI iframe).
//   2. Forward server → plugin and plugin → server messages.
//   3. Show a minimal status (file/page/selection, connection badge,
//      "AI is working…" banner) and a settings panel for host/port.
//
// Bumped on every plugin behaviour change the server can branch on.
const PLUGIN_VERSION = "1.2.0";
const PLUGIN_CAPABILITIES = [
  "image_cache_v1",
  "import_images_batch",
  "export_nodes_batch",
  "binary_frames_v1",
  "chunking_v1",
];
const RECONNECT_DELAY_MS = 1500;

type PluginMessage = Record<string, unknown> & {
  type?: string;
  requestId?: string;
};

const $ = <T extends HTMLElement>(id: string) => document.getElementById(id) as T;

const fileNameEl = $<HTMLSpanElement>("file-name");
const pageNameEl = $<HTMLSpanElement>("page-name");
const selectionCountEl = $<HTMLSpanElement>("selection-count");
const workingBanner = $<HTMLDivElement>("working-banner");
const serverAddrBtn = $<HTMLButtonElement>("server-addr");
const settingsPanel = $<HTMLDivElement>("settings-panel");
const addrInput = $<HTMLInputElement>("addr-input");
const portInput = $<HTMLInputElement>("port-input");
const applyBtn = $<HTMLButtonElement>("apply-btn");
const cancelBtn = $<HTMLButtonElement>("cancel-btn");
const connectionBadge = $<HTMLDivElement>("connection-badge");
const connectionText = $<HTMLSpanElement>("connection-text");

let serverHost = "127.0.0.1";
let serverPort = "1994";
let configLoaded = false;
let socket: WebSocket | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
const activeRequests = new Set<string>();

const setConnected = (connected: boolean) => {
  connectionBadge.classList.toggle("connected", connected);
  connectionBadge.classList.toggle("disconnected", !connected);
  connectionBadge.querySelector(".dot")?.classList.toggle("connected", connected);
  connectionText.textContent = connected ? "Connected" : "Disconnected";
};

const refreshWorkingBanner = () => {
  workingBanner.classList.toggle("visible", activeRequests.size > 0);
};

const updateAddrLabel = () => {
  serverAddrBtn.textContent = `${serverHost}:${serverPort}`;
};

const connect = () => {
  // Detach the old handler before closing so its onclose doesn't fire after
  // we've assigned a new socket, which would silently break the new one.
  if (socket) {
    socket.onclose = null;
    socket.close();
  }
  const ws = new WebSocket(`ws://${serverHost}:${serverPort}/ws`);
  // Binary frames carry image / PDF bytes without base64 inflation. Plugin
  // core decodes them into a regular request shape before dispatching.
  ws.binaryType = "arraybuffer";
  socket = ws;

  ws.onopen = () => {
    setConnected(true);
    // Hello handshake — first frame after the socket is up. Lets the server
    // log the plugin version and decide whether to enable forward features.
    try {
      ws.send(
        JSON.stringify({
          type: "hello",
          pluginVersion: PLUGIN_VERSION,
          capabilities: PLUGIN_CAPABILITIES,
        })
      );
    } catch {
      // Fail open — server tolerates a missing hello.
    }
    parent.postMessage({ pluginMessage: { type: "ui-ready" } }, "*");
  };

  ws.onclose = () => {
    if (socket !== ws) return; // stale handler — newer connect() took over
    setConnected(false);
    socket = null;
    activeRequests.clear();
    refreshWorkingBanner();
    if (reconnectTimer === null) {
      reconnectTimer = setTimeout(() => {
        reconnectTimer = null;
        connect();
      }, RECONNECT_DELAY_MS);
    }
  };

  ws.onerror = () => setConnected(false);

  ws.onmessage = (event) => {
    if (event.data instanceof ArrayBuffer) {
      // Binary frame — plugin core takes it as-is and parses our FMCP wire
      // format. We don't try to peek at the request id from here because
      // that would mean parsing the frame twice; the working-banner gets
      // turned on by the plugin core via `progress_update` messages anyway
      // for long-running tools.
      const payload = new Uint8Array(event.data);
      parent.postMessage(
        { pluginMessage: { type: "server-binary", payload } },
        "*"
      );
      return;
    }
    try {
      const payload = JSON.parse(event.data) as PluginMessage;
      if (typeof payload.requestId === "string") {
        activeRequests.add(payload.requestId);
        refreshWorkingBanner();
      }
      parent.postMessage(
        { pluginMessage: { type: "server-request", payload } },
        "*"
      );
    } catch {
      // Drop malformed frames — server should never send them.
    }
  };
};

const handlePluginMessage = (event: MessageEvent) => {
  const msg = (event.data?.pluginMessage ?? null) as PluginMessage | null;
  if (!msg) return;

  if (msg.type === "ws_config") {
    serverHost = (msg.host as string) ?? "127.0.0.1";
    serverPort = (msg.port as string) ?? "1994";
    updateAddrLabel();
    if (!configLoaded) {
      configLoaded = true;
      connect();
    }
    return;
  }

  if (msg.type === "plugin-status") {
    const payload = msg.payload as { fileName?: string; pageName?: string; selectionCount?: number };
    fileNameEl.textContent = payload.fileName ?? "—";
    fileNameEl.title = fileNameEl.textContent;
    pageNameEl.textContent = payload.pageName ?? "—";
    pageNameEl.title = pageNameEl.textContent;
    selectionCountEl.textContent = `${payload.selectionCount ?? 0} node(s)`;
    return;
  }

  // Plugin core handed us a pre-encoded binary frame to forward to the server
  // (e.g. export_* responses with raw image bytes). The frame has its own
  // requestId; we clear the working banner based on the matching id supplied
  // alongside the bytes. `payloads` carries an array of chunked frames for
  // payloads above the 1MB chunking threshold; each chunk is sent as its own
  // WebSocket binary message so the server's writer can interleave them.
  if (msg.type === "send-binary") {
    const id = typeof msg.requestId === "string" ? msg.requestId : "";
    if (id) {
      activeRequests.delete(id);
      refreshWorkingBanner();
    }
    if (socket?.readyState !== WebSocket.OPEN) return;
    const sendOne = (payload: Uint8Array | ArrayBuffer) => {
      // Browsers accept either ArrayBuffer or typed array; copy into a
      // standalone buffer so the structured-clone version we received from
      // postMessage doesn't keep the plugin's heap pinned.
      const buf =
        payload instanceof ArrayBuffer
          ? payload
          : (payload as Uint8Array).slice().buffer;
      socket.send(buf);
    };
    const payloads = msg.payloads as
      | (Uint8Array | ArrayBuffer)[]
      | undefined;
    if (payloads && payloads.length > 0) {
      for (const p of payloads) sendOne(p);
      return;
    }
    const payload = msg.payload as Uint8Array | ArrayBuffer | undefined;
    if (payload) sendOne(payload);
    return;
  }

  if (typeof msg.requestId === "string") {
    if (msg.type !== "progress_update") {
      activeRequests.delete(msg.requestId);
      refreshWorkingBanner();
    }
    if (socket?.readyState === WebSocket.OPEN) {
      socket.send(JSON.stringify(msg));
    }
  }
};

const openSettings = () => {
  addrInput.value = serverHost;
  portInput.value = serverPort;
  serverAddrBtn.style.display = "none";
  settingsPanel.classList.add("visible");
  addrInput.focus();
};

const closeSettings = () => {
  settingsPanel.classList.remove("visible");
  serverAddrBtn.style.display = "";
};

const applySettings = () => {
  serverHost = addrInput.value.trim() || "127.0.0.1";
  const p = parseInt(portInput.value, 10);
  serverPort = p > 0 && p <= 65535 ? String(p) : "1994";
  updateAddrLabel();
  parent.postMessage(
    { pluginMessage: { type: "save_ws_config", host: serverHost, port: serverPort } },
    "*"
  );
  closeSettings();
  if (reconnectTimer !== null) {
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }
  connect();
};

serverAddrBtn.addEventListener("click", openSettings);
applyBtn.addEventListener("click", applySettings);
cancelBtn.addEventListener("click", closeSettings);
[addrInput, portInput].forEach((input) => {
  input.addEventListener("keydown", (e) => {
    if (e.key === "Enter") applySettings();
    if (e.key === "Escape") closeSettings();
  });
});

window.addEventListener("message", handlePluginMessage);
window.addEventListener("beforeunload", () => {
  window.removeEventListener("message", handlePluginMessage);
  if (reconnectTimer !== null) clearTimeout(reconnectTimer);
  if (socket) socket.close();
});

// Request stored config from plugin core (responds with ws_config message).
// connect() is called once we receive the response.
parent.postMessage({ pluginMessage: { type: "get_ws_config" } }, "*");

// Fallback: if the plugin core doesn't respond within 500 ms (e.g. during
// dev / hot-reload without a running core), connect with defaults.
setTimeout(() => {
  if (!configLoaded) {
    configLoaded = true;
    connect();
  }
}, 500);

updateAddrLabel();
