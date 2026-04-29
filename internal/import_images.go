package internal

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// importItemResult mirrors what the plugin reports back per item, plus any
// server-side resolution failures we tracked before the batch was even sent.
type importItemResult struct {
	Index       int                    `json:"index"`
	Success     bool                   `json:"success"`
	Error       string                 `json:"error,omitempty"`
	ID          string                 `json:"id,omitempty"`
	Name        string                 `json:"name,omitempty"`
	Type        string                 `json:"type,omitempty"`
	Bounds      map[string]interface{} `json:"bounds,omitempty"`
	ImageHash   string                 `json:"imageHash,omitempty"`
	ContentHash string                 `json:"contentHash,omitempty"`
	Cached      bool                   `json:"cached,omitempty"`
}

// executeImportImages resolves each item server-side, forwards the batch to
// the plugin, and merges any pre-flight resolution errors with the plugin
// results so callers see one unified `results` array indexed by input order.
func executeImportImages(ctx context.Context, node *Node, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	defer startAutoProgress(ctx, req, "import_images")()
	args := req.GetArguments()
	rawItems, _ := args["items"].([]interface{})
	parentID, _ := args["parentId"].(string)

	if len(rawItems) == 0 {
		return mcp.NewToolResultError("items must be a non-empty array"), nil
	}

	resolvedItems := make([]map[string]interface{}, 0, len(rawItems))
	pluginIndexToInput := make([]int, 0, len(rawItems)) // maps batch index → original input index
	preFlightErrors := make([]importItemResult, 0)

	for i, raw := range rawItems {
		item, ok := raw.(map[string]interface{})
		if !ok {
			preFlightErrors = append(preFlightErrors, importItemResult{Index: i, Error: "item must be an object"})
			continue
		}
		resolved, err := ResolveImage(ctx, item)
		if err != nil {
			preFlightErrors = append(preFlightErrors, importItemResult{Index: i, Error: err.Error()})
			continue
		}
		out := map[string]interface{}{
			"imageData":   resolved.Base64Data(),
			"contentHash": resolved.ContentHash,
		}
		CopyFloat(out, item, "x")
		CopyFloat(out, item, "y")
		CopyFloat(out, item, "width")
		CopyFloat(out, item, "height")
		CopyString(out, item, "name")
		CopyString(out, item, "scaleMode")
		resolvedItems = append(resolvedItems, out)
		pluginIndexToInput = append(pluginIndexToInput, i)
	}

	results := make([]importItemResult, len(rawItems))
	for _, e := range preFlightErrors {
		results[e.Index] = e
	}

	if len(resolvedItems) > 0 {
		pluginResults, err := sendImportImagesBatch(ctx, node, resolvedItems, parentID)
		if err != nil {
			// Whole-batch failure — propagate as one error per resolved item so
			// callers can still see which inputs were affected.
			for _, idx := range pluginIndexToInput {
				results[idx] = importItemResult{Index: idx, Error: err.Error()}
			}
		} else {
			for batchIdx, r := range pluginResults {
				if batchIdx >= len(pluginIndexToInput) {
					break
				}
				origIdx := pluginIndexToInput[batchIdx]
				r.Index = origIdx
				results[origIdx] = r
			}
		}
	}

	succeeded, failed := 0, 0
	for _, r := range results {
		if r.Success {
			succeeded++
		} else {
			failed++
		}
	}

	out, err := json.Marshal(map[string]interface{}{
		"total":     len(results),
		"succeeded": succeeded,
		"failed":    failed,
		"hasErrors": failed > 0,
		"results":   results,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshal results: %v", err)), nil
	}
	return mcp.NewToolResultText(string(out)), nil
}

func sendImportImagesBatch(ctx context.Context, node *Node, items []map[string]interface{}, parentID string) ([]importItemResult, error) {
	wireItems := make([]interface{}, len(items))
	for i, m := range items {
		wireItems[i] = m
	}
	params := map[string]interface{}{"items": wireItems}
	if parentID != "" {
		params["parentId"] = parentID
	}

	resp, err := node.Send(ctx, "import_images", nil, params)
	if err != nil {
		return nil, err
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	// Plugin returns `{ items: [importItemResult, ...] }`. Round-trip through
	// JSON so we get strong typing without manually walking interface{} maps.
	b, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal plugin data: %w", err)
	}
	var wrapper struct {
		Items []importItemResult `json:"items"`
	}
	if err := json.Unmarshal(b, &wrapper); err != nil {
		return nil, fmt.Errorf("unmarshal plugin data: %w", err)
	}
	return wrapper.Items, nil
}

