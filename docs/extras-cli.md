# CLI extra

```bash
pip install "hanzo-agent[cli]"
```

The extra installs `click` (`>=8.1,<9`) and `rich` (`>=13,<14`). The package declares no console script, so installing it adds no command. Nothing in `src/agents` imports `click`; `rich` is a dependency for your own output.

## What the module is for

`agents.extensions.cli` is meant to let an agent hand its turn to an external command-line assistant. `CLIAgent` builds a prompt from the state and the recent history, runs the command as a subprocess, and reads the answer from its output. If the arguments include `-` or `--stdin`, the prompt goes to the command's stdin. Otherwise the prompt is written to a temporary file whose path is appended as the last argument. A non-zero exit code becomes an `Error: ...` answer. Lines that start with `TOOL:` or `@tool` and carry a JSON object are read as tool calls.

The module defines these presets:

| Class | Command | Arguments |
|---|---|---|
| `ClaudeCodeAgent` | `claude` | `--no-interactive` |
| `OpenAICodexAgent` | `openai` | `chat --model gpt-4` |
| `GeminiAgent` | `gemini` | `--format json` |
| `GrokAgent` | `grok` | `--mode code` |
| `CursorAgent` | `cursor` | `--headless` |
| `AiderAgent` | `aider` | `--no-pretty --yes` |

## Status

`import agents.extensions.cli` fails today with `ModuleNotFoundError: No module named 'hanzo_agents'`. The module is built on `hanzo_agents.core` (`Agent`, `State`, `History`, `InferenceResult`, `ToolCall`), a package this distribution does not contain; the current `Agent` has a different interface. It needs a port to the present SDK before it can be used, so treat the table above as the design, not as an API you can call.

## Calling an external CLI today

A function tool does the same job with the current SDK. The model decides when to call it, and the command receives the prompt on stdin. The command is fixed in code, so the model can supply only the prompt:

```python
import asyncio

from agents import Agent, function_tool

COMMAND = ["cat"]  # replace with your assistant's command line


@function_tool
async def ask_cli(prompt: str) -> str:
    """Ask the local command-line assistant and return its answer."""
    proc = await asyncio.create_subprocess_exec(
        *COMMAND,
        stdin=asyncio.subprocess.PIPE,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
    )
    out, err = await proc.communicate(prompt.encode())
    if proc.returncode != 0:
        return f"Error: {err.decode()}"
    return out.decode()


agent = Agent(
    name="cli-bridge",
    instructions="Use ask_cli to ask the local assistant.",
    tools=[ask_cli],
)
```
