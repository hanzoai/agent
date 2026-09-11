package agent

import (
	"math"
	"sort"
)

// The vector math every memory backend shares.

// cosine is the similarity of two vectors of equal width: 1 for the same
// direction, 0 for perpendicular, -1 for opposite. A zero vector has no
// direction, so it scores 0 rather than dividing by zero.
func cosine(a, b []float64) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// matches reports whether metadata satisfies every filter. A missing key fails
// its filter; numbers compare as float64, as JSON decodes them.
func matches(metadata map[string]any, filters map[string]any) bool {
	for key, want := range filters {
		got, ok := metadata[key]
		if !ok || !same(got, want) {
			return false
		}
	}
	return true
}

func same(a, b any) bool {
	if x, ok := number(a); ok {
		if y, ok := number(b); ok {
			return x == y
		}
		return false
	}
	return a == b
}

func number(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	default:
		return 0, false
	}
}

// rank orders results by score, strongest first, and cuts to the limit.
func rank(found []VectorSearchResult, limit int) []VectorSearchResult {
	sort.SliceStable(found, func(i, j int) bool { return found[i].Score > found[j].Score })
	if limit > 0 && len(found) > limit {
		found = found[:limit]
	}
	return found
}
