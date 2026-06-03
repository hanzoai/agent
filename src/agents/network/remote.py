"""Cross-process agent invocation over ZAP wire.

When `HANZO_AGENT_WIRE=zap` is set, an `AgentNetwork` can hold *remote* nodes
backed by a `ZapClient` instead of a local `Agent`. Calls to a remote node
are dispatched as ZAP `call_tool` requests at the peer endpoint named
`agent/<name>`; the peer side is expected to expose its agents as ZAP tools.

Default wire stays JSON/OpenAI-compatible. ZAP is intra-cluster only — it is
opt-in via env-var so external API consumers see no change.

Decomplected from `AgentNetwork`:
  - Network: owns the nodes dict + the router + dispatch.
  - RemoteAgentProxy: a value-shaped facade that quacks like `Agent` enough
    for routing/dispatch (name + handoff_description) and that, when invoked,
    pushes the payload over ZAP rather than driving a local LLM loop.
  - is_zap_wire_enabled: a single function in a single place that reads the
    feature flag. Profile gate, Hickey-style.
"""

from __future__ import annotations

import os
from dataclasses import dataclass, field
from typing import Any, Optional

# Soft import — ZapClient lives in agents/zap; importing here would create a
# cycle at module load time if zap pulled in network. So we import lazily
# from inside connect().


def is_zap_wire_enabled() -> bool:
    """Single source of truth for the ZAP cross-process feature flag.

    Read once per process via `os.environ`. The default (no env, or any value
    other than the literal string ``zap``) keeps the network on the JSON/local
    Agent path — preserving OpenAI compatibility for external API consumers.
    """
    return os.environ.get("HANZO_AGENT_WIRE", "").strip().lower() == "zap"


@dataclass
class RemoteAgentProxy:
    """A value-shaped proxy for an agent that lives at the other end of a ZAP wire.

    Shapes the public surface of `Agent` minimally — enough for the network
    router (which only reads `name` + `handoff_description`) and for the
    dispatcher (which calls `invoke` rather than driving the OpenAI runner).
    """

    name: str
    """The remote agent's name as it is registered on the peer side."""

    zap_uri: str
    """ZAP endpoint URI for the peer (e.g. ``zap://peer-2.cluster:9999``)."""

    handoff_description: Optional[str] = None
    """Human description used when the agent is exposed as a handoff."""

    capabilities: list[str] = field(default_factory=list)
    """Routing capabilities advertised by the remote peer."""

    metadata: dict[str, Any] = field(default_factory=dict)
    """Additional routing metadata (model, region, etc.)."""

    # The Agent dataclass contract carries these fields as well; we keep them
    # default-empty so any code that reads them via getattr does not break.
    handoffs: list[Any] = field(default_factory=list)
    tools: list[Any] = field(default_factory=list)
    instructions: Optional[str] = None

    async def invoke(self, input: Any, *, context: Any = None) -> Any:
        """Dispatch an invocation to the remote agent over ZAP.

        The peer side exposes each registered agent as a ZAP tool named
        ``agent/<name>`` taking a single ``input`` argument and returning the
        agent's response payload.
        """
        if not is_zap_wire_enabled():
            raise RuntimeError(
                "RemoteAgentProxy.invoke called with HANZO_AGENT_WIRE != zap"
            )

        # Lazy import to avoid network <-> zap module cycle on package init.
        from ..zap.client import ZapClient

        async with await ZapClient.from_uri(self.zap_uri) as client:
            return await client.call_tool(
                f"agent/{self.name}",
                {"input": input, "context": context},
            )

    def clone(self, **kwargs: Any) -> "RemoteAgentProxy":
        """Mirrors Agent.clone — produce a derived proxy with overrides."""
        return RemoteAgentProxy(
            name=kwargs.get("name", self.name),
            zap_uri=kwargs.get("zap_uri", self.zap_uri),
            handoff_description=kwargs.get(
                "handoff_description", self.handoff_description
            ),
            capabilities=list(kwargs.get("capabilities", self.capabilities)),
            metadata=dict(kwargs.get("metadata", self.metadata)),
            handoffs=list(kwargs.get("handoffs", self.handoffs)),
            tools=list(kwargs.get("tools", self.tools)),
            instructions=kwargs.get("instructions", self.instructions),
        )
