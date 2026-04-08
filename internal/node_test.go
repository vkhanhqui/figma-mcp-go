package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ── RoleName ─────────────────────────────────────────────────────────────────

func TestNodeRoleName(t *testing.T) {
	cases := []struct {
		role Role
		want string
	}{
		{RoleUnknown, "UNKNOWN"},
		{RoleLeader, "LEADER"},
		{RoleFollower, "FOLLOWER"},
	}
	for _, c := range cases {
		n := &Node{role: c.role}
		if got := n.RoleName(); got != c.want {
			t.Errorf("RoleName(%v) = %q, want %q", c.role, got, c.want)
		}
	}
}

// ── NewNode ───────────────────────────────────────────────────────────────────

func TestNewNode_StartsUnknown(t *testing.T) {
	n := NewNode("127.0.0.1", 19940, "test")
	if n.Role() != RoleUnknown {
		t.Errorf("new node role = %v, want UNKNOWN", n.Role())
	}
}

// ── BecomeLeader ─────────────────────────────────────────────────────────────

func TestNodeBecomeLeader(t *testing.T) {
	port := freePort(t)
	n := NewNode("127.0.0.1", port, "test")
	t.Cleanup(n.Stop)

	if err := n.BecomeLeader(); err != nil {
		t.Fatalf("BecomeLeader: %v", err)
	}
	if n.Role() != RoleLeader {
		t.Errorf("role = %v, want LEADER", n.Role())
	}
}

func TestNodeBecomeLeader_PortTaken(t *testing.T) {
	port := freePort(t)

	n1 := NewNode("127.0.0.1", port, "test")
	if err := n1.BecomeLeader(); err != nil {
		t.Fatalf("first BecomeLeader: %v", err)
	}
	t.Cleanup(n1.Stop)

	n2 := NewNode("127.0.0.1", port, "test")
	if err := n2.BecomeLeader(); err == nil {
		n2.Stop()
		t.Error("expected error when port is already taken")
	}
}

func TestNodeBecomeLeader_Idempotent(t *testing.T) {
	port := freePort(t)
	n := NewNode("127.0.0.1", port, "test")
	t.Cleanup(n.Stop)

	if err := n.BecomeLeader(); err != nil {
		t.Fatalf("first BecomeLeader: %v", err)
	}
	// Calling again on the same node should be a no-op.
	if err := n.BecomeLeader(); err != nil {
		t.Fatalf("second BecomeLeader: %v", err)
	}
}

// ── BecomeFollower ────────────────────────────────────────────────────────────

func TestNodeBecomeFollower(t *testing.T) {
	n := NewNode("127.0.0.1", 19940, "test")
	n.BecomeFollower()
	if n.Role() != RoleFollower {
		t.Errorf("role = %v, want FOLLOWER", n.Role())
	}
}

func TestNodeBecomeFollower_Idempotent(t *testing.T) {
	n := NewNode("127.0.0.1", 19940, "test")
	n.BecomeFollower()
	n.BecomeFollower() // should not panic
	if n.Role() != RoleFollower {
		t.Errorf("role = %v, want FOLLOWER", n.Role())
	}
}

func TestNodeBecomeFollower_FromLeader(t *testing.T) {
	port := freePort(t)
	n := NewNode("127.0.0.1", port, "test")

	if err := n.BecomeLeader(); err != nil {
		t.Fatalf("BecomeLeader: %v", err)
	}
	n.BecomeFollower()
	if n.Role() != RoleFollower {
		t.Errorf("role = %v, want FOLLOWER after BecomeFollower", n.Role())
	}

	// Give the OS a moment to fully release the port after Shutdown.
	time.Sleep(20 * time.Millisecond)

	// Port should be free now — a new leader can bind it.
	n2 := NewNode("127.0.0.1", port, "test")
	if err := n2.BecomeLeader(); err != nil {
		t.Fatalf("new node could not bind freed port: %v", err)
	}
	n2.Stop()
}

// ── Stop ─────────────────────────────────────────────────────────────────────

func TestNodeStop_ResetsRole(t *testing.T) {
	port := freePort(t)
	n := NewNode("127.0.0.1", port, "test")

	if err := n.BecomeLeader(); err != nil {
		t.Fatalf("BecomeLeader: %v", err)
	}
	n.Stop()
	if n.Role() != RoleUnknown {
		t.Errorf("role after Stop = %v, want UNKNOWN", n.Role())
	}
}

func TestNodeStop_Idempotent(t *testing.T) {
	n := NewNode("127.0.0.1", 19940, "test")
	n.Stop()
	n.Stop() // should not panic
}

// ── Send: ID normalisation ────────────────────────────────────────────────────

// TestNodeSend_NormalizesIDs verifies that hyphen-format node IDs are converted
// to colon format before being forwarded to the backend.
func TestNodeSend_NormalizesIDs(t *testing.T) {
	var capturedReq RPCRequest

	// Fake leader that records what the follower sends.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedReq)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(RPCResponse{Data: "ok"})
	}))
	t.Cleanup(srv.Close)

	// Build a follower node pointed at the fake server.
	n := &Node{
		role:     RoleFollower,
		follower: NewFollower(srv.URL),
	}

	params := map[string]any{
		"nodeId":   "100-200", // hyphen format
		"parentId": "300-400", // hyphen format
	}
	n.Send(context.Background(), "clone_node", []string{"1-1", "2-2"}, params) //nolint:errcheck

	// nodeIDs should be normalised.
	for _, id := range capturedReq.NodeIDs {
		if id == "1-1" || id == "2-2" {
			t.Errorf("nodeID %q was not normalised to colon format", id)
		}
	}

	// Params nodeId/parentId should be normalised.
	if nodeID, _ := capturedReq.Params["nodeId"].(string); nodeID == "100-200" {
		t.Error("params.nodeId was not normalised")
	}
	if parentID, _ := capturedReq.Params["parentId"].(string); parentID == "300-400" {
		t.Error("params.parentId was not normalised")
	}
}
