"""Workflow execution engine."""

from __future__ import annotations

import asyncio
import time
import uuid
from collections import deque
from copy import copy
from dataclasses import dataclass, field, replace
from typing import Any, Dict, List

from ..exceptions import AgentsException
from ..logger import logger
from ..network.network import AgentNetwork
from ..run_context import TContext
from ..state.store import StateStore
from ..tracing import custom_span
from .ui_stream import StreamUpdate, UIStreamer, UpdateType
from .workflow import StepType, Workflow, WorkflowStep


@dataclass
class StepResult:
    """Result of a workflow step execution."""
    
    step_id: str
    success: bool
    output: Any = None
    error: str | None = None
    start_time: float = field(default_factory=time.time)
    end_time: float | None = None
    retries: int = 0
    
    @property
    def duration(self) -> float:
        """Get execution duration."""
        if self.end_time:
            return self.end_time - self.start_time
        return 0.0


@dataclass
class ExecutionResult:
    """Result of a workflow execution."""
    
    workflow_id: str
    execution_id: str
    success: bool
    output: Any = None
    error: str | None = None
    steps_completed: int = 0
    total_steps: int = 0
    step_results: Dict[str, StepResult] = field(default_factory=dict)
    start_time: float = field(default_factory=time.time)
    end_time: float | None = None
    metadata: Dict[str, Any] = field(default_factory=dict)
    
    @property
    def duration(self) -> float:
        """Get execution duration."""
        if self.end_time:
            return self.end_time - self.start_time
        return 0.0


class _NestedStepFailed(AgentsException):
    """A child exhausted its own retry budget or could not run."""


class WorkflowExecutor:
    """Executes workflows with proper orchestration."""
    
    def __init__(
        self,
        workflow: Workflow,
        network: AgentNetwork,
        state_store: StateStore | None = None,
        retry_failed: bool = True,
        max_retries: int = 3,
    ):
        """Initialize executor.
        
        Args:
            workflow: Workflow to execute
            network: Agent network
            state_store: State store for persistence
            retry_failed: Whether to retry failed steps
            max_retries: Maximum retries per step
        """
        self.workflow = workflow
        self.network = network
        self.state_store = state_store
        self.retry_failed = retry_failed
        self.max_retries = max_retries
        self.execution_id = str(uuid.uuid4())
        self.ui_streamer: UIStreamer | None = None
        self._result: ExecutionResult | None = None
        
    def set_ui_streamer(self, streamer: UIStreamer) -> None:
        """Set UI streamer for updates."""
        self.ui_streamer = streamer
        
    async def execute(
        self,
        input: Any,
        context: TContext | None = None,
    ) -> ExecutionResult:
        """Execute the workflow.
        
        Args:
            input: Initial input data
            context: Execution context
            
        Returns:
            Execution result
        """
        workflow = self._expand_parallel_steps()
        errors = workflow.validate()
        result = ExecutionResult(
            workflow_id=workflow.id,
            execution_id=self.execution_id,
            success=True,
            total_steps=len(workflow.steps),
        )
        self._result = result
        if errors:
            result.success = False
            result.error = f"Workflow validation failed: {'; '.join(errors)}"
            return result

        await self._send_update(
            UpdateType.WORKFLOW_START,
            {
                "workflow_id": workflow.id,
                "workflow_name": workflow.name,
                "total_steps": result.total_steps,
            },
        )

        try:
            exec_context = ExecutionContext(
                input=input,
                context=context,
                workflow=workflow,
                result=result,
                descendants=self._step_descendants(workflow),
            )
            outcomes = await self._execute_batches(list(workflow.steps), exec_context)
            for outcome in outcomes.values():
                if outcome is not None and not outcome.success:
                    result.success = False
                    result.error = f"Step '{outcome.step_id}' failed: {outcome.error}"
                    break

            entry = result.step_results.get(workflow.entry_point) if workflow.entry_point else None
            if entry is not None and entry.success:
                result.output = entry.output

        except asyncio.CancelledError:
            result.success = False
            result.error = "Workflow cancelled"
            raise
        except Exception as e:
            logger.error(f"Workflow execution failed: {e}")
            result.success = False
            result.error = str(e)
        finally:
            result.end_time = time.time()
            result.steps_completed = sum(step.success for step in result.step_results.values())
            await self._send_update(
                UpdateType.WORKFLOW_COMPLETE,
                {
                    "success": result.success,
                    "duration": result.duration,
                    "steps_completed": result.steps_completed,
                    "error": result.error,
                },
            )

        return result

    def _expand_parallel_steps(self) -> Workflow:
        """Resolve children from both parallel builders in a per-run workflow copy."""
        workflow = copy(self.workflow)
        workflow.steps = self.workflow.steps.copy()
        pending = deque(workflow.steps.values())
        while pending:
            step = pending.popleft()
            if (
                step.type != StepType.PARALLEL
                or "step_ids" in step.config
                or "steps" not in step.config
            ):
                continue

            children = step.config["steps"]
            for child in children:
                if child.id not in workflow.steps:
                    workflow.steps[child.id] = child
                    pending.append(child)
            config = {**step.config, "step_ids": [child.id for child in children]}
            workflow.steps[step.id] = replace(step, config=config)
        return workflow

    @staticmethod
    def _step_descendants(workflow: Workflow) -> dict[str, set[str]]:
        """Collect inline children and error handlers under their owners."""
        descendants: dict[str, set[str]] = {}
        visiting: set[str] = set()

        def collect(step_id: str) -> set[str]:
            if step_id in visiting:
                raise AgentsException(f"Circular nested step reference at '{step_id}'")
            if step_id in descendants:
                return descendants[step_id]

            visiting.add(step_id)
            step = workflow.steps[step_id]
            children = [step.on_error] if step.on_error else []
            if step.type == StepType.PARALLEL:
                children.extend(step.config["step_ids"])
            elif step.type == StepType.CONDITIONAL:
                children.append(step.config["if_true"])
                if step.config.get("if_false"):
                    children.append(step.config["if_false"])
            elif step.type == StepType.LOOP:
                children.append(step.config["body"])

            nested = set()
            for child_id in children:
                if child_id in workflow.steps:
                    nested.add(child_id)
                    nested.update(collect(child_id))
            visiting.remove(step_id)
            descendants[step_id] = nested
            return nested

        for step_id in workflow.steps:
            collect(step_id)
        return descendants

    @staticmethod
    def _execution_order(step_ids: list[str], context: ExecutionContext) -> list[list[str]]:
        """Reuse the workflow sorter after mapping nested dependencies to their owners."""
        nested = set().union(*(context.descendants[step_id] for step_id in step_ids))
        groups = {
            step_id: {step_id} | context.descendants[step_id]
            for step_id in step_ids
            if step_id not in nested
        }
        owners: dict[str, set[str]] = {}
        for owner, group in groups.items():
            for step_id in group:
                owners.setdefault(step_id, set()).add(owner)

        scoped_workflow = copy(context.workflow)
        scoped_workflow.steps = {}
        for owner, group in groups.items():
            dependencies: set[str] = set()
            for step_id in group:
                for dependency in context.workflow.steps[step_id].depends_on:
                    dependencies.update(owners.get(dependency, ()))
            dependencies.discard(owner)
            scoped_workflow.steps[owner] = replace(
                context.workflow.steps[owner], depends_on=sorted(dependencies)
            )
        return scoped_workflow.get_execution_order()

    async def _execute_batches(
        self, step_ids: list[str], context: ExecutionContext
    ) -> dict[str, StepResult | None]:
        outcomes: dict[str, StepResult | None] = {}
        for batch in self._execution_order(step_ids, context):
            outcomes.update(dict.fromkeys(batch))
            steps = [context.workflow.steps[step_id] for step_id in batch]
            tasks = [
                self._execute_step(step, context)
                for step in steps
                if context.dependencies_succeeded(step)
            ]
            for outcome in await asyncio.gather(*tasks, return_exceptions=True):
                if isinstance(outcome, BaseException):
                    raise outcome
                outcomes[outcome.step_id] = outcome
        return outcomes

    async def _execute_step(
        self,
        step: WorkflowStep,
        context: ExecutionContext,
    ) -> StepResult:
        """Execute a single step."""
        step_result = StepResult(step_id=step.id, success=False)
        await self._send_update(
            UpdateType.STEP_START,
            {
                "step_id": step.id,
                "step_name": step.name,
                "step_type": step.type.value,
            },
        )

        try:
            while True:
                try:
                    step_result.output = await self._execute_step_attempt(step, context)
                except Exception as e:
                    logger.error(f"Step {step.id} failed: {e}")
                    if (
                        isinstance(e, _NestedStepFailed)
                        or not self.retry_failed
                        or step_result.retries >= self.max_retries
                    ):
                        step_result.error = str(e)
                        break
                    retry = step_result.retries + 1
                    logger.info(f"Retrying step {step.id} (retry {retry})")
                    await asyncio.sleep(2 ** retry)
                    step_result.retries = retry
                else:
                    step_result.success = True
                    break

            if not step_result.success and step.on_error is not None:
                handler = context.workflow.steps.get(step.on_error)
                if handler is not None:
                    await self._execute_step(handler, context)
        except asyncio.CancelledError:
            step_result.error = step_result.error or "Step cancelled"
            raise
        finally:
            step_result.end_time = time.time()
            context.result.step_results[step.id] = step_result
            await self._send_update(
                UpdateType.STEP_COMPLETE,
                {
                    "step_id": step.id,
                    "success": step_result.success,
                    "duration": step_result.duration,
                    "error": step_result.error,
                },
            )
        return step_result

    async def _execute_step_attempt(self, step: WorkflowStep, context: ExecutionContext) -> Any:
        if step.type == StepType.AGENT:
            return await self._execute_agent_step(step, context)
        if step.type == StepType.PARALLEL:
            return await self._execute_parallel_step(step, context)
        if step.type == StepType.CONDITIONAL:
            return await self._execute_conditional_step(step, context)
        if step.type == StepType.LOOP:
            return await self._execute_loop_step(step, context)
        if step.type == StepType.TRANSFORM:
            return await self._execute_transform_step(step, context)
        if step.type == StepType.WAIT:
            await self._execute_wait_step(step, context)
            return None
        raise AgentsException(f"Unknown step type: {step.type}")

    async def _execute_agent_step(
        self,
        step: WorkflowStep,
        context: ExecutionContext,
    ) -> Any:
        """Execute an agent step."""
        config = step.config
        agent_name = config["agent_name"]
        
        # Get input
        step_input = context.get_step_input(step)
        
        # Apply input transform if provided
        if transform := config.get("input_transform"):
            step_input = transform(step_input)
            
        # Run agent
        with custom_span("workflow_agent_step", {"agent": agent_name}):
            result = await self.network.run(
                input=step_input,
                starting_agent=agent_name,
                context=context.context,
            )
            
        # Extract output
        output = result.final_output
        
        # Apply output transform if provided
        if transform := config.get("output_transform"):
            output = transform(output)
            
        return output
        
    async def _execute_parallel_step(
        self,
        step: WorkflowStep,
        context: ExecutionContext,
    ) -> List[Any]:
        """Execute parallel steps."""
        step_ids = step.config["step_ids"]
        try:
            outcomes = await self._execute_batches(step_ids, context)
        except Exception as e:
            raise _NestedStepFailed(str(e)) from e
        return [
            self._child_output(step_id, outcomes[step_id])
            for step_id in step_ids
            if step_id in outcomes
        ]

    async def _execute_conditional_step(
        self,
        step: WorkflowStep,
        context: ExecutionContext,
    ) -> Any:
        """Execute conditional step."""
        condition = step.config["condition"]
        if_true = step.config["if_true"]
        if_false = step.config.get("if_false")
        
        # Evaluate condition
        step_input = context.get_step_input(step)
        
        if callable(condition):
            result = condition(step_input)
        else:
            # Simple expression evaluation
            result = eval(condition, {"input": step_input})
            
        selected = if_true if result else if_false
        if selected is None:
            return None
        return await self._execute_child(selected, context)
        
    async def _execute_loop_step(
        self,
        step: WorkflowStep,
        context: ExecutionContext,
    ) -> List[Any]:
        """Execute loop step."""
        over = step.config["over"]
        body = step.config["body"]
        max_iterations = step.config.get("max_iterations")
        
        # Get items to loop over
        step_input = context.get_step_input(step)
        
        if callable(over):
            items = over(step_input)
        else:
            # Simple path evaluation
            items = eval(f"input.{over}", {"input": step_input})
            
        # Limit iterations
        if max_iterations:
            items = items[:max_iterations]
            
        outputs = []
        for i, item in enumerate(items):
            iteration = replace(context, _loop_item=item, _loop_index=i)
            outputs.append(await self._execute_child(body, iteration))
        return outputs

    async def _execute_child(self, step_id: str, context: ExecutionContext) -> Any:
        step = context.workflow.steps.get(step_id)
        if step is None:
            raise _NestedStepFailed(f"Unknown nested step '{step_id}'")
        if not context.dependencies_succeeded(step):
            raise _NestedStepFailed(f"Dependencies of step '{step_id}' did not succeed")
        try:
            outcome = await self._execute_step(step, context)
        except Exception as e:
            raise _NestedStepFailed(f"Step '{step_id}' failed: {e}") from e
        return self._child_output(step_id, outcome)

    @staticmethod
    def _child_output(step_id: str, outcome: StepResult | None) -> Any:
        if outcome is None:
            raise _NestedStepFailed(f"Step '{step_id}' did not run")
        if not outcome.success:
            raise _NestedStepFailed(f"Step '{step_id}' failed: {outcome.error}")
        return outcome.output

    async def _execute_transform_step(
        self,
        step: WorkflowStep,
        context: ExecutionContext,
    ) -> Any:
        """Execute transform step."""
        transform = step.config["transform"]
        step_input = context.get_step_input(step)
        
        if callable(transform):
            return transform(step_input)
        else:
            raise AgentsException("Transform must be a callable")
            
    async def _execute_wait_step(
        self,
        step: WorkflowStep,
        context: ExecutionContext,
    ) -> None:
        """Execute wait step."""
        # Simple timeout wait for now
        timeout = step.config.get("timeout", 1.0)
        await asyncio.sleep(timeout)
        
    async def _send_update(self, type: UpdateType, data: Dict[str, Any]) -> None:
        """Send update to UI streamer."""
        if self.ui_streamer:
            update = StreamUpdate(
                execution_id=self.execution_id,
                type=type,
                data=data,
            )
            await self.ui_streamer.send(update)
            
    def get_status(self) -> Dict[str, Any]:
        """Get current execution status."""
        result = self._result
        outputs = {}
        if result is not None:
            outputs = {
                step_id: outcome.output
                for step_id, outcome in result.step_results.items()
                if outcome.success
            }
        return {
            "execution_id": self.execution_id,
            "workflow_id": self.workflow.id,
            "workflow_name": self.workflow.name,
            "steps_completed": len(outputs),
            "total_steps": result.total_steps if result is not None else len(self.workflow.steps),
            "completed_steps": list(outputs),
            "outputs": outputs,
        }


@dataclass
class ExecutionContext:
    """Context for workflow execution."""
    
    input: Any
    context: Any
    workflow: Workflow
    result: ExecutionResult
    descendants: dict[str, set[str]]
    _loop_item: Any = None
    _loop_index: int | None = None

    def dependencies_succeeded(self, step: WorkflowStep) -> bool:
        return all(
            step_id in self.result.step_results and self.result.step_results[step_id].success
            for step_id in step.depends_on
        )

    def get_step_input(self, step: WorkflowStep) -> Any:
        """Get input for a step."""
        if step.depends_on:
            outputs = [
                self.result.step_results[step_id].output
                for step_id in step.depends_on
                if step_id in self.result.step_results and self.result.step_results[step_id].success
            ]
            if len(step.depends_on) > 1:
                return outputs
            if outputs:
                return outputs[0]

        if self._loop_index is not None:
            return self._loop_item
        return self.input
