package internal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/vkhanhqui/figma-mcp-go/internal/prompts"
)

// RegisterTools registers all MCP tools on the server.
func RegisterTools(s *server.MCPServer, node *Node) {
	registerReadTools(s, node)
	registerWriteTools(s, node)
}

// RegisterPrompts registers MCP prompts on the server.
func RegisterPrompts(s *server.MCPServer) {
	prompts.RegisterAll(s)
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// makeHandler creates a simple tool handler with no parameters.
func makeHandler(node *Node, command string, nodeIDs []string, params map[string]interface{}) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		defer startAutoProgress(ctx, req, command)()
		resp, err := node.Send(ctx, command, nodeIDs, params)
		return renderResponse(resp, err)
	}
}

// renderResponse converts a BridgeResponse into an MCP tool result.
func renderResponse(resp BridgeResponse, err error) (*mcp.CallToolResult, error) {
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if resp.Error != "" {
		return mcp.NewToolResultError(resp.Error), nil
	}
	text, err := json.Marshal(resp.Data)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshal response: %v", err)), nil
	}
	return mcp.NewToolResultText(string(text)), nil
}

// toStringSlice converts []interface{} to []string.
func toStringSlice(raw []interface{}) []string {
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// ── save_screenshots ─────────────────────────────────────────────────────────

type saveItem struct {
	NodeID     string  `json:"nodeId"`
	OutputPath string  `json:"outputPath"`
	Format     string  `json:"format,omitempty"`
	Scale      float64 `json:"scale,omitempty"`
}

type saveResult struct {
	Index        int     `json:"index"`
	NodeID       string  `json:"nodeId"`
	NodeName     string  `json:"nodeName,omitempty"`
	OutputPath   string  `json:"outputPath"`
	Format       string  `json:"format,omitempty"`
	Width        float64 `json:"width,omitempty"`
	Height       float64 `json:"height,omitempty"`
	BytesWritten int     `json:"bytesWritten,omitempty"`
	Success      bool    `json:"success"`
	Error        string  `json:"error,omitempty"`
}

// preparedItem holds the validated server-side state for one save_screenshots
// row. It is built from the user's input and used both to drive the batch RPC
// and to write the resulting bytes to disk.
type preparedItem struct {
	Index        int
	NodeID       string
	OutputPath   string // resolved absolute path
	OriginalPath string // user-supplied path, used in error messages
	Format       string
	Scale        float64
	Err          string // pre-flight failure (e.g. format conflict, bad path)
}

func executeSaveScreenshots(ctx context.Context, node *Node, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	defer startAutoProgress(ctx, req, "save_screenshots")()
	rawItems, _ := req.GetArguments()["items"].([]interface{})
	defaultFormat, _ := req.GetArguments()["format"].(string)
	defaultScale, _ := req.GetArguments()["scale"].(float64)

	workDir, err := os.Getwd()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("getwd: %v", err)), nil
	}

	prepared := make([]preparedItem, len(rawItems))
	results := make([]saveResult, len(rawItems))

	exportItems := make([]map[string]interface{}, 0, len(rawItems))

	for i, rawItem := range rawItems {
		item, err := parseSaveItem(rawItem)
		if err != nil {
			prepared[i] = preparedItem{Index: i, Err: err.Error()}
			results[i] = saveResult{Index: i, Error: err.Error()}
			continue
		}
		p := buildPreparedItem(i, item, workDir, defaultFormat, defaultScale)
		prepared[i] = p
		if p.Err != "" {
			results[i] = saveResult{Index: i, NodeID: p.NodeID, OutputPath: p.OriginalPath, Error: p.Err}
			continue
		}
		exportItems = append(exportItems, map[string]interface{}{
			"index":  i,
			"nodeId": p.NodeID,
			"format": p.Format,
			"scale":  p.Scale,
		})
	}

	if len(exportItems) > 0 {
		exportResults, err := callExportNodesBatch(ctx, node, exportItems, defaultFormat, defaultScale)
		if err != nil {
			for _, p := range prepared {
				if p.Err == "" {
					results[p.Index] = saveResult{Index: p.Index, NodeID: p.NodeID, OutputPath: p.OutputPath, Error: err.Error()}
				}
			}
		} else {
			for _, exp := range exportResults {
				idx := exp.Index
				if idx < 0 || idx >= len(prepared) {
					continue
				}
				p := prepared[idx]
				if p.Err != "" {
					continue
				}
				if !exp.Success {
					results[idx] = saveResult{Index: idx, NodeID: p.NodeID, OutputPath: p.OutputPath, Error: exp.Error}
					continue
				}
				// Binary-frame path: plugin returns raw `Bytes` and no `Base64`.
				// Fall back to base64 decode for legacy plugins that still send
				// the text-encoded field.
				var bytes int
				var werr error
				if len(exp.Bytes) > 0 {
					bytes, werr = writeRawBytes(exp.Bytes, p.OutputPath)
				} else {
					bytes, werr = writeBase64(exp.Base64, p.OutputPath)
				}
				if werr != nil {
					results[idx] = saveResult{Index: idx, NodeID: p.NodeID, OutputPath: p.OutputPath, Error: werr.Error()}
					continue
				}
				results[idx] = saveResult{
					Index:        idx,
					NodeID:       exp.NodeID,
					NodeName:     exp.NodeName,
					OutputPath:   p.OutputPath,
					Format:       p.Format,
					Width:        exp.Width,
					Height:       exp.Height,
					BytesWritten: bytes,
					Success:      true,
				}
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

func buildPreparedItem(index int, item saveItem, workDir, defaultFormat string, defaultScale float64) preparedItem {
	resolvedPath, err := resolveOutputPath(item.OutputPath, workDir)
	if err != nil {
		return preparedItem{Index: index, NodeID: item.NodeID, OriginalPath: item.OutputPath, Err: err.Error()}
	}

	format := coalesce(item.Format, defaultFormat)
	inferredFormat := inferFormat(resolvedPath)
	if format == "" {
		format = inferredFormat
	}
	if format == "" {
		format = "PNG"
	}
	if inferredFormat != "" && format != inferredFormat {
		return preparedItem{
			Index: index, NodeID: item.NodeID, OutputPath: resolvedPath, OriginalPath: resolvedPath,
			Err: fmt.Sprintf("format %s conflicts with file extension %s", format, inferredFormat),
		}
	}

	scale := item.Scale
	if scale <= 0 {
		scale = defaultScale
	}
	if scale <= 0 {
		scale = 2
	}

	return preparedItem{
		Index:        index,
		NodeID:       item.NodeID,
		OutputPath:   resolvedPath,
		OriginalPath: resolvedPath,
		Format:       format,
		Scale:        scale,
	}
}

type exportNodesBatchResult struct {
	Index    int     `json:"index"`
	Success  bool    `json:"success"`
	Error    string  `json:"error,omitempty"`
	NodeID   string  `json:"nodeId"`
	NodeName string  `json:"nodeName,omitempty"`
	Format   string  `json:"format,omitempty"`
	// Bytes is populated when the plugin returns a binary frame; the wire
	// payload is spliced here directly. Base64 is the legacy text-frame field
	// and stays for backwards compatibility with pre-1.2.0 plugins.
	Bytes  []byte  `json:"bytes,omitempty"`
	Base64 string  `json:"base64,omitempty"`
	Width  float64 `json:"width,omitempty"`
	Height float64 `json:"height,omitempty"`
}

func callExportNodesBatch(ctx context.Context, node *Node, items []map[string]interface{}, defaultFormat string, defaultScale float64) ([]exportNodesBatchResult, error) {
	wireItems := make([]interface{}, len(items))
	for i, it := range items {
		wireItems[i] = it
	}
	params := map[string]interface{}{"items": wireItems}
	if defaultFormat != "" {
		params["format"] = defaultFormat
	}
	if defaultScale > 0 {
		params["scale"] = defaultScale
	}

	resp, err := node.Send(ctx, "export_nodes_batch", nil, params)
	if err != nil {
		return nil, err
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("%s", resp.Error)
	}

	b, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal plugin data: %w", err)
	}
	var wrapper struct {
		Results []exportNodesBatchResult `json:"results"`
	}
	if err := json.Unmarshal(b, &wrapper); err != nil {
		return nil, fmt.Errorf("unmarshal plugin data: %w", err)
	}
	return wrapper.Results, nil
}

type screenshotExport struct {
	NodeID   string  `json:"nodeId"`
	NodeName string  `json:"nodeName"`
	Bytes    []byte  `json:"bytes,omitempty"`
	Base64   string  `json:"base64,omitempty"`
	Width    float64 `json:"width"`
	Height   float64 `json:"height"`
}

func extractScreenshotExport(data interface{}) (screenshotExport, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return screenshotExport{}, err
	}
	var wrapper struct {
		Exports []screenshotExport `json:"exports"`
	}
	if err := json.Unmarshal(b, &wrapper); err != nil {
		return screenshotExport{}, err
	}
	if len(wrapper.Exports) == 0 {
		return screenshotExport{}, errors.New("no screenshot export returned by plugin")
	}
	return wrapper.Exports[0], nil
}

func writeBase64(b64, outputPath string) (int, error) {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return 0, fmt.Errorf("base64 decode: %w", err)
	}
	return writeRawBytes(data, outputPath)
}

// writeRawBytes writes raw bytes (already decoded from any wire format) to
// outputPath. Skips the base64 decode step on the binary-frame fast path.
func writeRawBytes(data []byte, outputPath string) (int, error) {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return 0, fmt.Errorf("mkdir: %w", err)
	}
	f, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		if os.IsExist(err) {
			return 0, fmt.Errorf("file already exists at outputPath: %s", outputPath)
		}
		return 0, err
	}
	defer f.Close()
	return f.Write(data)
}

func resolveOutputPath(outputPath, workDir string) (string, error) {
	if filepath.IsAbs(outputPath) {
		return mustBeInsideDir(filepath.Clean(outputPath), workDir)
	}
	return mustBeInsideDir(filepath.Join(workDir, outputPath), workDir)
}

func mustBeInsideDir(resolved, workDir string) (string, error) {
	rel, err := filepath.Rel(workDir, resolved)
	if err != nil {
		return "", fmt.Errorf("outputPath must be inside the working directory: %s", workDir)
	}
	// Convert to forward slashes before prefix check so Windows paths like
	// "C:\.." don't bypass the ".." detection.
	if strings.HasPrefix(filepath.ToSlash(rel), "..") {
		return "", fmt.Errorf("outputPath must be inside the working directory: %s", workDir)
	}
	return resolved, nil
}

func inferFormat(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return "PNG"
	case ".svg":
		return "SVG"
	case ".jpg", ".jpeg":
		return "JPG"
	case ".pdf":
		return "PDF"
	}
	return ""
}

func parseSaveItem(raw interface{}) (saveItem, error) {
	b, err := json.Marshal(raw)
	if err != nil {
		return saveItem{}, err
	}
	var item saveItem
	if err := json.Unmarshal(b, &item); err != nil {
		return saveItem{}, err
	}
	return item, nil
}

func coalesce(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
