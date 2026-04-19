package internal

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerReadDocumentTools(s *server.MCPServer, node *Node) {
	s.AddTool(mcp.NewTool("get_document",
		mcp.WithDescription("Get the full node tree of the current page (not the whole file — only the active page). Returns all nodes recursively and can be very large. Prefer get_design_context for exploration or when token efficiency matters."),
		mcp.WithBoolean("simplify",
			mcp.Description("Apply simplification to produce a token-efficient LLM-friendly response"),
		),
		mcp.WithNumber("simplifyDepth",
			mcp.Description("Maximum depth for simplification (0 = unlimited)"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		resp, err := node.Send(ctx, "get_document", nil, nil)
		return renderResponseWithSimplify(resp, err, req)
	})

	s.AddTool(mcp.NewTool("get_pages",
		mcp.WithDescription("List all pages in the document with their IDs and names. Lightweight alternative to get_document."),
	), makeHandler(node, "get_pages", nil, nil))

	s.AddTool(mcp.NewTool("get_metadata",
		mcp.WithDescription("Get metadata about the current Figma document: file name, pages, current page"),
	), makeHandler(node, "get_metadata", nil, nil))

	s.AddTool(mcp.NewTool("get_selection",
		mcp.WithDescription("Get the nodes currently selected in Figma. Returns an empty array if nothing is selected. Use get_design_context or get_node to retrieve deeper detail about a specific node by ID."),
		mcp.WithBoolean("simplify",
			mcp.Description("Apply simplification to produce a token-efficient LLM-friendly response"),
		),
		mcp.WithNumber("simplifyDepth",
			mcp.Description("Maximum depth for simplification (0 = unlimited)"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		resp, err := node.Send(ctx, "get_selection", nil, nil)
		return renderResponseWithSimplify(resp, err, req)
	})

	s.AddTool(mcp.NewTool("get_node",
		mcp.WithDescription("Get a single node by ID with full detail. Use get_nodes_info to fetch multiple nodes in one round-trip instead of calling this repeatedly. Node ID must be colon format e.g. '4029:12345', never hyphens."),
		mcp.WithString("nodeId",
			mcp.Required(),
			mcp.Description("Node ID in colon format e.g. '4029:12345'"),
		),
		mcp.WithBoolean("simplify",
			mcp.Description("Apply simplification to produce a token-efficient LLM-friendly response"),
		),
		mcp.WithNumber("simplifyDepth",
			mcp.Description("Maximum depth for simplification (0 = unlimited)"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nodeID, _ := req.GetArguments()["nodeId"].(string)
		resp, err := node.Send(ctx, "get_node", []string{nodeID}, nil)
		return renderResponseWithSimplify(resp, err, req)
	})

	s.AddTool(mcp.NewTool("get_nodes_info",
		mcp.WithDescription("Get full details for multiple nodes by ID in one round-trip. Prefer this over calling get_node repeatedly when you need several nodes."),
		mcp.WithArray("nodeIds",
			mcp.Required(),
			mcp.Description("List of node IDs in colon format e.g. ['4029:12345', '4029:67890']"),
			mcp.WithStringItems(),
		),
		mcp.WithBoolean("simplify",
			mcp.Description("Apply simplification to produce a token-efficient LLM-friendly response"),
		),
		mcp.WithNumber("simplifyDepth",
			mcp.Description("Maximum depth for simplification (0 = unlimited)"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, _ := req.GetArguments()["nodeIds"].([]interface{})
		nodeIDs := toStringSlice(raw)
		resp, err := node.Send(ctx, "get_nodes_info", nodeIDs, nil)
		return renderResponseWithSimplify(resp, err, req)
	})

	s.AddTool(mcp.NewTool("get_design_context",
		mcp.WithDescription("Get a depth-limited, token-efficient tree of the current selection or page. Use this instead of get_document when exploring large files. Supports detail levels (minimal/compact/full) and dedupe_components for pages heavy with repeated component instances."),
		mcp.WithNumber("depth",
			mcp.Description("How many levels deep to traverse (default 2)"),
		),
		mcp.WithString("detail",
			mcp.Description("Property verbosity: minimal (id/name/type/bounds only), compact (+fills/strokes/opacity), full (everything, default)"),
		),
		mcp.WithBoolean("dedupe_components",
			mcp.Description("When true, INSTANCE nodes are serialized compactly (mainComponentId + componentProperties + overrides array of differing text/nested content) and unique component definitions are collected once in a top-level componentDefs map. Highly token-efficient for screens with many repeated component instances."),
		),
		mcp.WithBoolean("simplify",
			mcp.Description("Apply simplification to produce a token-efficient LLM-friendly response"),
		),
		mcp.WithNumber("simplifyDepth",
			mcp.Description("Maximum depth for simplification (0 = unlimited)"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := map[string]interface{}{}
		if d, ok := req.GetArguments()["depth"].(float64); ok && d > 0 {
			params["depth"] = d
		}
		if det, ok := req.GetArguments()["detail"].(string); ok && det != "" {
			params["detail"] = det
		}
		if dd, ok := req.GetArguments()["dedupe_components"].(bool); ok && dd {
			params["dedupeComponents"] = true
		}
		resp, err := node.Send(ctx, "get_design_context", nil, params)
		return renderResponseWithSimplify(resp, err, req)
	})

	s.AddTool(mcp.NewTool("search_nodes",
		mcp.WithDescription("Search for nodes by name substring and/or type within a subtree. Use this when you know (part of) the node name. Use scan_nodes_by_types when you want all nodes of a type regardless of name."),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("Name substring to match (case-insensitive)"),
		),
		mcp.WithString("nodeId",
			mcp.Description("Scope search to this subtree (default: current page), colon format e.g. '4029:12345'"),
		),
		mcp.WithArray("types",
			mcp.Description("Filter by Figma node type e.g. ['TEXT', 'FRAME', 'COMPONENT']"),
			mcp.WithStringItems(),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum results to return (default: 50)"),
		),
		mcp.WithBoolean("simplify",
			mcp.Description("Apply simplification to produce a token-efficient LLM-friendly response"),
		),
		mcp.WithNumber("simplifyDepth",
			mcp.Description("Maximum depth for simplification (0 = unlimited)"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := map[string]interface{}{
			"query": req.GetArguments()["query"],
		}
		if id, ok := req.GetArguments()["nodeId"].(string); ok && id != "" {
			params["nodeId"] = id
		}
		if raw, ok := req.GetArguments()["types"].([]interface{}); ok && len(raw) > 0 {
			params["types"] = raw
		}
		if limit, ok := req.GetArguments()["limit"].(float64); ok && limit > 0 {
			params["limit"] = limit
		}
		resp, err := node.Send(ctx, "search_nodes", nil, params)
		return renderResponseWithSimplify(resp, err, req)
	})

	s.AddTool(mcp.NewTool("scan_text_nodes",
		mcp.WithDescription("Scan all TEXT nodes in a subtree and return their content. Shorthand for scan_nodes_by_types with ['TEXT'] — use when you only need text copy from a component or frame."),
		mcp.WithString("nodeId",
			mcp.Required(),
			mcp.Description("Root node ID to scan from, colon format e.g. '4029:12345'"),
		),
		mcp.WithBoolean("simplify",
			mcp.Description("Apply simplification to produce a token-efficient LLM-friendly response"),
		),
		mcp.WithNumber("simplifyDepth",
			mcp.Description("Maximum depth for simplification (0 = unlimited)"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nodeID, _ := req.GetArguments()["nodeId"].(string)
		resp, err := node.Send(ctx, "scan_text_nodes", nil, map[string]interface{}{"nodeId": nodeID})
		return renderResponseWithSimplify(resp, err, req)
	})

	s.AddTool(mcp.NewTool("scan_nodes_by_types",
		mcp.WithDescription("Find all nodes of specific types in a subtree, regardless of name. Use search_nodes instead when you need to filter by name."),
		mcp.WithString("nodeId",
			mcp.Required(),
			mcp.Description("Root node ID to scan from, colon format e.g. '4029:12345'"),
		),
		mcp.WithArray("types",
			mcp.Required(),
			mcp.Description("Node types to find e.g. ['FRAME', 'COMPONENT', 'INSTANCE']"),
			mcp.WithStringItems(),
		),
		mcp.WithBoolean("simplify",
			mcp.Description("Apply simplification to produce a token-efficient LLM-friendly response"),
		),
		mcp.WithNumber("simplifyDepth",
			mcp.Description("Maximum depth for simplification (0 = unlimited)"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nodeID, _ := req.GetArguments()["nodeId"].(string)
		raw, _ := req.GetArguments()["types"].([]interface{})
		resp, err := node.Send(ctx, "scan_nodes_by_types", nil, map[string]interface{}{
			"nodeId": nodeID,
			"types":  raw,
		})
		return renderResponseWithSimplify(resp, err, req)
	})

	s.AddTool(mcp.NewTool("get_reactions",
		mcp.WithDescription("Get the prototype reactions defined on a node. Returns an array of reaction objects — each has a trigger (e.g. ON_CLICK, ON_HOVER, AFTER_TIMEOUT) and an actions array (navigate to node, open URL, go back, etc.). Use set_reactions to add or replace reactions, remove_reactions to delete them."),
		mcp.WithString("nodeId",
			mcp.Required(),
			mcp.Description("Node ID in colon format e.g. '4029:12345'"),
		),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nodeID, _ := req.GetArguments()["nodeId"].(string)
		resp, err := node.Send(ctx, "get_reactions", []string{nodeID}, nil)
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("get_viewport",
		mcp.WithDescription("Get the current Figma viewport: scroll center, zoom level, and visible bounds."),
	), makeHandler(node, "get_viewport", nil, nil))

	s.AddTool(mcp.NewTool("get_fonts",
		mcp.WithDescription("List all fonts used in the current page, sorted by usage frequency. Useful for understanding typography without scanning all text nodes."),
	), makeHandler(node, "get_fonts", nil, nil))
}
