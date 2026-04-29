package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/vkhanhqui/figma-mcp-go/internal/observability"
)

var followerLogger = log.New(os.Stderr, "[follower] ", 0)

// followerHTTPSlack is the buffer added to the per-tool bridge timeout when
// calculating the follower's HTTP timeout. The leader needs to time out first
// so its error message reaches the follower; without slack the HTTP layer
// would race the bridge and the follower would surface a less informative
// "Client.Timeout exceeded" instead.
const followerHTTPSlack = 10 * time.Second

// Follower proxies MCP tool calls to the leader via HTTP /rpc.
type Follower struct {
	leaderURL string
	client    *http.Client
}

// NewFollower creates a Follower pointed at the given leader base URL.
func NewFollower(leaderURL string) *Follower {
	return &Follower{
		leaderURL: leaderURL,
		client: &http.Client{
			// Per-call timeout is set on each request via context. The client
			// itself only enforces the outer connection-level guarantees.
			Transport: &http.Transport{
				MaxIdleConnsPerHost:   4,
				IdleConnTimeout:       90 * time.Second,
				DisableCompression:    false,
				ResponseHeaderTimeout: 10 * time.Second,
			},
		},
	}
}

// Send proxies a tool call to the leader.
func (f *Follower) Send(ctx context.Context, tool string, nodeIDs []string, params map[string]interface{}) (BridgeResponse, error) {
	slog := observability.Component("follower")
	followerLogger.Printf("proxy %s nodeIDs=%v paramKeys=%v → %s/rpc", tool, nodeIDs, paramKeysOf(params), f.leaderURL)
	slog.Debug("proxy_start",
		"tool", tool,
		"node_ids", nodeIDs,
		"param_keys", paramKeysOf(params),
		"leader_url", f.leaderURL,
		"role", "follower",
	)
	start := time.Now()

	rpcReq := RPCRequest{
		Tool:    tool,
		NodeIDs: nodeIDs,
		Params:  params,
	}

	body, err := json.Marshal(rpcReq)
	if err != nil {
		return BridgeResponse{}, fmt.Errorf("marshal: %w", err)
	}

	// Adaptive timeout: track the bridge's timeout for this tool + payload
	// size, plus slack for the leader to time out first and respond cleanly.
	paramsLen := 0
	if d, ok := params["imageData"].(string); ok {
		paramsLen = len(d)
	}
	bridgeTimeout := timeoutFor(tool, paramsLen)
	httpTimeout := bridgeTimeout + followerHTTPSlack

	reqCtx, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, f.leaderURL+"/rpc", bytes.NewReader(body))
	if err != nil {
		return BridgeResponse{}, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		followerLogger.Printf("proxy %s rpc error: %v", tool, err)
		return BridgeResponse{}, fmt.Errorf("rpc call: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return BridgeResponse{}, fmt.Errorf("read response: %w", err)
	}

	var rpcResp RPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return BridgeResponse{}, fmt.Errorf("unmarshal: %w", err)
	}

	dur := time.Since(start)
	observability.RequestDurationSeconds.WithLabelValues(tool, "follower").Observe(dur.Seconds())
	if rpcResp.Error != "" {
		observability.RequestsTotal.WithLabelValues(tool, "plugin_error", "follower").Inc()
		followerLogger.Printf("proxy %s error from leader in %dms: %s", tool, dur.Milliseconds(), rpcResp.Error)
		slog.Warn("proxy_complete",
			"tool", tool,
			"latency_ms", dur.Milliseconds(),
			"plugin_error", true,
			"role", "follower",
		)
		return BridgeResponse{Error: rpcResp.Error}, nil
	}

	observability.RequestsTotal.WithLabelValues(tool, "ok", "follower").Inc()
	followerLogger.Printf("proxy %s ok in %dms", tool, dur.Milliseconds())
	slog.Info("proxy_complete",
		"tool", tool,
		"latency_ms", dur.Milliseconds(),
		"bytes_out", len(respBody),
		"plugin_error", false,
		"role", "follower",
	)
	return BridgeResponse{
		Type: tool,
		Data: rpcResp.Data,
	}, nil
}

// Ping checks if the leader is alive. Returns true if healthy.
func (f *Follower) Ping(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.leaderURL+"/ping", nil)
	if err != nil {
		followerLogger.Printf("ping new request error: %v", err)
		return false
	}

	resp, err := f.client.Do(req)
	if err != nil {
		followerLogger.Printf("ping %s failed: %v", f.leaderURL, err)
		return false
	}
	resp.Body.Close()
	ok := resp.StatusCode == http.StatusOK
	followerLogger.Printf("ping %s → %d (healthy=%v)", f.leaderURL, resp.StatusCode, ok)
	return ok
}
