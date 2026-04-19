package transform

// ExtractorCombo selects which extractors run during simplification.
type ExtractorCombo int

const (
	AllExtractors ExtractorCombo = iota // everything (Phase 1 default)
	LayoutAndText                       // structure + text only
	VisualsOnly                         // fills/strokes/effects only
	ContentOnly                         // text only
)

// Options controls which extractors run and traversal depth.
type Options struct {
	MaxDepth   int
	Extractors ExtractorCombo
}

// GlobalVars holds deduplicated style values referenced by SimplifiedNodes.
type GlobalVars struct {
	Layouts    map[string]SimplifiedLayout    `json:"layouts,omitempty"`
	Fills      map[string]any                 `json:"fills,omitempty"`
	Strokes    map[string]any                 `json:"strokes,omitempty"`
	Effects    map[string]any                 `json:"effects,omitempty"`
	TextStyles map[string]SimplifiedTextStyle `json:"textStyles,omitempty"`
}

// SimplifiedLayout describes CSS flex-like layout.
type SimplifiedLayout struct {
	Mode          string `json:"mode,omitempty"` // "row" | "column" | "none"
	Gap           string `json:"gap,omitempty"`  // e.g. "8px"
	PaddingTop    string `json:"paddingTop,omitempty"`
	PaddingRight  string `json:"paddingRight,omitempty"`
	PaddingBottom string `json:"paddingBottom,omitempty"`
	PaddingLeft   string `json:"paddingLeft,omitempty"`
	Width         string `json:"width,omitempty"`       // "fill" | "hug" | "fixed"
	Height        string `json:"height,omitempty"`      // "fill" | "hug" | "fixed"
	ItemSpacing   string `json:"itemSpacing,omitempty"` // e.g. "8px" (horizontal gap in row)
}

// SimplifiedTextStyle holds typography values.
type SimplifiedTextStyle struct {
	FontFamily    string  `json:"fontFamily,omitempty"`
	FontSize      float64 `json:"fontSize,omitempty"`
	FontWeight    float64 `json:"fontWeight,omitempty"`
	LineHeight    string  `json:"lineHeight,omitempty"` // e.g. "24px" or "1.5"
	LetterSpacing float64 `json:"letterSpacing,omitempty"`
}

// SimplifiedNode is the LLM-friendly node representation.
type SimplifiedNode struct {
	ID                  string            `json:"id,omitempty"`
	Name                string            `json:"name,omitempty"`
	Type                string            `json:"type,omitempty"`
	Text                string            `json:"text,omitempty"`
	TextStyle           string            `json:"textStyle,omitempty"` // ref to globalVars textStyles
	Fills               string            `json:"fills,omitempty"`     // ref to globalVars fills
	Strokes             string            `json:"strokes,omitempty"`   // ref to globalVars strokes
	Layout              string            `json:"layout,omitempty"`    // ref to globalVars layouts
	Effects             string            `json:"effects,omitempty"`   // ref to globalVars effects
	Opacity             float64           `json:"opacity,omitempty"`
	BorderRadius        string            `json:"borderRadius,omitempty"`
	ComponentID         string            `json:"componentId,omitempty"`
	ComponentProperties map[string]any    `json:"componentProperties,omitempty"`
	Children            []*SimplifiedNode `json:"children,omitempty"`
}

// SimplifiedDesign is the top-level output.
type SimplifiedDesign struct {
	Name          string            `json:"name,omitempty"`
	Nodes         []*SimplifiedNode `json:"nodes,omitempty"`
	Components    map[string]any    `json:"components,omitempty"`
	ComponentSets map[string]any    `json:"componentSets,omitempty"`
	GlobalVars    *GlobalVars       `json:"globalVars,omitempty"`
}

// TraversalContext holds state during tree walking.
type TraversalContext struct {
	Depth      int
	MaxDepth   int
	Vars       *GlobalVars
	IDCounters struct {
		Layout    int
		Fill      int
		Stroke    int
		Effect    int
		TextStyle int
	}
}
