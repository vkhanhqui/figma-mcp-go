package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

// toolsListResponse mirrors the subset of the MCP tools/list JSON-RPC response
// that we need to inspect for schema correctness.
type toolsListResponse struct {
	Result struct {
		Tools []struct {
			Name        string `json:"name"`
			InputSchema struct {
				Properties map[string]propertySchema `json:"properties"`
			} `json:"inputSchema"`
		} `json:"tools"`
	} `json:"result"`
}

type propertySchema struct {
	Type  string          `json:"type"`
	Items json.RawMessage `json:"items"`
}

// listTools calls tools/list through the server's HandleMessage path and returns
// the parsed response.
func listTools(t *testing.T) toolsListResponse {
	t.Helper()
	s, _ := newTestServer(t)
	msg := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`
	raw := s.HandleMessage(context.Background(), []byte(msg))
	if raw == nil {
		t.Fatal("HandleMessage returned nil for tools/list")
	}
	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal tools/list response: %v", err)
	}
	var resp toolsListResponse
	if err := json.Unmarshal(b, &resp); err != nil {
		t.Fatalf("unmarshal tools/list response: %v", err)
	}
	return resp
}

// TestToolSchemas_ArrayItemsHaveType ensures every array-typed parameter across
// all registered tools declares an items.type.  Missing items (or items without
// a type) is the exact class of bug that causes GitHub Copilot MCP validation to
// fail (see commit af0325c).
func TestToolSchemas_ArrayItemsHaveType(t *testing.T) {
	resp := listTools(t)

	if len(resp.Result.Tools) == 0 {
		t.Fatal("tools/list returned no tools — registration may have failed")
	}

	type violation struct {
		tool, param, reason string
	}
	var violations []violation

	for _, tool := range resp.Result.Tools {
		for param, prop := range tool.InputSchema.Properties {
			if prop.Type != "array" {
				continue
			}

			if len(prop.Items) == 0 || string(prop.Items) == "null" {
				violations = append(violations, violation{
					tool:   tool.Name,
					param:  param,
					reason: "items is missing",
				})
				continue
			}

			var items map[string]any
			if err := json.Unmarshal(prop.Items, &items); err != nil {
				violations = append(violations, violation{
					tool:   tool.Name,
					param:  param,
					reason: fmt.Sprintf("items is not a valid JSON object: %v", err),
				})
				continue
			}

			if _, ok := items["type"]; !ok {
				violations = append(violations, violation{
					tool:   tool.Name,
					param:  param,
					reason: "items.type is missing",
				})
			}
		}
	}

	for _, v := range violations {
		t.Errorf("tool %q param %q: %s", v.tool, v.param, v.reason)
	}
}

// TestToolSchemas_AllToolsRegistered asserts the expected tool count so that
// accidentally dropped registrations are caught.
func TestToolSchemas_AllToolsRegistered(t *testing.T) {
	resp := listTools(t)
	const want = 74
	got := len(resp.Result.Tools)
	if got != want {
		t.Errorf("expected %d registered tools, got %d — update the constant if tools were intentionally added or removed", want, got)
	}
}
