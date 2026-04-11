<script lang="ts">
  import { onMount } from "svelte";

  let connected = false;
  let fileName = "—";
  let pageName = "—";
  let selectionCount = 0;
  let activeRequests = new Set<string>();
  $: isWorking = activeRequests.size > 0;

  // Configurable server address.
  // Persisted via figma.clientStorage (through plugin core) because localStorage
  // is unavailable inside Figma's data: URL sandbox.
  let serverHost = "127.0.0.1";
  let serverPort = "1994";

  let showSettings = false;
  let editHost = serverHost;
  let editPort = serverPort;

  const RECONNECT_DELAY_MS = 1500;

  let socket: WebSocket | null = null;
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  let configLoaded = false;

  function connect() {
    // Detach the old handler before closing so its onclose doesn't fire
    // after we've already assigned a new socket, which would null out the
    // new reference and silently break the connection.
    if (socket) {
      socket.onclose = null;
      socket.close();
    }
    const ws = new WebSocket(`ws://${serverHost}:${serverPort}/ws`);
    socket = ws;

    ws.onopen = () => {
      connected = true;
      parent.postMessage({ pluginMessage: { type: "ui-ready" } }, "*");
    };

    ws.onclose = () => {
      if (socket !== ws) return; // stale handler — a newer connect() already took over
      connected = false;
      socket = null;
      activeRequests.clear();
      activeRequests = activeRequests;
      if (reconnectTimer === null) {
        reconnectTimer = setTimeout(() => {
          reconnectTimer = null;
          connect();
        }, RECONNECT_DELAY_MS);
      }
    };

    ws.onerror = () => {
      connected = false;
    };

    ws.onmessage = (event) => {
      try {
        const payload = JSON.parse(event.data);
        if (payload.requestId) {
          activeRequests.add(payload.requestId);
          activeRequests = activeRequests;
        }
        parent.postMessage({ pluginMessage: { type: "server-request", payload } }, "*");
      } catch {
        // ignore malformed frames
      }
    };
  }

  function handleMessage(event: MessageEvent) {
    const msg = event.data?.pluginMessage;
    if (!msg) return;

    if (msg.type === "ws_config") {
      serverHost = msg.host ?? "127.0.0.1";
      serverPort = msg.port ?? "1994";
      if (!configLoaded) {
        configLoaded = true;
        connect();
      }
      return;
    }

    if (msg.type === "plugin-status") {
      fileName = msg.payload.fileName;
      pageName = msg.payload.pageName ?? "—";
      selectionCount = msg.payload.selectionCount;
      return;
    }

    if ("requestId" in msg) {
      if (msg.type !== "progress_update") {
        activeRequests.delete(msg.requestId);
        activeRequests = activeRequests;
      }
      if (socket?.readyState === WebSocket.OPEN) {
        socket.send(JSON.stringify(msg));
      }
    }
  }

  function openSettings() {
    editHost = serverHost;
    editPort = serverPort;
    showSettings = true;
  }

  function applySettings() {
    serverHost = editHost.trim() || "127.0.0.1";
    const p = parseInt(editPort, 10);
    serverPort = p > 0 && p <= 65535 ? String(p) : "1994";
    // Persist via plugin core (figma.clientStorage), since localStorage is
    // unavailable in Figma's data: URL environment.
    parent.postMessage(
      { pluginMessage: { type: "save_ws_config", host: serverHost, port: serverPort } },
      "*"
    );
    showSettings = false;
    // Cancel any pending reconnect and reconnect immediately with the new address.
    if (reconnectTimer !== null) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
    connect();
  }

  function handleKeydown(event: KeyboardEvent) {
    if (event.key === "Enter") applySettings();
    if (event.key === "Escape") showSettings = false;
  }

  onMount(() => {
    window.addEventListener("message", handleMessage);

    // Request stored config from plugin core (responds with ws_config message).
    // connect() is called once we receive the response.
    parent.postMessage({ pluginMessage: { type: "get_ws_config" } }, "*");

    // Fallback: if the plugin core doesn't respond within 500 ms (e.g. during
    // dev / hot-reload without a running core), connect with defaults.
    const fallback = setTimeout(() => {
      if (!configLoaded) {
        configLoaded = true;
        connect();
      }
    }, 500);

    return () => {
      clearTimeout(fallback);
      window.removeEventListener("message", handleMessage);
      if (reconnectTimer !== null) clearTimeout(reconnectTimer);
      if (socket) socket.close();
    };
  });
</script>

<div class="container">
  <div class="info-section">
    <div class="info-row">
      <span class="info-label">File</span>
      <span class="info-value" title={fileName}>{fileName}</span>
    </div>
    <div class="info-row">
      <span class="info-label">Page</span>
      <span class="info-value" title={pageName}>{pageName}</span>
    </div>
    <div class="info-row">
      <span class="info-label">Selection</span>
      <span class="info-value">{selectionCount} node(s)</span>
    </div>
  </div>
  {#if isWorking}
    <div class="working-banner">
      <span class="spinner"></span>
      <span>AI is working…</span>
    </div>
  {/if}
  <div class="footer">
    <!-- Row 1: server address (left) + connection badge (right) -->
    <div class="footer-row">
      {#if showSettings}
        <div class="settings-panel">
          <input
            class="addr-input"
            bind:value={editHost}
            placeholder="127.0.0.1"
            on:keydown={handleKeydown}
          />
          <span class="addr-sep">:</span>
          <input
            class="port-input"
            bind:value={editPort}
            placeholder="1994"
            on:keydown={handleKeydown}
          />
          <button class="apply-btn" on:click={applySettings} title="Apply">✓</button>
          <button class="cancel-btn" on:click={() => showSettings = false} title="Cancel">✕</button>
        </div>
      {:else}
        <button
          class="server-addr"
          on:click={openSettings}
          title="Click to configure server address"
        >{serverHost}:{serverPort}</button>
      {/if}
      <div class="badge" class:connected class:disconnected={!connected}>
        <span class="dot" class:connected></span>
        <span>{connected ? "Connected" : "Disconnected"}</span>
      </div>
    </div>
    <!-- Row 2: author (left) + bug report + feature suggestion (right) -->
    <div class="footer-row">
      <a
        class="author"
        href="https://github.com/vkhanhqui/figma-mcp-go"
        target="_blank"
      >
        <img
          src="https://avatars.githubusercontent.com/u/64468109?v=4"
          alt="avatar"
        />
        vkhanhqui
      </a>
      <div class="links">
        <a
          class="footer-link"
          href="https://github.com/vkhanhqui/figma-mcp-go/issues/new?labels=bug"
          target="_blank"
          title="Report a bug"
        >
          <svg width="12" height="12" viewBox="0 0 16 16" fill="currentColor">
            <path d="M8 0a8 8 0 1 1 0 16A8 8 0 0 1 8 0ZM1.5 8a6.5 6.5 0 1 0 13 0 6.5 6.5 0 0 0-13 0Zm7-3.25v2.992l2.028.812.772-1.932-2.8-1.872ZM6.272 3.937 3.5 5.808l.772 1.932L6.3 6.928V3.873a.75.75 0 0 0-.028.064ZM8.75 9.75H7.25V11h1.5V9.75Zm0-5.5H7.25v4h1.5v-4Z"/>
          </svg>
          Bug
        </a>
        <a
          class="footer-link"
          href="https://github.com/vkhanhqui/figma-mcp-go/issues/new?labels=enhancement&title=Feature+request%3A+"
          target="_blank"
          title="Suggest a feature"
        >
          <svg width="12" height="12" viewBox="0 0 16 16" fill="currentColor">
            <path d="M8 1.5c-2.363 0-4 1.69-4 3.75 0 .984.424 1.625.984 2.304l.214.253c.223.264.47.556.673.848.284.411.537.896.621 1.49a.75.75 0 0 1-1.484.211c-.04-.282-.163-.547-.37-.847a8.456 8.456 0 0 0-.542-.68c-.084-.1-.173-.205-.268-.32C3.201 7.75 2.5 6.766 2.5 5.25 2.5 2.31 4.863 0 8 0s5.5 2.31 5.5 5.25c0 1.516-.701 2.5-1.328 3.259-.095.115-.184.22-.268.319-.207.245-.383.453-.541.681-.208.3-.33.565-.37.847a.751.751 0 0 1-1.485-.212c.084-.593.337-1.078.621-1.489.203-.292.45-.584.673-.848.075-.088.147-.173.213-.253.561-.679.985-1.32.985-2.304 0-2.06-1.637-3.75-4-3.75ZM5.75 12h4.5a.75.75 0 0 1 0 1.5h-4.5a.75.75 0 0 1 0-1.5ZM6 14.25a.75.75 0 0 1 .75-.75h2.5a.75.75 0 0 1 0 1.5h-2.5a.75.75 0 0 1-.75-.75Z"/>
          </svg>
          Suggest
        </a>
      </div>
    </div>
  </div>
</div>

<style>
  :global(*) {
    box-sizing: border-box;
    margin: 0;
    padding: 0;
  }

  :global(body) {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
    font-size: 12px;
    background: #1e1e1e;
    color: #e0e0e0;
    height: 100vh;
  }

  .container {
    display: flex;
    flex-direction: column;
    height: 100%;
    padding: 16px;
    gap: 12px;
  }

  .info-section {
    display: flex;
    flex-direction: column;
    gap: 8px;
    flex: 1;
  }

  .info-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .info-label {
    color: #888;
  }

  .info-value {
    color: #e0e0e0;
    font-weight: 500;
    max-width: 180px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .working-banner {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 6px 10px;
    background: #1a2e3a;
    border: 1px solid #2563eb44;
    border-radius: 8px;
    color: #60a5fa;
    font-size: 11px;
    font-weight: 500;
  }

  .spinner {
    width: 10px;
    height: 10px;
    border: 2px solid #60a5fa44;
    border-top-color: #60a5fa;
    border-radius: 50%;
    animation: spin 0.7s linear infinite;
    flex-shrink: 0;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  .footer {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .footer-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
  }

  .links {
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .footer-link {
    display: flex;
    align-items: center;
    gap: 4px;
    text-decoration: none;
    color: #888;
    font-size: 11px;
  }

  .footer-link:hover {
    color: #e0e0e0;
  }

  .author {
    display: flex;
    align-items: center;
    gap: 6px;
    text-decoration: none;
    color: #888;
    font-size: 11px;
  }

  .author:hover {
    color: #e0e0e0;
  }

  .author img {
    width: 20px;
    height: 20px;
    border-radius: 50%;
  }

  /* Server address button — shows current host:port, click to edit */
  .server-addr {
    background: none;
    border: none;
    color: #666;
    font-size: 10px;
    font-family: monospace;
    cursor: pointer;
    padding: 2px 4px;
    border-radius: 4px;
  }

  .server-addr:hover {
    color: #aaa;
    background: #2a2a2a;
  }

  /* Inline settings panel — takes remaining space so inputs aren't squished */
  .settings-panel {
    display: flex;
    align-items: center;
    gap: 6px;
    flex: 1;
  }

  .addr-input {
    width: 72px;
    background: #2a2a2a;
    border: 1px solid #444;
    border-radius: 4px;
    color: #e0e0e0;
    font-size: 10px;
    font-family: monospace;
    padding: 2px 4px;
    outline: none;
  }

  .addr-input:focus {
    border-color: #555;
  }

  .port-input {
    width: 36px;
    background: #2a2a2a;
    border: 1px solid #444;
    border-radius: 4px;
    color: #e0e0e0;
    font-size: 10px;
    font-family: monospace;
    padding: 2px 4px;
    outline: none;
  }

  .port-input:focus {
    border-color: #555;
  }

  .addr-sep {
    color: #666;
    font-size: 10px;
  }

  .apply-btn,
  .cancel-btn {
    background: none;
    border: none;
    cursor: pointer;
    font-size: 11px;
    padding: 1px 3px;
    border-radius: 3px;
  }

  .apply-btn {
    color: #4ade80;
  }

  .apply-btn:hover {
    background: #1a3a2a;
  }

  .cancel-btn {
    color: #f87171;
  }

  .cancel-btn:hover {
    background: #3a1a1a;
  }

  .badge {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 4px 10px;
    border-radius: 12px;
    font-size: 11px;
    font-weight: 600;
  }

  .badge.connected {
    background: #1a472a;
    color: #4ade80;
  }

  .badge.disconnected {
    background: #3a1a1a;
    color: #f87171;
  }

  .dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: #f87171;
  }

  .dot.connected {
    background: #4ade80;
  }
</style>
