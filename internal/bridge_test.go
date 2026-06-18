package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// setupBridgeWithClient creates a Bridge with an active WebSocket client connected to it.
// Returns the bridge and the client-side connection (already cleaned up on t.Cleanup).
func setupBridgeWithClient(t *testing.T) (*Bridge, *websocket.Conn) {
	t.Helper()
	bridge := NewBridge()

	srv := httptest.NewServer(http.HandlerFunc(bridge.HandleUpgrade))
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	clientConn, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	t.Cleanup(func() { clientConn.Close(websocket.StatusNormalClosure, "") })

	// Poll until bridge registers the server-side connection.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if bridge.IsConnected() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !bridge.IsConnected() {
		t.Fatal("bridge not connected after 500ms")
	}

	return bridge, clientConn
}

// ── NewBridge ─────────────────────────────────────────────────────────────────

func TestNewBridge(t *testing.T) {
	b := NewBridge()
	if b == nil {
		t.Fatal("NewBridge returned nil")
	}
	if b.IsConnected() {
		t.Error("new bridge should not be connected")
	}
}

func TestBridgeHandleUpgrade_RequiresAuthToken(t *testing.T) {
	bridge := NewBridgeWithAuth("secret")
	srv := httptest.NewServer(http.HandlerFunc(bridge.HandleUpgrade))
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, resp, err := websocket.Dial(context.Background(), wsURL, nil)
	if err == nil {
		conn.Close(websocket.StatusNormalClosure, "") //nolint:errcheck
		t.Fatal("expected unauthenticated websocket dial to fail")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %v, want 401", resp)
	}

	conn, _, err = websocket.Dial(context.Background(), wsURL+"?token=secret", nil)
	if err != nil {
		t.Fatalf("authenticated ws dial: %v", err)
	}
	t.Cleanup(func() { conn.Close(websocket.StatusNormalClosure, "") })
}

// ── nextID ────────────────────────────────────────────────────────────────────

func TestBridgeNextID(t *testing.T) {
	b := NewBridge()
	id1 := b.nextID()
	id2 := b.nextID()

	if id1 == id2 {
		t.Error("consecutive IDs must be unique")
	}
	if !strings.HasPrefix(id1, "req-") {
		t.Errorf("ID %q does not have req- prefix", id1)
	}
	// Format: req-HHMMSS-N  (14 chars min: "req-000000-1")
	parts := strings.Split(id1, "-")
	if len(parts) != 3 {
		t.Errorf("ID %q has wrong format (want 3 dash-separated parts)", id1)
	}
}

// ── MarshalJSON ───────────────────────────────────────────────────────────────

func TestBridgeMarshalJSON_Disconnected(t *testing.T) {
	b := NewBridge()
	data, err := b.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	var m map[string]any
	json.Unmarshal(data, &m)
	if m["connected"] != false {
		t.Errorf("connected = %v, want false", m["connected"])
	}
	if m["pending"] != float64(0) {
		t.Errorf("pending = %v, want 0", m["pending"])
	}
}

func TestBridgeMarshalJSON_Connected(t *testing.T) {
	b, _ := setupBridgeWithClient(t)
	data, err := b.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	var m map[string]any
	json.Unmarshal(data, &m)
	if m["connected"] != true {
		t.Errorf("connected = %v, want true", m["connected"])
	}
}

// ── Close ─────────────────────────────────────────────────────────────────────

func TestBridgeClose_NoPanic(t *testing.T) {
	b := NewBridge()
	// Close on an unconnected bridge should not panic.
	b.Close()
}

func TestBridgeClose_DrainsPending(t *testing.T) {
	b, _ := setupBridgeWithClient(t)

	// Manually insert a pending entry so we can verify Close drains it.
	ch := make(chan BridgeResponse, 1)
	entry := &pendingEntry{ch: ch}
	entry.timer = time.AfterFunc(10*time.Second, func() {})

	b.mu.Lock()
	b.pending["test-id"] = entry
	b.mu.Unlock()

	b.Close()

	// Channel must be closed (receive returns zero value, ok=false).
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected channel to be closed")
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("timed out waiting for channel to be closed")
	}
}

// ── Send ─────────────────────────────────────────────────────────────────────

func TestBridgeSend_NotConnected(t *testing.T) {
	b := NewBridge()
	_, err := b.Send(context.Background(), "get_node", []string{"1:1"}, nil)
	if err == nil {
		t.Error("expected error when not connected")
	}
}

func TestBridgeSend_ContextCancelled(t *testing.T) {
	b, _ := setupBridgeWithClient(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := b.Send(ctx, "get_node", []string{"1:1"}, nil)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestBridgeSend_Success(t *testing.T) {
	b, clientConn := setupBridgeWithClient(t)
	ctx := context.Background()

	// Goroutine: echo request back as a successful response.
	go func() {
		var req BridgeRequest
		if err := wsjson.Read(ctx, clientConn, &req); err != nil {
			return
		}
		resp := BridgeResponse{
			RequestID: req.RequestID,
			Type:      req.Type,
			Data:      map[string]any{"id": "1:1", "name": "Frame 1"},
		}
		wsjson.Write(ctx, clientConn, resp) //nolint:errcheck
	}()

	got, err := b.Send(ctx, "get_node", []string{"1:1"}, nil)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got.Data == nil {
		t.Error("expected non-nil data in response")
	}
}

func TestBridgeSend_PluginError(t *testing.T) {
	b, clientConn := setupBridgeWithClient(t)
	ctx := context.Background()

	go func() {
		var req BridgeRequest
		if err := wsjson.Read(ctx, clientConn, &req); err != nil {
			return
		}
		resp := BridgeResponse{
			RequestID: req.RequestID,
			Error:     "node not found",
		}
		wsjson.Write(ctx, clientConn, resp) //nolint:errcheck
	}()

	got, err := b.Send(ctx, "get_node", []string{"9:9"}, nil)
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if got.Error == "" {
		t.Error("expected error field from plugin")
	}
}

func TestBridgeSend_Timeout(t *testing.T) {
	b, _ := setupBridgeWithClient(t)
	// Don't send any response from the client — bridge should time out.
	// We manipulate the timeout via a very short context rather than waiting 30s.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := b.Send(ctx, "get_node", []string{"1:1"}, nil)
	if err == nil {
		t.Error("expected timeout error")
	}
}

// ── IsConnected ───────────────────────────────────────────────────────────────

func TestBridgeIsConnected(t *testing.T) {
	b := NewBridge()
	if b.IsConnected() {
		t.Error("should not be connected before any upgrade")
	}

	b2, _ := setupBridgeWithClient(t)
	if !b2.IsConnected() {
		t.Error("should be connected after upgrade")
	}
}
