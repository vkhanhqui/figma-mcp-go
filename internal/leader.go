package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
)

var leaderLogger = log.New(os.Stderr, "[leader] ", 0)

// Leader owns the WebSocket bridge to the Figma plugin and exposes
// HTTP endpoints for health checks and follower RPC proxying.
//
// Endpoints:
//
//	/ws   — WebSocket upgrade for the Figma plugin
//	/ping — Health check (GET)
//	/rpc  — JSON RPC for follower tool calls (POST)
type Leader struct {
	ip      string
	port    int
	bridge  *Bridge
	server  *http.Server
	version string
}

// NewLeader creates a Leader. Call Start() to bind the ip:port.
func NewLeader(ip string, port int, version string) *Leader {
	return &Leader{
		ip:      ip,
		port:    port,
		bridge:  NewBridge(),
		version: version,
	}
}

// GetBridge returns the underlying Bridge so Node can use it directly.
func (l *Leader) GetBridge() *Bridge {
	return l.bridge
}

// Start binds the port and begins serving. Returns an error immediately
// if the port is already in use (EADDRINUSE → caller detects another leader).
func (l *Leader) Start() error {
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", l.ip, l.port))
	if err != nil {
		return err // includes EADDRINUSE
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ping", l.handlePing)
	mux.HandleFunc("/rpc", l.handleRPC)
	mux.HandleFunc("/ws", l.handleWS)

	srv := &http.Server{Handler: mux}
	l.server = srv

	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			leaderLogger.Printf("serve error: %v", err)
		}
	}()

	leaderLogger.Printf("listening on %s:%d", l.ip, l.port)
	return nil
}

// Stop shuts down the HTTP server and closes the bridge.
func (l *Leader) Stop() {
	if l.server != nil {
		l.server.Shutdown(context.Background())
		l.server = nil
	}
	l.bridge.Close()
}

// handlePing responds to health checks from followers.
func (l *Leader) handlePing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"version": l.version,
	})
	if err != nil {
		leaderLogger.Printf("encode ping response error: %v", err)
	}
}

// handleWS upgrades the connection to WebSocket for the Figma plugin.
func (l *Leader) handleWS(w http.ResponseWriter, r *http.Request) {
	l.bridge.HandleUpgrade(w, r)
}

// handleRPC handles JSON RPC calls from follower processes.
func (l *Leader) handleRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		l.sendJSON(w, http.StatusBadRequest, RPCResponse{Error: "failed to read body"})
		return
	}

	var req RPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		l.sendJSON(w, http.StatusBadRequest, RPCResponse{Error: "invalid JSON"})
		return
	}

	leaderLogger.Printf("rpc %s nodeIDs=%v from %s", req.Tool, req.NodeIDs, r.RemoteAddr)

	if validationErr := ValidateRPC(req.Tool, req.NodeIDs, req.Params); validationErr != "" {
		leaderLogger.Printf("rpc %s validation error: %s", req.Tool, validationErr)
		l.sendJSON(w, http.StatusBadRequest, RPCResponse{Error: validationErr})
		return
	}

	resp, err := l.bridge.Send(r.Context(), req.Tool, req.NodeIDs, req.Params)
	if err != nil {
		leaderLogger.Printf("rpc %s bridge error: %v", req.Tool, err)
		l.sendJSON(w, http.StatusOK, RPCResponse{Error: err.Error()})
		return
	}

	if resp.Error != "" {
		leaderLogger.Printf("rpc %s plugin error: %s", req.Tool, resp.Error)
		l.sendJSON(w, http.StatusOK, RPCResponse{Error: resp.Error})
		return
	}

	l.sendJSON(w, http.StatusOK, RPCResponse{Data: resp.Data})
}

func (l *Leader) sendJSON(w http.ResponseWriter, status int, body RPCResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		leaderLogger.Printf("encode response error: %v", err)
	}
}
