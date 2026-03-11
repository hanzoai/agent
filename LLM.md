# LLM.md - Hanzo AI Agent SDK

## Overview
Python framework for building AI agents and multi-agent systems with orchestration, MCP tools, memory, and observability via Hanzo Cloud.

## Architecture
```
agent/
├── src/hanzoai/agents/    # Core agent SDK (multi-agent, routing, orchestration)
├── sdk/                   # Extended SDK packages
├── control-plane/         # Agent control plane
├── deployments/           # K8s deployment manifests
├── docs/                  # Documentation (MkDocs)
├── examples/              # Usage examples
├── tests/                 # Test suite
└── scripts/               # Dev/build scripts
```

## Tech Stack
- **Language**: Python 3.9+
- **Package**: `hanzoai` on PyPI (v0.0.4)
- **Dependencies**: openai, pydantic, griffe, requests
- **Optional extras**: `[web3]`, `[tee]`, `[marketplace]`, `[cli]`
- **Docs**: MkDocs
- **Build**: uv, Makefile

## Build & Run
```bash
uv sync --all-extras        # Install with all extras
uv run pytest               # Run tests
make dev                    # Development mode
make test                   # Run test suite
```

## Key Concepts
1. **Agents**: LLMs with instructions, tools, and memory
2. **Networks**: Multi-agent systems with semantic/rule-based/load-balanced routing
3. **Workflows**: Orchestrated multi-step processes (parallel, conditional, loop)
4. **State & Memory**: Shared state + vector search long-term memory
5. **Tools**: Enhanced tool system with MCP support
6. **Tracing**: Built-in observability via Hanzo Cloud

## Key Files
- `pyproject.toml` — Package config and dependencies
- `src/hanzoai/agents/` — Core agent implementation
- `docs/agents.md` — Agent documentation
- `docs/networks-and-orchestration.md` — Multi-agent docs
- `examples/` — Working examples

## Patterns
- OpenAI Chat Completions API compatible (works with 100+ providers via Hanzo Router)
- Pydantic models for configuration
- Async-first design
- MCP (Model Context Protocol) for tool integration
