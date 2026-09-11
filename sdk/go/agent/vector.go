package agent

import (
	"math"
	"sort"
)

// The vector math every memory backend needs, in one place.
//
// It lived nowhere: the in-memory backend answered a similarity search with an
// empty list and a comment saying vector math was required, which is the one
// answer a caller cannot tell from "nothing was similar". A search that cannot
// run should say so; a search that can should run. Both backends hold the
// vectors already, so the arithmetic is all that was missing.

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

// matches reports whether a record's metadata satisfies every filter.
//
// Equality on the JSON shapes metadata actually carries, so a filter written
// as a number matches a record decoded from JSON as a float. A filter naming a
// key the record does not carry excludes it — the caller asked for records
// where that key holds a value, and a record without the key is not one.
func matches(metadata map[string]any, filters map[string]any) bool {
	for key, want := range filters {
		got, ok := metadata[key]
		if !ok || !same(got, want) {
			return false
		}
	}
	return true
}

// same compares two decoded JSON values. Numbers are compared as float64
// because that is what a JSON decode produces, so a filter of `2` written as
// an int still matches a stored `2`.
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
//
// The threshold is applied while scoring, before this — cutting to the limit
// first would return fewer results than asked for whenever the tail is weak,
// which reads as missing data rather than as a weak tail.
func rank(found []VectorSearchResult, limit int) []VectorSearchResult {
	sort.SliceStable(found, func(i, j int) bool { return found[i].Score > found[j].Score })
	if limit > 0 && len(found) > limit {
		found = found[:limit]
	}
	return found
}
