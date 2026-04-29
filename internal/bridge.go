package internal

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/oklog/ulid/v2"
	"github.com/vkhanhqui/figma-mcp-go/internal/observability"
)

var bridgeLogger = log.New(os.Stderr, "[bridge] ", 0)

// toolTimeouts maps known tool names to their per-call timeout. Tools that
// produce or consume large payloads (full document export, PDF render,
// many-icon batch) need more headroom than the 30s default.
var toolTimeouts = map[string]time.Duration{
	"get_document":         60 * time.Second,
	"get_design_context":   60 * time.Second,
	"export_frames_to_pdf": 120 * time.Second,
	"export_nodes_batch":   90 * time.Second,
	"import_images":        90 * time.Second,
	"save_screenshots":     120 * time.Second,
}

const defaultBridgeTimeout = 30 * time.Second

// timeoutFor returns the configured timeout for a tool, falling back to the
// default. For import_image we add 1s per ~256KB of payload as a rough proxy
// for transfer + Figma create cost on large images.
func timeoutFor(tool string, paramsLen int) time.Duration {
	if t, ok := toolTimeouts[tool]; ok {
		return t
	}
	if tool == "import_image" {
		// 30s base + ~1s per 256KB payload, capped at 120s.
		extra := time.Duration(paramsLen/(256*1024)) * time.Second
		t := defaultBridgeTimeout + extra
		if t > 120*time.Second {
			return 120 * time.Second
		}
		return t
	}
	return defaultBridgeTimeout
}

// pendingEntry holds the response channel and inactivity timer for an in-flight request.
type pendingEntry struct {
	ch    chan BridgeResponse
	timer *time.Timer
	once  sync.Once // guards channel close/send — prevents panic on concurrent timeout + response
}

// outgoingMsg is one request queued for the writer goroutine.
// Exactly one of `req`, `binary`, or `chunks` is set:
//   - `req`: text frames carrying a JSON-encoded BridgeRequest
//   - `binary`: a pre-encoded FMCP frame fitting in a single WS message
//   - `chunks`: a sequence of FMCP chunk frames the writer should
//     interleave with other queue items (round-robin)
type outgoingMsg struct {
	req    *BridgeRequest
	binary []byte
	chunks [][]byte
}

// Bridge manages the single WebSocket connection from the Figma plugin
// and matches responses to pending requests via request IDs.
type Bridge struct {
	mu          sync.RWMutex
	conn        *websocket.Conn
	writeQ      chan outgoingMsg   // bound to current connection lifetime; replaced on each upgrade
	writeCtx    context.Context    // cancelled when the current connection is torn down
	writeCancel context.CancelFunc // cancels writeCtx
	pending     map[string]*pendingEntry
	counter     atomic.Int64
	// imageCache maps content hash → Figma imageHash. Populated from
	// import_image responses; consulted on subsequent calls so we can drop the
	// base64 payload from the wire. May be a PersistentImageCache that also
	// flushes to ~/.figma-mcp-go/cache/imagehash.json.
	imageCache ImageCacheStore

	// Plugin handshake state — populated when the plugin sends `{type:"hello"}`
	// after ws.onopen. Empty string means the plugin is legacy (pre-handshake)
	// and we should assume the lowest common feature set.
	pluginVersion      string
	pluginCapabilities []string

	// chunker reassembles inbound msgTypeChunk frames into the original
	// binary frame. Created lazily; reset on reconnect so partial streams
	// from a dead connection don't leak into the new one.
	chunker *chunkAssembler
}

// NewBridge creates a ready-to-use Bridge with a disk-backed image cache.
// If the cache file cannot be loaded (parse error, IO failure), startup
// proceeds with an empty cache — the cache is a perf optimisation, not a
// correctness guarantee.
func NewBridge() *Bridge {
	cache := NewPersistentImageCache(DefaultImageCachePath())
	if err := cache.Load(); err != nil {
		bridgeLogger.Printf("image cache load skipped: %v", err)
	}
	return &Bridge{
		pending:    make(map[string]*pendingEntry),
		imageCache: cache,
		chunker:    newChunkAssembler(),
	}
}

// NewBridgeWithCache lets tests inject an in-memory cache so they don't
// touch the user's home directory.
func NewBridgeWithCache(cache ImageCacheStore) *Bridge {
	if cache == nil {
		cache = NewImageCache()
	}
	return &Bridge{
		pending:    make(map[string]*pendingEntry),
		imageCache: cache,
		chunker:    newChunkAssembler(),
	}
}

// ImageCache returns the bridge-scoped image hash cache.
func (b *Bridge) ImageCache() ImageCacheStore {
	return b.imageCache
}

// PluginInfo returns the version + capabilities reported by the plugin in
// its hello message. Empty version indicates a legacy plugin (pre-handshake)
// or that no plugin is connected.
func (b *Bridge) PluginInfo() (version string, capabilities []string) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	caps := make([]string, len(b.pluginCapabilities))
	copy(caps, b.pluginCapabilities)
	return b.pluginVersion, caps
}

// HandleUpgrade upgrades an HTTP request to a WebSocket connection.
// Only one plugin connection is maintained at a time; a new connection
// replaces the old one (same behaviour as the TypeScript version).
func (b *Bridge) HandleUpgrade(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // skip Origin check — plugin connects via Figma's sandbox
		// Compress text frames (JSON, SVG, design context). PNG/JPG payloads are
		// already compressed and the encoder skips them automatically once it
		// detects no gain. Threshold 1KB avoids per-frame overhead on small
		// control messages.
		CompressionMode:      websocket.CompressionContextTakeover,
		CompressionThreshold: 1024,
	})
	if err != nil {
		bridgeLogger.Printf("upgrade error: %v", err)
		return
	}

	// Raise the read limit to 100 MB — Figma documents can be large.
	// Default is 32 KiB which causes "read limited at 32769 bytes" disconnects.
	conn.SetReadLimit(100 * 1024 * 1024)

	writeCtx, writeCancel := context.WithCancel(context.Background())
	q := make(chan outgoingMsg, 256)

	b.mu.Lock()
	replaced := b.conn != nil
	if replaced {
		// Tear down the previous connection: cancel its writer, fail any
		// pending requests bound to it, then close the socket.
		if b.writeCancel != nil {
			b.writeCancel()
		}
		b.cancelAllPendingLocked()
		if err := b.conn.Close(websocket.StatusNormalClosure, "replaced by new connection"); err != nil {
			bridgeLogger.Printf("close previous connection error: %v", err)
		}
	}
	b.conn = conn
	b.writeQ = q
	b.writeCtx = writeCtx
	b.writeCancel = writeCancel
	if b.chunker != nil {
		b.chunker.reset()
	}
	b.mu.Unlock()

	if replaced {
		bridgeLogger.Printf("plugin connected (replaced previous connection) from %s", r.RemoteAddr)
		observability.PluginDisconnectsTotal.WithLabelValues("replaced").Inc()
	} else {
		bridgeLogger.Printf("plugin connected from %s", r.RemoteAddr)
	}
	observability.PluginConnected.Set(1)
	observability.CacheSize.Set(float64(b.imageCache.Len()))
	go b.writeLoop(conn, q, writeCtx)
	go b.readLoop(conn, q, writeCancel)
}

// writeLoop is the single goroutine permitted to write to a given connection.
// coder/websocket forbids concurrent writes; serialising through a queue lets
// callers proceed in parallel without holding a global lock for the duration
// of large payloads.
//
// The loop runs in three phases each iteration:
//  1. Drain at most one queue item without blocking. Single-frame messages
//     are written immediately; chunked messages register their chunk slice
//     for round-robin draining.
//  2. If any chunked streams have remaining work, write one chunk from each
//     stream so a 30 MB transfer doesn't head-of-line block smaller calls.
//  3. If both the queue and pending streams are empty, block on the queue
//     (or context cancellation) until something arrives.
func (b *Bridge) writeLoop(conn *websocket.Conn, q chan outgoingMsg, ctx context.Context) {
	var pending [][][]byte // each element = remaining chunks for one stream

	writeBytes := func(payload []byte) bool {
		if err := conn.Write(ctx, websocket.MessageBinary, payload); err != nil {
			bridgeLogger.Printf("binary write error: %v", err)
			_ = conn.Close(websocket.StatusInternalError, "write error")
			return false
		}
		return true
	}

	handle := func(msg outgoingMsg) bool {
		if msg.binary != nil {
			return writeBytes(msg.binary)
		}
		if len(msg.chunks) > 0 {
			// Defer to round-robin phase. The first chunk goes out next tick;
			// keeps single-frame items ahead of the bulk stream.
			pending = append(pending, msg.chunks)
			return true
		}
		if msg.req != nil {
			if err := wsjson.Write(ctx, conn, msg.req); err != nil {
				bridgeLogger.Printf("write error on %s: %v", msg.req.RequestID, err)
				_ = conn.Close(websocket.StatusInternalError, "write error")
				return false
			}
		}
		return true
	}

	for {
		// Phase 1: non-blocking drain — keeps small/control messages flowing
		// even while a chunked stream is in progress.
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-q:
			if !ok {
				return
			}
			if !handle(msg) {
				return
			}
			continue
		default:
		}

		// Phase 2: round-robin one chunk from each pending stream.
		if len(pending) > 0 {
			keep := pending[:0]
			for _, st := range pending {
				if len(st) == 0 {
					continue
				}
				if !writeBytes(st[0]) {
					return
				}
				st = st[1:]
				if len(st) > 0 {
					keep = append(keep, st)
				}
			}
			pending = keep
			continue
		}

		// Phase 3: nothing ready — block.
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-q:
			if !ok {
				return
			}
			if !handle(msg) {
				return
			}
		}
	}
}

// readLoop reads messages from the plugin and resolves pending requests.
// On exit, it tears down the connection state so callers fail fast instead of
// waiting on the per-request timeout.
func (b *Bridge) readLoop(conn *websocket.Conn, ownedQ chan outgoingMsg, writeCancel context.CancelFunc) {
	defer func() {
		b.mu.Lock()
		if b.conn == conn {
			b.conn = nil
		}
		if b.writeQ == ownedQ {
			b.writeQ = nil
			b.writeCtx = nil
			b.writeCancel = nil
		}
		b.cancelAllPendingLocked()
		b.pluginVersion = ""
		b.pluginCapabilities = nil
		b.mu.Unlock()
		writeCancel()
		bridgeLogger.Printf("plugin disconnected")
		observability.PluginConnected.Set(0)
		observability.PluginDisconnectsTotal.WithLabelValues("read_loop_exit").Inc()
	}()

	ctx := context.Background()
	for {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				bridgeLogger.Printf("read error: %v", err)
			}
			return
		}

		var resp BridgeResponse
		if typ == websocket.MessageBinary {
			frame, err := decodeBinaryFrame(data)
			if err != nil {
				bridgeLogger.Printf("binary decode error: %v", err)
				continue
			}
			// Reassemble chunked streams transparently. Each chunk is its own
			// FMCP frame with msgType=0x05; the assembler buffers them under
			// the requestId and yields the original byte slice on completion.
			if frame.msgType == msgTypeChunk {
				meta, err := extractChunkMeta(frame)
				if err != nil {
					bridgeLogger.Printf("chunk meta decode error: %v", err)
					continue
				}
				assembled, err := b.chunker.receive(meta, frame.payload)
				if err != nil {
					bridgeLogger.Printf("chunk receive error on %s seq=%d: %v", meta.RequestID, meta.Seq, err)
					continue
				}
				if assembled == nil {
					continue // more chunks expected
				}
				frame, err = decodeBinaryFrame(assembled)
				if err != nil {
					bridgeLogger.Printf("assembled frame decode error: %v", err)
					continue
				}
			}
			resp, err = unmarshalBinaryResponse(frame)
			if err != nil {
				bridgeLogger.Printf("binary response: %v", err)
				continue
			}
		} else {
			if err := json.Unmarshal(data, &resp); err != nil {
				bridgeLogger.Printf("json decode error: %v", err)
				continue
			}
		}

		// Handle hello handshake — first message from a connected plugin.
		// We log the version and capabilities but never reject (yet); when v2.0
		// of the binary protocol lands we can ratchet the minimum here.
		if resp.Type == "hello" {
			b.mu.Lock()
			b.pluginVersion = resp.PluginVersion
			b.pluginCapabilities = append(b.pluginCapabilities[:0], resp.Capabilities...)
			b.mu.Unlock()
			bridgeLogger.Printf("plugin hello: version=%q capabilities=%v", resp.PluginVersion, resp.Capabilities)
			observability.Component("bridge").Info("plugin_hello",
				"plugin_version", resp.PluginVersion,
				"capabilities", resp.Capabilities,
			)
			continue
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
				// Mine the response for a (contentHash, imageHash) pair we can
				// remember for next time. Single-image and batch responses use
				// the same field names.
				b.harvestImageHashes(resp.Data)
			}
			entry.timer.Stop()
			// Use once to prevent sending on a channel already closed by timeout.
			entry.once.Do(func() { entry.ch <- resp })
		} else {
			bridgeLogger.Printf("← %s received but no pending entry (timed out?)", resp.RequestID)
		}
	}
}

// cancelAllPendingLocked closes all pending response channels. Caller must hold b.mu.
func (b *Bridge) cancelAllPendingLocked() {
	for id, entry := range b.pending {
		entry.timer.Stop()
		entry.once.Do(func() { close(entry.ch) })
		delete(b.pending, id)
	}
}

// Send sends a request to the plugin and waits for the response.
func (b *Bridge) Send(ctx context.Context, requestType string, nodeIDs []string, params map[string]interface{}) (BridgeResponse, error) {
	b.mu.RLock()
	conn := b.conn
	q := b.writeQ
	wctx := b.writeCtx
	binaryEnabled := hasCapability(b.pluginCapabilities, CapabilityBinaryFrames)
	chunkingEnabled := hasCapability(b.pluginCapabilities, CapabilityChunking)
	b.mu.RUnlock()

	if conn == nil || q == nil || wctx == nil {
		return BridgeResponse{}, errors.New("plugin not connected")
	}

	// Image cache fast path: if the caller provided a contentHash and we know
	// the matching Figma imageHash, drop the base64 payload from the wire and
	// signal the plugin to attach the existing fill directly.
	cacheHits := 0
	cacheTotal := 0
	if b.imageCache != nil {
		switch requestType {
		case "import_image":
			cacheTotal = 1
			if applyImageCacheToParams(b.imageCache, params) {
				cacheHits = 1
			}
		case "import_images":
			if items, ok := params["items"].([]interface{}); ok {
				for _, it := range items {
					m, ok := it.(map[string]interface{})
					if !ok {
						continue
					}
					cacheTotal++
					if applyImageCacheToParams(b.imageCache, m) {
						cacheHits++
					}
				}
			}
		}
	}
	cacheHit := cacheHits > 0

	requestID := b.nextID()
	req := BridgeRequest{
		Type:      requestType,
		RequestID: requestID,
		NodeIDs:   nodeIDs,
		Params:    params,
	}

	// Estimate payload size from the largest expected field (imageData base64)
	// to scale the timeout. 0 if no image — falls back to defaultBridgeTimeout.
	paramsLen := 0
	if params != nil {
		if d, ok := params["imageData"].(string); ok {
			paramsLen = len(d)
		}
	}
	timeout := timeoutFor(requestType, paramsLen)

	ch := make(chan BridgeResponse, 1)
	entry := &pendingEntry{ch: ch}

	// Register before sending to avoid a race where the response
	// arrives before we store the channel.
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

	slog := observability.Component("bridge")
	// Try the binary path. If the plugin doesn't support it, or this tool
	// doesn't have a binary encoding, fall back to JSON. Errors here are
	// only from base64 decode of bad imageData — rare, callers see a clean
	// error before the request is queued.
	var binaryFrame []byte
	useBinary := false
	if binaryEnabled {
		bin, ok, err := encodeBinaryRequest(req)
		if err != nil {
			return BridgeResponse{}, err
		}
		if ok {
			binaryFrame = bin
			useBinary = true
		}
	}

	if cacheHit {
		bridgeLogger.Printf("→ %s %s nodeIDs=%v paramKeys=%v cache=%d/%d binary=%v", requestID, requestType, nodeIDs, paramKeysOf(params), cacheHits, cacheTotal, useBinary)
	} else {
		bridgeLogger.Printf("→ %s %s nodeIDs=%v paramKeys=%v binary=%v", requestID, requestType, nodeIDs, paramKeysOf(params), useBinary)
	}
	slog.Debug("request_start",
		"request_id", requestID,
		"tool", requestType,
		"node_ids", nodeIDs,
		"param_keys", paramKeysOf(params),
		"bytes_in", paramsLen,
		"cache_hits", cacheHits,
		"cache_total", cacheTotal,
		"binary", useBinary,
		"role", "leader",
	)
	if cacheTotal > 0 {
		observability.CacheHitsTotal.WithLabelValues(requestType).Add(float64(cacheHits))
		observability.CacheMissesTotal.WithLabelValues(requestType).Add(float64(cacheTotal - cacheHits))
	}
	observability.PendingRequests.Inc()
	defer observability.PendingRequests.Dec()
	start := time.Now()

	// Hand off to the writer goroutine. Bail if the connection dies or the
	// caller's context is cancelled before we get a slot in the queue.
	var msg outgoingMsg
	if useBinary {
		// Big frames get split into 64KB chunks and sent round-robin so a
		// 30 MB image doesn't head-of-line block smaller calls. Receiver
		// reassembles transparently. Only enabled when the plugin advertises
		// chunking_v1 — older plugins receive the whole frame and rely on
		// websocket-level fragmentation.
		if chunkingEnabled && shouldChunk(binaryFrame) {
			chunks, err := splitIntoChunks(requestID, binaryFrame)
			if err != nil {
				return BridgeResponse{}, err
			}
			msg = outgoingMsg{chunks: chunks}
		} else {
			msg = outgoingMsg{binary: binaryFrame}
		}
	} else {
		reqCopy := req
		msg = outgoingMsg{req: &reqCopy}
	}
	select {
	case q <- msg:
	case <-wctx.Done():
		entry.timer.Stop()
		b.mu.Lock()
		delete(b.pending, requestID)
		b.mu.Unlock()
		return BridgeResponse{}, errors.New("plugin connection closed before send")
	case <-ctx.Done():
		entry.timer.Stop()
		b.mu.Lock()
		delete(b.pending, requestID)
		b.mu.Unlock()
		return BridgeResponse{}, ctx.Err()
	}

	select {
	case resp, ok := <-ch:
		dur := time.Since(start)
		observability.RequestDurationSeconds.WithLabelValues(requestType, "leader").Observe(dur.Seconds())
		if !ok {
			observability.RequestsTotal.WithLabelValues(requestType, "transport_error", "leader").Inc()
			slog.Warn("request_failed",
				"request_id", requestID,
				"tool", requestType,
				"latency_ms", dur.Milliseconds(),
				"reason", "timeout_or_disconnect",
				"role", "leader",
			)
			return BridgeResponse{}, errors.New("request timed out or connection lost")
		}
		bridgeLogger.Printf("→ %s %s completed in %dms", requestID, requestType, dur.Milliseconds())
		status := "ok"
		if resp.Error != "" {
			status = "plugin_error"
		}
		observability.RequestsTotal.WithLabelValues(requestType, status, "leader").Inc()
		slog.Info("request_complete",
			"request_id", requestID,
			"tool", requestType,
			"latency_ms", dur.Milliseconds(),
			"cache_hit", cacheHit,
			"plugin_error", resp.Error != "",
			"role", "leader",
		)
		// Reflect any cache writes triggered by harvestImageHashes.
		observability.CacheSize.Set(float64(b.imageCache.Len()))
		return resp, nil
	case <-ctx.Done():
		entry.timer.Stop()
		b.mu.Lock()
		delete(b.pending, requestID)
		b.mu.Unlock()
		dur := time.Since(start)
		observability.RequestDurationSeconds.WithLabelValues(requestType, "leader").Observe(dur.Seconds())
		observability.RequestsTotal.WithLabelValues(requestType, "transport_error", "leader").Inc()
		bridgeLogger.Printf("→ %s %s context cancelled: %v", requestID, requestType, ctx.Err())
		slog.Warn("request_cancelled",
			"request_id", requestID,
			"tool", requestType,
			"latency_ms", dur.Milliseconds(),
			"err", ctx.Err().Error(),
			"role", "leader",
		)
		return BridgeResponse{}, ctx.Err()
	}
}

// Close shuts down the bridge, rejecting all pending requests.
func (b *Bridge) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.cancelAllPendingLocked()

	if b.writeCancel != nil {
		b.writeCancel()
		b.writeCancel = nil
		b.writeCtx = nil
		b.writeQ = nil
	}

	if b.conn != nil {
		if err := b.conn.Close(websocket.StatusNormalClosure, "bridge closed"); err != nil {
			bridgeLogger.Printf("close connection error: %v", err)
		}
		b.conn = nil
	}

	// Flush any pending cache writes to disk so a graceful shutdown doesn't
	// drop entries that haven't hit the debounce window yet.
	if pc, ok := b.imageCache.(*PersistentImageCache); ok {
		if err := pc.Close(); err != nil {
			bridgeLogger.Printf("image cache close error: %v", err)
		}
	}
}

// ulidEntropy is shared across goroutines via b.ulidMu (in nextID); rand.New
// is not goroutine-safe so we serialise reads. Single-process use, low rate.
var ulidEntropy = ulid.Monotonic(rand.New(rand.NewSource(time.Now().UnixNano())), 0)
var ulidMu sync.Mutex

// nextID generates a sortable, globally unique request ID.
//
// ULID encodes the millisecond timestamp in the leading 10 chars, so log
// streams stay roughly time-ordered even when interleaved across processes.
func (b *Bridge) nextID() string {
	b.counter.Add(1) // kept so counters stay observable in MarshalJSON
	ulidMu.Lock()
	id := ulid.MustNew(ulid.Timestamp(time.Now()), ulidEntropy)
	ulidMu.Unlock()
	return id.String()
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

// paramKeysOf returns just the keys of a params map for logging — avoids
// dumping multi-MB base64 strings into the log when imageData is present.
func paramKeysOf(params map[string]interface{}) []string {
	if params == nil {
		return nil
	}
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	return keys
}

// harvestImageHashes inspects a successful response payload and records any
// (contentHash, imageHash) pairs it finds. Accepts either a single object
// (single import_image response) or `{ items: [...] }` (batch import_images).
// Tolerates the field being absent — cache.Put no-ops on empty input.
func (b *Bridge) harvestImageHashes(data interface{}) {
	if b == nil || b.imageCache == nil || data == nil {
		return
	}
	switch v := data.(type) {
	case map[string]interface{}:
		if items, ok := v["items"].([]interface{}); ok {
			for _, it := range items {
				if m, ok := it.(map[string]interface{}); ok {
					b.harvestOne(m)
				}
			}
			return
		}
		b.harvestOne(v)
	}
}

func (b *Bridge) harvestOne(m map[string]interface{}) {
	contentHash, _ := m["contentHash"].(string)
	imageHash, _ := m["imageHash"].(string)
	if contentHash == "" || imageHash == "" {
		return
	}
	b.imageCache.Put(contentHash, imageHash)
}

// applyImageCacheToParams swaps imageData → imageHash in-place when the cache
// already knows this content hash. Returns true on hit. Used for both single
// import_image params and per-item maps inside import_images batches.
func applyImageCacheToParams(cache ImageCacheStore, params map[string]interface{}) bool {
	contentHash, _ := params["contentHash"].(string)
	if contentHash == "" {
		return false
	}
	hash, ok := cache.Get(contentHash)
	if !ok {
		return false
	}
	params["imageHash"] = hash
	delete(params, "imageData")
	return true
}
