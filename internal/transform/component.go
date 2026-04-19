package transform

// simplifyComponentProperties extracts INSTANCE component properties.
// Phase 1 scope: BOOLEAN and TEXT only.
func simplifyComponentProperties(node map[string]any) map[string]any {
	props := getStringMap(node, "componentProperties")
	if props == nil {
		return nil
	}

	simplified := make(map[string]any)
	for key, val := range props {
		if prop, ok := val.(map[string]any); ok {
			typ := getString(prop, "type")
			if typ == "BOOLEAN" || typ == "TEXT" {
				simplified[key] = prop["value"]
			}
		}
	}

	if len(simplified) == 0 {
		return nil
	}
	return simplified
}
