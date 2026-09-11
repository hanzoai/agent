# Agent memory in Hanzo Base

`BaseMemoryBackend` keeps an agent's memory in Base, so its state lives in the
same single binary as everything else it runs beside. One record per scope and
key, in one collection.

## Create the collection

Import the schema once, as a superuser:

```sh
curl -X PUT http://127.0.0.1:8090/v1/collections/import \
  -H "Authorization: $(hanzo auth token)" \
  -H 'Content-Type: application/json' \
  --data @agent_memory.collection.json
```

Base's admin UI imports the same file from Settings.

It is not created on first write. A client that builds its own schema can build
the wrong one from a typo in a field name, and then read nothing from the
collection everyone else is looking at.

Two things in that schema are load-bearing:

- **The unique index** over `(scope, scope_id, mkey)` is what makes one key one
  record. Without it a losing race writes a second record at the same key and
  reads start depending on which one Base returns first.
- **The rules are null**, so out of the box the collection is reachable by a
  superuser alone. Open them deliberately, per deployment. An agent's memory is
  the agent's reasoning written down.

## Use it

```go
memory := agent.NewMemory(
    agent.NewBaseMemoryBackend("http://127.0.0.1:8090", token, "agent_memory"),
)

memory.Set(ctx, "tone", "plain")
```

`NewMemory(nil)` gives the in-memory backend instead, which is right for a test
and loses everything when the process ends.

## Similarity search

The embedding is the caller's to compute, so which model produced it stays one
decision made in one place. `SearchVector` reads the scope's vectors and ranks
them by cosine in the client — right for an agent's own working set, wrong for a
corpus. A scope large enough to need server-side scoring should use a Base hook.

A vector of a different width came from a different model, so it is skipped
rather than scored: cosine across two of those returns a number that means
nothing.
