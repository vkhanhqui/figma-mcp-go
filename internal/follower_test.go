package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ── Ping ─────────────────────────────────────────────────────────────────────

func TestFollowerPing_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ping" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	f := NewFollower(srv.URL)
	if !f.Ping(context.Background()) {
		t.Error("expected Ping to return true for a responding server")
	}
}

func TestFollowerPing_ServerDown(t *testing.T) {
	// Use a port that nothing is listening on.
	f := NewFollower("http://127.0.0.1:1")
	if f.Ping(context.Background()) {
		t.Error("expected Ping to return false when server is unreachable")
	}
}

func TestFollowerPing_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	f := NewFollower(srv.URL)
	if f.Ping(context.Background()) {
		t.Error("expected Ping to return false for non-200 response")
	}
}

// ── Send ─────────────────────────────────────────────────────────────────────

func TestFollowerSend_Success(t *testing.T) {
	wantData := map[string]any{"id": "1:1", "name": "Frame"}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rpc" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		// Decode request to verify it was marshaled correctly.
		var req RPCRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Tool == "" {
			http.Error(w, "missing tool", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(RPCResponse{Data: wantData})
	}))
	t.Cleanup(srv.Close)

	f := NewFollower(srv.URL)
	resp, err := f.Send(context.Background(), "get_node", []string{"1:1"}, nil)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.Type != "get_node" {
		t.Errorf("resp.Type = %q, want get_node", resp.Type)
	}
}

func TestFollowerSend_LeaderReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(RPCResponse{Error: "plugin not connected"})
	}))
	t.Cleanup(srv.Close)

	f := NewFollower(srv.URL)
	resp, err := f.Send(context.Background(), "get_node", []string{"1:1"}, nil)
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if resp.Error != "plugin not connected" {
		t.Errorf("resp.Error = %q, want 'plugin not connected'", resp.Error)
	}
}

func TestFollowerSend_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("not json")) //nolint:errcheck
	}))
	t.Cleanup(srv.Close)

	f := NewFollower(srv.URL)
	_, err := f.Send(context.Background(), "get_node", []string{"1:1"}, nil)
	if err == nil {
		t.Error("expected error for malformed JSON response")
	}
}

func TestFollowerSend_ServerDown(t *testing.T) {
	f := NewFollower("http://127.0.0.1:1")
	_, err := f.Send(context.Background(), "get_node", []string{"1:1"}, nil)
	if err == nil {
		t.Error("expected error when server is unreachable")
	}
}

func TestFollowerSend_ForwardsParams(t *testing.T) {
	var capturedReq RPCRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedReq)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(RPCResponse{Data: "ok"})
	}))
	t.Cleanup(srv.Close)

	f := NewFollower(srv.URL)
	params := map[string]any{"text": "hello", "fontSize": float64(16)}
	f.Send(context.Background(), "set_text", []string{"2:3"}, params) //nolint:errcheck

	if capturedReq.Tool != "set_text" {
		t.Errorf("tool = %q, want set_text", capturedReq.Tool)
	}
	if len(capturedReq.NodeIDs) != 1 || capturedReq.NodeIDs[0] != "2:3" {
		t.Errorf("nodeIDs = %v, want [2:3]", capturedReq.NodeIDs)
	}
}
