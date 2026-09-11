package ai_test

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/hanzoai/agent/sdk/go/ai"
)

// One prompt, one answer. A nil config reads the environment, so HANZO_API_KEY
// is all this needs.
func ExampleClient_Complete() {
	client, err := ai.NewClient(nil)
	if err != nil {
		fmt.Println("config:", err)
		return
	}

	// A model call is a network call; give it a deadline.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	answer, err := client.Complete(ctx, "Name the four colours of a standard deck of cards.")
	if err != nil {
		fmt.Println("complete:", err)
		return
	}

	// A refusal can leave Choices empty.
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
