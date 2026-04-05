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

	case "set_opacity":
		if len(nodeIDs) == 0 {
			return "nodeIds is required"
		}
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
				return fmt.Sprintf("invalid nodeId: %s — must use colon format e.g. 4029:12345", id)
			}
		}
		op, ok := params["opacity"].(float64)
		if !ok {
			return "opacity is required"
		}
		if op < 0 || op > 1 {
			return "opacity must be between 0 and 1"
		}

	case "set_corner_radius":
		if len(nodeIDs) == 0 {
			return "nodeIds is required"
		}
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
				return fmt.Sprintf("invalid nodeId: %s — must use colon format e.g. 4029:12345", id)
			}
		}
		_, hasUniform := params["cornerRadius"]
		_, hasTL := params["topLeftRadius"]
		_, hasTR := params["topRightRadius"]
		_, hasBL := params["bottomLeftRadius"]
		_, hasBR := params["bottomRightRadius"]
		if !hasUniform && !hasTL && !hasTR && !hasBL && !hasBR {
			return "at least one of cornerRadius, topLeftRadius, topRightRadius, bottomLeftRadius, or bottomRightRadius is required"
		}

	case "group_nodes":
		if len(nodeIDs) < 2 {
			return "nodeIds must contain at least 2 nodes to group"
		}
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
				return fmt.Sprintf("invalid nodeId: %s — must use colon format e.g. 4029:12345", id)
			}
		}

	case "ungroup_nodes":
		if len(nodeIDs) == 0 {
			return "nodeIds is required and must not be empty"
		}
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
				return fmt.Sprintf("invalid nodeId: %s — must use colon format e.g. 4029:12345", id)
			}
		}

	case "navigate_to_page":
		pageID, _ := params["pageId"].(string)
		pageName, _ := params["pageName"].(string)
		if pageID == "" && pageName == "" {
			return "pageId or pageName is required"
		}

	case "create_component":
		if len(nodeIDs) == 0 || nodeIDs[0] == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}

	case "export_tokens":
		if format, ok := params["format"].(string); ok && format != "" {
			switch format {
			case "json", "css":
			default:
				return fmt.Sprintf("format must be json or css, got: %s", format)
			}
		}

	case "create_frame":
		if w, ok := params["width"].(float64); ok && w <= 0 {
			return "width must be positive"
		}
		if h, ok := params["height"].(float64); ok && h <= 0 {
			return "height must be positive"
		}
		if pid, ok := params["parentId"].(string); ok && pid != "" && !ValidNodeID(pid) {
			return fmt.Sprintf("parentId must use colon format e.g. 4029:12345, got: %s", pid)
		}
		if msg := validateAutoLayoutParams(params); msg != "" {
			return msg
		}

	case "set_auto_layout":
		if len(nodeIDs) == 0 || nodeIDs[0] == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}
		if msg := validateAutoLayoutParams(params); msg != "" {
			return msg
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
		if mode, ok := params["mode"].(string); ok && mode != "replace" && mode != "append" {
			return "mode must be 'replace' or 'append'"
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
		if mode, ok := params["mode"].(string); ok && mode != "replace" && mode != "append" {
			return "mode must be 'replace' or 'append'"
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

	// ── Style tools ──────────────────────────────────────────────────────────

	case "create_paint_style":
		if name, _ := params["name"].(string); name == "" {
			return "name is required"
		}
		if color, _ := params["color"].(string); color == "" {
			return "color is required (hex string e.g. #FF5733)"
		}

	case "create_text_style":
		if name, _ := params["name"].(string); name == "" {
			return "name is required"
		}
		if td, ok := params["textDecoration"].(string); ok && td != "" {
			switch td {
			case "NONE", "UNDERLINE", "STRIKETHROUGH":
			default:
				return fmt.Sprintf("textDecoration must be NONE, UNDERLINE, or STRIKETHROUGH, got: %s", td)
			}
		}
		if unit, ok := params["lineHeightUnit"].(string); ok && unit != "" {
			switch unit {
			case "PIXELS", "PERCENT":
			default:
				return fmt.Sprintf("lineHeightUnit must be PIXELS or PERCENT, got: %s", unit)
			}
		}
		if unit, ok := params["letterSpacingUnit"].(string); ok && unit != "" {
			switch unit {
			case "PIXELS", "PERCENT":
			default:
				return fmt.Sprintf("letterSpacingUnit must be PIXELS or PERCENT, got: %s", unit)
			}
		}

	case "create_effect_style":
		if name, _ := params["name"].(string); name == "" {
			return "name is required"
		}
		if t, ok := params["type"].(string); ok && t != "" {
			switch t {
			case "DROP_SHADOW", "INNER_SHADOW", "LAYER_BLUR", "BACKGROUND_BLUR":
			default:
				return fmt.Sprintf("type must be DROP_SHADOW, INNER_SHADOW, LAYER_BLUR, or BACKGROUND_BLUR, got: %s", t)
			}
		}

	case "create_grid_style":
		if name, _ := params["name"].(string); name == "" {
			return "name is required"
		}
		if p, ok := params["pattern"].(string); ok && p != "" {
			switch p {
			case "GRID", "COLUMNS", "ROWS":
			default:
				return fmt.Sprintf("pattern must be GRID, COLUMNS, or ROWS, got: %s", p)
			}
		}
		if a, ok := params["alignment"].(string); ok && a != "" {
			switch a {
			case "STRETCH", "CENTER", "MIN", "MAX":
			default:
				return fmt.Sprintf("alignment must be STRETCH, CENTER, MIN, or MAX, got: %s", a)
			}
		}

	case "update_paint_style":
		if styleId, _ := params["styleId"].(string); styleId == "" {
			return "styleId is required"
		}
		_, hasName := params["name"]
		_, hasColor := params["color"]
		_, hasDesc := params["description"]
		if !hasName && !hasColor && !hasDesc {
			return "at least one of name, color, or description is required"
		}

	case "delete_style":
		if styleId, _ := params["styleId"].(string); styleId == "" {
			return "styleId is required"
		}

	// ── Variable tools ───────────────────────────────────────────────────────

	case "create_variable_collection":
		if name, _ := params["name"].(string); name == "" {
			return "name is required"
		}

	case "add_variable_mode":
		if collectionId, _ := params["collectionId"].(string); collectionId == "" {
			return "collectionId is required"
		}
		if modeName, _ := params["modeName"].(string); modeName == "" {
			return "modeName is required"
		}

	case "create_variable":
		if name, _ := params["name"].(string); name == "" {
			return "name is required"
		}
		if collectionId, _ := params["collectionId"].(string); collectionId == "" {
			return "collectionId is required"
		}
		varType, _ := params["type"].(string)
		switch varType {
		case "COLOR", "FLOAT", "STRING", "BOOLEAN":
		default:
			return fmt.Sprintf("type must be COLOR, FLOAT, STRING, or BOOLEAN, got: %s", varType)
		}

	case "set_variable_value":
		if variableId, _ := params["variableId"].(string); variableId == "" {
			return "variableId is required"
		}
		if modeId, _ := params["modeId"].(string); modeId == "" {
			return "modeId is required"
		}
		if _, ok := params["value"]; !ok {
			return "value is required"
		}

	case "delete_variable":
		vid, _ := params["variableId"].(string)
		cid, _ := params["collectionId"].(string)
		if vid == "" && cid == "" {
			return "variableId or collectionId is required"
		}

	// ── Linked tools ─────────────────────────────────────────────────────────

	case "apply_style_to_node":
		if len(nodeIDs) == 0 || nodeIDs[0] == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}
		if styleId, _ := params["styleId"].(string); styleId == "" {
			return "styleId is required"
		}
		if target, ok := params["target"].(string); ok && target != "" {
			switch target {
			case "fill", "stroke":
			default:
				return fmt.Sprintf("target must be fill or stroke, got: %s", target)
			}
		}

	case "bind_variable_to_node":
		if len(nodeIDs) == 0 || nodeIDs[0] == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}
		if variableId, _ := params["variableId"].(string); variableId == "" {
			return "variableId is required"
		}
		if field, _ := params["field"].(string); field == "" {
			return "field is required"
		}

	case "swap_component":
		if len(nodeIDs) == 0 || nodeIDs[0] == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}
		if componentId, _ := params["componentId"].(string); componentId == "" {
			return "componentId is required"
		}
		if cid, _ := params["componentId"].(string); cid != "" && !ValidNodeID(cid) {
			return fmt.Sprintf("componentId must use colon format e.g. 4029:12345, got: %s", cid)
		}

	case "detach_instance":
		if len(nodeIDs) == 0 {
			return "nodeIds is required and must not be empty"
		}
		for _, id := range nodeIDs {
			if !ValidNodeID(id) {
				return fmt.Sprintf("invalid nodeId: %s — must use colon format e.g. 4029:12345", id)
			}
		}

	// ── Prototype tools ──────────────────────────────────────────────────────

	case "set_reactions":
		if len(nodeIDs) == 0 || nodeIDs[0] == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}
		rawReactions, ok := params["reactions"]
		if !ok {
			return "reactions is required"
		}
		reactions, ok := rawReactions.([]any)
		if !ok {
			return "reactions must be an array"
		}
		if mode, ok := params["mode"].(string); ok && mode != "" {
			if mode != "replace" && mode != "append" {
				return fmt.Sprintf("mode must be 'replace' or 'append', got: %s", mode)
			}
		}
		for i, raw := range reactions {
			r, ok := raw.(map[string]any)
			if !ok {
				return fmt.Sprintf("reactions[%d] must be an object", i)
			}
			if msg := validateReaction(i, r); msg != "" {
				return msg
			}
		}

	case "remove_reactions":
		if len(nodeIDs) == 0 || nodeIDs[0] == "" {
			return "nodeId is required"
		}
		if !ValidNodeID(nodeIDs[0]) {
			return fmt.Sprintf("nodeId must use colon format e.g. 4029:12345, got: %s", nodeIDs[0])
		}
		if raw, ok := params["indices"].([]any); ok {
			for i, v := range raw {
				if _, ok := v.(float64); !ok {
					return fmt.Sprintf("indices[%d] must be a number", i)
				}
			}
		}
	}

	return ""
}

var validTriggerTypes = map[string]bool{
	"ON_CLICK": true, "ON_HOVER": true, "ON_PRESS": true, "ON_DRAG": true,
	"AFTER_TIMEOUT": true, "MOUSE_ENTER": true, "MOUSE_LEAVE": true,
	"MOUSE_UP": true, "MOUSE_DOWN": true,
}

var validActionTypes = map[string]bool{
	// Current Figma plugin API action types (plugin-api >= 1.0.0)
	"NODE": true, "BACK": true, "CLOSE": true, "URL": true,
	"CONDITIONAL": true, "SET_VARIABLE": true, "SET_VARIABLE_MODE": true,
	"UPDATE_MEDIA_RUNTIME": true,
}

func validateReaction(idx int, r map[string]any) string {
	if trigger, ok := r["trigger"].(map[string]any); ok {
		if msg := validateTriggerType(idx, trigger); msg != "" {
			return msg
		}
	}
	if action, ok := r["action"].(map[string]any); ok {
		if msg := validateActionType(idx, action); msg != "" {
			return msg
		}
	}
	return ""
}

func validateTriggerType(idx int, trigger map[string]any) string {
	t, _ := trigger["type"].(string)
	if t != "" && !validTriggerTypes[t] {
		return fmt.Sprintf("reactions[%d].trigger.type is invalid: %s", idx, t)
	}
	if t == "AFTER_TIMEOUT" {
		if _, ok := trigger["timeout"].(float64); !ok {
			return fmt.Sprintf("reactions[%d].trigger.timeout is required for AFTER_TIMEOUT and must be a number (milliseconds)", idx)
		}
	}
	return ""
}

func validateActionType(idx int, action map[string]any) string {
	t, _ := action["type"].(string)
	if t != "" && !validActionTypes[t] {
		return fmt.Sprintf("reactions[%d].action.type is invalid: %s", idx, t)
	}
	switch t {
	case "NODE":
		if nav, _ := action["navigation"].(string); nav == "" {
			return fmt.Sprintf("reactions[%d].action.navigation is required for NODE (e.g. NAVIGATE, OVERLAY, SCROLL_TO, SWAP, CHANGE_TO)", idx)
		}
	case "URL":
		if url, _ := action["url"].(string); url == "" {
			return fmt.Sprintf("reactions[%d].action.url is required for URL", idx)
		}
	}
	return ""
}

func validateAutoLayoutParams(params map[string]interface{}) string {
	if lm, ok := params["layoutMode"].(string); ok && lm != "" {
		switch lm {
		case "HORIZONTAL", "VERTICAL", "NONE":
		default:
			return fmt.Sprintf("layoutMode must be HORIZONTAL, VERTICAL, or NONE, got: %s", lm)
		}
	}
	if v, ok := params["primaryAxisAlignItems"].(string); ok && v != "" {
		switch v {
		case "MIN", "CENTER", "MAX", "SPACE_BETWEEN":
		default:
			return fmt.Sprintf("primaryAxisAlignItems must be MIN, CENTER, MAX, or SPACE_BETWEEN, got: %s", v)
		}
	}
	if v, ok := params["counterAxisAlignItems"].(string); ok && v != "" {
		switch v {
		case "MIN", "CENTER", "MAX", "BASELINE":
		default:
			return fmt.Sprintf("counterAxisAlignItems must be MIN, CENTER, MAX, or BASELINE, got: %s", v)
		}
	}
	if v, ok := params["primaryAxisSizingMode"].(string); ok && v != "" {
		switch v {
		case "FIXED", "AUTO":
		default:
			return fmt.Sprintf("primaryAxisSizingMode must be FIXED or AUTO, got: %s", v)
		}
	}
	if v, ok := params["counterAxisSizingMode"].(string); ok && v != "" {
		switch v {
		case "FIXED", "AUTO":
		default:
			return fmt.Sprintf("counterAxisSizingMode must be FIXED or AUTO, got: %s", v)
		}
	}
	if v, ok := params["layoutWrap"].(string); ok && v != "" {
		switch v {
		case "NO_WRAP", "WRAP":
		default:
			return fmt.Sprintf("layoutWrap must be NO_WRAP or WRAP, got: %s", v)
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
