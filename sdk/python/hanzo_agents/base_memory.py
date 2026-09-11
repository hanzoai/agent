"""Agent memory kept in Hanzo Base, in the records the Go and TypeScript stores use."""

import copy
import json
import math
from typing import Any, Dict, List, Optional, Sequence, Tuple, Union
from urllib.parse import quote as urlquote

import requests

from .execution_context import ExecutionContext
from .memory import _to_thread, _vector_to_list


class BaseMemory:
    """
    Memory over Base's records API, with ``MemoryClient``'s methods so a
    ``MemoryInterface`` takes either. Import ``agent_memory.collection.json``
    from the Go SDK once; nothing here creates schema.
    """

    def __init__(
        self,
        base_url: str,
        token: str,
        collection: str = "agent_memory",
        timeout: float = 30.0,
        context: Optional[ExecutionContext] = None,
    ):
        self.base_url = base_url.rstrip("/")
        self.token = token
        self.collection = collection
        self.timeout = timeout
        self.context = context

    @property
    def _records(self) -> str:
        return f"{self.base_url}/v1/collections/{urlquote(self.collection)}/records"

    def bind(self, context: Optional[ExecutionContext]) -> "BaseMemory":
        """This store, answering for one execution."""
        bound = copy.copy(self)
        bound.context = context
        return bound

    def _ids(self) -> Dict[str, Optional[str]]:
        ctx = self.context
        return {
            "workflow": ctx and (ctx.workflow_id or ctx.run_id),
            "session": ctx and ctx.session_id,
            "actor": ctx and ctx.actor_id,
            "global": "global",
        }

    def _where(self, scope: Optional[str], scope_id: Optional[str]) -> Tuple[str, str]:
        """
        The scope a call addresses and the id within it, resolved the way the
        control plane resolves them: an explicit scope takes its id from the
        caller or from the execution; no scope means the narrowest one the
        execution names, and global otherwise. Global's id is "global" in every
        SDK, so their records meet.
        """
        ids = self._ids()
        if scope:
            return scope, scope_id or ids.get(scope) or ""
        for name in ("workflow", "session", "actor"):
            if ids[name]:
                return name, ids[name]
        return "global", "global"

    async def _read(
        self, scope: Optional[str], scope_id: Optional[str], key: str
    ) -> Optional[Dict[str, Any]]:
        """
        The record holding a value at a key. An explicit scope is read alone; no
        scope reads workflow, session, actor and global in that order, as the
        control plane's get does.
        """
        if scope:
            record = await self._find(scope, scope_id, key)
            return record if record and record.get("value") else None
        ids = self._ids()
        for name in ("workflow", "session", "actor", "global"):
            if ids[name]:
                record = await self._find(name, ids[name], key)
                if record and record.get("value"):
                    return record
        return None

    async def set(
        self,
        key: str,
        data: Any,
        scope: Optional[str] = None,
        scope_id: Optional[str] = None,
    ) -> None:
        """Store a value at the given scope and key."""
        await self._write(scope, scope_id, key, {"value": json.dumps(data)})

    async def get(
        self,
        key: str,
        default: Any = None,
        scope: Optional[str] = None,
        scope_id: Optional[str] = None,
    ) -> Any:
        """Read a value, answering ``default`` where the key was never written."""
        record = await self._read(scope, scope_id, key)
        if not record:
            return default
        return json.loads(record["value"])

    async def exists(
        self,
        key: str,
        scope: Optional[str] = None,
        scope_id: Optional[str] = None,
    ) -> bool:
        """Whether a value is stored at this key."""
        return bool(await self._read(scope, scope_id, key))

    async def delete(
        self,
        key: str,
        scope: Optional[str] = None,
        scope_id: Optional[str] = None,
    ) -> None:
        """
        Remove a key. Absent is not an error: the caller asked for the key to
        be gone and it is.
        """
        record = await self._find(scope, scope_id, key)
        if not record:
            return
        await self._ask("DELETE", f"{self._records}/{urlquote(record['id'])}")

    async def list_keys(self, scope: str, scope_id: Optional[str] = None) -> List[str]:
        """Every key in a scope, across all of Base's pages."""
        keys: List[str] = []
        page = 1
        while True:
            answer = await self._ask(
                "GET",
                self._records,
                params={
                    "perPage": 200,
                    "page": page,
                    "filter": _filter_for(*self._where(scope, scope_id), None),
                },
            )
            items = answer.get("items") or []
            keys.extend(item["mkey"] for item in items if item.get("mkey"))
            if not items or len(keys) >= answer.get("totalItems", 0):
                return keys
            page += 1

    async def set_vector(
        self,
        key: str,
        embedding: Union[Sequence[float], Any],
        metadata: Optional[Dict[str, Any]] = None,
        scope: Optional[str] = None,
        scope_id: Optional[str] = None,
    ) -> None:
        """Store an embedding, with optional metadata, at the given key."""
        await self._write(
            scope,
            scope_id,
            key,
            {"embedding": _vector_to_list(embedding), "metadata": metadata},
        )

    async def delete_vector(
        self,
        key: str,
        scope: Optional[str] = None,
        scope_id: Optional[str] = None,
    ) -> None:
        """Clear the embedding and keep any value stored at the key."""
        record = await self._find(scope, scope_id, key)
        if not record:
            return
        await self._ask(
            "PATCH",
            f"{self._records}/{urlquote(record['id'])}",
            body={"embedding": None, "metadata": None},
        )

    async def similarity_search(
        self,
        query_embedding: Union[Sequence[float], Any],
        top_k: int = 10,
        scope: Optional[str] = None,
        scope_id: Optional[str] = None,
        filters: Optional[Dict[str, Any]] = None,
    ) -> List[Dict[str, Any]]:
        """
        Rank the scope's vectors by cosine similarity, strongest first, in the
        client: cost grows with the scope, which suits an agent's working set.
        """
        query = _vector_to_list(query_embedding)
        if not query:
            raise ValueError("search needs a query vector")

        where, of = self._where(scope, scope_id)
        found: List[Dict[str, Any]] = []
        page = 1
        while True:
            answer = await self._ask(
                "GET",
                self._records,
                params={
                    "perPage": 200,
                    "page": page,
                    "filter": _filter_for(where, of, None),
                },
            )
            items = answer.get("items") or []
            if not items:
                break

            for item in items:
                embedding = item.get("embedding") or []
                # A different width is a different embedding model; skip it.
                if len(embedding) != len(query):
                    continue
                if not _matches(item.get("metadata"), filters):
                    continue
                found.append(
                    {
                        "key": item["mkey"],
                        "scope": item["scope"],
                        "scope_id": item["scope_id"],
                        "score": _cosine(query, embedding),
                        "metadata": item.get("metadata"),
                    }
                )
            if len(items) >= answer.get("totalItems", 0):
                break
            page += 1

        found.sort(key=lambda hit: hit["score"], reverse=True)
        return found[:top_k]

    async def _find(
        self, scope: Optional[str], scope_id: Optional[str], key: str
    ) -> Optional[Dict[str, Any]]:
        """The one record at a key, or None where there is none."""
        answer = await self._ask(
            "GET",
            self._records,
            params={
                "perPage": 1,
                "filter": _filter_for(*self._where(scope, scope_id), key),
            },
        )
        items = answer.get("items") or []
        return items[0] if items else None

    async def _write(
        self,
        scope: Optional[str],
        scope_id: Optional[str],
        key: str,
        fields: Dict[str, Any],
    ) -> None:
        """
        Create or update the one record at a key.

        PATCH, not PUT: setting a value must not erase an embedding written
        beside it, and the two share a record.
        """
        where, of = self._where(scope, scope_id)
        existing = await self._find(where, of, key)
        if existing:
            await self._ask(
                "PATCH", f"{self._records}/{urlquote(existing['id'])}", body=fields
            )
            return

        try:
            await self._ask(
                "POST",
                self._records,
                body={"scope": where, "scope_id": of, "mkey": key, **fields},
            )
        except RuntimeError:
            # Another writer created the key between the read and this create,
            # and the unique index refused a second record. A set means the last
            # write wins, so update the record that won.
            winner = await self._find(where, of, key)
            if not winner:
                raise
            await self._ask(
                "PATCH", f"{self._records}/{urlquote(winner['id'])}", body=fields
            )

    async def _ask(
        self,
        method: str,
        url: str,
        params: Optional[Dict[str, Any]] = None,
        body: Optional[Dict[str, Any]] = None,
    ) -> Dict[str, Any]:
        headers = {"Content-Type": "application/json"}
        if self.token:
            headers["Authorization"] = f"Bearer {self.token}"

        answer = await _to_thread(
            requests.request,
            method,
            url,
            params=params,
            json=body,
            headers=headers,
            timeout=self.timeout,
        )

        # Base answers a path it does not route with its admin SPA, at 200.
        if answer.headers.get("Content-Type", "").startswith("text/html"):
            raise RuntimeError(
                f"base answered with HTML for {url}"
                " — that path is not served by this Base"
            )
        if answer.status_code >= 300:
            raise RuntimeError(
                f"base returned {answer.status_code} for {url}: {_snippet(answer.text)}"
            )
        if answer.status_code == 204 or not answer.content:
            return {}
        return answer.json()


def _filter_for(scope: str, scope_id: str, key: Optional[str]) -> str:
    """A clause in Base's filter grammar, with every value quoted."""
    parts = [f"scope={_quote(scope)}", f"scope_id={_quote(scope_id)}"]
    if key is not None:
        parts.append(f"mkey={_quote(key)}")
    return " && ".join(parts)


def _quote(value: str) -> str:
    """
    A single-quoted literal. Base's tokenizer reads a backslash as an escape, so
    backslashes double before quotes are escaped: the other order lets a value
    ending in one escape the closing quote.
    """
    return "'" + value.replace("\\", "\\\\").replace("'", "\\'") + "'"


def _cosine(a: Sequence[float], b: Sequence[float]) -> float:
    """1 for the same direction, 0 for perpendicular. A zero vector has none."""
    dot = sum(x * y for x, y in zip(a, b))
    na = math.sqrt(sum(x * x for x in a))
    nb = math.sqrt(sum(y * y for y in b))
    if not na or not nb:
        return 0.0
    return dot / (na * nb)


def _matches(
    metadata: Optional[Dict[str, Any]], filters: Optional[Dict[str, Any]]
) -> bool:
    """Whether metadata satisfies every filter; a missing key fails its filter."""
    if not filters:
        return True
    metadata = metadata or {}
    return all(metadata.get(key) == want for key, want in filters.items())


def _snippet(text: str) -> str:
    """
    Enough of a body to identify a refusal, and not enough to put a token or a
    stored value into a log.
    """
    return text[:200] + "…" if len(text) > 200 else text
