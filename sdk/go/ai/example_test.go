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
// A nil config reads the environment: HANZO_API_KEY selects api.hanzo.ai, and
// a third-party key selects that provider instead. Nothing here names an
// address, because the default is already ours.
func ExampleClient_Complete() {
	client, err := ai.NewClient(nil)
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
	// Spelled out, to show what the nil form above resolves to.
	client, err := ai.NewClient(&ai.Config{
		APIKey:  os.Getenv("HANZO_API_KEY"),
		BaseURL: "https://api.hanzo.ai/v1",
		Timeout: 30 * time.Second,
	})
	if err != nil {
		fmt.Println("config:", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	answer, err := client.Complete(ctx, "Explain a hash map.",
		ai.WithSystem("You answer in exactly one sentence."),
		ai.WithModel("zen3-vl"),
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
