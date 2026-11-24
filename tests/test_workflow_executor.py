from __future__ import annotations

import asyncio
from types import SimpleNamespace
from unittest.mock import AsyncMock, Mock, call

import pytest

from agents.orchestration import (
    OrchestrationConfig,
    Orchestrator,
    Step,
    UpdateType,
    Workflow,
    executor as executor_module,
)


@pytest.fixture
def backoff(monkeypatch):
    sleep = AsyncMock()
    monkeypatch.setattr(executor_module.asyncio, "sleep", sleep)
    return sleep


def register(workflow, *, retry_failed=True, max_retries=1):
    orchestrator = Orchestrator(
        OrchestrationConfig(retry_failed_steps=retry_failed, max_retries=max_retries)
    )
    orchestrator.register_workflow(workflow)
    return orchestrator


def events(orchestrator, event_type, step_id=None):
    return [
        update
        for update in orchestrator.ui_streamer.get_buffer()
        if update.type == event_type and (step_id is None or update.data.get("step_id") == step_id)
    ]


async def test_workflow_runs_step_and_reports_its_output():
    workflow = Workflow("transform")
    transform = Mock(return_value="transformed")
    step = workflow.add_transform_step("transform", transform)
    orchestrator = register(workflow)

    result = await orchestrator.execute_workflow(workflow.id, "input")

    transform.assert_called_once_with("input")
    assert result.success
    assert result.error is None
    assert result.output == "transformed"
    assert result.steps_completed == 1
    assert result.step_results[step.id].output == result.output
    assert [update.type for update in orchestrator.ui_streamer.get_buffer()] == [
        UpdateType.WORKFLOW_START,
        UpdateType.STEP_START,
        UpdateType.STEP_COMPLETE,
        UpdateType.WORKFLOW_COMPLETE,
    ]
    assert events(orchestrator, UpdateType.STEP_COMPLETE, step.id)[0].data["success"]


async def test_successful_retry_keeps_final_result_and_emits_one_completion(backoff):
    workflow = Workflow("retry")
    transform = Mock(side_effect=[ValueError("try again"), "recovered"])
    step = workflow.add_transform_step("transform", transform)
    orchestrator = register(workflow)

    result = await orchestrator.execute_workflow(workflow.id, "input")

    assert transform.call_args_list == [call("input"), call("input")]
    backoff.assert_awaited_once_with(2)
    assert result.success
    assert result.output == "recovered"
    assert result.step_results[step.id].retries == 1
    assert result.step_results[step.id].error is None
    assert len(events(orchestrator, UpdateType.STEP_START, step.id)) == 1
    [completion] = events(orchestrator, UpdateType.STEP_COMPLETE, step.id)
    assert completion.data["success"]
    assert completion.data["error"] is None


@pytest.mark.parametrize(
    ("retry_failed", "max_retries", "delays"),
    [(True, 1, [2]), (True, 2, [2, 4]), (True, 0, []), (False, 3, [])],
)
async def test_failed_step_exhausts_only_its_retry_budget(
    retry_failed, max_retries, delays, backoff
):
    workflow = Workflow("permanent failure")
    transform = Mock(side_effect=ValueError("cannot transform"))
    step = workflow.add_transform_step("transform", transform)
    orchestrator = register(workflow, retry_failed=retry_failed, max_retries=max_retries)

    result = await orchestrator.execute_workflow(workflow.id, "input")

    assert transform.call_count == 1 + len(delays)
    assert backoff.await_args_list == [call(delay) for delay in delays]
    assert not result.success
    assert "cannot transform" in result.error
    assert result.steps_completed == 0
    assert not result.step_results[step.id].success
    assert result.step_results[step.id].retries == len(delays)
    [step_event] = events(orchestrator, UpdateType.STEP_COMPLETE, step.id)
    [workflow_event] = events(orchestrator, UpdateType.WORKFLOW_COMPLETE)
    assert not step_event.data["success"]
    assert not workflow_event.data["success"]
    assert workflow_event.data["error"] == result.error


async def test_failed_dependency_blocks_only_its_dependents(monkeypatch):
    workflow = Workflow("independent branches")
    failed = workflow.add_step(Step.agent("fail", "fail"))
    independent = workflow.add_step(Step.agent("independent", "keep running"))
    blocked_transform = Mock()
    blocked = workflow.add_transform_step("blocked", blocked_transform)
    blocked.depends_on = [failed.id, independent.id]
    downstream_transform = Mock(return_value="followed")
    downstream = workflow.add_transform_step("follow independent", downstream_transform)
    downstream.depends_on = [independent.id]
    orchestrator = register(workflow, retry_failed=False)
    sibling_started = asyncio.Event()
    failure_raised = asyncio.Event()

    async def run_agent(*, starting_agent, **kwargs):
        if starting_agent == "fail":
            await sibling_started.wait()
            failure_raised.set()
            raise ValueError("first branch failed")
        sibling_started.set()
        await failure_raised.wait()
        return SimpleNamespace(final_output="sibling output")

    network_run = AsyncMock(side_effect=run_agent)
    monkeypatch.setattr(orchestrator.network, "run", network_run)

    result = await asyncio.wait_for(orchestrator.execute_workflow(workflow.id, "input"), timeout=5)

    assert network_run.await_count == 2
    assert not result.success
    assert result.steps_completed == 2
    assert result.step_results[independent.id].output == "sibling output"
    assert result.step_results[downstream.id].output == "followed"
    assert blocked.id not in result.step_results
    blocked_transform.assert_not_called()
    downstream_transform.assert_called_once_with("sibling output")


@pytest.mark.parametrize("fails", [False, True])
@pytest.mark.parametrize("nested", [False, True])
async def test_error_handler_runs_once_after_retries_are_exhausted(fails, nested, backoff):
    workflow = Workflow("error handler")
    attempts = []

    def run_step(value):
        attempts.append("step")
        if fails:
            raise ValueError("original failure")
        return value

    def handle_error(value):
        attempts.append("handler")
        return "handled"

    step = Step.transform(run_step)
    handler = Step.transform(handle_error)
    step.on_error = handler.id
    if nested:
        workflow.add_parallel_step("group", [step, handler])
    else:
        workflow.add_step(step)
        workflow.add_step(handler)
    orchestrator = register(workflow)

    result = await orchestrator.execute_workflow(workflow.id, "input")

    if fails:
        assert attempts == ["step", "step", "handler"]
        assert not result.success
        assert result.step_results[step.id].error == "original failure"
        assert result.step_results[handler.id].output == "handled"
    else:
        assert attempts == ["step"]
        assert result.success
        assert handler.id not in result.step_results


@pytest.mark.parametrize("inline_children", [False, True])
async def test_parallel_child_failure_does_not_replay_successful_sibling(inline_children, backoff):
    workflow = Workflow("parallel")
    failing = Mock(side_effect=ValueError("child failed"))
    succeeds = Mock(return_value="saved")
    failed_step = Step.transform(failing)
    successful_step = Step.transform(succeeds)
    if inline_children:
        group = workflow.add_step(Step.parallel([failed_step, successful_step]))
    else:
        group = workflow.add_parallel_step("group", [failed_step, successful_step])
    orchestrator = register(workflow)

    result = await orchestrator.execute_workflow(workflow.id, "input")

    assert failing.call_count == 2
    succeeds.assert_called_once_with("input")
    backoff.assert_awaited_once_with(2)
    assert not result.success
    assert not result.step_results[group.id].success
    assert "child failed" in result.step_results[group.id].error
    assert result.step_results[successful_step.id].output == "saved"
    assert len(events(orchestrator, UpdateType.STEP_COMPLETE, successful_step.id)) == 1


async def test_nested_dependencies_preserve_order_and_pass_child_output():
    workflow = Workflow("nested dependencies")
    first_transform = Mock(return_value="first child output")
    second_transform = Mock(return_value="second child output")
    first = Step.transform(first_transform)
    second = Step.transform(second_transform)
    group = workflow.add_parallel_step("group", [second, first])
    consume = Mock(return_value="workflow output")
    consumer = workflow.add_transform_step("consume child output", consume)
    prepare = workflow.add_transform_step("prepare", lambda _: "prepared")
    first.depends_on = [prepare.id]
    second.depends_on = [first.id]
    consumer.depends_on = [second.id]
    workflow.entry_point = consumer.id
    orchestrator = register(workflow)

    result = await orchestrator.execute_workflow(workflow.id, "input")

    assert result.success
    assert result.output == "workflow output"
    assert result.steps_completed == result.total_steps == 5
    first_transform.assert_called_once_with("prepared")
    second_transform.assert_called_once_with("first child output")
    consume.assert_called_once_with("second child output")
    assert result.step_results[group.id].output == ["second child output", "first child output"]


@pytest.mark.parametrize("condition", [True, False])
async def test_only_the_selected_conditional_branch_executes(condition):
    workflow = Workflow("conditional")
    true_transform = Mock(return_value="true")
    false_transform = Mock(return_value="false")
    true_step = workflow.add_transform_step("true", true_transform)
    false_step = workflow.add_transform_step("false", false_transform)
    branch = workflow.add_conditional_step("branch", bool, true_step.id, false_step.id)
    workflow.entry_point = branch.id
    orchestrator = register(workflow)

    result = await orchestrator.execute_workflow(workflow.id, condition)

    assert result.success
    assert result.output == ("true" if condition else "false")
    selected, skipped = (
        (true_transform, false_transform) if condition else (false_transform, true_transform)
    )
    selected.assert_called_once_with(condition)
    skipped.assert_not_called()


async def test_loop_failure_does_not_replay_prior_iterations(backoff):
    workflow = Workflow("loop")

    def process(item):
        if item == "bad":
            raise ValueError("bad item")
        return item

    transform = Mock(side_effect=process)
    body = workflow.add_transform_step("process", transform)
    loop = workflow.add_loop_step("loop", lambda items: items, body.id)
    workflow.entry_point = loop.id
    orchestrator = register(workflow)

    result = await orchestrator.execute_workflow(workflow.id, [None, "ok", "bad", "later"])

    assert transform.call_args_list == [call(None), call("ok"), call("bad"), call("bad")]
    backoff.assert_awaited_once_with(2)
    assert not result.success
    assert not result.step_results[loop.id].success
    assert "bad item" in result.step_results[body.id].error


async def test_cancellation_during_retry_reaches_caller_without_retrying(backoff):
    workflow = Workflow("cancel")
    transform = Mock(side_effect=ValueError("backoff before retry"))
    step = workflow.add_transform_step("transform", transform)
    orchestrator = register(workflow)
    backing_off = asyncio.Event()

    async def wait_in_backoff(delay):
        backing_off.set()
        await asyncio.Event().wait()

    backoff.side_effect = wait_in_backoff
    task = asyncio.create_task(orchestrator.execute_workflow(workflow.id, "input"))
    try:
        await asyncio.wait_for(backing_off.wait(), timeout=2)
        task.cancel()
        with pytest.raises(asyncio.CancelledError):
            await task
    finally:
        if not task.done():
            task.cancel()
            await asyncio.gather(task, return_exceptions=True)

    transform.assert_called_once_with("input")
    [step_event] = events(orchestrator, UpdateType.STEP_COMPLETE, step.id)
    [workflow_event] = events(orchestrator, UpdateType.WORKFLOW_COMPLETE)
    assert not step_event.data["success"]
    assert not workflow_event.data["success"]
    assert "cancel" in workflow_event.data["error"].lower()


async def test_child_cancellation_propagates_from_batch(backoff):
    workflow = Workflow("cancelled child")
    transform = Mock(side_effect=asyncio.CancelledError)
    workflow.add_transform_step("transform", transform)
    orchestrator = register(workflow)

    with pytest.raises(asyncio.CancelledError):
        await orchestrator.execute_workflow(workflow.id, "input")

    transform.assert_called_once_with("input")
    backoff.assert_not_awaited()
    [workflow_event] = events(orchestrator, UpdateType.WORKFLOW_COMPLETE)
    assert not workflow_event.data["success"]
