package agent

import (
	"context"
	"math"
	"testing"
)

func TestCosine(t *testing.T) {
	for _, c := range []struct {
		name string
		a, b []float64
		want float64
	}{
		{"identical", []float64{1, 2, 3}, []float64{1, 2, 3}, 1},
		{"scaled is the same direction", []float64{1, 0}, []float64{7, 0}, 1},
		{"perpendicular", []float64{1, 0}, []float64{0, 1}, 0},
		{"opposite", []float64{1, 0}, []float64{-1, 0}, -1},
		{"a zero vector has no direction", []float64{0, 0}, []float64{1, 0}, 0},
	} {
		if got := cosine(c.a, c.b); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestMatches(t *testing.T) {
	meta := map[string]any{"kind": "note", "depth": 2.0, "live": true}

	for _, c := range []struct {
		name    string
		filters map[string]any
		want    bool
	}{
		{"no filters match everything", nil, true},
		{"a value that agrees", map[string]any{"kind": "note"}, true},
		{"a value that disagrees", map[string]any{"kind": "task"}, false},
		{"every filter must hold", map[string]any{"kind": "note", "live": false}, false},
		{"an int filter meets a decoded float", map[string]any{"depth": 2}, true},
		{"a key the record lacks", map[string]any{"author": "z"}, false},
	} {
		if got := matches(meta, c.filters); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

// The default backend is the one a caller gets from NewMemory(nil), and it
// answered every similarity search with an empty list — which a caller cannot
// tell from "nothing was similar".
func TestInMemorySearchRanks(t *testing.T) {
	backend := NewInMemoryBackend()
	backend.SetVector(ScopeSession, "s1", "same", []float64{1, 0, 0}, nil)
	backend.SetVector(ScopeSession, "s1", "near", []float64{0.9, 0.4, 0}, nil)
	backend.SetVector(ScopeSession, "s1", "across", []float64{0, 1, 0}, map[string]any{"kind": "task"})

	hits, err := backend.SearchVector(ScopeSession, "s1", []float64{1, 0, 0}, SearchOptions{})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 3 {
		t.Fatalf("%d hits, want 3", len(hits))
	}
	if hits[0].Key != "same" || hits[2].Key != "across" {
		t.Fatalf("order %s %s %s", hits[0].Key, hits[1].Key, hits[2].Key)
	}

	filtered, _ := backend.SearchVector(ScopeSession, "s1", []float64{1, 0, 0},
		SearchOptions{Filters: map[string]any{"kind": "task"}})
	if len(filtered) != 1 || filtered[0].Key != "across" {
		t.Fatalf("filtered %#v", filtered)
	}

	capped, _ := backend.SearchVector(ScopeSession, "s1", []float64{1, 0, 0},
		SearchOptions{Threshold: 0.5, Limit: 1})
	if len(capped) != 1 || capped[0].Key != "same" {
		t.Fatalf("capped %#v", capped)
	}

	if _, err := backend.SearchVector(ScopeSession, "s1", nil, SearchOptions{}); err == nil {
		t.Fatal("searching with no query vector should refuse")
	}
}

// A search reaches the backend through the Memory type, which is how a handler
// asks. It used to arrive at a stub.
func TestMemorySearchReachesTheBackend(t *testing.T) {
	memory := NewMemory(nil)
	ctx := context.Background()

	if err := memory.SetVector(ctx, "recalled", []float64{1, 0}, nil); err != nil {
		t.Fatalf("set: %v", err)
	}
	hits, err := memory.SearchVector(ctx, []float64{1, 0}, SearchOptions{})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 || hits[0].Key != "recalled" {
		t.Fatalf("hits %#v", hits)
	}
}
