"""ZAP protocol type definitions.

These types mirror the Cap'n Proto schema but use Python dataclasses
for easier integration with the agent SDK.
"""

from __future__ import annotations

import json
from dataclasses import dataclass, field
from enum import Enum
from typing import Any


class Effect(str, Enum):
    """Side effect classification for tools."""
    PURE = "pure"
    DETERMINISTIC = "deterministic"
    NONDETERMINISTIC = "nondeterministic"


class Scope(str, Enum):
    """Scope level for tool operations."""
    SPAN = "span"
    FILE = "file"
    REPO = "repo"
    WORKSPACE = "workspace"
    NODE = "node"
    CHAIN = "chain"
    GLOBAL = "global"


class Stability(str, Enum):
    """Tool stability level."""
    EXPERIMENTAL = "experimental"
    BETA = "beta"
    STABLE = "stable"
    DEPRECATED = "deprecated"


class TaskState(str, Enum):
    """Task execution state."""
    PENDING = "pending"
    RUNNING = "running"
    COMPLETED = "completed"
    FAILED = "failed"
    CANCELLED = "cancelled"


class ErrorCode(str, Enum):
    """ZAP error codes."""
    UNKNOWN_ACTION = "unknownAction"
    INVALID_PARAMS = "invalidParams"
    NOT_FOUND = "notFound"
    CONFLICT = "conflict"
    PERMISSION_DENIED = "permissionDenied"
    TIMEOUT = "timeout"
    INTERNAL_ERROR = "internalError"
    RATE_LIMITED = "rateLimited"
    NOT_CONNECTED = "notConnected"
    PROTOCOL_ERROR = "protocolError"


@dataclass
class ToolId:
    """Unique identifier for a ZAP tool."""
    namespace: str
    name: str
    version: str = "1.0.0"

    def __str__(self) -> str:
        return f"{self.namespace}/{self.name}@{self.version}"

    @classmethod
    def parse(cls, s: str) -> ToolId:
        """Parse 'namespace/name@version' or 'namespace/name'."""
        if "@" in s:
            ns_name, version = s.rsplit("@", 1)
        else:
            ns_name, version = s, "1.0.0"
        if "/" in ns_name:
            namespace, name = ns_name.split("/", 1)
        else:
            namespace, name = "native", ns_name
        return cls(namespace=namespace, name=name, version=version)

    def to_dict(self) -> dict[str, str]:
        return {"namespace": self.namespace, "name": self.name, "version": self.version}


@dataclass
class Tool:
    """ZAP tool definition."""
    id: ToolId
    description: str
    effect: Effect = Effect.NONDETERMINISTIC
    idempotent: bool = False
    input_schema: dict[str, Any] = field(default_factory=dict)
    output_schema: dict[str, Any] = field(default_factory=dict)
    provider: str = ""
    stability: Stability = Stability.STABLE

    @property
    def name(self) -> str:
        return self.id.name

    @property
    def full_name(self) -> str:
        return str(self.id)

    def to_dict(self) -> dict[str, Any]:
        return {
            "id": self.id.to_dict(),
            "description": self.description,
            "effect": self.effect.value,
            "idempotent": self.idempotent,
            "inputSchema": self.input_schema,
            "outputSchema": self.output_schema,
            "provider": self.provider,
            "stability": self.stability.value,
        }

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> Tool:
        id_data = d.get("id", {})
        return cls(
            id=ToolId(
                namespace=id_data.get("namespace", "native"),
                name=id_data.get("name", ""),
                version=id_data.get("version", "1.0.0"),
            ),
            description=d.get("description", ""),
            effect=Effect(d.get("effect", "nondeterministic")),
            idempotent=d.get("idempotent", False),
            input_schema=d.get("inputSchema", {}),
            output_schema=d.get("outputSchema", {}),
            provider=d.get("provider", ""),
            stability=Stability(d.get("stability", "stable")),
        )


@dataclass
class Resource:
    """ZAP resource definition."""
    uri: str
    name: str
    description: str = ""
    mime_type: str = "text/plain"
    size: int = 0

    def to_dict(self) -> dict[str, Any]:
        return {
            "uri": self.uri,
            "name": self.name,
            "description": self.description,
            "mimeType": self.mime_type,
            "size": self.size,
        }

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> Resource:
        return cls(
            uri=d.get("uri", ""),
            name=d.get("name", ""),
            description=d.get("description", ""),
            mime_type=d.get("mimeType", "text/plain"),
            size=d.get("size", 0),
        )


@dataclass
class ToolResult:
    """Result from a ZAP tool invocation."""
    success: bool
    data: Any = None
    error: ZapError | None = None
    duration_ns: int = 0

    def to_dict(self) -> dict[str, Any]:
        result: dict[str, Any] = {"success": self.success}
        if self.data is not None:
            result["data"] = self.data
        if self.error is not None:
            result["error"] = self.error.to_dict()
        result["durationNs"] = self.duration_ns
        return result

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> ToolResult:
        error_data = d.get("error")
        return cls(
            success=d.get("success", False),
            data=d.get("data"),
            error=ZapError.from_dict(error_data) if error_data else None,
            duration_ns=d.get("durationNs", 0),
        )


@dataclass
class ZapError:
    """ZAP protocol error."""
    code: ErrorCode
    message: str
    details: Any | None = None

    def to_dict(self) -> dict[str, Any]:
        result = {"code": self.code.value, "message": self.message}
        if self.details is not None:
            result["details"] = self.details
        return result

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> ZapError:
        return cls(
            code=ErrorCode(d.get("code", "internalError")),
            message=d.get("message", "Unknown error"),
            details=d.get("details"),
        )


@dataclass
class CallContext:
    """Context for tool invocations."""
    trace_id: str = ""
    span_id: str = ""
    timeout_ms: int = 30000
    determinism: DeterminismContext | None = None

    def to_dict(self) -> dict[str, Any]:
        result = {
            "traceId": self.trace_id,
            "spanId": self.span_id,
            "timeout": self.timeout_ms,
        }
        if self.determinism:
            result["determinism"] = self.determinism.to_dict()
        return result


@dataclass
class DeterminismContext:
    """Context for deterministic execution."""
    timestamp: int = 0
    random_seed: bytes = b""
    chain_height: int = 0

    def to_dict(self) -> dict[str, Any]:
        return {
            "timestamp": self.timestamp,
            "randomSeed": self.random_seed.hex() if self.random_seed else "",
            "chainHeight": self.chain_height,
        }


@dataclass
class Progress:
    """Progress update for long-running operations."""
    done: int = 0
    total: int = 0
    message: str = ""

    def to_dict(self) -> dict[str, Any]:
        return {"done": self.done, "total": self.total, "message": self.message}


@dataclass
class TaskStatus:
    """Status of an async task."""
    state: TaskState
    progress: Progress = field(default_factory=Progress)
    started_at: int = 0
    updated_at: int = 0

    def to_dict(self) -> dict[str, Any]:
        return {
            "state": self.state.value,
            "progress": self.progress.to_dict(),
            "startedAt": self.started_at,
            "updatedAt": self.updated_at,
        }


@dataclass
class Implementation:
    """Client/server implementation info."""
    name: str
    version: str

    def to_dict(self) -> dict[str, str]:
        return {"name": self.name, "version": self.version}


@dataclass
class ClientCaps:
    """Client capabilities."""
    roots: bool = True
    sampling: bool = True
    elicitation: bool = False
    experimental: list[str] = field(default_factory=list)

    def to_dict(self) -> dict[str, Any]:
        return {
            "roots": self.roots,
            "sampling": self.sampling,
            "elicitation": self.elicitation,
            "experimental": self.experimental,
        }


@dataclass
class EndpointCaps:
    """Server endpoint capabilities."""
    tools: bool = True
    resources: bool = True
    prompts: bool = True
    tasks: bool = True
    logging: bool = True
    repl: bool = False
    notebook: bool = False
    browser: bool = False
    catalog: bool = True
    coordination: bool = True
    experimental: list[str] = field(default_factory=list)

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> EndpointCaps:
        return cls(
            tools=d.get("tools", True),
            resources=d.get("resources", True),
            prompts=d.get("prompts", True),
            tasks=d.get("tasks", True),
            logging=d.get("logging", True),
            repl=d.get("repl", False),
            notebook=d.get("notebook", False),
            browser=d.get("browser", False),
            catalog=d.get("catalog", True),
            coordination=d.get("coordination", True),
            experimental=d.get("experimental", []),
        )


@dataclass
class Hello:
    """Client handshake message."""
    protocol_version: str = "0.2.1"
    client_info: Implementation = field(
        default_factory=lambda: Implementation("hanzo-agent", "0.1.0")
    )
    capabilities: ClientCaps = field(default_factory=ClientCaps)
    schema_hash: bytes = b""

    def to_dict(self) -> dict[str, Any]:
        return {
            "protocolVersion": self.protocol_version,
            "clientInfo": self.client_info.to_dict(),
            "capabilities": self.capabilities.to_dict(),
            "schemaHash": self.schema_hash.hex() if self.schema_hash else "",
        }


@dataclass
class Welcome:
    """Server handshake response."""
    protocol_version: str = "0.2.1"
    endpoint_info: Implementation = field(
        default_factory=lambda: Implementation("zap-gateway", "0.2.1")
    )
    capabilities: EndpointCaps = field(default_factory=EndpointCaps)
    instructions: str = ""
    schema_hash: bytes = b""

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> Welcome:
        endpoint_data = d.get("endpointInfo", {})
        caps_data = d.get("capabilities", {})
        return cls(
            protocol_version=d.get("protocolVersion", "0.2.1"),
            endpoint_info=Implementation(
                name=endpoint_data.get("name", "zap-gateway"),
                version=endpoint_data.get("version", "0.2.1"),
            ),
            capabilities=EndpointCaps.from_dict(caps_data),
            instructions=d.get("instructions", ""),
            schema_hash=bytes.fromhex(d.get("schemaHash", "")) if d.get("schemaHash") else b"",
        )


@dataclass
class ConsensusConfig:
    """Configuration for consensus rounds."""
    rounds: int = 3
    k: int = 5  # Sample size per round
    alpha: float = 0.6  # Confidence threshold
    beta1: float = 0.8  # Phase I threshold
    beta2: float = 0.9  # Phase II (finality) threshold
    timeout_ms: int = 10000

    def to_dict(self) -> dict[str, Any]:
        return {
            "rounds": self.rounds,
            "k": self.k,
            "alpha": self.alpha,
            "beta1": self.beta1,
            "beta2": self.beta2,
            "timeoutMs": self.timeout_ms,
        }


@dataclass
class ConsensusVote:
    """A vote in a consensus round."""
    round: int
    peer_id: str
    vote: bytes
    confidence: float
    luminance: float = 1.0
    signature: bytes = b""
    timestamp: int = 0

    def to_dict(self) -> dict[str, Any]:
        return {
            "round": self.round,
            "peerId": self.peer_id,
            "vote": self.vote.hex(),
            "confidence": self.confidence,
            "luminance": self.luminance,
            "signature": self.signature.hex() if self.signature else "",
            "timestamp": self.timestamp,
        }

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> ConsensusVote:
        return cls(
            round=d.get("round", 0),
            peer_id=d.get("peerId", ""),
            vote=bytes.fromhex(d.get("vote", "")),
            confidence=d.get("confidence", 0.0),
            luminance=d.get("luminance", 1.0),
            signature=bytes.fromhex(d.get("signature", "")) if d.get("signature") else b"",
            timestamp=d.get("timestamp", 0),
        )


@dataclass
class Certificate:
    """Consensus certificate."""
    topic: bytes
    proposal_hash: bytes
    round: int
    confidence: float
    attestors: list[dict[str, Any]] = field(default_factory=list)
    timestamp: int = 0

    def to_dict(self) -> dict[str, Any]:
        return {
            "topic": self.topic.hex(),
            "proposalHash": self.proposal_hash.hex(),
            "round": self.round,
            "confidence": self.confidence,
            "attestors": self.attestors,
            "timestamp": self.timestamp,
        }

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> Certificate:
        return cls(
            topic=bytes.fromhex(d.get("topic", "")),
            proposal_hash=bytes.fromhex(d.get("proposalHash", "")),
            round=d.get("round", 0),
            confidence=d.get("confidence", 0.0),
            attestors=d.get("attestors", []),
            timestamp=d.get("timestamp", 0),
        )


@dataclass
class ConsensusResult:
    """Result of a consensus operation."""
    winner: bytes
    synthesis: str
    confidence: float
    round: int
    votes: list[ConsensusVote] = field(default_factory=list)
    certificate: Certificate | None = None
    duration_ns: int = 0

    def to_dict(self) -> dict[str, Any]:
        return {
            "winner": self.winner.hex(),
            "synthesis": self.synthesis,
            "confidence": self.confidence,
            "round": self.round,
            "votes": [v.to_dict() for v in self.votes],
            "certificate": self.certificate.to_dict() if self.certificate else None,
            "durationNs": self.duration_ns,
        }

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> ConsensusResult:
        cert_data = d.get("certificate")
        return cls(
            winner=bytes.fromhex(d.get("winner", "")),
            synthesis=d.get("synthesis", ""),
            confidence=d.get("confidence", 0.0),
            round=d.get("round", 0),
            votes=[ConsensusVote.from_dict(v) for v in d.get("votes", [])],
            certificate=Certificate.from_dict(cert_data) if cert_data else None,
            duration_ns=d.get("durationNs", 0),
        )


# Message envelope for wire protocol
@dataclass
class ZapMessage:
    """Wire protocol message envelope."""
    type: str
    id: str
    payload: dict[str, Any] = field(default_factory=dict)

    def encode(self) -> bytes:
        """Encode message to JSON bytes with length prefix."""
        data = json.dumps({
            "type": self.type,
            "id": self.id,
            "payload": self.payload,
        }).encode("utf-8")
        # 4-byte length prefix (big-endian)
        length = len(data)
        return length.to_bytes(4, "big") + data

    @classmethod
    def decode(cls, data: bytes) -> ZapMessage:
        """Decode message from JSON bytes (without length prefix)."""
        obj = json.loads(data.decode("utf-8"))
        return cls(
            type=obj.get("type", ""),
            id=obj.get("id", ""),
            payload=obj.get("payload", {}),
        )
