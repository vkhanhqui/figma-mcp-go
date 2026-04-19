package transform

import "fmt"

// buildFormattedText extracts plain text content from a TEXT node.
func buildFormattedText(node map[string]any) string {
	// characters field holds the plain text
	return getString(node, "characters")
}

// extractTextStyle extracts typography into SimplifiedTextStyle and returns varID.
func extractTextStyle(node map[string]any, ctx *TraversalContext) string {
	style := &SimplifiedTextStyle{}

	if fn, ok := node["fontName"].(map[string]any); ok {
		style.FontFamily = getString(fn, "family")
		style.FontWeight = getFloat64(fn, "style") // fontName.style holds numeric weight in some contexts
	}

	style.FontSize = getFloat64(node, "fontSize")

	// lineHeight
	if lh, ok := node["lineHeight"].(map[string]any); ok {
		unit := getString(lh, "unit")
		val := getFloat64(lh, "value")
		if unit == "PERCENT" {
			style.LineHeight = fmt.Sprintf("%.1f%%", val)
		} else if unit == "PIXELS" {
			style.LineHeight = cssLen(val, "px")
		} else if unit == "AUTO" {
			style.LineHeight = "auto"
		}
	}

	style.LetterSpacing = getFloat64(node, "letterSpacing")

	// Only store if we have meaningful typography
	if style.FontFamily == "" && style.FontSize == 0 && style.LineHeight == "" {
		return ""
	}

	ctx.IDCounters.TextStyle++
	id := generateVarId("style", ctx.IDCounters.TextStyle)
	ctx.Vars.TextStyles[id] = *style
	return id
}