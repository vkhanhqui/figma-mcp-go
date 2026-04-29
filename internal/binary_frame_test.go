package internal

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestBinaryFrame_RoundTrip_NoPayload(t *testing.T) {
	meta := map[string]any{"requestId": "01", "type": "ping"}
	frame, err := encodeBinaryFrame(msgTypeReq, 0, meta, nil)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	got, err := decodeBinaryFrame(frame)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.msgType != msgTypeReq {
		t.Errorf("msgType = 0x%02x, want 0x%02x", got.msgType, msgTypeReq)
	}
	if got.flags&flagHasPayload != 0 {
		t.Errorf("expected hasPayload=0 for empty payload")
	}
	if len(got.payload) != 0 {
		t.Errorf("payload = %d bytes, want 0", len(got.payload))
	}
	var m map[string]any
	if err := json.Unmarshal(got.meta, &m); err != nil {
		t.Fatalf("unmarshal meta: %v", err)
	}
	if m["requestId"] != "01" {
		t.Errorf("requestId = %v, want 01", m["requestId"])
	}
}

func TestBinaryFrame_RoundTrip_WithPayload(t *testing.T) {
	meta := map[string]any{"requestId": "01", "payloadField": "params.imageData"}
	payload := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}
	frame, err := encodeBinaryFrame(msgTypeReq, 0, meta, payload)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	got, err := decodeBinaryFrame(frame)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.flags&flagHasPayload == 0 {
		t.Errorf("expected hasPayload flag set")
	}
	if !bytes.Equal(got.payload, payload) {
		t.Errorf("payload mismatch: got %v want %v", got.payload, payload)
	}
}

func TestBinaryFrame_BadMagic(t *testing.T) {
	frame := []byte{'X', 'X', 'X', 'X', 0x02, 0x01, 0, 0, 0, 0, 0, 0}
	if _, err := decodeBinaryFrame(frame); err == nil {
		t.Error("expected error for bad magic")
	}
}

func TestBinaryFrame_BadVersion(t *testing.T) {
	frame := []byte{'F', 'M', 'C', 'P', 0xFF, 0x01, 0, 0, 0, 0, 0, 0}
	if _, err := decodeBinaryFrame(frame); err == nil {
		t.Error("expected error for bad version")
	}
}

func TestBinaryFrame_TruncatedHeader(t *testing.T) {
	frame := []byte{'F', 'M', 'C', 'P', 0x02}
	if _, err := decodeBinaryFrame(frame); err == nil {
		t.Error("expected error for short frame")
	}
}

func TestBinaryFrame_TruncatedMeta(t *testing.T) {
	// Header says metaLen=100 but only 4 bytes follow.
	frame := []byte{'F', 'M', 'C', 'P', 0x02, 0x01, 0, 0, 0, 0, 0, 100, 1, 2, 3, 4}
	if _, err := decodeBinaryFrame(frame); err == nil {
		t.Error("expected error for truncated meta")
	}
}

func TestSplicePayload_TopLevel(t *testing.T) {
	root := map[string]any{}
	splicePayload(root, "imageData", []byte{1, 2, 3})
	got, ok := root["imageData"].([]byte)
	if !ok {
		t.Fatalf("expected []byte at imageData, got %T", root["imageData"])
	}
	if !bytes.Equal(got, []byte{1, 2, 3}) {
		t.Errorf("payload mismatch")
	}
}

func TestSplicePayload_Nested(t *testing.T) {
	root := map[string]any{"params": map[string]any{"x": 1}}
	splicePayload(root, "params.imageData", []byte{0xAB})
	params := root["params"].(map[string]any)
	if params["x"] != 1 {
		t.Errorf("existing field x lost")
	}
	got, ok := params["imageData"].([]byte)
	if !ok {
		t.Fatalf("expected []byte, got %T", params["imageData"])
	}
	if got[0] != 0xAB {
		t.Errorf("payload mismatch")
	}
}

func TestSplicePayload_CreatesIntermediates(t *testing.T) {
	root := map[string]any{}
	splicePayload(root, "data.image_data", []byte{1})
	data, ok := root["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected map at data")
	}
	if got, _ := data["image_data"].([]byte); len(got) != 1 {
		t.Errorf("payload not spliced")
	}
}

func TestHasCapability(t *testing.T) {
	caps := []string{"image_cache_v1", "binary_frames_v1"}
	if !hasCapability(caps, "binary_frames_v1") {
		t.Error("expected match")
	}
	if hasCapability(caps, "missing") {
		t.Error("expected no match")
	}
	if hasCapability(nil, "x") {
		t.Error("nil slice should match nothing")
	}
}

func TestSplicePayload_ArrayIndex(t *testing.T) {
	root := map[string]any{
		"data": map[string]any{
			"exports": []any{
				map[string]any{"nodeId": "1"},
				map[string]any{"nodeId": "2"},
			},
		},
	}
	splicePayload(root, "data.exports.1.bytes", []byte{0xCC})
	exports := root["data"].(map[string]any)["exports"].([]any)
	got, ok := exports[1].(map[string]any)["bytes"].([]byte)
	if !ok || got[0] != 0xCC {
		t.Errorf("array splice failed: %v", exports[1])
	}
	// Sibling untouched.
	if _, has := exports[0].(map[string]any)["bytes"]; has {
		t.Error("first element should not be modified")
	}
}

func TestSplicePayload_OutOfRangeIndex(t *testing.T) {
	root := map[string]any{"items": []any{}}
	// Should silently no-op instead of panicking.
	splicePayload(root, "items.5.bytes", []byte{1})
	items := root["items"].([]any)
	if len(items) != 0 {
		t.Error("expected items unchanged")
	}
}

func TestEncodeBinaryRequest_ImportImageHappyPath(t *testing.T) {
	// Plain "AB" → base64 "QUI="
	req := BridgeRequest{
		Type:      "import_image",
		RequestID: "01",
		Params:    map[string]interface{}{"imageData": "QUI=", "x": 10},
	}
	frame, ok, err := encodeBinaryRequest(req)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !ok {
		t.Fatal("expected binary path to apply")
	}
	got, err := decodeBinaryFrame(frame)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.msgType != msgTypeReq {
		t.Errorf("msgType = 0x%02x", got.msgType)
	}
	if !bytes.Equal(got.payload, []byte("AB")) {
		t.Errorf("payload = %v", got.payload)
	}
	var meta binaryRequestMeta
	if err := json.Unmarshal(got.meta, &meta); err != nil {
		t.Fatalf("meta: %v", err)
	}
	if meta.PayloadField != "params.imageData" {
		t.Errorf("payloadField = %q", meta.PayloadField)
	}
	if _, has := meta.Params["imageData"]; has {
		t.Error("imageData should be stripped from metadata")
	}
	if v, _ := meta.Params["x"].(float64); v != 10 {
		t.Errorf("x lost or wrong: %v", meta.Params["x"])
	}
}

func TestEncodeBinaryRequest_NotApplicable(t *testing.T) {
	tests := []struct {
		name string
		req  BridgeRequest
	}{
		{"non-import tool", BridgeRequest{Type: "get_node", Params: map[string]interface{}{"imageData": "QUI="}}},
		{"no imageData", BridgeRequest{Type: "import_image", Params: map[string]interface{}{}}},
		{"empty imageData", BridgeRequest{Type: "import_image", Params: map[string]interface{}{"imageData": ""}}},
		{"nil params", BridgeRequest{Type: "import_image"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, ok, err := encodeBinaryRequest(tc.req)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if ok {
				t.Error("expected binary path to skip")
			}
		})
	}
}

func TestEncodeBinaryRequest_BadBase64(t *testing.T) {
	req := BridgeRequest{Type: "import_image", Params: map[string]interface{}{"imageData": "!!!not base64!!!"}}
	if _, _, err := encodeBinaryRequest(req); err == nil {
		t.Error("expected error for bad base64")
	}
}

func TestUnmarshalBinaryResponse_WithSlices(t *testing.T) {
	meta := binaryResponseMeta{
		Type:      "get_screenshot",
		RequestID: "01",
		Data: map[string]interface{}{
			"exports": []interface{}{
				map[string]interface{}{"nodeId": "1"},
				map[string]interface{}{"nodeId": "2"},
			},
		},
		PayloadSlices: []payloadSlice{
			{Path: "data.exports.0.bytes", Length: 3},
			{Path: "data.exports.1.bytes", Length: 5},
		},
	}
	payload := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	frame, err := encodeBinaryFrame(msgTypeResp, 0, meta, payload)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	parsed, err := decodeBinaryFrame(frame)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp, err := unmarshalBinaryResponse(parsed)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	exports := resp.Data.(map[string]interface{})["exports"].([]interface{})
	first := exports[0].(map[string]interface{})["bytes"].([]byte)
	second := exports[1].(map[string]interface{})["bytes"].([]byte)
	if !bytes.Equal(first, []byte{1, 2, 3}) {
		t.Errorf("first bytes wrong: %v", first)
	}
	if !bytes.Equal(second, []byte{4, 5, 6, 7, 8}) {
		t.Errorf("second bytes wrong: %v", second)
	}
}

func TestUnmarshalBinaryResponse_NoSlices(t *testing.T) {
	meta := binaryResponseMeta{
		Type:      "get_node",
		RequestID: "02",
		Data:      map[string]interface{}{"id": "1:1"},
	}
	frame, _ := encodeBinaryFrame(msgTypeResp, 0, meta, nil)
	parsed, _ := decodeBinaryFrame(frame)
	resp, err := unmarshalBinaryResponse(parsed)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Type != "get_node" || resp.RequestID != "02" {
		t.Errorf("unexpected resp: %+v", resp)
	}
}
