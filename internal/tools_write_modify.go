package internal

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerWriteModifyTools(s *server.MCPServer, node *Node) {
	s.AddTool(mcp.NewTool("set_text",
		mcp.WithDescription("Update the text content and/or truncation behaviour of an existing TEXT node. At least one of text, textTruncation, or maxLines must be provided."),
		mcp.WithString("nodeId",
			mcp.Required(),
			mcp.Description("TEXT node ID in colon format e.g. '4029:12345'"),
		),
		mcp.WithString("text", mcp.Description("New text content (optional — omit to keep the existing characters)")),
		mcp.WithString("textTruncation", mcp.Description("Truncation behaviour: 'DISABLED' (no truncation) or 'ENDING' (truncate with an ellipsis)")),
		mcp.WithNumber("maxLines", mcp.Description("Maximum number of lines before truncation (positive integer). Only applies when textTruncation is 'ENDING'. Pass 0 to clear the cap (interpreted as null).")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		nodeID, _ := args["nodeId"].(string)
		params := map[string]interface{}{}
		if t, ok := args["text"].(string); ok {
			params["text"] = t
		}
		if tt, ok := args["textTruncation"].(string); ok && tt != "" {
			params["textTruncation"] = tt
		}
		if ml, ok := args["maxLines"].(float64); ok {
			if ml == 0 {
				params["maxLines"] = nil
			} else {
				params["maxLines"] = ml
			}
		}
		resp, err := node.Send(ctx, "set_text", []string{nodeID}, params)
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("set_fills",
		mcp.WithDescription("Set the fill color on a single node (takes one nodeId, not an array). Use mode='append' to stack a new fill on top of existing fills instead of replacing them."),
		mcp.WithString("nodeId",
			mcp.Required(),
			mcp.Description("Node ID in colon format e.g. '4029:12345'"),
		),
		mcp.WithString("color",
			mcp.Required(),
			mcp.Description("Fill color as hex: #RRGGBB e.g. #FF5733 or #RRGGBBAA e.g. #FF573380 for 50% alpha"),
		),
		mcp.WithNumber("opacity", mcp.Description("Fill opacity 0–1 (default 1). Combines multiplicatively with any alpha in the color hex.")),
		mcp.WithString("mode", mcp.Description("'replace' (default) overwrites all existing fills; 'append' stacks this fill on top of existing ones")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nodeID, _ := req.GetArguments()["nodeId"].(string)
		params := map[string]interface{}{
			"color": req.GetArguments()["color"],
		}
		if op, ok := req.GetArguments()["opacity"].(float64); ok {
			params["opacity"] = op
		}
		if m, ok := req.GetArguments()["mode"].(string); ok {
			params["mode"] = m
		}
		resp, err := node.Send(ctx, "set_fills", []string{nodeID}, params)
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("set_strokes",
		mcp.WithDescription("Set the stroke color and weight on a single node (takes one nodeId, not an array). Use mode='append' to stack a new stroke on top of existing strokes instead of replacing them."),
		mcp.WithString("nodeId",
			mcp.Required(),
			mcp.Description("Node ID in colon format e.g. '4029:12345'"),
		),
		mcp.WithString("color",
			mcp.Required(),
			mcp.Description("Stroke color as hex e.g. #000000"),
		),
		mcp.WithNumber("strokeWeight", mcp.Description("Stroke weight in pixels (default 1)")),
		mcp.WithString("mode", mcp.Description("'replace' (default) overwrites all strokes; 'append' stacks on top of existing strokes")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nodeID, _ := req.GetArguments()["nodeId"].(string)
		params := map[string]interface{}{
			"color": req.GetArguments()["color"],
		}
		if sw, ok := req.GetArguments()["strokeWeight"].(float64); ok {
			params["strokeWeight"] = sw
		}
		if m, ok := req.GetArguments()["mode"].(string); ok {
			params["mode"] = m
		}
		resp, err := node.Send(ctx, "set_strokes", []string{nodeID}, params)
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("move_nodes",
		mcp.WithDescription("Move one or more nodes to an absolute canvas position. The same x/y is applied to every node independently (not a relative offset from current position)."),
		mcp.WithArray("nodeIds",
			mcp.Required(),
			mcp.Description("Node IDs in colon format e.g. ['4029:12345']"),
			mcp.WithStringItems(),
		),
		mcp.WithNumber("x", mcp.Description("Target X position")),
		mcp.WithNumber("y", mcp.Description("Target Y position")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, _ := req.GetArguments()["nodeIds"].([]interface{})
		nodeIDs := toStringSlice(raw)
		params := map[string]interface{}{}
		if x, ok := req.GetArguments()["x"].(float64); ok {
			params["x"] = x
		}
		if y, ok := req.GetArguments()["y"].(float64); ok {
			params["y"] = y
		}
		resp, err := node.Send(ctx, "move_nodes", nodeIDs, params)
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("resize_nodes",
		mcp.WithDescription("Resize one or more nodes. The same width/height is applied to every node in the list independently. Provide width, height, or both."),
		mcp.WithArray("nodeIds",
			mcp.Required(),
			mcp.Description("Node IDs in colon format e.g. ['4029:12345']"),
			mcp.WithStringItems(),
		),
		mcp.WithNumber("width", mcp.Description("New width in pixels")),
		mcp.WithNumber("height", mcp.Description("New height in pixels")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, _ := req.GetArguments()["nodeIds"].([]interface{})
		nodeIDs := toStringSlice(raw)
		params := map[string]interface{}{}
		if w, ok := req.GetArguments()["width"].(float64); ok {
			params["width"] = w
		}
		if h, ok := req.GetArguments()["height"].(float64); ok {
			params["height"] = h
		}
		resp, err := node.Send(ctx, "resize_nodes", nodeIDs, params)
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("rename_node",
		mcp.WithDescription("Rename a single node by ID. Returns the updated node with its new name. Use batch_rename_nodes to rename multiple nodes at once or to apply find/replace patterns across many nodes."),
		mcp.WithString("nodeId",
			mcp.Required(),
			mcp.Description("Node ID in colon format e.g. '4029:12345'"),
		),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("New name for the node. Figma supports slash-separated path notation e.g. 'Icons/Arrow/Left' to organise nodes in component panels."),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nodeID, _ := req.GetArguments()["nodeId"].(string)
		name, _ := req.GetArguments()["name"].(string)
		resp, err := node.Send(ctx, "rename_node", []string{nodeID}, map[string]interface{}{"name": name})
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("clone_node",
		mcp.WithDescription("Clone an existing node, optionally repositioning it or placing it in a new parent."),
		mcp.WithString("nodeId",
			mcp.Required(),
			mcp.Description("Source node ID in colon format e.g. '4029:12345'"),
		),
		mcp.WithNumber("x", mcp.Description("X position of the clone")),
		mcp.WithNumber("y", mcp.Description("Y position of the clone")),
		mcp.WithString("parentId", mcp.Description("Parent node ID for the clone. Defaults to same parent as source.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nodeID, _ := req.GetArguments()["nodeId"].(string)
		params := map[string]interface{}{}
		if x, ok := req.GetArguments()["x"].(float64); ok {
			params["x"] = x
		}
		if y, ok := req.GetArguments()["y"].(float64); ok {
			params["y"] = y
		}
		if pid, ok := req.GetArguments()["parentId"].(string); ok && pid != "" {
			params["parentId"] = pid
		}
		resp, err := node.Send(ctx, "clone_node", []string{nodeID}, params)
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("set_opacity",
		mcp.WithDescription("Set the opacity of one or more nodes (0 = fully transparent, 1 = fully opaque)."),
		mcp.WithArray("nodeIds",
			mcp.Required(),
			mcp.Description("Node IDs in colon format e.g. ['4029:12345']"),
			mcp.WithStringItems(),
		),
		mcp.WithNumber("opacity",
			mcp.Required(),
			mcp.Description("Opacity value between 0 and 1"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, _ := req.GetArguments()["nodeIds"].([]interface{})
		nodeIDs := toStringSlice(raw)
		opacity, _ := req.GetArguments()["opacity"].(float64)
		resp, err := node.Send(ctx, "set_opacity", nodeIDs, map[string]interface{}{"opacity": opacity})
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("set_corner_radius",
		mcp.WithDescription("Set corner radius on one or more nodes. Provide a uniform cornerRadius or individual per-corner values."),
		mcp.WithArray("nodeIds",
			mcp.Required(),
			mcp.Description("Node IDs in colon format e.g. ['4029:12345']"),
			mcp.WithStringItems(),
		),
		mcp.WithNumber("cornerRadius", mcp.Description("Uniform corner radius applied to all corners")),
		mcp.WithNumber("topLeftRadius", mcp.Description("Top-left corner radius")),
		mcp.WithNumber("topRightRadius", mcp.Description("Top-right corner radius")),
		mcp.WithNumber("bottomLeftRadius", mcp.Description("Bottom-left corner radius")),
		mcp.WithNumber("bottomRightRadius", mcp.Description("Bottom-right corner radius")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, _ := req.GetArguments()["nodeIds"].([]interface{})
		nodeIDs := toStringSlice(raw)
		params := map[string]interface{}{}
		if v, ok := req.GetArguments()["cornerRadius"].(float64); ok {
			params["cornerRadius"] = v
		}
		if v, ok := req.GetArguments()["topLeftRadius"].(float64); ok {
			params["topLeftRadius"] = v
		}
		if v, ok := req.GetArguments()["topRightRadius"].(float64); ok {
			params["topRightRadius"] = v
		}
		if v, ok := req.GetArguments()["bottomLeftRadius"].(float64); ok {
			params["bottomLeftRadius"] = v
		}
		if v, ok := req.GetArguments()["bottomRightRadius"].(float64); ok {
			params["bottomRightRadius"] = v
		}
		resp, err := node.Send(ctx, "set_corner_radius", nodeIDs, params)
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("set_auto_layout",
		mcp.WithDescription("Set or update auto-layout (flex) properties on an existing frame."),
		mcp.WithString("nodeId",
			mcp.Required(),
			mcp.Description("Frame node ID in colon format e.g. '4029:12345'"),
		),
		mcp.WithString("layoutMode", mcp.Description("Auto-layout direction: HORIZONTAL, VERTICAL, or NONE")),
		mcp.WithNumber("paddingTop", mcp.Description("Top padding")),
		mcp.WithNumber("paddingRight", mcp.Description("Right padding")),
		mcp.WithNumber("paddingBottom", mcp.Description("Bottom padding")),
		mcp.WithNumber("paddingLeft", mcp.Description("Left padding")),
		mcp.WithNumber("itemSpacing", mcp.Description("Gap between children")),
		mcp.WithString("primaryAxisAlignItems", mcp.Description("Main-axis alignment: MIN, CENTER, MAX, or SPACE_BETWEEN")),
		mcp.WithString("counterAxisAlignItems", mcp.Description("Cross-axis alignment: MIN, CENTER, MAX, or BASELINE")),
		mcp.WithString("primaryAxisSizingMode", mcp.Description("Main-axis sizing: FIXED or AUTO (hug)")),
		mcp.WithString("counterAxisSizingMode", mcp.Description("Cross-axis sizing: FIXED or AUTO (hug)")),
		mcp.WithString("layoutWrap", mcp.Description("Wrap behaviour: NO_WRAP or WRAP")),
		mcp.WithNumber("counterAxisSpacing", mcp.Description("Gap between wrapped rows/columns (only when layoutWrap is WRAP)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nodeID, _ := req.GetArguments()["nodeId"].(string)
		params := req.GetArguments()
		resp, err := node.Send(ctx, "set_auto_layout", []string{nodeID}, params)
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("delete_nodes",
		mcp.WithDescription("Delete one or more nodes. This cannot be undone via MCP — use with care."),
		mcp.WithArray("nodeIds",
			mcp.Required(),
			mcp.Description("Node IDs to delete in colon format e.g. ['4029:12345']"),
			mcp.WithStringItems(),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, _ := req.GetArguments()["nodeIds"].([]interface{})
		nodeIDs := toStringSlice(raw)
		resp, err := node.Send(ctx, "delete_nodes", nodeIDs, nil)
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("set_visible",
		mcp.WithDescription("Show or hide one or more nodes by setting their visibility."),
		mcp.WithArray("nodeIds",
			mcp.Required(),
			mcp.Description("Node IDs in colon format e.g. ['4029:12345']"),
			mcp.WithStringItems(),
		),
		mcp.WithBoolean("visible",
			mcp.Required(),
			mcp.Description("true to show the node, false to hide it"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, _ := req.GetArguments()["nodeIds"].([]interface{})
		nodeIDs := toStringSlice(raw)
		visible, _ := req.GetArguments()["visible"].(bool)
		resp, err := node.Send(ctx, "set_visible", nodeIDs, map[string]interface{}{"visible": visible})
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("lock_nodes",
		mcp.WithDescription("Lock one or more nodes to prevent accidental edits in Figma."),
		mcp.WithArray("nodeIds",
			mcp.Required(),
			mcp.Description("Node IDs in colon format e.g. ['4029:12345']"),
			mcp.WithStringItems(),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, _ := req.GetArguments()["nodeIds"].([]interface{})
		nodeIDs := toStringSlice(raw)
		resp, err := node.Send(ctx, "lock_nodes", nodeIDs, nil)
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("unlock_nodes",
		mcp.WithDescription("Unlock one or more nodes, allowing them to be edited again."),
		mcp.WithArray("nodeIds",
			mcp.Required(),
			mcp.Description("Node IDs in colon format e.g. ['4029:12345']"),
			mcp.WithStringItems(),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, _ := req.GetArguments()["nodeIds"].([]interface{})
		nodeIDs := toStringSlice(raw)
		resp, err := node.Send(ctx, "unlock_nodes", nodeIDs, nil)
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("rotate_nodes",
		mcp.WithDescription("Rotate one or more nodes to an absolute angle in degrees."),
		mcp.WithArray("nodeIds",
			mcp.Required(),
			mcp.Description("Node IDs in colon format e.g. ['4029:12345']"),
			mcp.WithStringItems(),
		),
		mcp.WithNumber("rotation",
			mcp.Required(),
			mcp.Description("Rotation angle in degrees (positive = counter-clockwise in Figma)"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, _ := req.GetArguments()["nodeIds"].([]interface{})
		nodeIDs := toStringSlice(raw)
		rotation, _ := req.GetArguments()["rotation"].(float64)
		resp, err := node.Send(ctx, "rotate_nodes", nodeIDs, map[string]interface{}{"rotation": rotation})
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("reorder_nodes",
		mcp.WithDescription("Change the z-order (layer stack position) of one or more nodes."),
		mcp.WithArray("nodeIds",
			mcp.Required(),
			mcp.Description("Node IDs in colon format e.g. ['4029:12345']"),
			mcp.WithStringItems(),
		),
		mcp.WithString("order",
			mcp.Required(),
			mcp.Description("Order operation: bringToFront, sendToBack, bringForward, or sendBackward"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, _ := req.GetArguments()["nodeIds"].([]interface{})
		nodeIDs := toStringSlice(raw)
		order, _ := req.GetArguments()["order"].(string)
		resp, err := node.Send(ctx, "reorder_nodes", nodeIDs, map[string]interface{}{"order": order})
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("set_blend_mode",
		mcp.WithDescription("Set the blend mode of one or more nodes (e.g. MULTIPLY, SCREEN, OVERLAY)."),
		mcp.WithArray("nodeIds",
			mcp.Required(),
			mcp.Description("Node IDs in colon format e.g. ['4029:12345']"),
			mcp.WithStringItems(),
		),
		mcp.WithString("blendMode",
			mcp.Required(),
			mcp.Description("Blend mode: NORMAL, MULTIPLY, SCREEN, OVERLAY, DARKEN, LIGHTEN, COLOR_DODGE, COLOR_BURN, HARD_LIGHT, SOFT_LIGHT, DIFFERENCE, EXCLUSION, HUE, SATURATION, COLOR, LUMINOSITY, PASS_THROUGH"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, _ := req.GetArguments()["nodeIds"].([]interface{})
		nodeIDs := toStringSlice(raw)
		blendMode, _ := req.GetArguments()["blendMode"].(string)
		resp, err := node.Send(ctx, "set_blend_mode", nodeIDs, map[string]interface{}{"blendMode": blendMode})
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("set_constraints",
		mcp.WithDescription("Set layout constraints (pinning behaviour) on one or more nodes relative to their parent."),
		mcp.WithArray("nodeIds",
			mcp.Required(),
			mcp.Description("Node IDs in colon format e.g. ['4029:12345']"),
			mcp.WithStringItems(),
		),
		mcp.WithString("horizontal", mcp.Description("Horizontal constraint: MIN (left), MAX (right), CENTER, STRETCH, or SCALE")),
		mcp.WithString("vertical", mcp.Description("Vertical constraint: MIN (top), MAX (bottom), CENTER, STRETCH, or SCALE")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, _ := req.GetArguments()["nodeIds"].([]interface{})
		nodeIDs := toStringSlice(raw)
		params := map[string]interface{}{}
		if h, ok := req.GetArguments()["horizontal"].(string); ok && h != "" {
			params["horizontal"] = h
		}
		if v, ok := req.GetArguments()["vertical"].(string); ok && v != "" {
			params["vertical"] = v
		}
		resp, err := node.Send(ctx, "set_constraints", nodeIDs, params)
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("reparent_nodes",
		mcp.WithDescription("Move one or more nodes to a different parent frame, group, or section."),
		mcp.WithArray("nodeIds",
			mcp.Required(),
			mcp.Description("Node IDs to move in colon format e.g. ['4029:12345']"),
			mcp.WithStringItems(),
		),
		mcp.WithString("parentId",
			mcp.Required(),
			mcp.Description("Target parent node ID in colon format e.g. '4029:99'"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, _ := req.GetArguments()["nodeIds"].([]interface{})
		nodeIDs := toStringSlice(raw)
		parentID, _ := req.GetArguments()["parentId"].(string)
		parentID = NormalizeNodeID(parentID)
		resp, err := node.Send(ctx, "reparent_nodes", nodeIDs, map[string]interface{}{"parentId": parentID})
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("batch_rename_nodes",
		mcp.WithDescription("Rename multiple nodes using find/replace, regex substitution, or prefix/suffix addition."),
		mcp.WithArray("nodeIds",
			mcp.Required(),
			mcp.Description("Node IDs in colon format e.g. ['4029:12345']"),
			mcp.WithStringItems(),
		),
		mcp.WithString("find", mcp.Description("String (or regex pattern when useRegex=true) to search for in the node name")),
		mcp.WithString("replace", mcp.Description("Replacement string. Required when find is provided.")),
		mcp.WithBoolean("useRegex", mcp.Description("Treat find as a regular expression (default false)")),
		mcp.WithString("regexFlags", mcp.Description("Regex flags e.g. 'gi' (default 'g'). Only used when useRegex=true.")),
		mcp.WithString("prefix", mcp.Description("String to prepend to the node name")),
		mcp.WithString("suffix", mcp.Description("String to append to the node name")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, _ := req.GetArguments()["nodeIds"].([]interface{})
		nodeIDs := toStringSlice(raw)
		params := map[string]interface{}{}
		for _, k := range []string{"find", "replace", "regexFlags", "prefix", "suffix"} {
			if v, ok := req.GetArguments()[k].(string); ok {
				params[k] = v
			}
		}
		if v, ok := req.GetArguments()["useRegex"].(bool); ok {
			params["useRegex"] = v
		}
		resp, err := node.Send(ctx, "batch_rename_nodes", nodeIDs, params)
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("find_replace_text",
		mcp.WithDescription("Find and replace text content across all TEXT nodes in a subtree. Searches the entire current page if no nodeId is given."),
		mcp.WithString("find",
			mcp.Required(),
			mcp.Description("Text string (or regex pattern when useRegex=true) to search for"),
		),
		mcp.WithString("replace",
			mcp.Required(),
			mcp.Description("Replacement string (use empty string to delete matches)"),
		),
		mcp.WithString("nodeId", mcp.Description("Root node ID to scope the search. Defaults to the entire current page.")),
		mcp.WithBoolean("useRegex", mcp.Description("Treat find as a regular expression (default false)")),
		mcp.WithString("regexFlags", mcp.Description("Regex flags e.g. 'gi' (default 'g'). Only used when useRegex=true.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := map[string]interface{}{
			"find":    req.GetArguments()["find"],
			"replace": req.GetArguments()["replace"],
		}
		if v, ok := req.GetArguments()["useRegex"].(bool); ok {
			params["useRegex"] = v
		}
		if v, ok := req.GetArguments()["regexFlags"].(string); ok && v != "" {
			params["regexFlags"] = v
		}
		var nodeIDs []string
		if nodeID, ok := req.GetArguments()["nodeId"].(string); ok && nodeID != "" {
			nodeID = NormalizeNodeID(nodeID)
			nodeIDs = []string{nodeID}
		}
		resp, err := node.Send(ctx, "find_replace_text", nodeIDs, params)
		return renderResponse(resp, err)
	})
}
