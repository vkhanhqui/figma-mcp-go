package transform

import (
	"testing"
)

func TestSimplify_SingleFrame(t *testing.T) {
	input := map[string]any{
		"id":   "4029:123",
		"name": "Header",
		"type": "FRAME",
		"fills": []any{
			map[string]any{
				"type":  "SOLID",
				"color": map[string]any{"r": 0.23, "g": 0.51, "b": 0.96},
			},
		},
		"cornerRadius": 8.0,
		"opacity":      0.5,
		"layoutMode":   "HORIZONTAL",
		"spacing":      16.0,
	}

	opts := Options{MaxDepth: 10, Extractors: AllExtractors}
	result, err := Simplify(input, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(result.Nodes))
	}

	node := result.Nodes[0]
	if node.ID != "4029:123" {
		t.Errorf("expected ID 4029:123, got %s", node.ID)
	}
	if node.BorderRadius != "8.00px" {
		t.Errorf("expected borderRadius 8.00px, got %s", node.BorderRadius)
	}
	if node.Opacity != 0.5 {
		t.Errorf("expected opacity 0.5, got %f", node.Opacity)
	}
	if node.Layout == "" {
		t.Error("expected layout ref, got empty")
	}
	if node.Fills == "" {
		t.Error("expected fills ref, got empty")
	}

	if result.GlobalVars == nil {
		t.Fatal("expected globalVars, got nil")
	}
}

func TestSimplify_ArrayResponse(t *testing.T) {
	input := []any{
		map[string]any{
			"id":   "4029:1",
			"name": "Node1",
			"type": "FRAME",
		},
		map[string]any{
			"id":   "4029:2",
			"name": "Node2",
			"type": "FRAME",
		},
	}

	opts := Options{Extractors: AllExtractors}
	result, err := Simplify(input, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(result.Nodes))
	}
}

func TestSimplify_FallbackOnError(t *testing.T) {
	opts := Options{Extractors: AllExtractors}
	_, err := Simplify(nil, opts)
	if err != nil {
		t.Fatalf("unexpected error on nil: %v", err)
	}
}

func TestPixelRound(t *testing.T) {
	tests := []struct {
		input    float64
		expected float64
	}{
		{123.99999999999999, 124.0},
		{123.456, 123.46},
		{0.0, 0.0},
		{0.001, 0.0},
	}

	for _, tt := range tests {
		result := pixelRound(tt.input)
		if result != tt.expected {
			t.Errorf("pixelRound(%f) = %f; want %f", tt.input, result, tt.expected)
		}
	}
}

func TestPixelRound_Negative(t *testing.T) {
	tests := []struct {
		input    float64
		expected float64
	}{
		{-0.006, -0.01},
		{-123.456, -123.46},
		{-0.5, -0.5},
		{-2.0, -2.0},
	}

	for _, tt := range tests {
		result := pixelRound(tt.input)
		if result != tt.expected {
			t.Errorf("pixelRound(%f) = %f; want %f", tt.input, result, tt.expected)
		}
	}
}

func TestParsePaint_Solid(t *testing.T) {
	paint := map[string]any{
		"type": "SOLID",
		"color": map[string]any{
			"r": 0.23,
			"g": 0.51,
			"b": 0.96,
		},
	}
	result := parsePaint(paint)
	if result != "#3A82F4" {
		t.Errorf("expected #3A82F4, got %s", result)
	}
}

func TestParsePaint_SolidWithOpacity(t *testing.T) {
	paint := map[string]any{
		"type": "SOLID",
		"color": map[string]any{
			"r": 0.23,
			"g": 0.51,
			"b": 0.96,
		},
		"opacity": 0.5,
	}
	result := parsePaint(paint)
	if result != "rgba(58,130,244,0.50)" {
		t.Errorf("expected rgba(58,130,244,0.50), got %s", result)
	}
}

func TestBuildSimplifiedLayout_Row(t *testing.T) {
	node := map[string]any{
		"type":       "FRAME",
		"layoutMode": "HORIZONTAL",
		"spacing":    8.0,
		"paddingTop": 16.0,
	}
	ctx := &TraversalContext{
		Vars: &GlobalVars{
			Layouts: make(map[string]SimplifiedLayout),
		},
	}
	id, layout := buildSimplifiedLayout(node, ctx)
	if id == "" {
		t.Error("expected layout id")
	}
	if layout.Mode != "row" {
		t.Errorf("expected mode 'row', got %s", layout.Mode)
	}
	if layout.Gap != "8.00px" {
		t.Errorf("expected gap '8.00px', got %s", layout.Gap)
	}
}

func TestBuildSimplifiedLayout_Column(t *testing.T) {
	node := map[string]any{
		"type":       "FRAME",
		"layoutMode": "VERTICAL",
		"spacing":    12.0,
	}
	ctx := &TraversalContext{
		Vars: &GlobalVars{
			Layouts: make(map[string]SimplifiedLayout),
		},
	}
	_, layout := buildSimplifiedLayout(node, ctx)
	if layout.Mode != "column" {
		t.Errorf("expected mode 'column', got %s", layout.Mode)
	}
}

func TestBuildSimplifiedLayout_NoLayout(t *testing.T) {
	node := map[string]any{
		"type": "RECTANGLE",
	}
	ctx := &TraversalContext{
		Vars: &GlobalVars{
			Layouts: make(map[string]SimplifiedLayout),
		},
	}
	id, layout := buildSimplifiedLayout(node, ctx)
	if id != "" {
		t.Error("expected no layout id for non-frame node without layoutMode")
	}
	if layout != nil {
		t.Error("expected nil layout for non-frame node")
	}
}

func TestIsVisible(t *testing.T) {
	tests := []struct {
		node     map[string]any
		expected bool
	}{
		{map[string]any{}, true},
		{map[string]any{"visible": true}, true},
		{map[string]any{"visible": false}, false},
	}

	for _, tt := range tests {
		result := isVisible(tt.node)
		if result != tt.expected {
			t.Errorf("isVisible(%v) = %v; want %v", tt.node, result, tt.expected)
		}
	}
}

func TestBuildSimplifiedStrokes(t *testing.T) {
	node := map[string]any{
		"type": "FRAME",
		"strokes": []any{
			map[string]any{
				"type":  "SOLID",
				"color": map[string]any{"r": 1.0, "g": 0.0, "b": 0.0},
			},
		},
	}
	ctx := &TraversalContext{Vars: &GlobalVars{Strokes: make(map[string]any)}}
	id, val := buildSimplifiedStrokes(node, ctx)
	if id == "" {
		t.Error("expected stroke id")
	}
	if val == nil {
		t.Error("expected stroke value")
	}
	if ctx.Vars.Strokes[id] == nil {
		t.Error("stroke not stored in Strokes map")
	}
}

func TestSimplify_WithStrokes(t *testing.T) {
	input := map[string]any{
		"id":   "4029:1",
		"name": "Box",
		"type": "FRAME",
		"strokes": []any{
			map[string]any{
				"type":  "SOLID",
				"color": map[string]any{"r": 0.0, "g": 0.0, "b": 0.0},
			},
		},
	}

	result, err := Simplify(input, Options{Extractors: AllExtractors})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	node := result.Nodes[0]
	if node.Strokes == "" {
		t.Error("expected Strokes ref on node, got empty")
	}
	if _, ok := result.GlobalVars.Strokes[node.Strokes]; !ok {
		t.Errorf("stroke ref %s not found in globalVars.Strokes", node.Strokes)
	}
}

func TestMapNodeType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"VECTOR", "IMAGE-SVG"},
		{"TEXT", "TEXT"},
		{"FRAME", "FRAME"},
		{"INSTANCE", "INSTANCE"},
	}

	for _, tt := range tests {
		result := mapNodeType(tt.input)
		if result != tt.expected {
			t.Errorf("mapNodeType(%s) = %s; want %s", tt.input, result, tt.expected)
		}
	}
}
