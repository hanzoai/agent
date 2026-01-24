"""ZAP-enabled agent with gateway integration.

Provides an agent class that automatically connects to ZAP gateways
and uses ZAP tools for execution.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, Callable

from ..agent import Agent
from ..guardrail import InputGuardrail, OutputGuardrail
from ..handoffs import Handoff
from ..lifecycle import AgentHooks
from ..model_settings import ModelSettings
from ..models.interface import Model
from ..run_context import TContext
from ..tool import FunctionTool, Tool
from .client import ZapClient
from .tools import ZapToolProvider, create_canonical_tools
from .types import EndpointCaps


@dataclass
class ZapAgentConfig:
    """Configuration for ZAP-enabled agents."""

    # Gateway connection
    gateway_uri: str = "zap://localhost:9999"
    connect_timeout: float = 5.0
    request_timeout: float = 30.0

    # Tool selection
    tool_namespaces: list[str] | None = None  # Filter tools by namespace
    tool_prefixes: list[str] | None = None  # Filter tools by prefix
    include_canonical: bool = True  # Include fs, proc, vcs, net tools
    certified_only: bool = False  # Only use certified tools

    # Consensus (for multi-agent)
    enable_consensus: bool = False
    consensus_participants: list[str] = field(default_factory=list)


class ZapAgent(Agent[TContext]):
    """Agent with ZAP gateway integration.

    Automatically connects to a ZAP gateway and uses ZAP tools for execution.
    Supports tool discovery, MCP aggregation, and optional consensus.

    Example:
        # Create agent with ZAP tools
        agent = ZapAgent(
            name="code-agent",
            instructions="You are a helpful coding assistant.",
            zap_config=ZapAgentConfig(gateway_uri="zap://localhost:9999"),
        )

        # Connect and run
        await agent.connect()
        result = await Runner.run(agent, input="Read /etc/hosts")
    """

    def __init__(
        self,
        name: str,
        zap_config: ZapAgentConfig | None = None,
        instructions: str | Callable[..., Any] | None = None,
        handoff_description: str | None = None,
        handoffs: list[Agent[Any] | Handoff[TContext]] | None = None,
        model: str | Model | None = None,
        model_settings: ModelSettings | None = None,
        tools: list[Tool] | None = None,
        input_guardrails: list[InputGuardrail[TContext]] | None = None,
        output_guardrails: list[OutputGuardrail[TContext]] | None = None,
        output_type: type | None = None,
        hooks: AgentHooks[TContext] | None = None,
    ):
        """Initialize ZAP agent.

        Args:
            name: Agent name.
            zap_config: ZAP configuration.
            instructions: System prompt for the agent.
            handoff_description: Description for handoff use.
            handoffs: List of handoff agents.
            model: Model to use.
            model_settings: Model configuration.
            tools: Additional tools (ZAP tools will be added).
            input_guardrails: Input validation guardrails.
            output_guardrails: Output validation guardrails.
            output_type: Expected output type.
            hooks: Lifecycle hooks.
        """
        # Initialize base agent with provided tools
        super().__init__(
            name=name,
            instructions=instructions,
            handoff_description=handoff_description,
            handoffs=handoffs or [],
            model=model,
            model_settings=model_settings or ModelSettings(),
            tools=tools or [],
            input_guardrails=input_guardrails or [],
            output_guardrails=output_guardrails or [],
            output_type=output_type,
            hooks=hooks,
        )

        self.zap_config = zap_config or ZapAgentConfig()
        self._client: ZapClient | None = None
        self._provider: ZapToolProvider | None = None
        self._connected = False

    @property
    def client(self) -> ZapClient | None:
        """Get the ZAP client."""
        return self._client

    @property
    def capabilities(self) -> EndpointCaps | None:
        """Get gateway capabilities."""
        return self._client.capabilities if self._client else None

    async def connect(self) -> None:
        """Connect to ZAP gateway and load tools.

        This must be called before using the agent.
        """
        if self._connected:
            return

        # Create and connect client
        self._client = ZapClient.from_uri(self.zap_config.gateway_uri)
        self._client.connect_timeout = self.zap_config.connect_timeout
        self._client.request_timeout = self.zap_config.request_timeout
        await self._client.connect()

        # Create tool provider
        self._provider = ZapToolProvider(
            host=self._client.host,
            port=self._client.port,
            tls=self._client.tls,
            unix_socket=self._client.unix_socket,
        )
        self._provider._client = self._client  # Reuse connection

        # Load ZAP tools
        zap_tools: list[FunctionTool] = []

        if self.zap_config.include_canonical:
            zap_tools.extend(create_canonical_tools(self._client))

        # Load tools from catalog based on filters
        if self.zap_config.tool_namespaces or self.zap_config.tool_prefixes:
            namespaces: list[str | None] = self.zap_config.tool_namespaces or [None]
            prefixes: list[str | None] = self.zap_config.tool_prefixes or [None]

            for ns in namespaces:
                for prefix in prefixes:
                    tools = await self._provider.get_tools(
                        namespace=ns,
                        prefix=prefix,
                        certified_only=self.zap_config.certified_only,
                    )
                    zap_tools.extend(tools)
        elif not self.zap_config.include_canonical:
            # Load all tools if no filters and no canonical
            tools = await self._provider.get_tools(
                certified_only=self.zap_config.certified_only
            )
            zap_tools.extend(tools)

        # Add ZAP tools to agent (avoiding duplicates)
        existing_names = {t.name for t in self.tools}
        for tool in zap_tools:
            if tool.name not in existing_names:
                self.tools.append(tool)
                existing_names.add(tool.name)

        self._connected = True

    async def disconnect(self) -> None:
        """Disconnect from ZAP gateway."""
        if self._client:
            await self._client.close()
            self._client = None
        self._provider = None
        self._connected = False

    async def __aenter__(self) -> ZapAgent:
        await self.connect()
        return self

    async def __aexit__(self, *args: Any) -> None:
        await self.disconnect()

    def clone(self, **kwargs: Any) -> ZapAgent:
        """Clone the agent with modifications."""
        # Create new agent with same config
        new_agent = ZapAgent(
            name=kwargs.get("name", self.name),
            zap_config=kwargs.get("zap_config", self.zap_config),
            instructions=kwargs.get("instructions", self.instructions),
            handoff_description=kwargs.get("handoff_description", self.handoff_description),
            handoffs=kwargs.get("handoffs", self.handoffs),
            model=kwargs.get("model", self.model),
            model_settings=kwargs.get("model_settings", self.model_settings),
            tools=kwargs.get("tools", list(self.tools)),
            input_guardrails=kwargs.get("input_guardrails", self.input_guardrails),
            output_guardrails=kwargs.get("output_guardrails", self.output_guardrails),
            output_type=kwargs.get("output_type", self.output_type),
            hooks=kwargs.get("hooks", self.hooks),
        )
        return new_agent


async def create_zap_agent(
    name: str,
    gateway_uri: str = "zap://localhost:9999",
    instructions: str | None = None,
    model: str | None = None,
    **kwargs: Any,
) -> ZapAgent:
    """Create and connect a ZAP agent.

    Convenience function for quickly creating a connected ZAP agent.

    Args:
        name: Agent name.
        gateway_uri: ZAP gateway URI.
        instructions: System prompt.
        model: Model to use.
        **kwargs: Additional agent configuration.

    Returns:
        Connected ZapAgent instance.

    Example:
        agent = await create_zap_agent(
            "helper",
            gateway_uri="zap://localhost:9999",
            instructions="You are helpful.",
        )
    """
    config = ZapAgentConfig(gateway_uri=gateway_uri)
    agent = ZapAgent(
        name=name,
        zap_config=config,
        instructions=instructions,
        model=model,
        **kwargs,
    )
    await agent.connect()
    return agent
