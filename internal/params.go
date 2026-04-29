package internal

// Param helpers — small typed accessors over the loosely-typed argument map
// that mcp-go hands to tool handlers. The goal is to replace boilerplate like
//
//	if x, ok := args["x"].(float64); ok { params["x"] = x }
//
// with a one-liner that's harder to misspell:
//
//	CopyFloat(params, args, "x")

// GetString returns args[key] as string, or def if absent or non-string.
func GetString(args map[string]interface{}, key, def string) string {
	if v, ok := args[key].(string); ok && v != "" {
		return v
	}
	return def
}

// GetFloat returns args[key] as float64. The bool reports presence.
func GetFloat(args map[string]interface{}, key string) (float64, bool) {
	v, ok := args[key].(float64)
	return v, ok
}

// GetBool returns args[key] as bool, or def if absent or non-bool.
func GetBool(args map[string]interface{}, key string, def bool) bool {
	if v, ok := args[key].(bool); ok {
		return v
	}
	return def
}

// CopyString copies args[key] into dst[key] iff it is a non-empty string.
func CopyString(dst, src map[string]interface{}, key string) {
	if v, ok := src[key].(string); ok && v != "" {
		dst[key] = v
	}
}

// CopyFloat copies args[key] into dst[key] iff it is a float64.
func CopyFloat(dst, src map[string]interface{}, key string) {
	if v, ok := src[key].(float64); ok {
		dst[key] = v
	}
}

// CopyBool copies args[key] into dst[key] iff it is a bool.
func CopyBool(dst, src map[string]interface{}, key string) {
	if v, ok := src[key].(bool); ok {
		dst[key] = v
	}
}

// CopyAny copies args[key] into dst[key] iff present (any type).
func CopyAny(dst, src map[string]interface{}, key string) {
	if v, ok := src[key]; ok {
		dst[key] = v
	}
}
