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


def test_autonomous_bug_solver_wires_hanzo_provider_and_workflow_steps() -> None:
    """Bug solver attaches HanzoModelProvider models and wires the review refinement gate."""
    from agents import OpenAIChatCompletionsModel
    from agents.orchestration.workflow import StepType
    from examples import autonomous_bug_solver as abs_mod

    for node in abs_mod.bug_solver_network.nodes.values():
        assert isinstance(node.agent.model, OpenAIChatCompletionsModel)
        assert node.agent.model._client is abs_mod.model_provider.client

    primary_steps = [
        abs_mod.analyze_step,
        abs_mod.design_step,
        abs_mod.fix_step,
        abs_mod.test_step,
        abs_mod.review_step,
    ]
    assert [step.config["agent_name"] for step in primary_steps] == [
        "BugAnalyzer",
        "SolutionDesigner",
        "CodeFixer",
        "TestWriter",
        "QualityReviewer",
    ]
    for prev, curr in zip(primary_steps, primary_steps[1:]):
        assert curr.depends_on == [prev.id]

    assert abs_mod.refinement_gate.type == StepType.CONDITIONAL
    assert abs_mod.refinement_gate.depends_on == [abs_mod.review_step.id]
    assert abs_mod.refinement_gate.config["if_true"] == abs_mod.refinement_pipeline.id
    assert abs_mod.refinement_gate.config["if_false"] is None
    assert abs_mod.refinement_pipeline.type == StepType.PARALLEL
    assert abs_mod.refinement_pipeline.config["step_ids"] == [
        abs_mod.refine_fix_step.id,
        abs_mod.refine_test_step.id,
        abs_mod.re_review_step.id,
    ]
    assert abs_mod.refine_fix_step.depends_on == [abs_mod.review_step.id]
    assert abs_mod.refine_test_step.depends_on == [abs_mod.refine_fix_step.id]
    assert abs_mod.re_review_step.depends_on == [abs_mod.refine_test_step.id]


@pytest.mark.asyncio
async def test_fix_and_test_serializes_json_and_runs_tests(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """fix_and_test safely serializes quotes/newlines and awaits run_tests."""
    from types import SimpleNamespace
    from examples import autonomous_bug_solver as abs_mod

    target = tmp_path / "sample.py"
    sample_code = 'def greet():\n    print("x")\n'

    def fake_run(cmd, capture_output=True, text=True, timeout=30):
        return SimpleNamespace(returncode=0, stdout=f"ran {' '.join(cmd)}", stderr="")

    import subprocess

    monkeypatch.setattr(subprocess, "run", fake_run)
    raw = await abs_mod.fix_and_test.on_invoke_tool(
        None,
        json.dumps(
            {
                "file_path": str(target),
                "content": sample_code,
                "test_command": "pytest -q",
            }
        ),
    )
    payload = ast.literal_eval(raw) if isinstance(raw, str) else raw
    assert target.read_text(encoding="utf-8") == sample_code
    assert "successfully" in payload["write_result"]
    test_res = payload["test_result"]
    if isinstance(test_res, str):
        test_res = ast.literal_eval(test_res)
    assert test_res["success"] is True
    assert test_res["stdout"] == "ran pytest -q"


@pytest.mark.asyncio
async def test_solve_bug_runs_workflow_with_mocked_network(monkeypatch: pytest.MonkeyPatch) -> None:
    """solve_bug executes the five primary workflow steps when QualityReviewer approves."""
    from unittest.mock import AsyncMock
    from types import SimpleNamespace
    from examples import autonomous_bug_solver as abs_mod

    called_agents: list[str] = []

    async def fake_network_run(input, *, starting_agent=None, context=None, **kwargs):
        called_agents.append(starting_agent)
        if starting_agent == "QualityReviewer":
            return SimpleNamespace(final_output="APPROVED: all checks pass")
        return SimpleNamespace(final_output=f"{starting_agent} done")

    monkeypatch.setattr(abs_mod.bug_solver_network, "run", AsyncMock(side_effect=fake_network_run))

    result = await abs_mod.solve_bug("ValueError: boom")
    assert result.success is True
    assert called_agents == [
        "BugAnalyzer",
        "SolutionDesigner",
        "CodeFixer",
        "TestWriter",
        "QualityReviewer",
    ]


@pytest.mark.asyncio
async def test_solve_bug_refines_when_quality_reviewer_requests_changes(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """solve_bug runs refinement steps when changes are requested and fails if still rejected."""
    from unittest.mock import AsyncMock
    from types import SimpleNamespace
    from examples import autonomous_bug_solver as abs_mod

    called_agents: list[str] = []
    review_calls = 0

    async def fake_run_then_approve(input, *, starting_agent=None, context=None, **kwargs):
        nonlocal review_calls
        called_agents.append(starting_agent)
        if starting_agent == "QualityReviewer":
            review_calls += 1
            if review_calls == 1:
                return SimpleNamespace(final_output="CHANGES_REQUESTED: handle empty list")
            return SimpleNamespace(final_output="APPROVED: fixed")
        return SimpleNamespace(final_output=f"{starting_agent} done")

    monkeypatch.setattr(abs_mod.bug_solver_network, "run", AsyncMock(side_effect=fake_run_then_approve))
    approved_result = await abs_mod.solve_bug("ValueError: boom")
    assert approved_result.success is True
    assert called_agents == [
        "BugAnalyzer",
        "SolutionDesigner",
        "CodeFixer",
        "TestWriter",
        "QualityReviewer",
        "CodeFixer",
        "TestWriter",
        "QualityReviewer",
    ]

    called_agents.clear()

    async def fake_run_always_reject(input, *, starting_agent=None, context=None, **kwargs):
        called_agents.append(starting_agent)
        if starting_agent == "QualityReviewer":
            return SimpleNamespace(final_output="CHANGES_REQUESTED: still broken")
        return SimpleNamespace(final_output=f"{starting_agent} done")

    monkeypatch.setattr(abs_mod.bug_solver_network, "run", AsyncMock(side_effect=fake_run_always_reject))
    rejected_result = await abs_mod.solve_bug("ValueError: boom")
    assert rejected_result.success is False
    assert "QualityReviewer" in (rejected_result.error or "")
    assert len(called_agents) == 8

