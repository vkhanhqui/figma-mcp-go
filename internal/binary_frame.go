package internal

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
)

// Wire format for binary WebSocket frames:
//
//	| 4 bytes magic "FMCP" | 1 byte version=0x02 | 1 byte msgType | 2 bytes flags |
//	| 4 bytes metaLen      | metaLen bytes JSON metadata                            |
//	| remaining bytes      | raw payload (image bytes, PDF bytes, ...)              |
//
// The metadata field carries a `payloadField` key (dotted path) telling the
// receiver where in the JSON tree to splice the raw payload back in. This
// lets handlers stay agnostic to how the payload arrived on the wire.
//
// Binary frames are opt-in: enabled only when both peers advertise the
// `binary_frames_v1` capability during the hello handshake.

const (
	binaryMagic   = "FMCP"
	binaryVersion = 0x02

	// Message types — one byte each. Keep stable; future revisions bump version.
	msgTypeReq       byte = 0x01
	msgTypeResp      byte = 0x02
	msgTypeProgress  byte = 0x03
	msgTypeHeartbeat byte = 0x04
	msgTypeChunk     byte = 0x05

	// Flag bits — big-endian uint16. flagHasPayload is set automatically by
	// encodeBinaryFrame whenever payload is non-empty.
	flagHasPayload uint16 = 0x0001
	flagCompressed uint16 = 0x0002 // reserved for app-level compression on top of WS deflate
	flagLastChunk  uint16 = 0x0004 // chunked transfer terminator (M3.3)

	// CapabilityBinaryFrames is advertised by the plugin to opt into the
	// binary wire format. Server-side checks plugin.Capabilities for this.
	CapabilityBinaryFrames = "binary_frames_v1"
	CapabilityChunking     = "chunking_v1"

	// Minimum frame size — magic + version + msgType + flags + metaLen.
	binaryHeaderSize = 12
)

// binaryFrame is the parsed form of a binary WebSocket message.
type binaryFrame struct {
	msgType byte
	flags   uint16
	meta    json.RawMessage
	payload []byte
}

// encodeBinaryFrame produces the byte slice ready for conn.Write with
// MessageBinary. flagHasPayload is set automatically when payload is non-empty.
func encodeBinaryFrame(msgType byte, flags uint16, meta interface{}, payload []byte) ([]byte, error) {
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}
	if len(payload) > 0 {
		flags |= flagHasPayload
	}
	if len(metaBytes) > 0xFFFFFFFF {
		return nil, errors.New("metadata exceeds u32 length")
	}

	buf := bytes.NewBuffer(make([]byte, 0, binaryHeaderSize+len(metaBytes)+len(payload)))
	buf.WriteString(binaryMagic)
	buf.WriteByte(binaryVersion)
	buf.WriteByte(msgType)
	if err := binary.Write(buf, binary.BigEndian, flags); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, uint32(len(metaBytes))); err != nil {
		return nil, err
	}
	buf.Write(metaBytes)
	if len(payload) > 0 {
		buf.Write(payload)
	}
	return buf.Bytes(), nil
}

// decodeBinaryFrame parses a binary WebSocket message. Returns an error for
// any malformed frame so callers can log and drop without panicking.
func decodeBinaryFrame(data []byte) (*binaryFrame, error) {
	if len(data) < binaryHeaderSize {
		return nil, fmt.Errorf("binary frame too short: %d bytes", len(data))
	}
	if string(data[:4]) != binaryMagic {
		return nil, fmt.Errorf("bad magic: %q", string(data[:4]))
	}
	if data[4] != binaryVersion {
		return nil, fmt.Errorf("unsupported binary version: 0x%02x", data[4])
	}
	msgType := data[5]
	flags := binary.BigEndian.Uint16(data[6:8])
	metaLen := binary.BigEndian.Uint32(data[8:12])
	end := binaryHeaderSize + int(metaLen)
	if end > len(data) {
		return nil, fmt.Errorf("metadata truncated: len=%d frame=%d", metaLen, len(data))
	}
	meta := json.RawMessage(data[binaryHeaderSize:end])
	payload := data[end:]
	return &binaryFrame{msgType: msgType, flags: flags, meta: meta, payload: payload}, nil
}

// hasCapability reports whether the slice contains the named capability.
// Used to gate features on the plugin handshake.
func hasCapability(caps []string, want string) bool {
	for _, c := range caps {
		if c == want {
			return true
		}
	}
	return false
}

// payloadSlice describes one byte range inside the concatenated payload of a
// binary frame. Used for multi-image responses (export_*) where the wire
// carries a flat blob and metadata says how to split it back into per-item
// fields.
type payloadSlice struct {
	Path   string `json:"path"`
	Length int    `json:"length"`
}

// splicePayload walks a JSON tree by dotted path and assigns the given value
// at the leaf. Path components that parse as non-negative integers are
// interpreted as array indices (e.g. "data.exports.0.bytes"). Creates
// intermediate maps when missing; arrays must already exist in the tree.
func splicePayload(root map[string]interface{}, path string, value interface{}) {
	if path == "" || root == nil {
		return
	}
	parts := splitPath(path)
	var cur interface{} = root
	for i, p := range parts {
		last := i == len(parts)-1
		if idx, ok := parseIndex(p); ok {
			arr, isArr := cur.([]interface{})
			if !isArr || idx < 0 || idx >= len(arr) {
				return
			}
			if last {
				arr[idx] = value
				return
			}
			cur = arr[idx]
			continue
		}
		m, isMap := cur.(map[string]interface{})
		if !isMap {
			return
		}
		if last {
			m[p] = value
			return
		}
		next, exists := m[p]
		if !exists || next == nil {
			next = map[string]interface{}{}
			m[p] = next
		}
		cur = next
	}
}

func splitPath(path string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(path); i++ {
		if path[i] == '.' {
			parts = append(parts, path[start:i])
			start = i + 1
		}
	}
	parts = append(parts, path[start:])
	return parts
}

// parseIndex interprets a path component as a non-negative integer index.
// Returns (idx, true) on success.
func parseIndex(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}

// binaryRequestMeta is the metadata JSON written into a binary req frame.
// Only set the payload-related fields when raw bytes follow the metadata.
type binaryRequestMeta struct {
	Type         string                 `json:"type"`
	RequestID    string                 `json:"requestId"`
	NodeIDs      []string               `json:"nodeIds,omitempty"`
	Params       map[string]interface{} `json:"params,omitempty"`
	PayloadField string                 `json:"payloadField,omitempty"`
}

// binaryResponseMeta mirrors BridgeResponse plus payload-slice info. Plugin
// emits this for tools that produce raw bytes (export_*, get_screenshot).
type binaryResponseMeta struct {
	Type          string         `json:"type"`
	RequestID     string         `json:"requestId"`
	Data          interface{}    `json:"data,omitempty"`
	Error         string         `json:"error,omitempty"`
	Progress      int            `json:"progress,omitempty"`
	Message       string         `json:"message,omitempty"`
	PluginVersion string         `json:"pluginVersion,omitempty"`
	Capabilities  []string       `json:"capabilities,omitempty"`
	PayloadSlices []payloadSlice `json:"payloadSlices,omitempty"`
}

// unmarshalBinaryResponse parses a binary response frame and splices the raw
// payload back into the locations named in `payloadSlices`. Tool consumers
// receive `[]byte` at those paths instead of base64 strings; helpers like
// decodeImageBytes accept either form so behaviour stays uniform.
func unmarshalBinaryResponse(frame *binaryFrame) (BridgeResponse, error) {
	var meta binaryResponseMeta
	if err := json.Unmarshal(frame.meta, &meta); err != nil {
		return BridgeResponse{}, fmt.Errorf("decode meta: %w", err)
	}
	resp := BridgeResponse{
		Type:          meta.Type,
		RequestID:     meta.RequestID,
		Data:          meta.Data,
		Error:         meta.Error,
		Progress:      meta.Progress,
		Message:       meta.Message,
		PluginVersion: meta.PluginVersion,
		Capabilities:  meta.Capabilities,
	}
	if len(meta.PayloadSlices) == 0 {
		return resp, nil
	}
	// Wrap data so payload-slice paths can be expressed against the wire
	// structure (e.g. "data.exports.0.bytes") instead of relative to data.
	// The wrapper aliases resp.Data — mutations to the inner maps/arrays
	// remain visible to callers without having to reassign resp.Data.
	root := map[string]interface{}{"data": resp.Data}
	offset := 0
	for _, sl := range meta.PayloadSlices {
		end := offset + sl.Length
		if end > len(frame.payload) {
			return resp, fmt.Errorf("payload slice %q out of range: %d > %d", sl.Path, end, len(frame.payload))
		}
		// Copy the slice — the frame buffer may be reused by the websocket
		// reader, and tool callers might hold the bytes past this call.
		buf := make([]byte, sl.Length)
		copy(buf, frame.payload[offset:end])
		splicePayload(root, sl.Path, buf)
		offset = end
	}
	return resp, nil
}

// encodeBinaryRequest converts a text BridgeRequest into a binary frame when
// the request has a single image payload that benefits from binary transport.
// Returns (nil, false) if no binary path applies — caller should fall back
// to JSON. Today only `import_image` requests with `params.imageData` set
// are encoded; we leave room for `import_images` and others later.
func encodeBinaryRequest(req BridgeRequest) ([]byte, bool, error) {
	if req.Type != "import_image" || req.Params == nil {
		return nil, false, nil
	}
	raw, ok := req.Params["imageData"].(string)
	if !ok || raw == "" {
		return nil, false, nil
	}
	bytes, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, false, fmt.Errorf("decode imageData: %w", err)
	}
	// Strip the base64 from a copy of the params so the wire-side metadata
	// stays small. The original BridgeRequest is untouched.
	stripped := make(map[string]interface{}, len(req.Params))
	for k, v := range req.Params {
		if k == "imageData" {
			continue
		}
		stripped[k] = v
	}
	meta := binaryRequestMeta{
		Type:         req.Type,
		RequestID:    req.RequestID,
		NodeIDs:      req.NodeIDs,
		Params:       stripped,
		PayloadField: "params.imageData",
	}
	frame, err := encodeBinaryFrame(msgTypeReq, 0, meta, bytes)
	if err != nil {
		return nil, false, err
	}
	return frame, true, nil
}
