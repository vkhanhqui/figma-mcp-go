package transform

import "fmt"

// parsePaint converts a single paint object into a CSS string.
func parsePaint(paint map[string]any) string {
	paintType, _ := paint["type"].(string)

	switch paintType {
	case "SOLID":
		color, _ := paint["color"].(map[string]any)
		if color == nil {
			return ""
		}
		r := pixelRound(getFloat64(color, "r") * 255)
		g := pixelRound(getFloat64(color, "g") * 255)
		b := pixelRound(getFloat64(color, "b") * 255)
		opacity := 1.0
		if op, ok := paint["opacity"].(float64); ok {
			opacity = op
		}
		if opacity == 1 {
			return fmt.Sprintf("#%02X%02X%02X", int(r), int(g), int(b))
		}
		a := pixelRound(opacity * 255)
		return fmt.Sprintf("rgba(%d,%d,%d,%d)", int(r), int(g), int(b), int(a))

	case "GRADIENT_LINEAR":
		return "linear-gradient(...)"

	case "GRADIENT_RADIAL":
		return "radial-gradient(...)"

	case "GRADIENT_ANGULAR", "GRADIENT_DIAMOND":
		return "gradient(...)"
	}
	return ""
}

// buildSimplifiedFills extracts fills and returns (varID, cssValue).
func buildSimplifiedFills(node map[string]any, ctx *TraversalContext) (string, any) {
	fills := getSlice(node, "fills")
	if len(fills) == 0 {
		return "", nil
	}

	var cssValues []string
	for _, f := range fills {
		if paint, ok := f.(map[string]any); ok {
			if css := parsePaint(paint); css != "" {
				cssValues = append(cssValues, css)
			}
		}
	}
	if len(cssValues) == 0 {
		return "", nil
	}

	ctx.IDCounters.Fill++
	id := generateVarId("fill", ctx.IDCounters.Fill)

	var val any
	if len(cssValues) == 1 {
		val = cssValues[0]
	} else {
		val = cssValues
	}
	ctx.Vars.Fills[id] = val
	return id, val
}

// buildSimplifiedStrokes extracts strokes and returns (varID, cssValue).
func buildSimplifiedStrokes(node map[string]any, ctx *TraversalContext) (string, any) {
	strokes := getSlice(node, "strokes")
	if len(strokes) == 0 {
		return "", nil
	}

	var cssValues []string
	for _, s := range strokes {
		if stroke, ok := s.(map[string]any); ok {
			if css := parsePaint(stroke); css != "" {
				cssValues = append(cssValues, css)
			}
		}
	}
	if len(cssValues) == 0 {
		return "", nil
	}

	ctx.IDCounters.Stroke++
	id := generateVarId("stroke", ctx.IDCounters.Stroke)

	var val any
	if len(cssValues) == 1 {
		val = cssValues[0]
	} else {
		val = cssValues
	}
	ctx.Vars.Strokes[id] = val
	return id, val
}