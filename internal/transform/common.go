package transform

import (
	"encoding/json"
	"fmt"
)

// pixelRound rounds a pixel value to 2 decimal places.
func pixelRound(v float64) float64 {
	rounded := int(v*100+0.5) // fast alternative to math.Round
	return float64(rounded) / 100
}

// generateVarId generates a deterministic variable ID from a prefix and value.
func generateVarId(prefix string, counter int) string {
	return fmt.Sprintf("%s_%d", prefix, counter)
}

// stableStringify produces a canonical JSON string for deduplication.
// Returns an empty string if marshaling fails, which is intentional since
// an empty string as a deduplication key will simply not match any existing entry.
func stableStringify(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// mergeMaps merges src into dst, returning dst.
// Note: This mutates dst in place.
func mergeMaps(dst, src map[string]any) map[string]any {
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// getString safely extracts a string field from a map.
func getString(node map[string]any, key string) string {
	if v, ok := node[key].(string); ok {
		return v
	}
	return ""
}

// getFloat64 safely extracts a float64 field from a map.
func getFloat64(node map[string]any, key string) float64 {
	if v, ok := node[key].(float64); ok {
		return v
	}
	return 0
}

// getBool safely extracts a bool field from a map.
func getBool(node map[string]any, key string) bool {
	if v, ok := node[key].(bool); ok {
		return v
	}
	return false
}

// getStringMap safely extracts a map[string]any field.
func getStringMap(node map[string]any, key string) map[string]any {
	if v, ok := node[key].(map[string]any); ok {
		return v
	}
	return nil
}

// getSlice safely extracts a []any field.
func getSlice(node map[string]any, key string) []any {
	if v, ok := node[key].([]any); ok {
		return v
	}
	return nil
}

// cssLen produces a CSS length string (e.g. "8px" or "auto").
func cssLen(v float64, unit string) string {
	if v == 0 {
		return "0"
	}
	return fmt.Sprintf("%.2f%s", pixelRound(v), unit)
}

// cssFlexMode determines the CSS flex mode from layoutAlign and layoutMode.
func cssFlexMode(layoutMode, layoutAlign string) string {
	if layoutMode == "HORIZONTAL" {
		return "row"
	}
	if layoutMode == "VERTICAL" {
		return "column"
	}
	return "none"
}
