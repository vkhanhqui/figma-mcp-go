package internal

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"
)

// ── itoa ─────────────────────────────────────────────────────────────────────

func TestItoa(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{1994, "1994"},
		{-1, "-1"},
	}
	for _, c := range cases {
		got := itoa(c.in)
		if got != c.want {
			t.Errorf("itoa(%d) = %q, want %q", c.in, got, c.want)
		}
		// Cross-check with stdlib.
		if got != strconv.Itoa(c.in) {
			t.Errorf("itoa(%d) diverges from strconv.Itoa", c.in)
		}
	}
}

// ── tick: RoleLeader ──────────────────────────────────────────────────────────

func TestElectionTick_LeaderDoesNothing(t *testing.T) {
	port := freePort(t)
	n := NewNode("127.0.0.1", port, "test")

	if err := n.BecomeLeader(); err != nil {
		t.Fatalf("BecomeLeader: %v", err)
	}
	t.Cleanup(n.Stop)

	e := NewElection("127.0.0.1", port, n)
	if err := e.tick(context.Background()); err != nil {
		t.Errorf("tick for LEADER: %v", err)
	}
	// Role should remain LEADER.
	if n.Role() != RoleLeader {
		t.Errorf("role = %v after tick, want LEADER", n.Role())
	}
}

// ── tick: RoleFollower — healthy leader ───────────────────────────────────────

func TestElectionTick_FollowerHealthyLeader(t *testing.T) {
	// Fake leader server that responds to /ping.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	// Extract port from the test server listener.
	tcpAddr, ok := srv.Listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("unexpected addr type: %T", srv.Listener.Addr())
	}
	testPort := tcpAddr.Port

	n := NewNode("127.0.0.1", testPort, "test")
	n.BecomeFollower()

	e := NewElection("127.0.0.1", testPort, n)
	if err := e.tick(context.Background()); err != nil {
		t.Errorf("tick: %v", err)
	}
	// Leader is healthy → node stays FOLLOWER.
	if n.Role() != RoleFollower {
		t.Errorf("role = %v, want FOLLOWER", n.Role())
	}
}

// ── tick: RoleFollower — dead leader → takeover ───────────────────────────────

func TestElectionTick_FollowerDeadLeader_TakesOver(t *testing.T) {
	port := freePort(t)

	n := NewNode("127.0.0.1", port, "test")
	n.BecomeFollower()
	t.Cleanup(n.Stop)

	e := NewElection("127.0.0.1", port, n)
	if err := e.tick(context.Background()); err != nil {
		t.Errorf("tick: %v", err)
	}
	// No leader on port → node should try BecomeLeader.
	// Give it a moment for the goroutine to finish (tick is synchronous, so immediate).
	if n.Role() != RoleLeader {
		t.Errorf("role = %v, want LEADER after dead-leader takeover", n.Role())
	}
}

// ── tick: RoleUnknown → determineRole ────────────────────────────────────────

func TestElectionTick_UnknownBecomesLeader(t *testing.T) {
	port := freePort(t)
	n := NewNode("127.0.0.1", port, "test")
	// Role stays UNKNOWN — no BecomeLeader/BecomeFollower called.
	t.Cleanup(n.Stop)

	e := NewElection("127.0.0.1", port, n)
	if err := e.tick(context.Background()); err != nil {
		t.Errorf("tick: %v", err)
	}
	// Port is free → determineRole should elect us as LEADER.
	if n.Role() != RoleLeader {
		t.Errorf("role = %v, want LEADER", n.Role())
	}
}

// ── Start / Stop ──────────────────────────────────────────────────────────────

func TestElectionStart_Stop(t *testing.T) {
	port := freePort(t)
	n := NewNode("127.0.0.1", port, "test")
	t.Cleanup(n.Stop)

	e := NewElection("127.0.0.1", port, n)
	ctx := context.Background()

	if err := e.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// After Start the node should have a role (leader or follower).
	time.Sleep(50 * time.Millisecond)
	if n.Role() == RoleUnknown {
		t.Error("expected node to have a role after election Start")
	}

	e.Stop() // must not panic
}

// ── Concurrent: two nodes race to become leader ───────────────────────────────

// TestElection_ConcurrentStart_OneLeader starts two elections on the same port
// simultaneously and asserts that exactly one node becomes leader and the other
// settles as follower.
func TestElection_ConcurrentStart_OneLeader(t *testing.T) {
	port := freePort(t)

	n1 := NewNode("127.0.0.1", port, "test")
	n2 := NewNode("127.0.0.1", port, "test")
	t.Cleanup(n1.Stop)
	t.Cleanup(n2.Stop)

	e1 := NewElection("127.0.0.1", port, n1)
	e2 := NewElection("127.0.0.1", port, n2)
	t.Cleanup(e1.Stop)
	t.Cleanup(e2.Stop)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); e1.Start(context.Background()) }() //nolint:errcheck
	go func() { defer wg.Done(); e2.Start(context.Background()) }() //nolint:errcheck
	wg.Wait()

	// Allow monitor ticks to resolve any RoleUnknown node.
	time.Sleep(200 * time.Millisecond)

	leaders, followers := 0, 0
	for _, n := range []*Node{n1, n2} {
		switch n.Role() {
		case RoleLeader:
			leaders++
		case RoleFollower:
			followers++
		}
	}
	if leaders != 1 {
		t.Errorf("expected 1 leader, got %d (n1=%s n2=%s)", leaders, n1.RoleName(), n2.RoleName())
	}
	if followers != 1 {
		t.Errorf("expected 1 follower, got %d (n1=%s n2=%s)", followers, n1.RoleName(), n2.RoleName())
	}
}

// ── Concurrent: multiple followers race for takeover ─────────────────────────

// TestElection_ConcurrentTakeover_OneLeader verifies that when a leader dies and
// several followers call tick() simultaneously, exactly one wins the port.
func TestElection_ConcurrentTakeover_OneLeader(t *testing.T) {
	port := freePort(t)

	// Elect a real leader so followers can ping it.
	leader := NewNode("127.0.0.1", port, "test")
	if err := leader.BecomeLeader(); err != nil {
		t.Fatalf("BecomeLeader: %v", err)
	}

	const n = 3
	nodes := make([]*Node, n)
	elections := make([]*Election, n)
	for i := range nodes {
		nodes[i] = NewNode("127.0.0.1", port, "test")
		nodes[i].BecomeFollower()
		elections[i] = NewElection("127.0.0.1", port, nodes[i])
		t.Cleanup(nodes[i].Stop)
	}

	// Kill the leader — port is now free for takeover.
	leader.Stop()

	// All followers attempt takeover at the same time.
	var wg sync.WaitGroup
	ctx := context.Background()
	for _, e := range elections {
		wg.Add(1)
		e := e
		go func() { defer wg.Done(); e.tick(ctx) }() //nolint:errcheck
	}
	wg.Wait()

	newLeaders := 0
	for _, node := range nodes {
		if node.Role() == RoleLeader {
			newLeaders++
		}
	}
	if newLeaders != 1 {
		t.Errorf("expected exactly 1 new leader, got %d", newLeaders)
	}
}
