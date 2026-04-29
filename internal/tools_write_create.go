package internal

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerWriteCreateTools(s *server.MCPServer, node *Node) {
	s.AddTool(mcp.NewTool("create_frame",
		mcp.WithDescription("Create a new frame on the current page or inside a parent node."),
		mcp.WithNumber("x", mcp.Description("X position (default 0)")),
		mcp.WithNumber("y", mcp.Description("Y position (default 0)")),
		mcp.WithNumber("width", mcp.Description("Width in pixels (default 100)")),
		mcp.WithNumber("height", mcp.Description("Height in pixels (default 100)")),
		mcp.WithString("name", mcp.Description("Frame name")),
		mcp.WithString("fillColor", mcp.Description("Fill color as hex e.g. #FFFFFF")),
		mcp.WithString("layoutMode", mcp.Description("Auto-layout direction: HORIZONTAL, VERTICAL, or NONE")),
		mcp.WithNumber("paddingTop", mcp.Description("Auto-layout top padding")),
		mcp.WithNumber("paddingRight", mcp.Description("Auto-layout right padding")),
		mcp.WithNumber("paddingBottom", mcp.Description("Auto-layout bottom padding")),
		mcp.WithNumber("paddingLeft", mcp.Description("Auto-layout left padding")),
		mcp.WithNumber("itemSpacing", mcp.Description("Auto-layout gap between children")),
		mcp.WithString("primaryAxisAlignItems", mcp.Description("Main-axis alignment: MIN, CENTER, MAX, or SPACE_BETWEEN")),
		mcp.WithString("counterAxisAlignItems", mcp.Description("Cross-axis alignment: MIN, CENTER, MAX, or BASELINE")),
		mcp.WithString("primaryAxisSizingMode", mcp.Description("Main-axis sizing: FIXED or AUTO (hug)")),
		mcp.WithString("counterAxisSizingMode", mcp.Description("Cross-axis sizing: FIXED or AUTO (hug)")),
		mcp.WithString("layoutWrap", mcp.Description("Wrap behaviour: NO_WRAP or WRAP")),
		mcp.WithNumber("counterAxisSpacing", mcp.Description("Gap between wrapped rows/columns (only when layoutWrap is WRAP)")),
		mcp.WithString("parentId", mcp.Description("Parent node ID in colon format. Defaults to current page.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := req.GetArguments()
		resp, err := node.Send(ctx, "create_frame", nil, params)
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("create_rectangle",
		mcp.WithDescription("Create a new rectangle on the current page or inside a parent node."),
		mcp.WithNumber("x", mcp.Description("X position (default 0)")),
		mcp.WithNumber("y", mcp.Description("Y position (default 0)")),
		mcp.WithNumber("width", mcp.Description("Width in pixels (default 100)")),
		mcp.WithNumber("height", mcp.Description("Height in pixels (default 100)")),
		mcp.WithString("name", mcp.Description("Rectangle name")),
		mcp.WithString("fillColor", mcp.Description("Fill color as hex e.g. #FF5733")),
		mcp.WithNumber("cornerRadius", mcp.Description("Corner radius in pixels")),
		mcp.WithString("parentId", mcp.Description("Parent node ID in colon format. Defaults to current page.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := req.GetArguments()
		resp, err := node.Send(ctx, "create_rectangle", nil, params)
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("create_ellipse",
		mcp.WithDescription("Create a new ellipse (circle/oval) on the current page or inside a parent node."),
		mcp.WithNumber("x", mcp.Description("X position (default 0)")),
		mcp.WithNumber("y", mcp.Description("Y position (default 0)")),
		mcp.WithNumber("width", mcp.Description("Width in pixels (default 100)")),
		mcp.WithNumber("height", mcp.Description("Height in pixels (default 100)")),
		mcp.WithString("name", mcp.Description("Ellipse name")),
		mcp.WithString("fillColor", mcp.Description("Fill color as hex e.g. #3B82F6")),
		mcp.WithString("parentId", mcp.Description("Parent node ID in colon format. Defaults to current page.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := req.GetArguments()
		resp, err := node.Send(ctx, "create_ellipse", nil, params)
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("create_text",
		mcp.WithDescription("Create a new text node on the current page or inside a parent node. The font is loaded automatically before insertion. Returns the created node ID and bounds. Use set_text to update the content of an existing text node."),
		mcp.WithString("text",
			mcp.Required(),
			mcp.Description("Text content to display"),
		),
		mcp.WithNumber("x", mcp.Description("X position in pixels (default 0)")),
		mcp.WithNumber("y", mcp.Description("Y position in pixels (default 0)")),
		mcp.WithNumber("fontSize", mcp.Description("Font size in pixels (default 14)")),
		mcp.WithString("fontFamily", mcp.Description("Font family name e.g. 'Inter', 'Roboto', 'SF Pro Display' (default Inter). Must be a font installed in Figma.")),
		mcp.WithString("fontStyle", mcp.Description("Font style variant e.g. 'Regular', 'Bold', 'Italic', 'Medium', 'SemiBold' (default Regular). Must match an available style for the chosen fontFamily.")),
		mcp.WithString("fillColor", mcp.Description("Text color as hex e.g. #000000 (default black)")),
		mcp.WithString("name", mcp.Description("Node name shown in the layers panel (defaults to the text content)")),
		mcp.WithString("parentId", mcp.Description("Parent node ID in colon format. Defaults to current page.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := req.GetArguments()
		resp, err := node.Send(ctx, "create_text", nil, params)
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("import_image",
		mcp.WithDescription("Import an image into Figma as a rectangle with an image fill. Provide exactly one of imageData (base64 PNG/JPG), imagePath (local file), or imageUrl (http/https). Repeated imports of the same bytes hit a content-addressed cache and skip the wire payload."),
		mcp.WithString("imageData", mcp.Description("Base64-encoded image data (PNG or JPG). Mutually exclusive with imagePath and imageUrl.")),
		mcp.WithString("imagePath", mcp.Description("Absolute path to a local image file. Server reads + hashes; preferred over imageData for files on disk.")),
		mcp.WithString("imageUrl", mcp.Description("HTTP or HTTPS URL of an image. Server fetches it (10s timeout, 50MB max, image/* content-type required).")),
		mcp.WithNumber("x", mcp.Description("X position (default 0)")),
		mcp.WithNumber("y", mcp.Description("Y position (default 0)")),
		mcp.WithNumber("width", mcp.Description("Width in pixels (default 200)")),
		mcp.WithNumber("height", mcp.Description("Height in pixels (default 200)")),
		mcp.WithString("name", mcp.Description("Node name")),
		mcp.WithString("scaleMode", mcp.Description("Image scale mode: FILL (default), FIT, CROP, or TILE")),
		mcp.WithString("parentId", mcp.Description("Parent node ID in colon format. Defaults to current page.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		defer startAutoProgress(ctx, req, "import_image")()
		args := req.GetArguments()
		resolved, err := ResolveImage(ctx, args)
		if err != nil {
			return renderResponse(BridgeResponse{}, err)
		}
		params := map[string]interface{}{
			"imageData":   resolved.Base64Data(),
			"contentHash": resolved.ContentHash,
		}
		CopyFloat(params, args, "x")
		CopyFloat(params, args, "y")
		CopyFloat(params, args, "width")
		CopyFloat(params, args, "height")
		CopyString(params, args, "name")
		CopyString(params, args, "scaleMode")
		CopyString(params, args, "parentId")
		resp, err := node.Send(ctx, "import_image", nil, params)
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("import_images",
		mcp.WithDescription("Batch import many images in a single round-trip. Each item provides exactly one of imageData (base64), imagePath (local file), or imageUrl (http/https). Items hit the same per-session imageHash cache as import_image; repeats skip the wire payload and the figma.createImage call entirely. Per-item errors are reported in `results` rather than failing the whole batch."),
		mcp.WithArray("items",
			mcp.Required(),
			mcp.Description("List of image items to import."),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"imageData": map[string]any{"type": "string", "description": "Base64-encoded image (PNG/JPG)."},
					"imagePath": map[string]any{"type": "string", "description": "Absolute path to a local image file."},
					"imageUrl":  map[string]any{"type": "string", "description": "HTTP/HTTPS URL of an image."},
					"x":         map[string]any{"type": "number", "description": "X position (default 0)."},
					"y":         map[string]any{"type": "number", "description": "Y position (default 0)."},
					"width":     map[string]any{"type": "number", "description": "Width in pixels (default 200)."},
					"height":    map[string]any{"type": "number", "description": "Height in pixels (default 200)."},
					"name":      map[string]any{"type": "string", "description": "Node name."},
					"scaleMode": map[string]any{"type": "string", "description": "FILL (default), FIT, CROP, or TILE."},
				},
			}),
		),
		mcp.WithString("parentId", mcp.Description("Parent node ID for all items. Defaults to current page.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return executeImportImages(ctx, node, req)
	})

	s.AddTool(mcp.NewTool("create_component",
		mcp.WithDescription("Convert an existing FRAME node into a reusable COMPONENT. The frame is replaced in place by the new component."),
		mcp.WithString("nodeId",
			mcp.Required(),
			mcp.Description("FRAME node ID to convert, in colon format e.g. '4029:12345'"),
		),
		mcp.WithString("name", mcp.Description("Optional name for the component. Defaults to the frame's current name.")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nodeID, _ := req.GetArguments()["nodeId"].(string)
		nodeID = NormalizeNodeID(nodeID)
		params := map[string]interface{}{}
		if name, ok := req.GetArguments()["name"].(string); ok && name != "" {
			params["name"] = name
		}
		resp, err := node.Send(ctx, "create_component", []string{nodeID}, params)
		return renderResponse(resp, err)
	})

	s.AddTool(mcp.NewTool("create_section",
		mcp.WithDescription("Create a Figma Section node on the current page. Sections are the modern way to organize frames and groups on a page."),
		mcp.WithString("name", mcp.Description("Section name (default 'Section')")),
		mcp.WithNumber("x", mcp.Description("X position (default 0)")),
		mcp.WithNumber("y", mcp.Description("Y position (default 0)")),
		mcp.WithNumber("width", mcp.Description("Width in pixels")),
		mcp.WithNumber("height", mcp.Description("Height in pixels")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		params := map[string]interface{}{}
		CopyString(params, args, "name")
		CopyFloat(params, args, "x")
		CopyFloat(params, args, "y")
		CopyFloat(params, args, "width")
		CopyFloat(params, args, "height")
		resp, err := node.Send(ctx, "create_section", nil, params)
		return renderResponse(resp, err)
	})
}
