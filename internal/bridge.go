package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

var bridgeLogger = log.New(os.Stderr, "[bridge] ", 0)

// pendingEntry holds the response channel and inactivity timer for an in-flight request.
type pendingEntry struct {
	ch    chan BridgeResponse
	timer *time.Timer
	once  sync.Once // guards channel close/send — prevents panic on concurrent timeout + response
}

// Bridge manages the single WebSocket connection from the Figma plugin
// and matches responses to pending requests via request IDs.
type Bridge struct {
	mu      sync.RWMutex
	wmu     sync.Mutex // serialises concurrent WebSocket writes (coder/websocket does not support concurrent writes)
	conn    *websocket.Conn
	pending map[string]*pendingEntry
	counter atomic.Int64
}

// NewBridge creates a ready-to-use Bridge.
func NewBridge() *Bridge {
	return &Bridge{
		pending: make(map[string]*pendingEntry),
	}
}

// HandleUpgrade upgrades an HTTP request to a WebSocket connection.
// Only one plugin connection is maintained at a time; a new connection
// replaces the old one (same behaviour as the TypeScript version).
func (b *Bridge) HandleUpgrade(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // skip Origin check — plugin connects via Figma's sandbox
	})
	if err != nil {
		bridgeLogger.Printf("upgrade error: %v", err)
		return
	}

	// Raise the read limit to 100 MB — Figma documents can be large.
	// Default is 32 KiB which causes "read limited at 32769 bytes" disconnects.
	conn.SetReadLimit(100 * 1024 * 1024)

	b.mu.Lock()
	replaced := b.conn != nil
	if replaced {
		if err := b.conn.Close(websocket.StatusNormalClosure, "replaced by new connection"); err != nil {
			bridgeLogger.Printf("close previous connection error: %v", err)
		}
	}
	b.conn = conn
	b.mu.Unlock()

	if replaced {
		bridgeLogger.Printf("plugin connected (replaced previous connection) from %s", r.RemoteAddr)
	} else {
		bridgeLogger.Printf("plugin connected from %s", r.RemoteAddr)
	}
	go b.readLoop(conn)
}

// readLoop reads messages from the plugin and resolves pending requests.
func (b *Bridge) readLoop(conn *websocket.Conn) {
	defer func() {
		b.mu.Lock()
		if b.conn == conn {
			b.conn = nil
		}
		b.mu.Unlock()
		bridgeLogger.Printf("plugin disconnected")
	}()

	ctx := context.Background()
	for {
		var resp BridgeResponse
		if err := wsjson.Read(ctx, conn, &resp); err != nil {
			if !errors.Is(err, context.Canceled) {
				bridgeLogger.Printf("read error: %v", err)
			}
			return
		}

		// Handle progress updates — extend timeout, do not resolve.
		if resp.Progress > 0 && resp.RequestID != "" {
			b.mu.RLock()
			entry, ok := b.pending[resp.RequestID]
			b.mu.RUnlock()
			if ok {
				// Stop before Reset to avoid the AfterFunc firing during Reset.
				entry.timer.Stop()
				entry.timer.Reset(60 * time.Second)
				bridgeLogger.Printf("progress %s: %d%% %s", resp.RequestID, resp.Progress, resp.Message)
			} else {
				bridgeLogger.Printf("progress %s: %d%% %s (no pending entry — already resolved or timed out)", resp.RequestID, resp.Progress, resp.Message)
			}
			continue
		}

		if resp.RequestID == "" {
			bridgeLogger.Printf("received message with empty requestID — ignored")
			continue
		}

		b.mu.Lock()
		entry, ok := b.pending[resp.RequestID]
		if ok {
			delete(b.pending, resp.RequestID)
		}
		b.mu.Unlock()

		if ok {
			if resp.Error != "" {
				bridgeLogger.Printf("← %s error: %s", resp.RequestID, resp.Error)
			} else {
				bridgeLogger.Printf("← %s ok", resp.RequestID)
			}
			entry.timer.Stop()
			// Use once to prevent sending on a channel already closed by timeout.
			entry.once.Do(func() { entry.ch <- resp })
		} else {
			bridgeLogger.Printf("← %s received but no pending entry (timed out?)", resp.RequestID)
		}
	}
}

// Send sends a request to the plugin and waits for the response.
func (b *Bridge) Send(ctx context.Context, requestType string, nodeIDs []string, params map[string]interface{}) (BridgeResponse, error) {
	b.mu.RLock()
	conn := b.conn
	b.mu.RUnlock()

	if conn == nil {
		return BridgeResponse{}, errors.New("plugin not connected")
	}

	requestID := b.nextID()
	req := BridgeRequest{
		Type:      requestType,
		RequestID: requestID,
		NodeIDs:   nodeIDs,
		Params:    params,
	}

	ch := make(chan BridgeResponse, 1)
	entry := &pendingEntry{ch: ch}

	// Register before sending to avoid a race where the response
	// arrives before we store the channel.
	// get_document on large files can take longer; give it more headroom.
	timeout := 30 * time.Second
	if requestType == "get_document" {
		timeout = 60 * time.Second
	}
	entry.timer = time.AfterFunc(timeout, func() {
		bridgeLogger.Printf("→ %s %s timed out after %s", requestID, requestType, timeout)
		b.mu.Lock()
		delete(b.pending, requestID)
		b.mu.Unlock()
		// Use once to prevent closing a channel already consumed by the read goroutine.
		entry.once.Do(func() { close(ch) })
	})

	b.mu.Lock()
	b.pending[requestID] = entry
	b.mu.Unlock()

	bridgeLogger.Printf("→ %s %s nodeIDs=%v params=%v", requestID, requestType, nodeIDs, params)
	start := time.Now()

	b.wmu.Lock()
	writeErr := wsjson.Write(ctx, conn, req)
	b.wmu.Unlock()
	if writeErr != nil {
		entry.timer.Stop()
		b.mu.Lock()
		delete(b.pending, requestID)
		b.mu.Unlock()
		bridgeLogger.Printf("→ %s %s write error: %v", requestID, requestType, writeErr)
		return BridgeResponse{}, fmt.Errorf("send: %w", writeErr)
	}

	select {
	case resp, ok := <-ch:
		if !ok {
			return BridgeResponse{}, errors.New("request timed out")
		}
		bridgeLogger.Printf("→ %s %s completed in %dms", requestID, requestType, time.Since(start).Milliseconds())
		return resp, nil
	case <-ctx.Done():
		entry.timer.Stop()
		b.mu.Lock()
		delete(b.pending, requestID)
		b.mu.Unlock()
		bridgeLogger.Printf("→ %s %s context cancelled: %v", requestID, requestType, ctx.Err())
		return BridgeResponse{}, ctx.Err()
	}
}

// Close shuts down the bridge, rejecting all pending requests.
func (b *Bridge) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for id, entry := range b.pending {
		entry.timer.Stop()
		entry.once.Do(func() { close(entry.ch) })
		delete(b.pending, id)
	}

	if b.conn != nil {
		if err := b.conn.Close(websocket.StatusNormalClosure, "bridge closed"); err != nil {
			bridgeLogger.Printf("close connection error: %v", err)
		}
		b.conn = nil
	}
}

// nextID generates a request ID in the format req-HHMMSS-N.
func (b *Bridge) nextID() string {
	n := b.counter.Add(1)
	now := time.Now()
	return fmt.Sprintf("req-%02d%02d%02d-%d",
		now.Hour(), now.Minute(), now.Second(), n)
}

// IsConnected reports whether the plugin is currently connected.
func (b *Bridge) IsConnected() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.conn != nil
}

// MarshalJSON is used when logging — avoid printing full conn object.
func (b *Bridge) MarshalJSON() ([]byte, error) {
	b.mu.RLock()
	connected := b.conn != nil
	pending := len(b.pending)
	b.mu.RUnlock()
	return json.Marshal(map[string]interface{}{
		"connected": connected,
		"pending":   pending,
	})
}
