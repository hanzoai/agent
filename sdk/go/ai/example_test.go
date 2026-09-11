package ai_test

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/hanzoai/agents/sdk/go/ai"
)

// The shortest thing this client does: one prompt, one answer.
//
// BaseURL is set rather than left to the default. The zero value points at
// api.openai.com, which is not where this estate's models are — api.hanzo.ai
// is the one endpoint, and an example that quietly talked to somebody else's
// would teach the wrong address.
func ExampleClient_Complete() {
	client, err := ai.NewClient(&ai.Config{
		APIKey:  os.Getenv("HANZO_API_KEY"),
		BaseURL: "https://api.hanzo.ai/v1",
		Model:   "zen-3",
	})
	if err != nil {
		fmt.Println("config:", err)
		return
	}

	// A deadline, because a model call is a network call and an example without
	// one teaches a program that can hang.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	answer, err := client.Complete(ctx, "Name the four colours of a standard deck of cards.")
	if err != nil {
		fmt.Println("complete:", err)
		return
	}

	// Choices is a slice and a refusal can leave it empty, so it is checked
	// rather than indexed — Choices[0] on an empty answer panics, which is a
	// worse failure than the one that produced it.
	if len(answer.Choices) == 0 {
		fmt.Println("the model returned no choices")
		return
	}
	fmt.Println(answer.Choices[0].Message.Content)
}

// The same call with a system instruction and a ceiling on the reply.
//
// Options are variadic and each is independent, which is what lets a caller
// add one without restating the rest.
func ExampleClient_Complete_options() {
	client, err := ai.NewClient(&ai.Config{
		APIKey:  os.Getenv("HANZO_API_KEY"),
		BaseURL: "https://api.hanzo.ai/v1",
	})
	if err != nil {
		fmt.Println("config:", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	answer, err := client.Complete(ctx, "Explain a hash map.",
		ai.WithSystem("You answer in exactly one sentence."),
		ai.WithModel("zen-3"),
		ai.WithTemperature(0),
		ai.WithMaxTokens(120),
	)
	if err != nil {
		fmt.Println("complete:", err)
		return
	}
	if len(answer.Choices) > 0 {
		fmt.Println(answer.Choices[0].Message.Content)
	}
}
