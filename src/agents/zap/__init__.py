"""ZAP (Zero-copy Agent Protocol) integration for Hanzo agents.

ZAP provides a unified interface to tools across MCP servers, with
consensus-backed routing and zero-copy performance.

Example usage:

    # Using ZAP tools with an agent
    from agents import Agent
    from agents.zap import ZapToolProvider, create_zap_tools

    # Quick way to get tools
    tools = await create_zap_tools("zap://localhost:9999")
    agent = Agent(name="helper", tools=tools)

    # Or use ZapAgent for automatic connection
    from agents.zap import ZapAgent, ZapAgentConfig

    agent = ZapAgent(
        name="code-agent",
        instructions="You help with coding.",
        zap_config=ZapAgentConfig(gateway_uri="zap://localhost:9999"),
    )
    async with agent:
        result = await Runner.run(agent, input="Read /etc/hosts")

    # Multi-agent consensus
    from agents.zap import ConsensusAgent, consensus_decide

    decision = await consensus_decide(
        "What database should we use?",
        agents=[architect, dba, developer],
    )
    print(f"Decision: {decision.answer}")

For more details, see the ZAP documentation at https://zap.hanzo.ai
"""

from .agent import (
    ZapAgent,
    ZapAgentConfig,
    create_zap_agent,
)
from .client import (
    ZapClient,
    ZapConnectionError,
    ZapProtocolError,
)
from .consensus import (
    ConsensusAgent,
    ConsensusDecision,
    ParticipantResponse,
    consensus_decide,
    gateway_consensus,
)
from .tools import (
    ZapToolProvider,
    ZapToolRegistry,
    create_canonical_tools,
    create_zap_tools,
)
from .types import (
    # Context
    CallContext,
    Certificate,
    ClientCaps,
    # Consensus types
    ConsensusConfig,
    ConsensusResult,
    ConsensusVote,
    DeterminismContext,
    # Enums
    Effect,
    EndpointCaps,
    ErrorCode,
    # Protocol types
    Hello,
    Implementation,
    Progress,
    Resource,
    Scope,
    Stability,
    TaskState,
    TaskStatus,
    # Core types
    Tool,
    ToolId,
    ToolResult,
    Welcome,
    ZapError,
    # Wire protocol
    ZapMessage,
)

__all__ = [
    # Client
    "ZapClient",
    "ZapConnectionError",
    "ZapProtocolError",
    # Agent
    "ZapAgent",
    "ZapAgentConfig",
    "create_zap_agent",
    # Tools
    "ZapToolProvider",
    "ZapToolRegistry",
    "create_zap_tools",
    "create_canonical_tools",
    # Consensus
    "ConsensusAgent",
    "ConsensusDecision",
    "ParticipantResponse",
    "consensus_decide",
    "gateway_consensus",
    # Types
    "Tool",
    "ToolId",
    "ToolResult",
    "Resource",
    "ZapError",
    "ErrorCode",
    "Effect",
    "Scope",
    "Stability",
    "TaskState",
    "CallContext",
    "DeterminismContext",
    "Progress",
    "TaskStatus",
    "Hello",
    "Welcome",
    "Implementation",
    "ClientCaps",
    "EndpointCaps",
    "ConsensusConfig",
    "ConsensusVote",
    "ConsensusResult",
    "Certificate",
    "ZapMessage",
]
