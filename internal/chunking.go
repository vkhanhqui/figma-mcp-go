package internal

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Chunking splits oversized binary frames into ~64KB pieces so the writer
// goroutine can interleave them with smaller messages. Each chunk is its own
// WebSocket binary frame with msgTypeChunk and metadata describing its place
// in the stream. The receiver reassembles the original byte slice and decodes
// it through the normal binary-frame path — chunking is transport-only.

const (
	chunkSize          = 64 * 1024        // per-chunk payload byte limit
	chunkThresholdSize = 1 * 1024 * 1024  // frames above this size get chunked
	chunkAssembleTTL   = 30 * time.Second // drop partials older than this
)

// chunkMeta is the JSON metadata carried by each chunk frame.
type chunkMeta struct {
	RequestID string `json:"requestId"`
	Seq       int    `json:"seq"`
	Total     int    `json:"total"`
}

// shouldChunk reports whether a fully-encoded binary frame is large enough
// that splitting will help interleaving (true) or whether the writer should
// just send it as a single frame (false).
func shouldChunk(frame []byte) bool {
	return len(frame) > chunkThresholdSize
}

// splitIntoChunks produces a slice of pre-encoded chunk frames covering the
// given full binary frame. The caller passes the requestId so receivers can
// correlate chunks to a stream; the same id is also embedded in the original
// frame's metadata. Returns at least 2 chunks for any input above threshold.
func splitIntoChunks(requestID string, frame []byte) ([][]byte, error) {
	if len(frame) == 0 {
		return nil, nil
	}
	total := (len(frame) + chunkSize - 1) / chunkSize
	if total < 2 {
		// Caller should have used shouldChunk first; tolerate by emitting one.
		total = 1
	}
	out := make([][]byte, 0, total)
	for i := 0; i < total; i++ {
		start := i * chunkSize
		end := start + chunkSize
		if end > len(frame) {
			end = len(frame)
		}
		flags := uint16(0)
		if i == total-1 {
			flags |= flagLastChunk
		}
		chunk, err := encodeBinaryFrame(msgTypeChunk, flags, chunkMeta{
			RequestID: requestID,
			Seq:       i,
			Total:     total,
		}, frame[start:end])
		if err != nil {
			return nil, err
		}
		out = append(out, chunk)
	}
	return out, nil
}

// chunkAssembler collects chunks for a given requestId until the full frame
// can be reconstructed. Stream-level lifetime: entries time out after 30s
// without progress so a misbehaving sender can't pin memory forever.
type chunkAssembler struct {
	mu      sync.Mutex
	streams map[string]*chunkStream
}

type chunkStream struct {
	parts    [][]byte
	total    int
	received int
	expiry   time.Time
}

// newChunkAssembler returns a ready-to-use assembler.
func newChunkAssembler() *chunkAssembler {
	return &chunkAssembler{streams: make(map[string]*chunkStream)}
}

// receive consumes one chunk's metadata + payload. When the chunk completes
// the original frame, the assembled bytes are returned and the stream is
// dropped. Returns (nil, nil) while the stream is still incomplete.
func (a *chunkAssembler) receive(meta chunkMeta, payload []byte) ([]byte, error) {
	if meta.RequestID == "" {
		return nil, fmt.Errorf("chunk meta missing requestId")
	}
	if meta.Total <= 0 {
		return nil, fmt.Errorf("chunk meta has non-positive total: %d", meta.Total)
	}
	if meta.Seq < 0 || meta.Seq >= meta.Total {
		return nil, fmt.Errorf("chunk seq out of range: %d/%d", meta.Seq, meta.Total)
	}

	a.mu.Lock()
	a.gcExpiredLocked()
	st, exists := a.streams[meta.RequestID]
	if !exists {
		st = &chunkStream{
			parts:  make([][]byte, meta.Total),
			total:  meta.Total,
			expiry: time.Now().Add(chunkAssembleTTL),
		}
		a.streams[meta.RequestID] = st
	} else if st.total != meta.Total {
		a.mu.Unlock()
		return nil, fmt.Errorf("chunk total mismatch: existing=%d incoming=%d", st.total, meta.Total)
	}
	if st.parts[meta.Seq] != nil {
		a.mu.Unlock()
		return nil, fmt.Errorf("duplicate chunk seq %d for %s", meta.Seq, meta.RequestID)
	}
	// Copy because the websocket reader may reuse the underlying buffer.
	cp := make([]byte, len(payload))
	copy(cp, payload)
	st.parts[meta.Seq] = cp
	st.received++
	st.expiry = time.Now().Add(chunkAssembleTTL)

	if st.received < st.total {
		a.mu.Unlock()
		return nil, nil
	}
	delete(a.streams, meta.RequestID)
	a.mu.Unlock()

	total := 0
	for _, p := range st.parts {
		total += len(p)
	}
	out := make([]byte, 0, total)
	for _, p := range st.parts {
		out = append(out, p...)
	}
	return out, nil
}

// gcExpiredLocked drops any partial streams that haven't received a chunk in
// the last chunkAssembleTTL. Caller holds a.mu.
func (a *chunkAssembler) gcExpiredLocked() {
	now := time.Now()
	for id, st := range a.streams {
		if now.After(st.expiry) {
			delete(a.streams, id)
		}
	}
}

// pending reports the number of in-flight streams. Used by tests + metrics.
func (a *chunkAssembler) pending() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.streams)
}

// reset drops all in-flight streams. Called when the websocket reconnects.
func (a *chunkAssembler) reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.streams = make(map[string]*chunkStream)
}

// extractChunkMeta parses a chunk frame's metadata. Convenience for the
// readLoop so it doesn't have to inline json.Unmarshal each time.
func extractChunkMeta(frame *binaryFrame) (chunkMeta, error) {
	var meta chunkMeta
	if err := json.Unmarshal(frame.meta, &meta); err != nil {
		return chunkMeta{}, fmt.Errorf("decode chunk meta: %w", err)
	}
	return meta, nil
}
