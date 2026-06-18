# Figma MCP Go Security Hardening Plan

**Goal:** Reduce the risk of unauthorized local or network access to the Figma bridge without breaking the existing default setup.

**Requirements:**
- [x] Make a new branch for PR work.
- [x] Write a concrete security hardening plan.
- [x] Improve security as much as practical without breaking existing users.
- [x] Update README/security guidance for a public fork and PR.
- [x] Keep changes small enough to review and test.

**Architecture:** Add opt-in shared-token authentication at the bridge boundary while keeping unauthenticated localhost behavior as the default for compatibility. The Go server will enforce the token for WebSocket plugin connections, follower RPC, and health checks when configured; the plugin UI will persist and pass the token in the WebSocket URL. Add conservative request-size limiting and log redaction around sensitive fields.

**Tech Stack:** Go HTTP/WebSocket server, `github.com/coder/websocket`, Figma plugin TypeScript/Svelte, existing Go and Bun tests.

---

## Tasks

### Task 1: Branch and Baseline

**Files:**
- Modify: git branch `security-hardening-bridge`
- Inspect: `cmd/figma-mcp-go/main.go`
- Inspect: `internal/bridge.go`
- Inspect: `internal/leader.go`
- Inspect: `internal/follower.go`
- Inspect: `internal/node.go`
- Inspect: `plugin/src/main.ts`
- Inspect: `plugin/src/ui/App.svelte`

**Steps:**
- [x] Clone `MiranDaniel/figma-mcp-go`.
- [x] Add `upstream` remote for `vkhanhqui/figma-mcp-go`.
- [x] Create branch `security-hardening-bridge`.
- [x] Confirm no local `CLAUDE.md`, `AGENTS.md`, or `GEMINI.md` constraints.

### Task 2: Add Optional Bridge Authentication

**Files:**
- Modify: `cmd/figma-mcp-go/main.go`
- Modify: `internal/bridge.go`
- Modify: `internal/leader.go`
- Modify: `internal/follower.go`
- Modify: `internal/node.go`
- Modify: `internal/election.go`
- Test: `internal/leader_test.go`
- Test: `internal/follower_test.go`
- Test: `internal/node_test.go`
- Test: `internal/election_test.go`

**Steps:**
- [x] Add `--auth-token` CLI flag and `FIGMA_MCP_AUTH_TOKEN` environment fallback.
- [x] Thread the token through `Node`, `Leader`, `Follower`, and `Election`.
- [x] Require the token for `/ws`, `/rpc`, and `/ping` only when configured.
- [x] Accept `Authorization: Bearer <token>` for HTTP RPC/ping.
- [x] Accept `?token=<token>` for Figma WebSocket connections.
- [x] Preserve current behavior when no token is configured.
- [x] Add tests for accepted token, missing token, wrong token, and no-token compatibility.

### Task 3: Limit and Redact

**Files:**
- Modify: `internal/leader.go`
- Modify: `internal/follower.go`
- Modify: `internal/bridge.go`
- Test: `internal/leader_test.go`

**Steps:**
- [x] Limit `/rpc` request bodies before JSON decoding.
- [x] Return HTTP 413 for oversized RPC bodies.
- [x] Redact token-like fields from bridge/follower logs.
- [x] Avoid logging full params maps where user content or secrets may appear.

### Task 4: Plugin Token Setting

**Files:**
- Modify: `plugin/src/main.ts`
- Modify: `plugin/src/ui/App.svelte`

**Steps:**
- [x] Persist `authToken` alongside host and port in `figma.clientStorage`.
- [x] Add a compact token input in settings.
- [x] Append the token to the WebSocket URL only when present.
- [x] Keep existing host/port-only plugin configuration working.

### Task 5: README Safe-Use Guidance

**Files:**
- Modify: `README.md`

**Steps:**
- [x] Document the threat model: local bridge, full Figma read/write, optional disk writes.
- [x] Recommend keeping `--ip 127.0.0.1`.
- [x] Document `FIGMA_MCP_AUTH_TOKEN` / `--auth-token` setup for MCP clients and the plugin UI.
- [x] Warn against `--ip 0.0.0.0` unless firewalling and token auth are used.
- [x] Mention that tokens should not be committed to project MCP config.

### Task 6: Dependency Audit

**Files:**
- Modify: `plugin/package.json`
- Modify: `plugin/bun.lock`

**Steps:**
- [x] Audit plugin dependencies.
- [x] Update vulnerable plugin build dependencies within the existing major versions.
- [x] Confirm the plugin dependency audit reports no known vulnerabilities.

### Task 7: CI Verification

**Files:**
- Verify in CI: Go package tests
- Verify in CI: plugin tests/build

**Steps:**
- [ ] Run Go tests in CI.
- [ ] Run plugin tests in CI.
- [ ] Run plugin build in CI.
- [ ] Review CI logs before merge.

**Note:** Local test and build execution is intentionally excluded from this workflow. Verification should run in CI for the PR.

### Commit Points

- Commit 1: optional bridge authentication and tests.
- Commit 2: request limit, log redaction, and tests.
- Commit 3: plugin token settings, dependency updates, and README guidance.
