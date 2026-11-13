# ZAP extra

```bash
pip install "hanzo-agent[zap]"
```

The extra adds no dependencies. `agents.zap` is part of the base package, and its transport is the standard library (`asyncio` and `json`). It does not use Cap'n Proto, so nothing needs installing for it.

`agents.zap` is a **client**. It connects to a ZAP gateway, which is a separate service, and turns the gateway's tool catalog into tools an agent can call. This repo ships no gateway.

## The wire

Every message is a 4-byte big-endian length followed by that many bytes of UTF-8 JSON:

```json
{"type": "catalog.invoke", "id": "5c1e...", "payload": {"id": {...}, "args": {...}, "ctx": {...}}}
```

- A request's `type` is the method name. The client generates `id`.
- The reply carries the same `id`. Its `payload` is the result, or, when `type` is `"error"`, `{"code", "message", "details"?}`.
- The first request after connecting is `initialize` (the client's `Hello`). The reply is the gateway's `Welcome`.

Addresses:

| URI | Transport |
|---|---|
| `zap://host:port` | TCP (port defaults to 9999) |
| `zap+tls://host:port` | TLS over TCP |
| `zap+unix:///path/to.sock` | Unix socket |

## Client

```python
import asyncio
from agents.zap import ZapClient

async def main():
    async with ZapClient.from_uri("zap://localhost:9999") as client:
        print(client.capabilities)                       # what the gateway offers
        tools = await client.list_tools()                # catalog.listTools
        result = await client.call_tool("native/fs.read", {"path": "/etc/hosts"})
        if result.success:
            print(result.data)
        else:
            print(result.error.code, result.error.message)

asyncio.run(main())
```

A tool is named `namespace/name@version`. A bare name means namespace `native`, version `1.0.0`, so `"fs.read"` is `"native/fs.read@1.0.0"`.

| Method | Wire `type` |
|---|---|
| `connect()` / `async with` | `initialize` |
| `ping()` returns `(latency_ns, server_time)` | `ping` |
| `list_tools(certified_only=False)`, `get_tool(id)`, `search_tools(query)` | `catalog.listTools`, `catalog.getTool`, `catalog.search` |
| `call_tool(name, arguments)` returns a `ToolResult` | `catalog.invoke` |
| `fs_read`, `fs_write`, `fs_glob`, `proc_run`, `vcs_status`, `net_fetch` | `catalog.invoke` on `native/fs.read`, `fs.write`, `fs.glob`, `proc.run`, `vcs.status`, `net.fetch` |
| `list_mcp_tools()`, `call_mcp_tool(name, json_args)`, `register_mcp_server(name, endpoint)` | `gateway.listMcpTools`, `gateway.callMcpTool`, `gateway.registerMcpServer` |
| `propose_consensus(topic, proposal, config)`, `committee_query(question, participants, config)` | `coordination.propose`, `coordination.committee` |

`call_tool` returns a failure as `ToolResult(success=False, error=...)`; the convenience methods (`fs_read` and the rest) and the other calls raise `ZapProtocolError` instead. A request with no reply within `request_timeout` (30 s) fails with code `timeout`. A failed connection raises `ZapConnectionError`.

MCP aggregation is the gateway's job. `register_mcp_server` tells the gateway about an MCP server, and the gateway serves that server's tools through the catalog under their own namespace (the provider's docstring uses `mcp.github`).

## Give an agent the gateway's tools

`ZapAgent` is an `Agent` that connects on entry and adds the gateway's tools to itself.

```python
from agents import Runner
from agents.zap import ZapAgent, ZapAgentConfig

config = ZapAgentConfig(
    gateway_uri="zap://localhost:9999",
    tool_namespaces=["mcp.github"],        # optional: only these namespaces
)

async with ZapAgent(name="code-agent", instructions="You help with code.", zap_config=config) as agent:
    result = await Runner.run(agent, input="Read /etc/hosts")
```

`ZapAgentConfig` fields:

| Field | Default | Meaning |
|---|---|---|
| `gateway_uri` | `zap://localhost:9999` | the gateway |
| `connect_timeout`, `request_timeout` | 5 s, 30 s | |
| `include_canonical` | `True` | add `fs_read`, `fs_write`, `fs_glob`, `proc_run`, `vcs_status`, `net_fetch` |
| `tool_namespaces`, `tool_prefixes` | `None` | add catalog tools in these namespaces or whose names start with these prefixes |
| `certified_only` | `False` | only catalog tools whose stability is `stable` |
| `enable_consensus`, `consensus_participants` | `False`, `[]` | stored on the config; `ZapAgent` does not act on them |

With `include_canonical=False` and no filters, the agent gets every tool in the catalog. A catalog tool's name has its dots replaced by underscores (`issues.list` becomes `issues_list`), and a tool whose name the agent already has is skipped. `create_zap_agent(name, gateway_uri, instructions, model)` builds and connects one in a single call.

To pick tools yourself, use a provider:

```python
from agents import Agent
from agents.zap import ZapToolProvider

async with ZapToolProvider.from_uri("zap://localhost:9999") as provider:
    tools = await provider.get_tools(namespace="native", prefix="fs.")
    agent = Agent(name="file-agent", tools=tools)
    ...  # run the agent while the provider is open
```

The tools call back through the provider's connection, so run the agent inside the `async with`. `ZapToolRegistry` holds several providers and lists their tools together.

## Consensus

`ConsensusAgent` decides a question two ways.

**Through the gateway.** The gateway asks a committee of models or agents and returns an answer with a certificate.

```python
from agents.zap import gateway_consensus

decision = await gateway_consensus(
    "Which database should we use?",
    "zap://localhost:9999",
    participants=["model-a", "model-b", "model-c"],
)
print(decision.answer, decision.confidence, decision.round)
```

`ConsensusConfig` (`rounds` 3, `k` 5, `alpha` 0.6, `beta1` 0.8, `beta2` 0.9, `timeout_ms` 10000) travels to the gateway, which owns the protocol. `decision.votes` is empty; the gateway does not return individual votes.

**With local agents.** `consensus_decide(question, agents)` runs every agent on the question and takes a vote.

- Responses are grouped after lowercasing and trimming whitespace. Punctuation still separates two answers.
- `confidence` is the share of agents in the largest group, and `answer` is that group's first response.
- No further rounds run: `round` is `1` when `confidence` reaches `beta2` and `2` otherwise. `rounds`, `k`, `alpha` and `beta1` apply only at the gateway.
- An agent that errors counts as a vote for the text `Error: ...`.
- The certificate lists the agreeing agents with empty signatures.

Pass `synthesizer=` an agent to `ConsensusAgent.from_agents` to have it write the final text from all responses.

## Known gaps

- `create_zap_tools(uri)` closes its connection before it returns, so every tool it returns raises `RuntimeError: Not connected` when called. Use `ZapToolProvider` in an `async with`, or `ZapAgent`.
- `ZapClient.list_resources()` raises `TypeError: Object of type bytes is not JSON serializable`.
