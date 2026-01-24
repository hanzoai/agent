"""Async ZAP client for connecting to ZAP gateways.

Implements the ZAP wire protocol using length-prefixed JSON messages.
For production use with Cap'n Proto, the message encoding can be swapped
while maintaining the same interface.
"""

from __future__ import annotations

import asyncio
import time
import uuid
from dataclasses import dataclass, field
from typing import Any

from .types import (
    CallContext,
    Certificate,
    ConsensusConfig,
    ConsensusResult,
    EndpointCaps,
    ErrorCode,
    Hello,
    Resource,
    Tool,
    ToolId,
    ToolResult,
    Welcome,
    ZapError,
    ZapMessage,
)


class ZapConnectionError(Exception):
    """Raised when connection to ZAP gateway fails."""

    pass


class ZapProtocolError(Exception):
    """Raised when protocol-level error occurs."""

    def __init__(self, error: ZapError):
        self.error = error
        super().__init__(f"{error.code.value}: {error.message}")


@dataclass
class ZapClient:
    """Async client for ZAP gateway connections.

    Supports TCP, Unix socket, and TLS transports via URI scheme:
    - zap://host:port - TCP
    - zap+unix:///path/to/socket - Unix socket
    - zap+tls://host:port - TLS over TCP

    Example:
        async with ZapClient("localhost", 9999) as client:
            tools = await client.list_tools()
            result = await client.call_tool("fs.read", {"path": "/etc/hosts"})
    """

    host: str
    port: int
    tls: bool = False
    unix_socket: str | None = None
    connect_timeout: float = 5.0
    request_timeout: float = 30.0

    _reader: asyncio.StreamReader | None = field(default=None, repr=False)
    _writer: asyncio.StreamWriter | None = field(default=None, repr=False)
    _welcome: Welcome | None = field(default=None, repr=False)
    _pending: dict[str, asyncio.Future[dict[str, Any]]] = field(
        default_factory=dict, repr=False
    )
    _recv_task: asyncio.Task[None] | None = field(default=None, repr=False)
    _connected: bool = field(default=False, repr=False)
    _lock: asyncio.Lock = field(default_factory=asyncio.Lock, repr=False)

    @classmethod
    def from_uri(cls, uri: str) -> ZapClient:
        """Create client from ZAP URI.

        Supported schemes:
        - zap://host:port
        - zap+unix:///path/to/socket
        - zap+tls://host:port
        """
        if uri.startswith("zap+unix://"):
            path = uri[len("zap+unix://") :]
            return cls(host="", port=0, unix_socket=path)
        elif uri.startswith("zap+tls://"):
            hostport = uri[len("zap+tls://") :]
            host, port_str = hostport.rsplit(":", 1) if ":" in hostport else (hostport, "9999")
            return cls(host=host, port=int(port_str), tls=True)
        elif uri.startswith("zap://"):
            hostport = uri[len("zap://") :]
            host, port_str = hostport.rsplit(":", 1) if ":" in hostport else (hostport, "9999")
            return cls(host=host, port=int(port_str))
        else:
            raise ValueError(f"Invalid ZAP URI: {uri}")

    async def connect(self) -> Welcome:
        """Connect to ZAP gateway and perform handshake.

        Returns the Welcome message with server capabilities.
        """
        if self._connected:
            if self._welcome:
                return self._welcome
            raise ZapConnectionError("Already connected but no welcome received")

        try:
            if self.unix_socket:
                self._reader, self._writer = await asyncio.wait_for(
                    asyncio.open_unix_connection(self.unix_socket),
                    timeout=self.connect_timeout,
                )
            else:
                ssl_context = None
                if self.tls:
                    import ssl

                    ssl_context = ssl.create_default_context()
                self._reader, self._writer = await asyncio.wait_for(
                    asyncio.open_connection(self.host, self.port, ssl=ssl_context),
                    timeout=self.connect_timeout,
                )
        except asyncio.TimeoutError:
            raise ZapConnectionError(
                f"Connection timeout to {self.host}:{self.port}"
            ) from None
        except OSError as e:
            raise ZapConnectionError(f"Connection failed: {e}") from e

        self._connected = True
        self._recv_task = asyncio.create_task(self._receive_loop())

        # Perform handshake
        hello = Hello()
        response = await self._request("initialize", hello.to_dict())
        self._welcome = Welcome.from_dict(response)
        return self._welcome

    async def close(self) -> None:
        """Close the connection."""
        self._connected = False
        if self._recv_task:
            self._recv_task.cancel()
            try:
                await self._recv_task
            except asyncio.CancelledError:
                pass
            self._recv_task = None

        if self._writer:
            self._writer.close()
            try:
                await self._writer.wait_closed()
            except Exception:
                pass
            self._writer = None
            self._reader = None

        # Cancel any pending requests
        for future in self._pending.values():
            if not future.done():
                future.cancel()
        self._pending.clear()

    async def __aenter__(self) -> ZapClient:
        await self.connect()
        return self

    async def __aexit__(self, *args: Any) -> None:
        await self.close()

    @property
    def capabilities(self) -> EndpointCaps | None:
        """Get server capabilities from handshake."""
        return self._welcome.capabilities if self._welcome else None

    @property
    def is_connected(self) -> bool:
        """Check if client is connected."""
        return self._connected and self._writer is not None

    # =========================================================================
    # Wire Protocol
    # =========================================================================

    async def _send(self, msg: ZapMessage) -> None:
        """Send a message to the server."""
        if not self._writer:
            raise ZapConnectionError("Not connected")
        data = msg.encode()
        self._writer.write(data)
        await self._writer.drain()

    async def _recv(self) -> ZapMessage:
        """Receive a message from the server."""
        if not self._reader:
            raise ZapConnectionError("Not connected")

        # Read 4-byte length prefix
        length_bytes = await self._reader.readexactly(4)
        length = int.from_bytes(length_bytes, "big")

        # Read message body
        data = await self._reader.readexactly(length)
        return ZapMessage.decode(data)

    async def _receive_loop(self) -> None:
        """Background task to receive messages."""
        try:
            while self._connected:
                try:
                    msg = await self._recv()
                except asyncio.IncompleteReadError:
                    break
                except Exception:
                    if self._connected:
                        # Log error but continue
                        pass
                    break

                # Route response to pending request
                if msg.id in self._pending:
                    future = self._pending.pop(msg.id)
                    if msg.type == "error":
                        error = ZapError.from_dict(msg.payload)
                        future.set_exception(ZapProtocolError(error))
                    else:
                        future.set_result(msg.payload)
        except asyncio.CancelledError:
            pass

    async def _request(
        self, method: str, params: dict[str, Any], timeout: float | None = None
    ) -> dict[str, Any]:
        """Send request and wait for response."""
        if not self.is_connected:
            raise ZapConnectionError("Not connected")

        msg_id = str(uuid.uuid4())
        msg = ZapMessage(type=method, id=msg_id, payload=params)

        future: asyncio.Future[dict[str, Any]] = asyncio.get_event_loop().create_future()
        self._pending[msg_id] = future

        try:
            await self._send(msg)
            return await asyncio.wait_for(
                future, timeout=timeout or self.request_timeout
            )
        except asyncio.TimeoutError:
            self._pending.pop(msg_id, None)
            raise ZapProtocolError(
                ZapError(ErrorCode.TIMEOUT, f"Request {method} timed out")
            ) from None

    # =========================================================================
    # Catalog Interface
    # =========================================================================

    async def list_tools(self, certified_only: bool = False) -> list[Tool]:
        """List all available tools from the gateway catalog.

        Args:
            certified_only: Only return tools with consensus certification.

        Returns:
            List of Tool definitions.
        """
        response = await self._request(
            "catalog.listTools",
            {"certifiedOnly": certified_only, "ctx": self._make_context().to_dict()},
        )
        return [Tool.from_dict(t) for t in response.get("tools", [])]

    async def get_tool(self, tool_id: str) -> Tool:
        """Get a specific tool by ID.

        Args:
            tool_id: Tool identifier (namespace/name or just name).

        Returns:
            Tool definition.
        """
        tid = ToolId.parse(tool_id)
        response = await self._request(
            "catalog.getTool",
            {"id": tid.to_dict(), "ctx": self._make_context().to_dict()},
        )
        return Tool.from_dict(response.get("tool", {}))

    async def search_tools(self, query: str) -> list[Tool]:
        """Search tools by query string.

        Args:
            query: Search query.

        Returns:
            List of matching tools.
        """
        response = await self._request(
            "catalog.search",
            {"query": query, "ctx": self._make_context().to_dict()},
        )
        return [Tool.from_dict(t) for t in response.get("tools", [])]

    async def call_tool(
        self,
        name: str,
        arguments: dict[str, Any],
        context: CallContext | None = None,
    ) -> ToolResult:
        """Call a tool on the gateway.

        Args:
            name: Tool name (can include namespace/version).
            arguments: Tool arguments.
            context: Optional call context with tracing info.

        Returns:
            ToolResult with success status and data/error.
        """
        tid = ToolId.parse(name)
        ctx = context or self._make_context()

        start_time = time.time_ns()
        try:
            response = await self._request(
                "catalog.invoke",
                {
                    "id": tid.to_dict(),
                    "args": arguments,
                    "ctx": ctx.to_dict(),
                },
            )
            duration = time.time_ns() - start_time
            return ToolResult(
                success=True,
                data=response.get("result"),
                duration_ns=duration,
            )
        except ZapProtocolError as e:
            duration = time.time_ns() - start_time
            return ToolResult(
                success=False,
                error=e.error,
                duration_ns=duration,
            )

    # =========================================================================
    # Resources Interface
    # =========================================================================

    async def list_resources(self, cursor: str | None = None) -> tuple[list[Resource], str | None]:
        """List available resources.

        Args:
            cursor: Pagination cursor.

        Returns:
            Tuple of (resources, next_cursor).
        """
        response = await self._request(
            "resources.list",
            {
                "cursor": {"token": cursor.encode() if cursor else b""},
                "ctx": self._make_context().to_dict(),
            },
        )
        page = response.get("page", {})
        resources = [Resource.from_dict(r) for r in page.get("resources", [])]
        next_cursor = page.get("nextCursor", {}).get("token")
        if next_cursor and isinstance(next_cursor, list):
            next_cursor = bytes(next_cursor).decode()
        return resources, next_cursor if page.get("hasMore") else None

    async def read_resource(self, uri: str) -> tuple[str, bytes]:
        """Read a resource by URI.

        Args:
            uri: Resource URI.

        Returns:
            Tuple of (mime_type, content).
        """
        response = await self._request(
            "resources.read",
            {"uri": uri, "ctx": self._make_context().to_dict()},
        )
        content = response.get("content", {})
        mime_type = content.get("mimeType", "text/plain")
        if "text" in content:
            return mime_type, content["text"].encode()
        elif "blob" in content:
            blob = content["blob"]
            if isinstance(blob, str):
                return mime_type, bytes.fromhex(blob)
            return mime_type, bytes(blob)
        return mime_type, b""

    # =========================================================================
    # Coordination Interface
    # =========================================================================

    async def propose_consensus(
        self,
        topic: bytes,
        proposal: bytes,
        config: ConsensusConfig | None = None,
    ) -> ConsensusResult:
        """Propose a value for consensus.

        Args:
            topic: Topic identifier.
            proposal: Proposed value.
            config: Optional consensus configuration.

        Returns:
            ConsensusResult with winner and certificate.
        """
        cfg = config or ConsensusConfig()
        response = await self._request(
            "coordination.propose",
            {
                "topic": topic.hex(),
                "proposal": proposal.hex(),
                "config": cfg.to_dict(),
                "ctx": self._make_context().to_dict(),
            },
        )
        return ConsensusResult.from_dict(response.get("result", {}))

    async def committee_query(
        self,
        question: str,
        participants: list[str],
        config: ConsensusConfig | None = None,
    ) -> tuple[str, Certificate]:
        """Query an LLM committee for consensus answer.

        Args:
            question: Question to ask the committee.
            participants: List of participant model/agent IDs.
            config: Optional consensus configuration.

        Returns:
            Tuple of (answer, certificate).
        """
        cfg = config or ConsensusConfig()
        response = await self._request(
            "coordination.committee",
            {
                "question": question,
                "participants": participants,
                "config": cfg.to_dict(),
                "ctx": self._make_context().to_dict(),
            },
        )
        answer = response.get("answer", "")
        cert_data = response.get("certificate", {})
        return answer, Certificate.from_dict(cert_data)

    # =========================================================================
    # Canonical Tools (convenience methods)
    # =========================================================================

    async def fs_read(
        self, path: str, offset: int = 0, limit: int = 2000
    ) -> dict[str, Any]:
        """Read a file (fs.read tool)."""
        result = await self.call_tool(
            "native/fs.read",
            {"path": path, "offset": offset, "limit": limit},
        )
        if not result.success:
            err = result.error or ZapError(ErrorCode.INTERNAL_ERROR, "Read failed")
            raise ZapProtocolError(err)
        return result.data

    async def fs_write(self, path: str, content: str, create_dirs: bool = False) -> str:
        """Write a file (fs.write tool)."""
        result = await self.call_tool(
            "native/fs.write",
            {"path": path, "content": content, "createDirs": create_dirs},
        )
        if not result.success:
            err = result.error or ZapError(ErrorCode.INTERNAL_ERROR, "Write failed")
            raise ZapProtocolError(err)
        return result.data.get("path", path)

    async def fs_glob(self, pattern: str, path: str = ".") -> list[str]:
        """Glob for files (fs.glob tool)."""
        result = await self.call_tool(
            "native/fs.glob",
            {"pattern": pattern, "path": path},
        )
        if not result.success:
            err = result.error or ZapError(ErrorCode.INTERNAL_ERROR, "Glob failed")
            raise ZapProtocolError(err)
        return result.data.get("paths", [])

    async def proc_run(
        self,
        command: str,
        args: list[str] | None = None,
        cwd: str | None = None,
        timeout: int = 120000,
    ) -> dict[str, Any]:
        """Run a process (proc.run tool)."""
        result = await self.call_tool(
            "native/proc.run",
            {
                "command": command,
                "args": args or [],
                "cwd": cwd or "",
                "timeout": timeout,
            },
        )
        if not result.success:
            err = result.error or ZapError(ErrorCode.INTERNAL_ERROR, "Run failed")
            raise ZapProtocolError(err)
        return result.data

    async def vcs_status(self) -> dict[str, Any]:
        """Get VCS status (vcs.status tool)."""
        result = await self.call_tool("native/vcs.status", {})
        if not result.success:
            err = result.error or ZapError(ErrorCode.INTERNAL_ERROR, "Status failed")
            raise ZapProtocolError(err)
        return result.data

    async def net_fetch(
        self,
        url: str,
        method: str = "GET",
        headers: dict[str, str] | None = None,
        body: bytes | None = None,
    ) -> dict[str, Any]:
        """Fetch a URL (net.fetch tool)."""
        result = await self.call_tool(
            "native/net.fetch",
            {
                "url": url,
                "method": method,
                "headers": [{"name": k, "value": v} for k, v in (headers or {}).items()],
                "body": body.hex() if body else "",
            },
        )
        if not result.success:
            err = result.error or ZapError(ErrorCode.INTERNAL_ERROR, "Fetch failed")
            raise ZapProtocolError(err)
        return result.data

    # =========================================================================
    # MCP Gateway Interface
    # =========================================================================

    async def list_mcp_tools(self) -> list[dict[str, Any]]:
        """List tools from connected MCP servers."""
        response = await self._request(
            "gateway.listMcpTools",
            {"ctx": self._make_context().to_dict()},
        )
        return response.get("tools", [])

    async def call_mcp_tool(self, name: str, json_args: str) -> str:
        """Call an MCP tool by name."""
        response = await self._request(
            "gateway.callMcpTool",
            {
                "name": name,
                "jsonArgs": json_args,
                "ctx": self._make_context().to_dict(),
            },
        )
        return response.get("jsonResult", "{}")

    async def register_mcp_server(self, name: str, endpoint: str) -> bool:
        """Register an MCP server with the gateway."""
        response = await self._request(
            "gateway.registerMcpServer",
            {
                "name": name,
                "endpoint": endpoint,
                "ctx": self._make_context().to_dict(),
            },
        )
        return response.get("success", False)

    # =========================================================================
    # Health & Utilities
    # =========================================================================

    async def ping(self) -> tuple[int, int]:
        """Ping the server.

        Returns:
            Tuple of (latency_ns, server_time).
        """
        start = time.time_ns()
        response = await self._request("ping", {})
        latency = time.time_ns() - start
        return latency, response.get("serverTime", 0)

    def _make_context(self) -> CallContext:
        """Create a default call context."""
        return CallContext(
            trace_id=str(uuid.uuid4()),
            span_id=str(uuid.uuid4())[:16],
            timeout_ms=int(self.request_timeout * 1000),
        )
