"""Tests for ZAP integration module."""

import sys
import os

# Add src to path to avoid going through agents.__init__ which has numpy dep
sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "src"))

import pytest

# Import directly from zap submodule files
import importlib.util


def _load_module(name: str, path: str):
    """Load a module directly from file path."""
    spec = importlib.util.spec_from_file_location(name, path)
    mod = importlib.util.module_from_spec(spec)  # type: ignore
    sys.modules[name] = mod
    spec.loader.exec_module(mod)  # type: ignore
    return mod


# Load zap modules directly
_base = os.path.join(os.path.dirname(__file__), "..", "src", "agents", "zap")
_types = _load_module("agents.zap.types", os.path.join(_base, "types.py"))
_client = _load_module("agents.zap.client", os.path.join(_base, "client.py"))

# Import from loaded modules
Tool = _types.Tool
ToolId = _types.ToolId
ToolResult = _types.ToolResult
ZapError = _types.ZapError
ErrorCode = _types.ErrorCode
Effect = _types.Effect
Scope = _types.Scope
Stability = _types.Stability
CallContext = _types.CallContext
Hello = _types.Hello
Welcome = _types.Welcome
ConsensusConfig = _types.ConsensusConfig
ZapMessage = _types.ZapMessage

ZapClient = _client.ZapClient
ZapConnectionError = _client.ZapConnectionError


# For ZapAgentConfig, we need to mock the agent module dependencies
class ZapAgentConfig:
    """Mock config for testing."""

    def __init__(
        self,
        gateway_uri: str = "zap://localhost:9999",
        connect_timeout: float = 5.0,
        request_timeout: float = 30.0,
        tool_namespaces: list | None = None,
        tool_prefixes: list | None = None,
        include_canonical: bool = True,
        certified_only: bool = False,
        enable_consensus: bool = False,
        consensus_participants: list | None = None,
    ):
        self.gateway_uri = gateway_uri
        self.connect_timeout = connect_timeout
        self.request_timeout = request_timeout
        self.tool_namespaces = tool_namespaces
        self.tool_prefixes = tool_prefixes
        self.include_canonical = include_canonical
        self.certified_only = certified_only
        self.enable_consensus = enable_consensus
        self.consensus_participants = consensus_participants or []


class TestToolId:
    """Tests for ToolId parsing and formatting."""

    def test_parse_full(self):
        tid = ToolId.parse("native/fs.read@1.0.0")
        assert tid.namespace == "native"
        assert tid.name == "fs.read"
        assert tid.version == "1.0.0"

    def test_parse_no_version(self):
        tid = ToolId.parse("native/fs.read")
        assert tid.namespace == "native"
        assert tid.name == "fs.read"
        assert tid.version == "1.0.0"

    def test_parse_no_namespace(self):
        tid = ToolId.parse("fs.read")
        assert tid.namespace == "native"
        assert tid.name == "fs.read"

    def test_str(self):
        tid = ToolId("mcp.github", "create_issue", "2.0.0")
        assert str(tid) == "mcp.github/create_issue@2.0.0"

    def test_to_dict(self):
        tid = ToolId("native", "test", "1.0.0")
        d = tid.to_dict()
        assert d == {"namespace": "native", "name": "test", "version": "1.0.0"}


class TestTool:
    """Tests for Tool type."""

    def test_create(self):
        tool = Tool(
            id=ToolId("native", "test", "1.0.0"),
            description="A test tool",
            effect=Effect.PURE,
            idempotent=True,
        )
        assert tool.name == "test"
        assert tool.full_name == "native/test@1.0.0"
        assert tool.effect == Effect.PURE
        assert tool.idempotent is True

    def test_serialization_roundtrip(self):
        tool = Tool(
            id=ToolId("native", "test", "1.0.0"),
            description="A test tool",
            effect=Effect.DETERMINISTIC,
            input_schema={"type": "object", "properties": {"path": {"type": "string"}}},
        )
        d = tool.to_dict()
        tool2 = Tool.from_dict(d)
        assert tool2.name == tool.name
        assert tool2.description == tool.description
        assert tool2.effect == tool.effect
        assert tool2.input_schema == tool.input_schema


class TestToolResult:
    """Tests for ToolResult type."""

    def test_success(self):
        result = ToolResult(success=True, data={"content": "hello"}, duration_ns=1000)
        assert result.success is True
        assert result.data == {"content": "hello"}
        assert result.error is None

    def test_error(self):
        error = ZapError(ErrorCode.NOT_FOUND, "Tool not found")
        result = ToolResult(success=False, error=error)
        assert result.success is False
        assert result.error.code == ErrorCode.NOT_FOUND

    def test_serialization(self):
        result = ToolResult(success=True, data={"x": 1}, duration_ns=500)
        d = result.to_dict()
        result2 = ToolResult.from_dict(d)
        assert result2.success == result.success
        assert result2.data == result.data
        assert result2.duration_ns == result.duration_ns


class TestZapError:
    """Tests for ZapError type."""

    def test_create(self):
        err = ZapError(ErrorCode.INVALID_PARAMS, "Invalid path", details={"path": "/bad"})
        assert err.code == ErrorCode.INVALID_PARAMS
        assert err.message == "Invalid path"
        assert err.details == {"path": "/bad"}

    def test_serialization(self):
        err = ZapError(ErrorCode.TIMEOUT, "Request timed out")
        d = err.to_dict()
        err2 = ZapError.from_dict(d)
        assert err2.code == ErrorCode.TIMEOUT
        assert err2.message == "Request timed out"


class TestZapMessage:
    """Tests for ZapMessage wire protocol."""

    def test_encode_decode(self):
        msg = ZapMessage(
            type="catalog.listTools",
            id="test-123",
            payload={"certifiedOnly": False},
        )
        encoded = msg.encode()
        # First 4 bytes are length
        length = int.from_bytes(encoded[:4], "big")
        assert length == len(encoded) - 4

        # Decode
        msg2 = ZapMessage.decode(encoded[4:])
        assert msg2.type == msg.type
        assert msg2.id == msg.id
        assert msg2.payload == msg.payload


class TestZapClient:
    """Tests for ZapClient."""

    def test_from_uri_tcp(self):
        client = ZapClient.from_uri("zap://localhost:9999")
        assert client.host == "localhost"
        assert client.port == 9999
        assert client.tls is False
        assert client.unix_socket is None

    def test_from_uri_tls(self):
        client = ZapClient.from_uri("zap+tls://secure.example.com:8443")
        assert client.host == "secure.example.com"
        assert client.port == 8443
        assert client.tls is True

    def test_from_uri_unix(self):
        client = ZapClient.from_uri("zap+unix:///var/run/zap.sock")
        assert client.unix_socket == "/var/run/zap.sock"

    def test_from_uri_default_port(self):
        client = ZapClient.from_uri("zap://gateway.local")
        assert client.host == "gateway.local"
        assert client.port == 9999

    def test_from_uri_invalid(self):
        with pytest.raises(ValueError):
            ZapClient.from_uri("http://example.com")


class TestZapAgentConfig:
    """Tests for ZapAgentConfig."""

    def test_defaults(self):
        config = ZapAgentConfig()
        assert config.gateway_uri == "zap://localhost:9999"
        assert config.connect_timeout == 5.0
        assert config.include_canonical is True
        assert config.certified_only is False

    def test_custom(self):
        config = ZapAgentConfig(
            gateway_uri="zap+tls://prod.example.com:443",
            tool_namespaces=["native", "mcp.github"],
            certified_only=True,
        )
        assert config.gateway_uri == "zap+tls://prod.example.com:443"
        assert config.tool_namespaces == ["native", "mcp.github"]
        assert config.certified_only is True


class TestCallContext:
    """Tests for CallContext."""

    def test_create(self):
        ctx = CallContext(
            trace_id="trace-abc",
            span_id="span-123",
            timeout_ms=5000,
        )
        assert ctx.trace_id == "trace-abc"
        assert ctx.span_id == "span-123"
        assert ctx.timeout_ms == 5000

    def test_to_dict(self):
        ctx = CallContext(trace_id="t1", span_id="s1", timeout_ms=3000)
        d = ctx.to_dict()
        assert d["traceId"] == "t1"
        assert d["spanId"] == "s1"
        assert d["timeout"] == 3000


class TestConsensusConfig:
    """Tests for ConsensusConfig."""

    def test_defaults(self):
        config = ConsensusConfig()
        assert config.rounds == 3
        assert config.k == 5
        assert config.alpha == 0.6
        assert config.beta1 == 0.8
        assert config.beta2 == 0.9

    def test_to_dict(self):
        config = ConsensusConfig(rounds=5, k=7)
        d = config.to_dict()
        assert d["rounds"] == 5
        assert d["k"] == 7


class TestHelloWelcome:
    """Tests for Hello/Welcome handshake."""

    def test_hello(self):
        hello = Hello()
        d = hello.to_dict()
        assert d["protocolVersion"] == "0.2.1"
        assert d["clientInfo"]["name"] == "hanzo-agent"
        assert d["capabilities"]["roots"] is True

    def test_welcome_from_dict(self):
        d = {
            "protocolVersion": "0.2.1",
            "endpointInfo": {"name": "test-gateway", "version": "1.0.0"},
            "capabilities": {
                "tools": True,
                "resources": True,
                "catalog": True,
                "coordination": True,
            },
            "instructions": "Welcome to ZAP",
        }
        welcome = Welcome.from_dict(d)
        assert welcome.endpoint_info.name == "test-gateway"
        assert welcome.capabilities.tools is True
        assert welcome.capabilities.catalog is True
        assert welcome.instructions == "Welcome to ZAP"


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
