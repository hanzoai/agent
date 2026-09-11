package agent_test

import (
	"context"
	"fmt"

	"github.com/hanzoai/agent/sdk/go/agent"
)

// Keeping an agent's memory in Hanzo Base. Import agent_memory.collection.json
// once first; MEMORY.md shows how.
func ExampleNewBaseMemoryBackend() {
	memory := agent.NewMemory(
		agent.NewBaseMemoryBackend("http://127.0.0.1:8090", token, "agent_memory"),
	)

	ctx := context.Background()
	if err := memory.Set(ctx, "tone", "plain"); err != nil {
		panic(err)
	}

	// Memory.Get cannot tell an absent key from a stored nil; this can.
	tone, err := memory.GetWithDefault(ctx, "tone", "plain")
	if err != nil {
		panic(err)
	}
	fmt.Println(tone)
}

// Recalling by similarity. The caller computes the embedding.
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

// Stand-ins: a real token comes from Hanzo IAM, a real embedding from a model.
var token = "..."

func embed(string) []float64 { return nil }
