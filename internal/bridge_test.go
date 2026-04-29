package internal

import (
	"bytes"
	"context"
	"encoding/base64"
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
	// Match the server-side read cap so chunked transfers can land in tests.
	clientConn.SetReadLimit(100 * 1024 * 1024)
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

// ── nextID ────────────────────────────────────────────────────────────────────

func TestBridgeNextID(t *testing.T) {
	b := NewBridge()
	id1 := b.nextID()
	id2 := b.nextID()

	if id1 == id2 {
		t.Error("consecutive IDs must be unique")
	}
	// ULID format: 26 characters of Crockford base32 (uppercase + digits).
	if len(id1) != 26 {
		t.Errorf("ID %q has length %d, want 26 (ULID)", id1, len(id1))
	}
	// Monotonic ULIDs from the same instant must sort lexicographically.
	if id1 >= id2 {
		t.Errorf("expected id1 < id2 for monotonic ULID (got %q, %q)", id1, id2)
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

// ── Hello handshake ───────────────────────────────────────────────────────────

func TestBridgeHello_RecordsPluginInfo(t *testing.T) {
	b, clientConn := setupBridgeWithClient(t)
	ctx := context.Background()

	// Plugin sends hello — bridge should record but not respond.
	hello := BridgeResponse{
		Type:          "hello",
		PluginVersion: "1.1.0",
		Capabilities:  []string{"a", "b"},
	}
	if err := wsjson.Write(ctx, clientConn, hello); err != nil {
		t.Fatalf("write hello: %v", err)
	}

	// Poll PluginInfo until populated.
	deadline := time.Now().Add(500 * time.Millisecond)
	var version string
	var caps []string
	for time.Now().Before(deadline) {
		version, caps = b.PluginInfo()
		if version != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if version != "1.1.0" {
		t.Errorf("PluginInfo version = %q, want 1.1.0", version)
	}
	if len(caps) != 2 || caps[0] != "a" || caps[1] != "b" {
		t.Errorf("PluginInfo caps = %v, want [a b]", caps)
	}
}

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

// ── Binary frames ─────────────────────────────────────────────────────────────

// TestBridgeSend_BinaryFrame_ImportImage verifies that when the plugin
// advertises binary_frames_v1, the bridge encodes import_image as a binary
// frame, strips imageData from the JSON metadata, and the plugin sees the
// raw bytes in the binary payload.
func TestBridgeSend_BinaryFrame_ImportImage(t *testing.T) {
	b, clientConn := setupBridgeWithClient(t)
	ctx := context.Background()

	// Plugin sends hello with binary capability.
	hello := BridgeResponse{
		Type:          "hello",
		PluginVersion: "1.1.0",
		Capabilities:  []string{CapabilityBinaryFrames},
	}
	if err := wsjson.Write(ctx, clientConn, hello); err != nil {
		t.Fatalf("write hello: %v", err)
	}
	// Wait for bridge to register the capability.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		_, caps := b.PluginInfo()
		if len(caps) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Read whatever the bridge sends; expect binary message.
	gotFrame := make(chan *binaryFrame, 1)
	go func() {
		typ, data, err := clientConn.Read(ctx)
		if err != nil {
			t.Errorf("read: %v", err)
			return
		}
		if typ != websocket.MessageBinary {
			t.Errorf("expected binary message, got %v", typ)
			return
		}
		f, err := decodeBinaryFrame(data)
		if err != nil {
			t.Errorf("decode: %v", err)
			return
		}
		gotFrame <- f

		// Echo back a successful response.
		resp := BridgeResponse{Type: "import_image", Data: map[string]any{"id": "1:1"}}
		// Need to grab requestId from the frame to match.
		var meta binaryRequestMeta
		_ = json.Unmarshal(f.meta, &meta)
		resp.RequestID = meta.RequestID
		wsjson.Write(ctx, clientConn, resp) //nolint:errcheck
	}()

	// "AB" base64
	_, err := b.Send(ctx, "import_image", nil, map[string]interface{}{"imageData": "QUI=", "x": 5})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	select {
	case f := <-gotFrame:
		if !bytes.Equal(f.payload, []byte("AB")) {
			t.Errorf("payload bytes = %v, want %v", f.payload, []byte("AB"))
		}
		var meta binaryRequestMeta
		if err := json.Unmarshal(f.meta, &meta); err != nil {
			t.Fatalf("unmarshal meta: %v", err)
		}
		if _, has := meta.Params["imageData"]; has {
			t.Error("imageData should be stripped from metadata")
		}
		if meta.PayloadField != "params.imageData" {
			t.Errorf("payloadField = %q", meta.PayloadField)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for binary frame")
	}
}

// TestBridgeSend_TextFallback_WithoutCapability verifies that a plugin that
// does NOT advertise binary_frames_v1 still gets text JSON for import_image.
func TestBridgeSend_TextFallback_WithoutCapability(t *testing.T) {
	b, clientConn := setupBridgeWithClient(t)
	ctx := context.Background()

	hello := BridgeResponse{Type: "hello", PluginVersion: "1.0.0", Capabilities: []string{"image_cache_v1"}}
	wsjson.Write(ctx, clientConn, hello) //nolint:errcheck

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		_, caps := b.PluginInfo()
		if len(caps) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	gotText := make(chan bool, 1)
	go func() {
		typ, _, err := clientConn.Read(ctx)
		if err != nil {
			return
		}
		gotText <- typ == websocket.MessageText
	}()

	// Don't actually wait for response — just verify the wire type.
	go func() { b.Send(ctx, "import_image", nil, map[string]interface{}{"imageData": "QUI="}) }() //nolint:errcheck

	select {
	case isText := <-gotText:
		if !isText {
			t.Error("expected text frame when binary capability missing")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("no message received")
	}
}

// TestBridgeReadLoop_DecodesBinaryResponse verifies that the bridge can
// receive a binary response from the plugin, splice payload slices into
// data, and resolve the pending request with the correct bytes.
func TestBridgeReadLoop_DecodesBinaryResponse(t *testing.T) {
	b, clientConn := setupBridgeWithClient(t)
	ctx := context.Background()

	// Hello with binary cap.
	wsjson.Write(ctx, clientConn, BridgeResponse{ //nolint:errcheck
		Type:         "hello",
		Capabilities: []string{CapabilityBinaryFrames},
	})

	go func() {
		// Consume the import_image request from the bridge…
		typ, data, err := clientConn.Read(ctx)
		if err != nil {
			return
		}
		var requestID string
		if typ == websocket.MessageBinary {
			f, _ := decodeBinaryFrame(data)
			var meta binaryRequestMeta
			_ = json.Unmarshal(f.meta, &meta)
			requestID = meta.RequestID
		} else {
			var req BridgeRequest
			_ = json.Unmarshal(data, &req)
			requestID = req.RequestID
		}

		// …then send a binary response with payload slices.
		respMeta := binaryResponseMeta{
			Type:      "import_image",
			RequestID: requestID,
			Data: map[string]interface{}{
				"image": map[string]interface{}{"id": "abc"},
			},
			PayloadSlices: []payloadSlice{
				{Path: "data.image.bytes", Length: 4},
			},
		}
		frame, _ := encodeBinaryFrame(msgTypeResp, 0, respMeta, []byte{0xDE, 0xAD, 0xBE, 0xEF})
		clientConn.Write(ctx, websocket.MessageBinary, frame) //nolint:errcheck
	}()

	resp, err := b.Send(ctx, "import_image", nil, map[string]interface{}{"imageData": "QUI="})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	got := resp.Data.(map[string]interface{})["image"].(map[string]interface{})["bytes"].([]byte)
	if !bytes.Equal(got, []byte{0xDE, 0xAD, 0xBE, 0xEF}) {
		t.Errorf("spliced bytes wrong: %v", got)
	}
}

// TestBridgeSend_ChunksLargeBinaryFrame verifies the writer splits oversized
// binary frames into msgTypeChunk pieces when the plugin advertises
// chunking_v1, and each piece round-trips through decodeBinaryFrame.
func TestBridgeSend_ChunksLargeBinaryFrame(t *testing.T) {
	b, clientConn := setupBridgeWithClient(t)
	ctx := context.Background()

	hello := BridgeResponse{
		Type:         "hello",
		Capabilities: []string{CapabilityBinaryFrames, CapabilityChunking},
	}
	if err := wsjson.Write(ctx, clientConn, hello); err != nil {
		t.Fatalf("hello: %v", err)
	}
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		_, caps := b.PluginInfo()
		if len(caps) >= 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Build a 2MB raw payload then base64-encode it. Once decoded by
	// encodeBinaryRequest, the binary frame easily exceeds the 1MB chunk
	// threshold. (Concatenating padded blocks like "QUJDRA==" produces
	// invalid base64 — pad once at the very end.)
	rawPayload := bytes.Repeat([]byte{0x7E}, 2*1024*1024)
	imageData := base64.StdEncoding.EncodeToString(rawPayload)

	gotChunks := make(chan int, 1)
	asm := newChunkAssembler()
	go func() {
		seen := 0
		for {
			typ, data, err := clientConn.Read(ctx)
			if err != nil {
				return
			}
			if typ != websocket.MessageBinary {
				continue
			}
			f, err := decodeBinaryFrame(data)
			if err != nil {
				t.Errorf("decode: %v", err)
				return
			}
			if f.msgType != msgTypeChunk {
				t.Errorf("expected chunk msgType, got 0x%02x", f.msgType)
				return
			}
			meta, err := extractChunkMeta(f)
			if err != nil {
				t.Errorf("chunk meta: %v", err)
				return
			}
			seen++
			out, err := asm.receive(meta, f.payload)
			if err != nil {
				t.Errorf("assemble: %v", err)
				return
			}
			if out == nil {
				continue
			}
			full, err := decodeBinaryFrame(out)
			if err != nil {
				t.Errorf("assembled decode: %v", err)
				return
			}
			var rmeta binaryRequestMeta
			if err := json.Unmarshal(full.meta, &rmeta); err != nil {
				t.Errorf("meta unmarshal: %v", err)
				return
			}
			gotChunks <- seen
			resp := BridgeResponse{Type: "import_image", RequestID: rmeta.RequestID, Data: map[string]any{"id": "1:1"}}
			wsjson.Write(ctx, clientConn, resp) //nolint:errcheck
			return
		}
	}()

	if _, err := b.Send(ctx, "import_image", nil, map[string]interface{}{"imageData": imageData}); err != nil {
		t.Fatalf("Send: %v", err)
	}

	select {
	case n := <-gotChunks:
		if n < 2 {
			t.Errorf("expected multiple chunks, got %d", n)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for chunks")
	}
}

// TestBridgeReadLoop_AssemblesChunks verifies the readLoop reassembles a
// chunked binary response back into a single BridgeResponse with the payload
// spliced into the right field. Mirrors the plugin → server side.
func TestBridgeReadLoop_AssemblesChunks(t *testing.T) {
	b, clientConn := setupBridgeWithClient(t)
	ctx := context.Background()

	wsjson.Write(ctx, clientConn, BridgeResponse{ //nolint:errcheck
		Type:         "hello",
		Capabilities: []string{CapabilityBinaryFrames, CapabilityChunking},
	})

	go func() {
		typ, data, err := clientConn.Read(ctx)
		if err != nil {
			return
		}
		var requestID string
		if typ == websocket.MessageBinary {
			f, _ := decodeBinaryFrame(data)
			var meta binaryRequestMeta
			_ = json.Unmarshal(f.meta, &meta)
			requestID = meta.RequestID
		} else {
			var req BridgeRequest
			_ = json.Unmarshal(data, &req)
			requestID = req.RequestID
		}

		payload := bytes.Repeat([]byte{0x33}, 1536*1024)
		respMeta := binaryResponseMeta{
			Type:      "get_screenshot",
			RequestID: requestID,
			Data: map[string]interface{}{
				"exports": []interface{}{
					map[string]interface{}{"nodeId": "1:1"},
				},
			},
			PayloadSlices: []payloadSlice{
				{Path: "data.exports.0.bytes", Length: len(payload)},
			},
		}
		fullFrame, _ := encodeBinaryFrame(msgTypeResp, 0, respMeta, payload)
		chunks, err := splitIntoChunks(requestID, fullFrame)
		if err != nil {
			t.Errorf("split: %v", err)
			return
		}
		for _, c := range chunks {
			if err := clientConn.Write(ctx, websocket.MessageBinary, c); err != nil {
				t.Errorf("write chunk: %v", err)
				return
			}
		}
	}()

	resp, err := b.Send(ctx, "get_screenshot", nil, nil)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	exports := resp.Data.(map[string]interface{})["exports"].([]interface{})
	first := exports[0].(map[string]interface{})
	got := first["bytes"].([]byte)
	if len(got) != 1536*1024 {
		t.Errorf("got %d bytes, want %d", len(got), 1536*1024)
	}
	for i, c := range got {
		if c != 0x33 {
			t.Fatalf("byte %d = 0x%02x, want 0x33", i, c)
		}
	}
}
