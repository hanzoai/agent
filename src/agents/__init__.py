import logging
import sys
from typing import Literal

from openai import AsyncOpenAI

from . import _config
from .agent import Agent
from .agent_output import AgentOutputSchema
from .computer import AsyncComputer, Button, Computer, Environment
from .exceptions import (
    AgentsException,
    InputGuardrailTripwireTriggered,
    MaxTurnsExceeded,
    ModelBehaviorError,
    OutputGuardrailTripwireTriggered,
    UserError,
)
from .guardrail import (
    GuardrailFunctionOutput,
    InputGuardrail,
    InputGuardrailResult,
    OutputGuardrail,
    OutputGuardrailResult,
    input_guardrail,
    output_guardrail,
)
from .handoffs import Handoff, HandoffInputData, HandoffInputFilter, handoff
from .items import (
    HandoffCallItem,
    HandoffOutputItem,
    ItemHelpers,
    MessageOutputItem,
    ModelResponse,
    ReasoningItem,
    RunItem,
    ToolCallItem,
    ToolCallOutputItem,
    TResponseInputItem,
)
from .lifecycle import AgentHooks, RunHooks
from .model_settings import ModelSettings
from .models.interface import Model, ModelProvider, ModelTracing
from .models.openai_chatcompletions import OpenAIChatCompletionsModel
from .models.openai_provider import OpenAIProvider
from .models.openai_responses import OpenAIResponsesModel
from .result import RunResult, RunResultStreaming
from .run import RunConfig, Runner
from .run_context import RunContextWrapper, TContext
from .stream_events import (
    AgentUpdatedStreamEvent,
    RawResponsesStreamEvent,
    RunItemStreamEvent,
    StreamEvent,
)
from .tool import (
    ComputerTool,
    FileSearchTool,
    FunctionTool,
    Tool,
    WebSearchTool,
    default_tool_error_function,
    function_tool,
)
from .tracing import (
    AgentSpanData,
    CustomSpanData,
    FunctionSpanData,
    GenerationSpanData,
    GuardrailSpanData,
    HandoffSpanData,
    Span,
    SpanData,
    SpanError,
    Trace,
    add_trace_processor,
    agent_span,
    custom_span,
    function_span,
    gen_span_id,
    gen_trace_id,
    generation_span,
    get_current_span,
    get_current_trace,
    guardrail_span,
    handoff_span,
    set_trace_processors,
    set_tracing_disabled,
    set_tracing_export_api_key,
    trace,
)
from .usage import Usage

# Network and orchestration imports
from .network import (
    AgentNetwork,
    NetworkConfig,
    Router,
    RoutingDecision,
    RoutingStrategy,
    SemanticRouter,
    RuleBasedRouter,
    LoadBalancingRouter,
    routing_strategy,
    NetworkNode,
    NodeStatus,
)
from .state import (
    StateStore,
    InMemoryStateStore,
    RedisStateStore,
    FileStateStore,
    StateNamespace,
    StateSerializer,
    JSONSerializer,
    PickleSerializer,
)
from .memory import (
    Memory,
    MemoryEntry,
    MemoryType,
    MemoryStore,
    InMemoryMemoryStore,
    VectorMemoryStore,
    MemoryRetriever,
    SemanticRetriever,
    RecencyRetriever,
    HybridRetriever,
)
from .orchestration import (
    Orchestrator,
    OrchestrationConfig,
    Workflow,
    WorkflowStep,
    StepType,
    WorkflowExecutor,
    ExecutionResult,
    UIStreamer,
    StreamUpdate,
    UpdateType,
)


def set_default_openai_key(key: str, use_for_tracing: bool = True) -> None:
    """Set the default OpenAI API key to use for LLM requests (and optionally tracing(). This is
    only necessary if the OPENAI_API_KEY environment variable is not already set.

    If provided, this key will be used instead of the OPENAI_API_KEY environment variable.

    Args:
        key: The OpenAI key to use.
        use_for_tracing: Whether to also use this key to send traces to OpenAI. Defaults to True
            If False, you'll either need to set the OPENAI_API_KEY environment variable or call
            set_tracing_export_api_key() with the API key you want to use for tracing.
    """
    _config.set_default_openai_key(key, use_for_tracing)


def set_default_openai_client(client: AsyncOpenAI, use_for_tracing: bool = True) -> None:
    """Set the default OpenAI client to use for LLM requests and/or tracing. If provided, this
    client will be used instead of the default OpenAI client.

    Args:
        client: The OpenAI client to use.
        use_for_tracing: Whether to use the API key from this client for uploading traces. If False,
            you'll either need to set the OPENAI_API_KEY environment variable or call
            set_tracing_export_api_key() with the API key you want to use for tracing.
    """
    _config.set_default_openai_client(client, use_for_tracing)


def set_default_openai_api(api: Literal["chat_completions", "responses"]) -> None:
    """Set the default API to use for OpenAI LLM requests. By default, we will use the responses API
    but you can set this to use the chat completions API instead.
    """
    _config.set_default_openai_api(api)


def enable_verbose_stdout_logging():
    """Enables verbose logging to stdout. This is useful for debugging."""
    logger = logging.getLogger("openai.agents")
    logger.setLevel(logging.DEBUG)
    logger.addHandler(logging.StreamHandler(sys.stdout))


__all__ = [
    "Agent",
    "Runner",
    "Model",
    "ModelProvider",
    "ModelTracing",
    "ModelSettings",
    "OpenAIChatCompletionsModel",
    "OpenAIProvider",
    "OpenAIResponsesModel",
    "AgentOutputSchema",
    "Computer",
    "AsyncComputer",
    "Environment",
    "Button",
    "AgentsException",
    "InputGuardrailTripwireTriggered",
    "OutputGuardrailTripwireTriggered",
    "MaxTurnsExceeded",
    "ModelBehaviorError",
    "UserError",
    "InputGuardrail",
    "InputGuardrailResult",
    "OutputGuardrail",
    "OutputGuardrailResult",
    "GuardrailFunctionOutput",
    "input_guardrail",
    "output_guardrail",
    "handoff",
    "Handoff",
    "HandoffInputData",
    "HandoffInputFilter",
    "TResponseInputItem",
    "MessageOutputItem",
    "ModelResponse",
    "RunItem",
    "HandoffCallItem",
    "HandoffOutputItem",
    "ToolCallItem",
    "ToolCallOutputItem",
    "ReasoningItem",
    "ModelResponse",
    "ItemHelpers",
    "RunHooks",
    "AgentHooks",
    "RunContextWrapper",
    "TContext",
    "RunResult",
    "RunResultStreaming",
    "RunConfig",
    "RawResponsesStreamEvent",
    "RunItemStreamEvent",
    "AgentUpdatedStreamEvent",
    "StreamEvent",
    "FunctionTool",
    "ComputerTool",
    "FileSearchTool",
    "Tool",
    "WebSearchTool",
    "function_tool",
    "Usage",
    "add_trace_processor",
    "agent_span",
    "custom_span",
    "function_span",
    "generation_span",
    "get_current_span",
    "get_current_trace",
    "guardrail_span",
    "handoff_span",
    "set_trace_processors",
    "set_tracing_disabled",
    "trace",
    "Trace",
    "SpanError",
    "Span",
    "SpanData",
    "AgentSpanData",
    "CustomSpanData",
    "FunctionSpanData",
    "GenerationSpanData",
    "GuardrailSpanData",
    "HandoffSpanData",
    "set_default_openai_key",
    "set_default_openai_client",
    "set_default_openai_api",
    "set_tracing_export_api_key",
    "enable_verbose_stdout_logging",
    "gen_trace_id",
    "gen_span_id",
    "default_tool_error_function",
    # Network exports
    "AgentNetwork",
    "NetworkConfig",
    "Router",
    "RoutingDecision",
    "RoutingStrategy",
    "SemanticRouter",
    "RuleBasedRouter",
    "LoadBalancingRouter",
    "routing_strategy",
    "NetworkNode",
    "NodeStatus",
    # State exports
    "StateStore",
    "InMemoryStateStore",
    "RedisStateStore",
    "FileStateStore",
    "StateNamespace",
    "StateSerializer",
    "JSONSerializer",
    "PickleSerializer",
    # Memory exports
    "Memory",
    "MemoryEntry",
    "MemoryType",
    "MemoryStore",
    "InMemoryMemoryStore",
    "VectorMemoryStore",
    "MemoryRetriever",
    "SemanticRetriever",
    "RecencyRetriever",
    "HybridRetriever",
    # Orchestration exports
    "Orchestrator",
    "OrchestrationConfig",
    "Workflow",
    "WorkflowStep",
    "StepType",
    "WorkflowExecutor",
    "ExecutionResult",
    "UIStreamer",
    "StreamUpdate",
    "UpdateType",
]
