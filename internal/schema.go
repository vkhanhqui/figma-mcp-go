package internal

import (
	"fmt"
	"regexp"
	"strings"
)

// nodeIDPattern matches Figma node IDs:
//   simple:   "4029:12345"
//   compound: "I2167:9091;186:1579;186:1745" (instances/variants)
var nodeIDPattern = regexp.MustCompile(`^I?\d+:\d+(;\d+:\d+)*$`)

// NormalizeNodeID converts hyphen-format node IDs (LLM output artifact) to colon format.
// "4029-12345" → "4029:12345". No-ops for already-valid or unrecognized strings.
func NormalizeNodeID(s string) string {
	if strings.Contains(s, "-") && !strings.Contains(s, ":") {
		normalized := strings.ReplaceAll(s, "-", ":")
		if nodeIDPattern.MatchString(normalized) {
			return normalized
		}
	}
	return s
}

// ValidNodeID reports whether s is a valid Figma node ID.
func ValidNodeID(s string) bool {
	return nodeIDPattern.MatchString(s)
}

// ValidateRPC validates an incoming RPC request against the tool's expected
// input shape. Returns an error string on failure, empty string if valid.
func ValidateRPC(tool string, nodeIDs []string, params map[string]interface{}) string {
	switch tool {
	case "get_node":
		if len(nodeIDs) == 0 || nodeIDs[0] == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}

	case "get_nodes_info":
		if len(nodeIDs) == 0 {
			return "nodeIds is required and must not be empty"
		}
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
				return fmt.Sprintf("invalid nodeId: %s — must use colon format e.g. 4029:12345", id)
			}
		}

	case "get_screenshot":
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
				return fmt.Sprintf("invalid nodeId: %s — must use colon format e.g. 4029:12345", id)
			}
		}
		if format, ok := params["format"].(string); ok {
			if !validExportFormat(format) {
				return fmt.Sprintf("format must be PNG, SVG, JPG, or PDF, got: %s", format)
			}
		}

	case "save_screenshots":
		items, ok := params["items"]
		if !ok {
			return "items is required"
		}
		itemList, ok := items.([]interface{})
		if !ok || len(itemList) == 0 {
			return "items must be a non-empty array"
		}
		for i, item := range itemList {
			m, ok := item.(map[string]interface{})
			if !ok {
				return fmt.Sprintf("items[%d] must be an object", i)
			}
			nodeID, _ := m["nodeId"].(string)
			if !ValidNodeID(nodeID) {
				return fmt.Sprintf("items[%d].nodeId must use colon format e.g. 4029:12345", i)
			}
			outputPath, _ := m["outputPath"].(string)
			if outputPath == "" {
				return fmt.Sprintf("items[%d].outputPath is required", i)
			}
		}

	case "get_design_context":
		if depth, ok := params["depth"].(float64); ok {
			if depth < 0 {
				return "depth must be a non-negative number"
			}
		}
		if detail, ok := params["detail"].(string); ok && detail != "" {
			switch detail {
			case "minimal", "compact", "full":
			default:
				return fmt.Sprintf("detail must be minimal, compact, or full, got: %s", detail)
			}
		}

	case "search_nodes":
		query, _ := params["query"].(string)
		if query == "" {
			return "query is required"
		}
		if nodeID, ok := params["nodeId"].(string); ok && nodeID != "" {
			if !ValidNodeID(nodeID) {
				return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeID)
			}
		}
		if limit, ok := params["limit"].(float64); ok && limit <= 0 {
			return "limit must be a positive number"
		}

	case "get_reactions":
		if len(nodeIDs) == 0 || nodeIDs[0] == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}

	case "scan_text_nodes", "scan_nodes_by_types":
		nodeID, _ := params["nodeId"].(string)
		if nodeID == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeID) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeID)
		}
		if tool == "scan_nodes_by_types" {
			types, ok := params["types"].([]interface{})
			if !ok || len(types) == 0 {
				return "types must be a non-empty array"
			}
		}

	// ── Write tools ──────────────────────────────────────────────────────────

	case "create_frame":
		if w, ok := params["width"].(float64); ok && w <= 0 {
			return "width must be positive"
		}
		if h, ok := params["height"].(float64); ok && h <= 0 {
			return "height must be positive"
		}
		if lm, ok := params["layoutMode"].(string); ok && lm != "" {
			switch lm {
			case "HORIZONTAL", "VERTICAL", "NONE":
			default:
				return fmt.Sprintf("layoutMode must be HORIZONTAL, VERTICAL, or NONE, got: %s", lm)
			}
		}
		if pid, ok := params["parentId"].(string); ok && pid != "" && !ValidNodeID(pid) {
			return fmt.Sprintf("parentId must use colon format e.g. 4029:12345, got: %s", pid)
		}

	case "create_rectangle", "create_ellipse":
		if w, ok := params["width"].(float64); ok && w <= 0 {
			return "width must be positive"
		}
		if h, ok := params["height"].(float64); ok && h <= 0 {
			return "height must be positive"
		}
		if pid, ok := params["parentId"].(string); ok && pid != "" && !ValidNodeID(pid) {
			return fmt.Sprintf("parentId must use colon format e.g. 4029:12345, got: %s", pid)
		}

	case "create_text":
		if text, _ := params["text"].(string); text == "" {
			return "text is required"
		}
		if pid, ok := params["parentId"].(string); ok && pid != "" && !ValidNodeID(pid) {
			return fmt.Sprintf("parentId must use colon format e.g. 4029:12345, got: %s", pid)
		}

	case "set_text":
		if len(nodeIDs) == 0 || nodeIDs[0] == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}
		if _, ok := params["text"].(string); !ok {
			return "text is required"
		}

	case "set_fills":
		if len(nodeIDs) == 0 || nodeIDs[0] == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}
		if color, _ := params["color"].(string); color == "" {
			return "color is required (hex string e.g. #FF5733)"
		}

	case "set_strokes":
		if len(nodeIDs) == 0 || nodeIDs[0] == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}
		if color, _ := params["color"].(string); color == "" {
			return "color is required (hex string e.g. #FF5733)"
		}

	case "move_nodes":
		if len(nodeIDs) == 0 {
			return "nodeIds is required"
		}
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
				return fmt.Sprintf("invalid nodeId: %s — must use colon format e.g. 4029:12345", id)
			}
		}
		_, hasX := params["x"]
		_, hasY := params["y"]
		if !hasX && !hasY {
			return "at least one of x or y is required"
		}

	case "resize_nodes":
		if len(nodeIDs) == 0 {
			return "nodeIds is required"
		}
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
				return fmt.Sprintf("invalid nodeId: %s — must use colon format e.g. 4029:12345", id)
			}
		}
		_, hasW := params["width"]
		_, hasH := params["height"]
		if !hasW && !hasH {
			return "at least one of width or height is required"
		}

	case "delete_nodes":
		if len(nodeIDs) == 0 {
			return "nodeIds is required and must not be empty"
		}
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
				return fmt.Sprintf("invalid nodeId: %s — must use colon format e.g. 4029:12345", id)
			}
		}

	case "rename_node":
		if len(nodeIDs) == 0 || nodeIDs[0] == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}
		if name, _ := params["name"].(string); name == "" {
			return "name is required"
		}

	case "clone_node":
		if len(nodeIDs) == 0 || nodeIDs[0] == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}
		if pid, ok := params["parentId"].(string); ok && pid != "" && !ValidNodeID(pid) {
			return fmt.Sprintf("parentId must use colon format e.g. 4029:12345, got: %s", pid)
		}

	case "import_image":
		if imageData, _ := params["imageData"].(string); imageData == "" {
			return "imageData (base64) is required"
		}
		if sm, ok := params["scaleMode"].(string); ok && sm != "" {
			switch sm {
			case "FILL", "FIT", "CROP", "TILE":
			default:
				return fmt.Sprintf("scaleMode must be FILL, FIT, CROP, or TILE, got: %s", sm)
			}
		}
		if pid, ok := params["parentId"].(string); ok && pid != "" && !ValidNodeID(pid) {
			return fmt.Sprintf("parentId must use colon format e.g. 4029:12345, got: %s", pid)
		}
	}

	return ""
}

func validExportFormat(f string) bool {
	switch f {
	case "PNG", "SVG", "JPG", "PDF":
		return true
	}
	return false
}
