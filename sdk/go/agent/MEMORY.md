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

Nothing creates it on first write.

Two things in that schema are load-bearing:

- **The unique index** over `(scope, scope_id, mkey)` is what makes one key one
  record. Without it a losing race writes a second record at the same key and
  reads start depending on which one Base returns first.
- **The rules are null**, so out of the box the collection is reachable by a
  superuser alone. Open them deliberately, per deployment.

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

The caller computes embeddings. `SearchVector` ranks the scope's vectors by
cosine in the client, which suits an agent's working set, not a corpus. Vectors
of another width came from another model and are skipped.
