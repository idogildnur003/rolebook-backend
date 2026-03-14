package handler

// toInt converts a JSON-decoded number (float64) or int to int.
// Returns (0, false) if the value is neither type.
func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	default:
		return 0, false
	}
}
