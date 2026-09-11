package agent_test

import (
	"context"
	"fmt"

	"github.com/hanzoai/agents/sdk/go/agent"
)

// Keeping an agent's memory in Hanzo Base.
//
// The collection must already exist, carrying the fields the backend writes:
// scope, scope_id, mkey and value as text, embedding and metadata as json.
// Create it once with the Base admin UI or its CLI — an SDK that creates
// schema on first write can create the WRONG schema from a typo, and then read
// nothing from the collection everyone else is looking at.
func ExampleNewBaseMemoryBackend() {
	memory := agent.NewMemory(
		agent.NewBaseMemoryBackend("http://127.0.0.1:8090", token, "agent_memory"),
	)

	ctx := context.Background()
	if err := memory.Set(ctx, "tone", "plain"); err != nil {
		panic(err)
	}

	// GetWithDefault, because Memory.Get folds an absent key and a stored nil
	// into one answer — the backend tells them apart, the wrapper does not.
	tone, err := memory.GetWithDefault(ctx, "tone", "plain")
	if err != nil {
		panic(err)
	}
	fmt.Println(tone)
}

// Recalling by similarity. The embedding is the caller's to compute — the
// backend stores the vector it is handed and scores against it, so which model
// produced it stays one decision made in one place.
func ExampleNewBaseMemoryBackend_recall() {
	memory := agent.NewMemory(
		agent.NewBaseMemoryBackend("http://127.0.0.1:8090", token, "agent_memory"),
	)

	ctx := context.Background()
	if err := memory.SetVector(ctx, "the deploy runbook", embed("the deploy runbook"),
		map[string]any{"kind": "doc"}); err != nil {
		panic(err)
	}

	hits, err := memory.SearchVector(ctx, embed("how do I ship this"), agent.SearchOptions{
		Limit:     5,
		Threshold: 0.7,
		Filters:   map[string]any{"kind": "doc"},
	})
	if err != nil {
		panic(err)
	}
	for _, hit := range hits {
		fmt.Printf("%s %.2f\n", hit.Key, hit.Score)
	}
}

// Stand-ins so the examples above read as the calling code someone writes,
// rather than as setup. A real token comes from Hanzo IAM and a real embedding
// from whichever model the caller has chosen.
var token = "..."

func embed(string) []float64 { return nil }
