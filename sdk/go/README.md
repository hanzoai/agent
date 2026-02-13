# Hanzo Agents Go SDK

The Hanzo Agents Go SDK provides idiomatic Go bindings for interacting with the Hanzo Agents control plane.

## Installation

```bash
go get github.com/hanzoai/agents/sdk/go
```

## Quick Start

```go
package main

import (
    "context"
    "log"

    hanzo-agentsagent "github.com/hanzoai/agents/sdk/go/agent"
)

func main() {
    agent, err := hanzo-agentsagent.New(hanzo-agentsagent.Config{
        NodeID:   "example-agent",
        Hanzo AgentsURL: "http://localhost:8080",
    })
    if err != nil {
        log.Fatal(err)
    }

    agent.RegisterSkill("health", func(ctx context.Context, _ map[string]any) (any, error) {
        return map[string]any{"status": "ok"}, nil
    })

    if err := agent.Run(context.Background()); err != nil {
        log.Fatal(err)
    }
}
```

## Modules

- `agent`: Build Hanzo Agents-compatible agents and register reasoners/skills.
- `client`: Low-level HTTP client for the Hanzo Agents control plane.
- `types`: Shared data structures and contracts.
- `ai`: Helpers for interacting with AI providers via the control plane.

## Testing

```bash
go test ./...
```

## License

Distributed under the Apache 2.0 License. See the repository root for full details.
