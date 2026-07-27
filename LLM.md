# Hanzo AI Agent SDK

## Overview
Python framework for building AI agents and multi-agent systems with orchestration, MCP tools, memory, and observability via Hanzo Cloud.

## Architecture
```
agent/
├── src/agents/            # Core agent SDK (multi-agent, routing, orchestration)
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
- **Package**: `hanzo-agent` on PyPI (v0.0.4) — `hanzoai` is the API client from hanzoai/python-sdk
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
- `src/agents/` — Core agent implementation
- `docs/agents.md` — Agent documentation
- `docs/networks-and-orchestration.md` — Multi-agent docs
- `examples/` — Working examples

## Patterns
- OpenAI Chat Completions API compatible (works with 100+ providers via Hanzo Router)
- Pydantic models for configuration
- Async-first design
- MCP (Model Context Protocol) for tool integration

## How this ships

One way, and it runs on our own stack:

    push  ->  github.com/hanzoai/agent          (a mirror)
              .github/workflows/sync.yml         carries refs onward
      ->  git.hanzo.ai/hanzoai/agent             CANONICAL
              .hanzo/workflows/ci.yml            the checks
              .hanzo/workflows/deploy.yml        builds ghcr.io/hanzoai/agent

**git.hanzo.ai is canonical; GitHub is a mirror.** `.github/workflows/` holds
exactly one file, `sync.yml`, and its only job is getting refs to the forge.
Every build, check and deploy is a workflow under `.hanzo/workflows/`, which the
forge reads. `.hanzo/workflows` uses GitHub Actions syntax, so a workflow moves
between the two by changing directory and nothing else.

`deploy.yml` builds `Dockerfile.sdk` and tags `sdk-*`. It does **not** build
`control-plane`, which is what the workflow it replaces did: there is no
`control-plane/` directory on `main`. The orchestrator was hoisted into the root
Go module (tag `v0.1.1`, "drop control-plane/pkg nesting"), and the directory
survives only on the unmerged `consolidate/held-work` branch. Two things still
point at the old layout and are therefore stale, not load-bearing:
`deployments/docker/Dockerfile.control-plane` (COPYs from `control-plane/`) and
`Makefile.agents` (`cd control-plane && go build`).

A build never deploys itself. Nothing in `hanzoai/universe` references
`ghcr.io/hanzoai/agent`, so this image has **no consumer and no App CR**. That is
a real open question rather than an oversight: either a CR should exist, or the
image should stop being published. It is published today because it was
published yesterday, and because deleting the only producer of a live artifact is
how something goes stale with nothing red to show it.

### What is checked, and what is not

`ci.yml` gates the Go module (`gofmt`, `go build`, `go vet`, `go test` — all
pass) and the Python install the image depends on (`uv sync --frozen --no-dev`,
then `import agents`, matching `Dockerfile.sdk`'s `HEALTHCHECK`).

It deliberately does not run the Python lint/type/test suite, because none of it
passes on `main`:

| Command | State |
|---|---|
| `make sync` | **cannot resolve** — `requires-python` is `>=3.9`, a transitive `numpy` needs `>=3.10` |
| `make lint` (`ruff check`) | 2337 errors |
| `make tests` (`pytest`) | `INTERNALERROR` at collection; a test module `sys.exit(1)`s at import without `ANTHROPIC_API_KEY` |
| `make old_version_tests` | runs 3.9, the version `make sync` cannot resolve |

`tests.yml` ran `make sync` as the first step of all five of its jobs, so none of
them could have gone green. Fixing those four is real work with an owner; a
permanently red check is read as broken CI rather than as broken code, so the
debt is written down here instead.

### Deleted rather than migrated

- `test.yml` — a second copy of `tests.yml` with `|| true` on most steps, so it
  could not fail. Folded into one `ci.yml`.
- `docs.yml` — `make deploy-docs` is `mkdocs gh-deploy`, i.e. GitHub Pages. There
  is no `gh-pages` branch and `hanzoai.github.io/agent` 404s, so it never
  published anything. Pages is not how Hanzo serves sites.
- `publish.yml` — published this repo's dist to PyPI. `pyproject.toml` is
  `hanzo-agent 0.0.4`; when it was named `hanzoai` this workflow pointed at the
  package `hanzoai/python-sdk` owns (now 3.1.1), so a release here could have
  published a downgrade over it. Removed in "fix(pypi): stop publishing this
  repo's dist as hanzoai" and deliberately not restored.
- `issues.yml` — upstream's stale-issue bot.
