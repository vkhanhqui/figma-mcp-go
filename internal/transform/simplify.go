package transform

import "encoding/json"

// Simplify converts raw Figma plugin data into a SimplifiedDesign.
// It walks the node tree, applies extractors, and deduplicates style values.
func Simplify(data any, opts Options) (*SimplifiedDesign, error) {
	if data == nil {
		return &SimplifiedDesign{}, nil
	}

	// Normalize to map
	var root map[string]any
	switch v := data.(type) {
	case map[string]any:
		root = v
	case []any:
		// Some responses are arrays of nodes
		return simplifyArray(v, opts)
	default:
		// Try to unmarshal
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(b, &root); err != nil {
			return nil, err
		}
	}

	ctx := &TraversalContext{
		MaxDepth: opts.MaxDepth,
		Vars: &GlobalVars{
			Layouts:    make(map[string]SimplifiedLayout),
			Fills:      make(map[string]any),
			Strokes:    make(map[string]any),
			Effects:    make(map[string]any),
			TextStyles: make(map[string]SimplifiedTextStyle),
		},
	}

	// Determine if this is a document/page response or a single node
	var nodes []*SimplifiedNode
	if children, ok := root["children"].([]any); ok {
		// Document/page with children
		name := getString(root, "name")
		for _, child := range children {
			if childMap, ok := child.(map[string]any); ok {
				sn := walkNode(childMap, ctx, opts)
				nodes = append(nodes, sn)
			}
		}
		return &SimplifiedDesign{
			Name:       name,
			Nodes:      nodes,
			GlobalVars: ctx.Vars,
		}, nil
	}

	// Single node response
	sn := walkNode(root, ctx, opts)
	return &SimplifiedDesign{
		Nodes:      []*SimplifiedNode{sn},
		GlobalVars: ctx.Vars,
	}, nil
}

func simplifyArray(arr []any, opts Options) (*SimplifiedDesign, error) {
	ctx := &TraversalContext{
		MaxDepth: opts.MaxDepth,
		Vars: &GlobalVars{
			Layouts:    make(map[string]SimplifiedLayout),
			Fills:      make(map[string]any),
			Strokes:    make(map[string]any),
			Effects:    make(map[string]any),
			TextStyles: make(map[string]SimplifiedTextStyle),
		},
	}
	nodes := make([]*SimplifiedNode, 0, len(arr))
	for _, item := range arr {
		if nodeMap, ok := item.(map[string]any); ok {
			sn := walkNode(nodeMap, ctx, opts)
			nodes = append(nodes, sn)
		}
	}
	return &SimplifiedDesign{Nodes: nodes, GlobalVars: ctx.Vars}, nil
}

// walkNode recursively transforms a raw node into SimplifiedNode.
func walkNode(node map[string]any, ctx *TraversalContext, opts Options) *SimplifiedNode {
	sn := &SimplifiedNode{
		ID:   getString(node, "id"),
		Name: getString(node, "name"),
		Type: mapNodeType(getString(node, "type")),
	}

	// Opacity
	if opacity := getFloat64(node, "opacity"); opacity != 1 && opacity != 0 {
		sn.Opacity = opacity
	}

	// Border radius
	if cr := getFloat64(node, "cornerRadius"); cr > 0 {
		sn.BorderRadius = cssLen(cr, "px")
	}

	// Skip extraction if at max depth
	if ctx.MaxDepth > 0 && ctx.Depth >= ctx.MaxDepth {
		return sn
	}

	// Component properties for INSTANCE nodes
	if isInstance(node) {
		sn.ComponentProperties = simplifyComponentProperties(node)
		if mc := getString(node, "mainComponentId"); mc != "" {
			sn.ComponentID = mc
		}
	}

	// Extractors
	switch opts.Extractors {
	case AllExtractors, LayoutAndText:
		// Layout
		if layoutID, layout := buildSimplifiedLayout(node, ctx); layoutID != "" {
			ctx.Vars.Layouts[layoutID] = *layout
			sn.Layout = layoutID
		}

		// Fills
		if fillsID, _ := buildSimplifiedFills(node, ctx); fillsID != "" {
			sn.Fills = fillsID
		}

		// Strokes (stored in fills map with stroke_ prefix for now)
		if strokeID, strokeVal := buildSimplifiedStrokes(node, ctx); strokeID != "" {
			ctx.Vars.Fills["stroke_"+strokeID[5:]] = strokeVal
		}

		// Effects
		if effectsID, _ := buildSimplifiedEffects(node, ctx); effectsID != "" {
			sn.Effects = effectsID
		}
		// fallthrough to get text
	case VisualsOnly:
		// Layout
		if layoutID, layout := buildSimplifiedLayout(node, ctx); layoutID != "" {
			ctx.Vars.Layouts[layoutID] = *layout
			sn.Layout = layoutID
		}

		// Fills
		if fillsID, _ := buildSimplifiedFills(node, ctx); fillsID != "" {
			sn.Fills = fillsID
		}

		// Strokes (stored in fills map with stroke_ prefix for now)
		if strokeID, strokeVal := buildSimplifiedStrokes(node, ctx); strokeID != "" {
			ctx.Vars.Fills["stroke_"+strokeID[5:]] = strokeVal
		}

		// Effects
		if effectsID, _ := buildSimplifiedEffects(node, ctx); effectsID != "" {
			sn.Effects = effectsID
		}
		// NO fallthrough - VisualsOnly does NOT get text
	case ContentOnly:
		// Text only - handled below
	}

	// Text extraction: AllExtractors, LayoutAndText, and ContentOnly get text
	// VisualsOnly does NOT get text
	if opts.Extractors != VisualsOnly && isTextNode(node) {
		sn.Text = buildFormattedText(node)
		if styleID := extractTextStyle(node, ctx); styleID != "" {
			sn.TextStyle = styleID
		}
	}

	// Children
	children := getSlice(node, "children")
	if len(children) > 0 {
		ctx.Depth++
		for _, child := range children {
			if childMap, ok := child.(map[string]any); ok {
				if isVisible(childMap) {
					sn.Children = append(sn.Children, walkNode(childMap, ctx, opts))
				}
			}
		}
		ctx.Depth--
	}

	return sn
}

// mapNodeType maps Figma node types to output types.
func mapNodeType(t string) string {
	switch t {
	case "VECTOR":
		return "IMAGE-SVG"
	case "TEXT":
		return "TEXT"
	default:
		return t
	}
}