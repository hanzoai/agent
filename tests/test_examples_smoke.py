"""Mocked smoke tests for Hanzo Agent SDK examples (Python 3.12+, zero API key)."""

from __future__ import annotations

import ast
import importlib
import json
from pathlib import Path

import pytest

from agents import Agent, Runner, function_tool, handoff

from .fake_model import FakeModel
from .test_responses import (
    get_function_tool_call,
    get_handoff_tool_call,
    get_text_message,
)

EXAMPLES_DIR = Path(__file__).resolve().parent.parent / "examples"


def test_all_python_examples_compile() -> None:
    """Every non-notebook example script must parse cleanly on Python 3.12+."""
    checked = 0
    for py_file in sorted(EXAMPLES_DIR.rglob("*.py")):
        if py_file.name == "hello_world_jupyter.py":
            continue
        source = py_file.read_text(encoding="utf-8")
        ast.parse(source, filename=str(py_file))
        checked += 1
    assert checked >= 25


@pytest.mark.parametrize(
    "module_name",
    [
        "examples.basic.hello_world",
        "examples.basic.stream_text",
        "examples.agent_patterns.agents_as_tools",
        "examples.agent_patterns.deterministic",
        "examples.agent_patterns.routing",
        "examples.handoffs.message_filter",
        "examples.handoffs.message_filter_streaming",
        "examples.customer_service.main",
        "examples.hanzo_backend_example",
        "examples.network_example",
        "examples.autonomous_bug_solver",
        "examples.research_bot.main",
    ],
)
def test_example_modules_import_cleanly(module_name: str) -> None:
    """Core, handoff, network, and backend example modules import without drift."""
    mod = importlib.import_module(module_name)
    assert mod is not None


@pytest.mark.asyncio
async def test_mocked_multi_agent_handoff_and_tool_loop() -> None:
    """Smoke test a mocked handoff + function-tool execution loop."""
    model = FakeModel()

    @function_tool
    def lookup_status(service: str) -> str:
        return f"{service}:healthy"

    specialist = Agent(
        name="OpsSpecialist",
        instructions="Check service status and summarize.",
        tools=[lookup_status],
        model=model,
    )
    triage = Agent(
        name="Triage",
        instructions="Route service questions to OpsSpecialist.",
        handoffs=[handoff(specialist)],
        model=model,
    )

    model.add_multiple_turn_outputs(
        [
            [get_handoff_tool_call(specialist)],
            [get_function_tool_call("lookup_status", json.dumps({"service": "router"}))],
            [get_text_message("router is healthy")],
        ]
    )

    result = await Runner.run(triage, input="Check router health")
    assert result.last_agent == specialist
    assert result.final_output == "router is healthy"
