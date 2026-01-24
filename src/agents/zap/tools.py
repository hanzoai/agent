"""ZAP tool provider for Hanzo agents.

Discovers tools from ZAP gateway and wraps them as FunctionTool instances
compatible with the agent SDK.
"""

from __future__ import annotations

import json
from dataclasses import dataclass, field
from typing import Any

from ..run_context import RunContextWrapper
from ..tool import FunctionTool
from .client import ZapClient
from .types import Tool, ToolId


@dataclass
class ZapToolProvider:
    """Provides FunctionTool instances from ZAP gateway tools.

    Connects to a ZAP gateway, discovers available tools, and wraps them
    as FunctionTool instances that can be used with Hanzo agents.

    Example:
        provider = ZapToolProvider("localhost", 9999)
        await provider.connect()

        # Get all tools
        tools = await provider.get_tools()

        # Create agent with ZAP tools
        agent = Agent(name="my-agent", tools=tools)

        # Or filter by namespace
        fs_tools = await provider.get_tools(namespace="native", prefix="fs.")
    """

    host: str
    port: int
    tls: bool = False
    unix_socket: str | None = None

    _client: ZapClient | None = field(default=None, repr=False)
    _tools_cache: dict[str, Tool] = field(default_factory=dict, repr=False)
    _function_tools: dict[str, FunctionTool] = field(default_factory=dict, repr=False)

    @classmethod
    def from_uri(cls, uri: str) -> ZapToolProvider:
        """Create provider from ZAP URI."""
        client = ZapClient.from_uri(uri)
        return cls(
            host=client.host,
            port=client.port,
            tls=client.tls,
            unix_socket=client.unix_socket,
        )

    async def connect(self) -> None:
        """Connect to ZAP gateway and discover tools."""
        if self._client is not None:
            return

        self._client = ZapClient(
            host=self.host,
            port=self.port,
            tls=self.tls,
            unix_socket=self.unix_socket,
        )
        await self._client.connect()
        await self._refresh_tools()

    async def disconnect(self) -> None:
        """Disconnect from ZAP gateway."""
        if self._client:
            await self._client.close()
            self._client = None
        self._tools_cache.clear()
        self._function_tools.clear()

    async def __aenter__(self) -> ZapToolProvider:
        await self.connect()
        return self

    async def __aexit__(self, *args: Any) -> None:
        await self.disconnect()

    @property
    def client(self) -> ZapClient:
        """Get the underlying ZAP client."""
        if self._client is None:
            raise RuntimeError("Not connected. Call connect() first.")
        return self._client

    async def _refresh_tools(self) -> None:
        """Refresh tool cache from gateway."""
        tools = await self.client.list_tools()
        self._tools_cache = {str(t.id): t for t in tools}
        self._function_tools.clear()

    async def get_tools(
        self,
        namespace: str | None = None,
        prefix: str | None = None,
        certified_only: bool = False,
    ) -> list[FunctionTool]:
        """Get FunctionTool instances for ZAP tools.

        Args:
            namespace: Filter by namespace (e.g., "native", "mcp.github").
            prefix: Filter by name prefix (e.g., "fs.", "vcs.").
            certified_only: Only return tools with consensus certification.

        Returns:
            List of FunctionTool instances.
        """
        if not self._tools_cache:
            await self._refresh_tools()

        tools: list[FunctionTool] = []
        for tool_key, tool in self._tools_cache.items():
            # Apply filters
            if namespace and tool.id.namespace != namespace:
                continue
            if prefix and not tool.id.name.startswith(prefix):
                continue
            if certified_only and tool.stability.value != "stable":
                continue

            # Get or create FunctionTool
            if tool_key not in self._function_tools:
                self._function_tools[tool_key] = self._wrap_tool(tool)
            tools.append(self._function_tools[tool_key])

        return tools

    async def get_tool(self, name: str) -> FunctionTool:
        """Get a specific tool by name.

        Args:
            name: Tool name (with or without namespace/version).

        Returns:
            FunctionTool instance.
        """
        tool_id = ToolId.parse(name)
        tool_key = str(tool_id)

        if tool_key not in self._tools_cache:
            # Try to fetch from gateway
            tool = await self.client.get_tool(name)
            self._tools_cache[tool_key] = tool

        if tool_key not in self._function_tools:
            self._function_tools[tool_key] = self._wrap_tool(self._tools_cache[tool_key])

        return self._function_tools[tool_key]

    def _wrap_tool(self, tool: Tool) -> FunctionTool:
        """Wrap a ZAP Tool as a FunctionTool."""
        # Convert ZAP schema to JSON Schema for function tool
        json_schema = self._convert_schema(tool.input_schema)

        # Create the invocation function
        async def invoke_tool(ctx: RunContextWrapper[Any], input_json: str) -> str:
            return await self._invoke_tool(tool.full_name, input_json)

        return FunctionTool(
            name=tool.id.name.replace(".", "_"),  # Replace dots for compatibility
            description=tool.description,
            params_json_schema=json_schema,
            on_invoke_tool=invoke_tool,
            strict_json_schema=True,
        )

    async def _invoke_tool(self, tool_name: str, input_json: str) -> str:
        """Invoke a ZAP tool and return the result as string."""
        try:
            args = json.loads(input_json) if input_json else {}
        except json.JSONDecodeError:
            return json.dumps({"error": "Invalid JSON input"})

        result = await self.client.call_tool(tool_name, args)

        if result.success:
            if isinstance(result.data, str):
                return result.data
            return json.dumps(result.data)
        else:
            error_msg = result.error.message if result.error else "Unknown error"
            return json.dumps({"error": error_msg})

    def _convert_schema(self, zap_schema: dict[str, Any]) -> dict[str, Any]:
        """Convert ZAP schema to JSON Schema.

        ZAP uses Cap'n Proto schemas internally, but the input_schema
        field contains a JSON Schema representation for compatibility.
        """
        if not zap_schema:
            return {
                "type": "object",
                "properties": {},
                "required": [],
                "additionalProperties": False,
            }

        # ZAP schemas are already JSON Schema compatible
        schema = dict(zap_schema)
        schema.setdefault("type", "object")
        schema.setdefault("properties", {})
        schema.setdefault("required", [])
        schema.setdefault("additionalProperties", False)
        return schema


@dataclass
class ZapToolRegistry:
    """Registry for managing multiple ZAP tool providers.

    Useful when connecting to multiple ZAP gateways or managing
    tool namespaces from different sources.
    """

    _providers: dict[str, ZapToolProvider] = field(default_factory=dict)

    async def register(self, name: str, provider: ZapToolProvider) -> None:
        """Register a tool provider."""
        await provider.connect()
        self._providers[name] = provider

    async def unregister(self, name: str) -> None:
        """Unregister and disconnect a tool provider."""
        if name in self._providers:
            await self._providers[name].disconnect()
            del self._providers[name]

    async def close(self) -> None:
        """Close all providers."""
        for provider in self._providers.values():
            await provider.disconnect()
        self._providers.clear()

    async def get_all_tools(self) -> list[FunctionTool]:
        """Get all tools from all providers."""
        tools: list[FunctionTool] = []
        for provider in self._providers.values():
            tools.extend(await provider.get_tools())
        return tools

    async def get_tool(self, name: str) -> FunctionTool:
        """Get a tool by name, searching all providers."""
        ToolId.parse(name)

        # Try to find provider by namespace
        for _provider_name, provider in self._providers.items():
            try:
                return await provider.get_tool(name)
            except Exception:
                continue

        raise KeyError(f"Tool not found: {name}")


# Convenience function for creating tools from ZAP
async def create_zap_tools(
    uri: str,
    namespace: str | None = None,
    prefix: str | None = None,
) -> list[FunctionTool]:
    """Create FunctionTool instances from a ZAP gateway.

    This is a convenience function for quickly getting tools from a gateway.
    For more control, use ZapToolProvider directly.

    Args:
        uri: ZAP gateway URI (e.g., "zap://localhost:9999").
        namespace: Filter by namespace.
        prefix: Filter by name prefix.

    Returns:
        List of FunctionTool instances.

    Example:
        tools = await create_zap_tools("zap://localhost:9999", prefix="fs.")
        agent = Agent(name="file-agent", tools=tools)
    """
    provider = ZapToolProvider.from_uri(uri)
    await provider.connect()
    try:
        return await provider.get_tools(namespace=namespace, prefix=prefix)
    finally:
        await provider.disconnect()


def create_canonical_tools(client: ZapClient) -> list[FunctionTool]:
    """Create FunctionTool wrappers for canonical ZAP tools.

    These are the core tools that map to Claude Code tools:
    - fs.read, fs.write, fs.edit, fs.glob (Read, Write, Edit, Glob)
    - proc.run (Bash)
    - vcs.status, vcs.diff, vcs.commit (git operations)
    - net.fetch, net.search (WebFetch, WebSearch)

    Args:
        client: Connected ZapClient instance.

    Returns:
        List of FunctionTool instances for canonical tools.
    """
    tools: list[FunctionTool] = []

    # fs.read
    async def fs_read(ctx: RunContextWrapper[Any], input_json: str) -> str:
        args = json.loads(input_json) if input_json else {}
        result = await client.fs_read(
            path=args.get("path", ""),
            offset=args.get("offset", 0),
            limit=args.get("limit", 2000),
        )
        return json.dumps(result)

    tools.append(FunctionTool(
        name="fs_read",
        description="Read a file from the filesystem. Returns content, mime type, and size.",
        params_json_schema={
            "type": "object",
            "properties": {
                "path": {"type": "string", "description": "Absolute path to file"},
                "offset": {
                    "type": "integer",
                    "description": "Line offset to start reading",
                    "default": 0,
                },
                "limit": {
                    "type": "integer",
                    "description": "Maximum lines to read",
                    "default": 2000,
                },
            },
            "required": ["path"],
            "additionalProperties": False,
        },
        on_invoke_tool=fs_read,
    ))

    # fs.write
    async def fs_write(ctx: RunContextWrapper[Any], input_json: str) -> str:
        args = json.loads(input_json) if input_json else {}
        path = await client.fs_write(
            path=args.get("path", ""),
            content=args.get("content", ""),
            create_dirs=args.get("createDirs", False),
        )
        return json.dumps({"path": path, "success": True})

    tools.append(FunctionTool(
        name="fs_write",
        description="Write content to a file. Creates the file if it doesn't exist.",
        params_json_schema={
            "type": "object",
            "properties": {
                "path": {"type": "string", "description": "Absolute path to file"},
                "content": {"type": "string", "description": "Content to write"},
                "createDirs": {
                    "type": "boolean",
                    "description": "Create parent directories",
                    "default": False,
                },
            },
            "required": ["path", "content"],
            "additionalProperties": False,
        },
        on_invoke_tool=fs_write,
    ))

    # fs.glob
    async def fs_glob(ctx: RunContextWrapper[Any], input_json: str) -> str:
        args = json.loads(input_json) if input_json else {}
        paths = await client.fs_glob(
            pattern=args.get("pattern", ""),
            path=args.get("path", "."),
        )
        return json.dumps({"paths": paths})

    tools.append(FunctionTool(
        name="fs_glob",
        description="Find files matching a glob pattern.",
        params_json_schema={
            "type": "object",
            "properties": {
                "pattern": {"type": "string", "description": "Glob pattern (e.g., '**/*.py')"},
                "path": {"type": "string", "description": "Base path to search", "default": "."},
            },
            "required": ["pattern"],
            "additionalProperties": False,
        },
        on_invoke_tool=fs_glob,
    ))

    # proc.run (Bash equivalent)
    async def proc_run(ctx: RunContextWrapper[Any], input_json: str) -> str:
        args = json.loads(input_json) if input_json else {}
        result = await client.proc_run(
            command=args.get("command", ""),
            args=args.get("args"),
            cwd=args.get("cwd"),
            timeout=args.get("timeout", 120000),
        )
        return json.dumps(result)

    tools.append(FunctionTool(
        name="proc_run",
        description="Execute a command. Returns exit code, stdout, and stderr.",
        params_json_schema={
            "type": "object",
            "properties": {
                "command": {"type": "string", "description": "Command to execute"},
                "args": {
                    "type": "array",
                    "items": {"type": "string"},
                    "description": "Command arguments",
                },
                "cwd": {"type": "string", "description": "Working directory"},
                "timeout": {
                    "type": "integer",
                    "description": "Timeout in milliseconds",
                    "default": 120000,
                },
            },
            "required": ["command"],
            "additionalProperties": False,
        },
        on_invoke_tool=proc_run,
    ))

    # vcs.status
    async def vcs_status(ctx: RunContextWrapper[Any], input_json: str) -> str:
        result = await client.vcs_status()
        return json.dumps(result)

    tools.append(FunctionTool(
        name="vcs_status",
        description="Get VCS status. Returns branch, staged/modified files, etc.",
        params_json_schema={
            "type": "object",
            "properties": {},
            "required": [],
            "additionalProperties": False,
        },
        on_invoke_tool=vcs_status,
    ))

    # net.fetch
    async def net_fetch(ctx: RunContextWrapper[Any], input_json: str) -> str:
        args = json.loads(input_json) if input_json else {}
        result = await client.net_fetch(
            url=args.get("url", ""),
            method=args.get("method", "GET"),
            headers=args.get("headers"),
            body=bytes.fromhex(args["body"]) if args.get("body") else None,
        )
        return json.dumps(result)

    tools.append(FunctionTool(
        name="net_fetch",
        description="Fetch content from a URL. Returns status, headers, and body.",
        params_json_schema={
            "type": "object",
            "properties": {
                "url": {"type": "string", "description": "URL to fetch"},
                "method": {"type": "string", "description": "HTTP method", "default": "GET"},
                "headers": {"type": "object", "description": "Request headers"},
                "body": {"type": "string", "description": "Request body (hex encoded)"},
            },
            "required": ["url"],
            "additionalProperties": False,
        },
        on_invoke_tool=net_fetch,
    ))

    return tools
