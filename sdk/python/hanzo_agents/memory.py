"""
Cross-Agent Persistent Memory Client for Hanzo Agents SDK.

This module provides the memory interface that enables seamless, automatic memory
sharing and synchronization across distributed agents.
"""

import asyncio
import json
import sys
from functools import wraps
from typing import Any, Callable, Dict, List, Optional, Sequence, Union
from .client import HanzoAgentsClient
from .execution_context import ExecutionContext
from .memory_events import MemoryEventClient, ScopedMemoryEventClient


# Python 3.8 compatibility: asyncio.to_thread was added in Python 3.9
if sys.version_info >= (3, 9):
    from asyncio import to_thread as _to_thread
else:
    async def _to_thread(func, *args, **kwargs):
        """Compatibility shim for asyncio.to_thread on Python 3.8."""
        loop = asyncio.get_event_loop()
        return await loop.run_in_executor(None, lambda: func(*args, **kwargs))


def _vector_to_list(values: Union[Sequence[float], Any]) -> List[float]:
    """
    Normalize numpy arrays, tuples, or other sequences to a plain float list.
    """
    if hasattr(values, "tolist"):
        values = values.tolist()
    return [float(x) for x in values]  # type: ignore[arg-type]


class MemoryClient:
    """
    Core memory client that communicates with the Hanzo Agents server's memory API.

    This client handles the low-level HTTP operations for memory management
    and automatically includes execution context headers for proper scoping.
    """

    def __init__(
        self,
        hanzo_agents_client: HanzoAgentsClient,
        execution_context: ExecutionContext,
        agent_node_id: Optional[str] = None,
    ):
        self.hanzo_agents_client = hanzo_agents_client
        self.execution_context = execution_context
        self.agent_node_id = agent_node_id

    def _build_headers(
        self, scope: Optional[str] = None, scope_id: Optional[str] = None
    ) -> Dict[str, str]:
        """Merge execution context headers with explicit scope overrides."""

        headers = self.execution_context.to_headers()

        if (not headers.get("X-Agent-Node-ID")) and self.agent_node_id:
            headers["X-Agent-Node-ID"] = self.agent_node_id

        if scope_id is not None:
            header_name = {
                "workflow": "X-Workflow-ID",
                "session": "X-Session-ID",
                "actor": "X-Actor-ID",
            }.get(scope or "")

            if header_name:
                headers[header_name] = scope_id

        return headers

    async def _async_request(self, method: str, url: str, **kwargs):
        """Internal helper to perform HTTP requests with graceful fallbacks."""
        if hasattr(self.hanzo_agents_client, "_async_request"):
            return await self.hanzo_agents_client._async_request(method, url, **kwargs)

        try:
            import httpx

            async with httpx.AsyncClient() as client:
                return await client.request(method, url, **kwargs)
        except ImportError:
            import requests

            return await _to_thread(requests.request, method, url, **kwargs)

    async def set(
        self, key: str, data: Any, scope: Optional[str] = None, scope_id: Optional[str] = None
    ) -> None:
        """
        Set a memory value with automatic scoping.

        Args:
            key: The memory key
            data: The data to store (will be JSON serialized)
            scope: Optional explicit scope override
        """
        from hanzo_agents.logger import log_debug

        headers = self._build_headers(scope, scope_id)

        payload = {"key": key, "data": data}

        if scope:
            payload["scope"] = scope

        # Test JSON serialization before sending
        try:
            json.dumps(payload)
            log_debug(f"Memory set operation for key: {key}")
        except Exception as json_error:
            log_debug(
                f"JSON serialization failed for memory key {key}: {type(json_error).__name__}: {json_error}"
            )
            raise

        # Use synchronous requests to avoid event loop conflicts with Hanzo Agents SDK
        url = f"{self.hanzo_agents_client.api_base}/memory/set"

        try:
            if hasattr(self.hanzo_agents_client, "_async_request"):
                response = await self.hanzo_agents_client._async_request(
                    "POST",
                    url,
                    json=payload,
                    headers=headers,
                    timeout=10.0,
                )
            else:
                import requests

                response = await _to_thread(
                    requests.post,
                    url,
                    json=payload,
                    headers=headers,
                    timeout=10.0,
                )
            response.raise_for_status()
            log_debug(f"Memory set successful for key: {key}")
        except Exception as e:
            log_debug(f"Memory set failed for key {key}: {type(e).__name__}: {e}")
            raise

    async def set_vector(
        self,
        key: str,
        embedding: Union[Sequence[float], Any],
        metadata: Optional[Dict[str, Any]] = None,
        scope: Optional[str] = None,
        scope_id: Optional[str] = None,
    ) -> None:
        """
        Store a vector embedding with optional metadata.
        """
        headers = self._build_headers(scope, scope_id)
        payload: Dict[str, Any] = {
            "key": key,
            "embedding": _vector_to_list(embedding),
        }
        if metadata:
            payload["metadata"] = metadata
        if scope:
            payload["scope"] = scope

        response = await self._async_request(
            "POST",
            f"{self.hanzo_agents_client.api_base}/memory/vector/set",
            json=payload,
            headers=headers,
            timeout=15.0,
        )
        response.raise_for_status()

    async def get(
        self,
        key: str,
        default: Any = None,
        scope: Optional[str] = None,
        scope_id: Optional[str] = None,
    ) -> Any:
        """
        Get a memory value with hierarchical lookup.

        Args:
            key: The memory key
            default: Default value if key not found
            scope: Optional explicit scope override

        Returns:
            The stored value or default if not found
        """
        headers = self._build_headers(scope, scope_id)

        payload = {"key": key}

        if scope:
            payload["scope"] = scope

        response = await self._async_request(
            "POST",
            f"{self.hanzo_agents_client.api_base}/memory/get",
            json=payload,
            headers=headers,
            timeout=10.0,
        )

        if response.status_code == 404:
            return default

        response.raise_for_status()
        result = response.json()

        # Extract the actual data from the memory response
        if isinstance(result, dict) and "data" in result:
            # The server returns JSON-encoded data, so we need to decode it
            data = result["data"]
            if isinstance(data, str):
                try:
                    return json.loads(data)
                except json.JSONDecodeError:
                    return data
            return data

        return result

    async def exists(
        self, key: str, scope: Optional[str] = None, scope_id: Optional[str] = None
    ) -> bool:
        """
        Check if a memory key exists.

        Args:
            key: The memory key
            scope: Optional explicit scope override

        Returns:
            True if key exists, False otherwise
        """
        try:
            await self.get(key, scope=scope, scope_id=scope_id)
            return True
        except Exception:
            return False

    async def delete(
        self, key: str, scope: Optional[str] = None, scope_id: Optional[str] = None
    ) -> None:
        """
        Delete a memory value.

        Args:
            key: The memory key
            scope: Optional explicit scope override
        """
        headers = self._build_headers(scope, scope_id)

        payload = {"key": key}

        if scope:
            payload["scope"] = scope

        response = await self._async_request(
            "POST",
            f"{self.hanzo_agents_client.api_base}/memory/delete",
            json=payload,
            headers=headers,
            timeout=10.0,
        )
        response.raise_for_status()

    async def delete_vector(
        self, key: str, scope: Optional[str] = None, scope_id: Optional[str] = None
    ) -> None:
        """
        Delete a stored vector embedding.
        """
        headers = self._build_headers(scope, scope_id)
        payload: Dict[str, Any] = {"key": key}
        if scope:
            payload["scope"] = scope
        response = await self._async_request(
            "POST",
            f"{self.hanzo_agents_client.api_base}/memory/vector/delete",
            json=payload,
            headers=headers,
            timeout=10.0,
        )
        response.raise_for_status()

    async def list_keys(
        self, scope: str, scope_id: Optional[str] = None
    ) -> List[str]:
        """
        List all keys in a specific scope.

        Args:
            scope: The scope to list keys from

        Returns:
            List of memory keys in the scope
        """
        headers = self._build_headers(scope, scope_id)

        response = await self._async_request(
            "GET",
            f"{self.hanzo_agents_client.api_base}/memory/list",
            params={"scope": scope},
            headers=headers,
            timeout=10.0,
        )
        response.raise_for_status()
        result = response.json()

        # Extract keys from the memory list response
        if isinstance(result, list):
            return [item.get("key", "") for item in result if "key" in item]

        return []

    async def similarity_search(
        self,
        query_embedding: Union[Sequence[float], Any],
        top_k: int = 10,
        scope: Optional[str] = None,
        scope_id: Optional[str] = None,
        filters: Optional[Dict[str, Any]] = None,
    ) -> List[Dict[str, Any]]:
        """
        Perform a similarity search against stored vectors.
        """
        headers = self._build_headers(scope, scope_id)
        payload: Dict[str, Any] = {
            "query_embedding": _vector_to_list(query_embedding),
            "top_k": top_k,
            "filters": filters or {},
        }
        if scope:
            payload["scope"] = scope

        response = await self._async_request(
            "POST",
            f"{self.hanzo_agents_client.api_base}/memory/vector/search",
            json=payload,
            headers=headers,
            timeout=15.0,
        )
        response.raise_for_status()
        return response.json()


class ScopedMemoryClient:
    """
    Memory client that operates within a specific scope.

    This provides a scoped view of memory operations, automatically
    using the specified scope for all operations.
    """

    def __init__(
        self,
        memory_client: MemoryClient,
        scope: str,
        scope_id: str,
        event_client: Optional[MemoryEventClient] = None,
    ):
        self.memory_client = memory_client
        self.scope = scope
        self.scope_id = scope_id
        self.events = (
            ScopedMemoryEventClient(event_client, scope, scope_id)
            if event_client
            else None
        )

    async def set(self, key: str, data: Any) -> None:
        """Set a value in this specific scope."""
        await self.memory_client.set(
            key, data, scope=self.scope, scope_id=self.scope_id
        )

    async def get(self, key: str, default: Any = None) -> Any:
        """Get a value from this specific scope."""
        return await self.memory_client.get(
            key, default=default, scope=self.scope, scope_id=self.scope_id
        )

    async def exists(self, key: str) -> bool:
        """Check if a key exists in this specific scope."""
        return await self.memory_client.exists(
            key, scope=self.scope, scope_id=self.scope_id
        )

    async def delete(self, key: str) -> None:
        """Delete a value from this specific scope."""
        await self.memory_client.delete(
            key, scope=self.scope, scope_id=self.scope_id
        )

    async def list_keys(self) -> List[str]:
        """List all keys in this specific scope."""
        return await self.memory_client.list_keys(self.scope, scope_id=self.scope_id)

    async def set_vector(
        self,
        key: str,
        embedding: Union[Sequence[float], Any],
        metadata: Optional[Dict[str, Any]] = None,
    ) -> None:
        """Store a vector within this scope."""
        await self.memory_client.set_vector(
            key,
            embedding,
            metadata=metadata,
            scope=self.scope,
            scope_id=self.scope_id,
        )

    async def delete_vector(self, key: str) -> None:
        """Delete a vector within this scope."""
        await self.memory_client.delete_vector(
            key, scope=self.scope, scope_id=self.scope_id
        )

    async def similarity_search(
        self,
        query_embedding: Union[Sequence[float], Any],
        top_k: int = 10,
        filters: Optional[Dict[str, Any]] = None,
    ) -> List[Dict[str, Any]]:
        """Search vectors within this scope."""
        return await self.memory_client.similarity_search(
            query_embedding,
            top_k=top_k,
            scope=self.scope,
            scope_id=self.scope_id,
            filters=filters,
        )

    def on_change(self, patterns: Union[str, List[str]]):
        """
        Decorator for subscribing to memory change events in this scope.

        Args:
            patterns: Pattern(s) to match against memory keys

        Returns:
            Decorator function
        """
        if self.events:
            return self.events.on_change(patterns)
        else:
            # Return a no-op decorator if events are not available
            def decorator(func):
                return func

            return decorator


class GlobalMemoryClient:
    """
    Memory client for global scope operations.

    This provides access to the global memory scope that is shared
    across all agents and sessions.
    """

    def __init__(
        self,
        memory_client: MemoryClient,
        event_client: Optional[MemoryEventClient] = None,
    ):
        self.memory_client = memory_client
        self.event_client = event_client

    async def set(self, key: str, data: Any) -> None:
        """Set a value in global scope."""
        await self.memory_client.set(key, data, scope="global")

    async def get(self, key: str, default: Any = None) -> Any:
        """Get a value from global scope."""
        return await self.memory_client.get(key, default=default, scope="global")

    async def exists(self, key: str) -> bool:
        """Check if a key exists in global scope."""
        return await self.memory_client.exists(key, scope="global")

    async def delete(self, key: str) -> None:
        """Delete a value from global scope."""
        await self.memory_client.delete(key, scope="global")

    async def list_keys(self) -> List[str]:
        """List all keys in global scope."""
        return await self.memory_client.list_keys("global")

    async def set_vector(
        self,
        key: str,
        embedding: Union[Sequence[float], Any],
        metadata: Optional[Dict[str, Any]] = None,
    ) -> None:
        """Store a vector in global scope."""
        await self.memory_client.set_vector(
            key, embedding, metadata=metadata, scope="global"
        )

    async def delete_vector(self, key: str) -> None:
        """Delete a vector in global scope."""
        await self.memory_client.delete_vector(key, scope="global")

    async def similarity_search(
        self,
        query_embedding: Union[Sequence[float], Any],
        top_k: int = 10,
        filters: Optional[Dict[str, Any]] = None,
    ) -> List[Dict[str, Any]]:
        """Search vectors in global scope."""
        return await self.memory_client.similarity_search(
            query_embedding, top_k=top_k, scope="global", filters=filters
        )

    def on_change(self, patterns: Union[str, List[str]]) -> Callable:
        """
        Decorator for subscribing to global-scope memory change events.

        Args:
            patterns: Pattern(s) to match against memory keys

        Returns:
            Decorator function
        """

        if not self.event_client:
            # No event client available (e.g., during unit tests) — return no-op decorator
            def decorator(func: Callable) -> Callable:
                return func

            return decorator

        def decorator(func: Callable) -> Callable:
            @wraps(func)
            async def wrapper(event):
                return await func(event)

            self.event_client.subscribe(
                patterns,
                wrapper,
                scope="global",
                scope_id=None,
            )

            setattr(wrapper, "_memory_event_listener", True)
            setattr(
                wrapper,
                "_memory_event_patterns",
                patterns if isinstance(patterns, list) else [patterns],
            )
            setattr(wrapper, "_memory_event_scope", "global")
            setattr(wrapper, "_memory_event_scope_id", None)

            return wrapper

        return decorator


class MemoryInterface:
    """
    Developer-facing memory interface that provides the intuitive app.memory API.

    This class provides the main interface that developers interact with,
    offering automatic scoping, hierarchical lookup, and explicit scope access.
    """

    def __init__(self, memory_client: MemoryClient, event_client: MemoryEventClient):
        self.memory_client = memory_client
        self.events = event_client

    async def set(self, key: str, data: Any) -> None:
        """
        Set a memory value with automatic scoping.

        The value will be stored in the most specific available scope
        based on the current execution context.

        Args:
            key: The memory key
            data: The data to store
        """
        await self.memory_client.set(key, data)

    async def set_vector(
        self,
        key: str,
        embedding: Union[Sequence[float], Any],
        metadata: Optional[Dict[str, Any]] = None,
    ) -> None:
        """
        Store a vector embedding with automatic scoping.
        """
        await self.memory_client.set_vector(key, embedding, metadata=metadata)

    async def get(self, key: str, default: Any = None) -> Any:
        """
        Get a memory value with hierarchical lookup.

        This will search through scopes in order: workflow -> session -> actor -> global
        and return the first match found.

        Args:
            key: The memory key
            default: Default value if key not found in any scope

        Returns:
            The stored value or default if not found
        """
        return await self.memory_client.get(key, default=default)

    async def exists(self, key: str) -> bool:
        """
        Check if a memory key exists in any scope.

        Args:
            key: The memory key

        Returns:
            True if key exists in any scope, False otherwise
        """
        return await self.memory_client.exists(key)

    async def delete(self, key: str) -> None:
        """
        Delete a memory value from the current scope.

        Args:
            key: The memory key
        """
        await self.memory_client.delete(key)

    async def delete_vector(self, key: str) -> None:
        """
        Delete a vector embedding from the current scope.
        """
        await self.memory_client.delete_vector(key)

    async def similarity_search(
        self,
        query_embedding: Union[Sequence[float], Any],
        top_k: int = 10,
        filters: Optional[Dict[str, Any]] = None,
    ) -> List[Dict[str, Any]]:
        """
        Search stored vectors using similarity matching.
        """
        return await self.memory_client.similarity_search(
            query_embedding, top_k=top_k, filters=filters
        )

    def on_change(self, patterns: Union[str, List[str]]):
        """
        Decorator for subscribing to memory change events.

        Args:
            patterns: Pattern(s) to match against memory keys

        Returns:
            Decorator function
        """
        return self.events.on_change(patterns)

    def session(self, session_id: str) -> ScopedMemoryClient:
        """
        Get a memory client scoped to a specific session.

        Args:
            session_id: The session ID to scope to

        Returns:
            ScopedMemoryClient for the specified session
        """
        return ScopedMemoryClient(
            self.memory_client, "session", session_id, self.events
        )

    def actor(self, actor_id: str) -> ScopedMemoryClient:
        """
        Get a memory client scoped to a specific actor.

        Args:
            actor_id: The actor ID to scope to

        Returns:
            ScopedMemoryClient for the specified actor
        """
        return ScopedMemoryClient(self.memory_client, "actor", actor_id, self.events)

    def workflow(self, workflow_id: str) -> ScopedMemoryClient:
        """
        Get a memory client scoped to a specific workflow.

        Args:
            workflow_id: The workflow ID to scope to

        Returns:
            ScopedMemoryClient for the specified workflow
        """
        return ScopedMemoryClient(
            self.memory_client, "workflow", workflow_id, self.events
        )

    @property
    def global_scope(self) -> GlobalMemoryClient:
        """
        Get a memory client for global scope operations.

        Returns:
            GlobalMemoryClient for global scope access
        """
        return GlobalMemoryClient(self.memory_client, self.events)
