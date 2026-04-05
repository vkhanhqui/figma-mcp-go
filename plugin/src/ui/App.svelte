<script lang="ts">
  import { onMount } from "svelte";

  let connected = false;
  let fileName = "—";
  let pageName = "—";
  let selectionCount = 0;
  let activeRequests = new Set<string>();
  $: isWorking = activeRequests.size > 0;

  const WS_URL = "ws://localhost:1994/ws";
  const RECONNECT_DELAY_MS = 1500;

  let socket: WebSocket | null = null;
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;

  function connect() {
    if (socket) socket.close();
    socket = new WebSocket(WS_URL);

    socket.onopen = () => {
      connected = true;
      parent.postMessage({ pluginMessage: { type: "ui-ready" } }, "*");
    };

    socket.onclose = () => {
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

    socket.onerror = () => {
      connected = false;
    };

    socket.onmessage = (event) => {
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

  onMount(() => {
    window.addEventListener("message", handleMessage);
    connect();

    return () => {
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
    <div class="footer-right">
      <a
        class="bug-report"
        href="https://github.com/vkhanhqui/figma-mcp-go/issues/new"
        target="_blank"
        title="Report a bug"
      >
        <svg width="12" height="12" viewBox="0 0 16 16" fill="currentColor">
          <path d="M8 0a8 8 0 1 1 0 16A8 8 0 0 1 8 0ZM1.5 8a6.5 6.5 0 1 0 13 0 6.5 6.5 0 0 0-13 0Zm7-3.25v2.992l2.028.812.772-1.932-2.8-1.872ZM6.272 3.937 3.5 5.808l.772 1.932L6.3 6.928V3.873a.75.75 0 0 0-.028.064ZM8.75 9.75H7.25V11h1.5V9.75Zm0-5.5H7.25v4h1.5v-4Z"/>
        </svg>
        Bug report
      </a>
      <div class="badge" class:connected class:disconnected={!connected}>
        <span class="dot" class:connected></span>
        <span>{connected ? "Connected" : "Disconnected"}</span>
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
    align-items: center;
    justify-content: space-between;
  }

  .footer-right {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .bug-report {
    display: flex;
    align-items: center;
    gap: 4px;
    text-decoration: none;
    color: #888;
    font-size: 11px;
  }

  .bug-report:hover {
    color: #f87171;
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
