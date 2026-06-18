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
)

var followerLogger = log.New(os.Stderr, "[follower] ", 0)

// Follower proxies MCP tool calls to the leader via HTTP /rpc.
type Follower struct {
	leaderURL string
	client    *http.Client
	authToken string
}

// NewFollower creates a Follower pointed at the given leader base URL.
func NewFollower(leaderURL string) *Follower {
	return NewFollowerWithAuth(leaderURL, "")
}

// NewFollowerWithAuth creates a Follower that sends authToken when it is set.
func NewFollowerWithAuth(leaderURL string, authToken string) *Follower {
	return &Follower{
		leaderURL: leaderURL,
		authToken: authToken,
		client: &http.Client{
			// 35s > 30s bridge timeout — gives the leader time to time out first
			Timeout: 35 * time.Second,
		},
	}
}

// Send proxies a tool call to the leader.
func (f *Follower) Send(ctx context.Context, tool string, nodeIDs []string, params map[string]interface{}) (BridgeResponse, error) {
	followerLogger.Printf("proxy %s nodeIDs=%v paramKeys=%v → %s/rpc", tool, nodeIDs, paramKeys(params), f.leaderURL)
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.leaderURL+"/rpc", bytes.NewReader(body))
	if err != nil {
		return BridgeResponse{}, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	f.authorize(req)

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

	if rpcResp.Error != "" {
		followerLogger.Printf("proxy %s error from leader in %dms: %s", tool, time.Since(start).Milliseconds(), rpcResp.Error)
		return BridgeResponse{Error: rpcResp.Error}, nil
	}

	followerLogger.Printf("proxy %s ok in %dms", tool, time.Since(start).Milliseconds())
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
	f.authorize(req)

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

func (f *Follower) authorize(req *http.Request) {
	if f.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+f.authToken)
	}
}
