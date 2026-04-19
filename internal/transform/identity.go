package transform

// hasValue reports whether v is a non-nil, non-zero value.
func hasValue(v any) bool {
	if v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case float64:
		return val != 0
	case string:
		return val != ""
	case []any:
		return len(val) > 0
	case map[string]any:
		return len(val) > 0
	}
	return true
}

// isVisible reports whether the node is visible.
func isVisible(node map[string]any) bool {
	if v, ok := node["visible"]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return true
}

// isFrame reports whether the node type is a frame-like container.
func isFrame(node map[string]any) bool {
	t, _ := node["type"].(string)
	return t == "FRAME" || t == "SECTION" || t == "COMPONENT" || t == "INSTANCE"
}

// isTextNode reports whether the node is a TEXT node.
func isTextNode(node map[string]any) bool {
	t, _ := node["type"].(string)
	return t == "TEXT"
}

// isComponent reports whether the node is a COMPONENT node.
func isComponent(node map[string]any) bool {
	t, _ := node["type"].(string)
	return t == "COMPONENT"
}

// isInstance reports whether the node is an INSTANCE node.
func isInstance(node map[string]any) bool {
	t, _ := node["type"].(string)
	return t == "INSTANCE"
}

// isVector reports whether the node type is VECTOR (mapped to IMAGE-SVG in output).
func isVector(node map[string]any) bool {
	t, _ := node["type"].(string)
	return t == "VECTOR"
}
