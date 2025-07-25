# Hanzo AI Agent SDK

A powerful Python framework for building AI agents and multi-agent systems with built-in orchestration, inspired by [AgentKit](https://github.com/inngest/agent-kit).

<img src="https://cdn.openai.com/API/docs/images/orchestration.png" alt="Image of the Agents Tracing UI" style="max-height: 803px;">

## ✨ Features

- 🤖 **Multi-Agent Networks**: Build systems where multiple specialized agents collaborate
- 🧠 **Intelligent Routing**: Semantic, rule-based, and load-balanced routing strategies  
- 🛠️ **Powerful Tools**: Enhanced tool system with MCP (Model Context Protocol) support
- 📊 **Shared State**: Agents can share information through network state
- 🔄 **Orchestration**: Define complex workflows with parallel, conditional, and loop steps
- 💾 **Memory System**: Long-term memory with vector search and reflection capabilities
- ⚡ **UI Streaming**: Real-time updates for building responsive interfaces
- 🔍 **Observability**: Built-in tracing and monitoring via Hanzo Cloud dashboard
- 🌐 **Backend Flexibility**: Use with Hanzo Router for 100+ LLM providers

### Core concepts:

1. [**Agents**](docs/agents.md): LLMs configured with instructions, tools, and memory
2. [**Networks**](docs/networks-and-orchestration.md): Multi-agent systems with intelligent routing
3. [**Workflows**](docs/networks-and-orchestration.md#orchestration-and-workflows): Orchestrate complex multi-step processes
4. [**State & Memory**](docs/networks-and-orchestration.md#state-management): Shared state and long-term memory
5. [**Tools**](docs/tools.md): Enhanced tool system with MCP support
6. [**Tracing**](docs/tracing.md): Built-in tracking and observability

Explore the [examples](examples) directory to see the SDK in action, and read our [documentation](https://openai.github.io/openai-agents-python/) for more details.

Notably, our SDK [is compatible](https://openai.github.io/openai-agents-python/models/) with any model providers that support the Open AI Chat Completions API format.

## Get started

1. Set up your Python environment

```
python -m venv env
source env/bin/activate
```

2. Install Hanzo AI SDK

```
pip install hanzoai
```

## Quick Examples

### Simple Agent

```python
from hanzoai import Agent, Runner

agent = Agent(name="Assistant", instructions="You are a helpful assistant")

result = Runner.run_sync(agent, "Write a haiku about recursion in programming.")
print(result.final_output)

# Code within the code,
# Functions calling themselves,
# Infinite loop's dance.
```

### Multi-Agent Network

```python
from agents import Agent, create_network
from agents.routers import SemanticRouter

# Create specialized agents
researcher = Agent(
    name="Researcher",
    instructions="You find and analyze information.",
    tools=[search_tool, analyze_tool]
)

writer = Agent(
    name="Writer",
    instructions="You create content based on research.",
    tools=[format_tool]
)

# Create a network
network = create_network(
    agents=[researcher, writer],
    router=SemanticRouter(),
    default_model="gpt-4"
)

# Run the network
result = await network.run("Research and write about quantum computing")
```

### Orchestrated Workflow

```python
from agents import create_workflow, Step

workflow = create_workflow(
    name="Content Pipeline",
    steps=[
        Step.agent("researcher", "Research {topic}"),
        Step.parallel([
            Step.agent("writer", "Write introduction"),
            Step.agent("writer", "Write main content")
        ]),
        Step.agent("reviewer", "Review and edit"),
        Step.conditional(
            condition=lambda state: state.get("quality_score") < 8,
            true_step=Step.agent("writer", "Revise based on feedback"),
            false_step=Step.transform(lambda x: {"status": "published"})
        )
    ]
)

result = await workflow.run({"topic": "AI Safety"})
```

(_Configure backend with `HANZO_ROUTER_URL` and `HANZO_API_KEY` environment variables_)

## Handoffs example

```python
from hanzoai import Agent, Runner
import asyncio

spanish_agent = Agent(
    name="Spanish agent",
    instructions="You only speak Spanish.",
)

english_agent = Agent(
    name="English agent",
    instructions="You only speak English",
)

triage_agent = Agent(
    name="Triage agent",
    instructions="Handoff to the appropriate agent based on the language of the request.",
    handoffs=[spanish_agent, english_agent],
)


async def main():
    result = await Runner.run(triage_agent, input="Hola, ¿cómo estás?")
    print(result.final_output)
    # ¡Hola! Estoy bien, gracias por preguntar. ¿Y tú, cómo estás?


if __name__ == "__main__":
    asyncio.run(main())
```

## Functions example

```python
import asyncio

from hanzoai import Agent, Runner, function_tool


@function_tool
def get_weather(city: str) -> str:
    return f"The weather in {city} is sunny."


agent = Agent(
    name="Hello world",
    instructions="You are a helpful agent.",
    tools=[get_weather],
)


async def main():
    result = await Runner.run(agent, input="What's the weather in Tokyo?")
    print(result.final_output)
    # The weather in Tokyo is sunny.


if __name__ == "__main__":
    asyncio.run(main())
```

## The agent loop

When you call `Runner.run()`, we run a loop until we get a final output.

1. We call the LLM, using the model and settings on the agent, and the message history.
2. The LLM returns a response, which may include tool calls.
3. If the response has a final output (see below for more on this), we return it and end the loop.
4. If the response has a handoff, we set the agent to the new agent and go back to step 1.
5. We process the tool calls (if any) and append the tool responses messages. Then we go to step 1.

There is a `max_turns` parameter that you can use to limit the number of times the loop executes.

### Final output

Final output is the last thing the agent produces in the loop.

1.  If you set an `output_type` on the agent, the final output is when the LLM returns something of that type. We use [structured outputs](https://platform.openai.com/docs/guides/structured-outputs) for this.
2.  If there's no `output_type` (i.e. plain text responses), then the first LLM response without any tool calls or handoffs is considered as the final output.

As a result, the mental model for the agent loop is:

1. If the current agent has an `output_type`, the loop runs until the agent produces structured output matching that type.
2. If the current agent does not have an `output_type`, the loop runs until the current agent produces a message without any tool calls/handoffs.

## Common agent patterns

The Agent SDK is designed to be highly flexible, allowing you to model a wide range of LLM workflows including deterministic flows, iterative loops, and more. See examples in [`examples/agent_patterns`](examples/agent_patterns).

## Tracing

The Agent SDK automatically traces your agent runs, making it easy to track and debug the behavior of your agents. Tracing is extensible by design, supporting custom spans and a wide variety of external destinations, including [Logfire](https://logfire.pydantic.dev/docs/integrations/llms/openai/#openai-agents), [AgentOps](https://docs.agentops.ai/v1/integrations/agentssdk), [Braintrust](https://braintrust.dev/docs/guides/traces/integrations#openai-agents-sdk), [Scorecard](https://docs.scorecard.io/docs/documentation/features/tracing#openai-agents-sdk-integration), and [Keywords AI](https://docs.keywordsai.co/integration/development-frameworks/openai-agent). For more details about how to customize or disable tracing, see [Tracing](http://openai.github.io/openai-agents-python/tracing).

## Development (only needed if you need to edit the SDK/examples)

0. Ensure you have [`uv`](https://docs.astral.sh/uv/) installed.

```bash
uv --version
```

1. Install dependencies

```bash
make sync
```

2. (After making changes) lint/test

```
make tests  # run tests
make mypy   # run typechecker
make lint   # run linter
```

## Acknowledgements

We'd like to acknowledge the excellent work of the open-source community, especially:

-   [Pydantic](https://docs.pydantic.dev/latest/) (data validation) and [PydanticAI](https://ai.pydantic.dev/) (advanced agent framework)
-   [MkDocs](https://github.com/squidfunk/mkdocs-material)
-   [Griffe](https://github.com/mkdocstrings/griffe)
-   [uv](https://github.com/astral-sh/uv) and [ruff](https://github.com/astral-sh/ruff)

We're committed to continuing to build the Agent SDK as an open source framework so others in the community can expand on our approach.
