"""
Agent memory kept in Hanzo Base.

Base is the single-binary Go backend, so an agent's state sits in the same
process tree as everything it runs beside: no second database, no separate
migration, and the records are visible in Base's own admin view while the
agent is running.

One record per scope and key, in one collection. A collection per scope would
make the number of collections a function of how many workflows have ever run.

The same records, schema and escaping rules as the Go and TypeScript stores, so
all three SDKs read each other's memory.
"""

import json
import math
from typing import Any, Dict, List, Optional, Sequence, Union
from urllib.parse import quote as urlquote

import requests

from .memory import _to_thread, _vector_to_list


class BaseMemory:
    """
    Memory over Base's records API.

    Stands where ``MemoryClient`` stands, with the same eight methods, so a
    ``MemoryInterface`` takes either.

    The collection must exist, carrying the fields this writes. Import
    ``agent_memory.collection.json`` from the Go SDK once — the schema is
    shared. It is not created on first write: a client that builds its own
    schema can build the wrong one from a typo and then read nothing from the
    collection everyone else is looking at.
    """

    def __init__(
        self,
        base_url: str,
        token: str,
        collection: str = "agent_memory",
        default_scope: str = "workflow",
        timeout: float = 30.0,
    ):
        self.base_url = base_url.rstrip("/")
        self.token = token
        self.collection = collection
        self.default_scope = default_scope
        self.timeout = timeout

    @property
    def _records(self) -> str:
        return f"{self.base_url}/v1/collections/{urlquote(self.collection)}/records"

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
        record = await self._find(scope, scope_id, key)
        if not record or not record.get("value"):
            return default
        return json.loads(record["value"])

    async def exists(
        self,
        key: str,
        scope: Optional[str] = None,
        scope_id: Optional[str] = None,
    ) -> bool:
        """Whether a value is stored at this key."""
        record = await self._find(scope, scope_id, key)
        return bool(record and record.get("value"))

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
        """
        Every key in a scope, following Base's pages to the end. A caller
        asking for all the keys and receiving the first page would read that as
        the whole answer.
        """
        keys: List[str] = []
        page = 1
        while True:
            answer = await self._ask(
                "GET",
                self._records,
                params={
                    "perPage": 200,
                    "page": page,
                    "filter": _filter_for(scope, scope_id or "", None),
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
        """
        Remove an embedding. The embedding, not the record: a value may be
        stored at the same key and is not the caller's to lose here.
        """
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
        Rank the scope's vectors by cosine similarity, strongest first.

        Scored here rather than in the database, and that is a limit worth
        stating: it reads the scope's vectors and ranks them in the client, so
        cost grows with the size of the scope. Right for an agent's own working
        set, wrong for a corpus. Base can score server-side through a hook, and
        a scope large enough to need that should use one.
        """
        query = _vector_to_list(query_embedding)
        if not query:
            raise ValueError("search needs a query vector")

        where = scope or self.default_scope
        of = scope_id or ""
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
                # A vector of a different width came from a different embedding
                # model. Cosine across two of those returns a number that means
                # nothing, so it is skipped rather than scored.
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
                "filter": _filter_for(
                    scope or self.default_scope, scope_id or "", key
                ),
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
        existing = await self._find(scope, scope_id, key)
        if existing:
            await self._ask(
                "PATCH",
                f"{self._records}/{urlquote(existing['id'])}",
                body=fields,
            )
            return

        await self._ask(
            "POST",
            self._records,
            body={
                "scope": scope or self.default_scope,
                "scope_id": scope_id or "",
                "mkey": key,
                **fields,
            },
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

        # An HTML body on a JSON endpoint means the path was not served and the
        # admin SPA answered. Saying so beats a parse error, which reads as a
        # broken server rather than a wrong collection.
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
    A single-quoted literal, escaped.

    Base's tokenizer treats a backslash as the escape rune, so a quote preceded
    by one does not close the literal. That makes the order load-bearing:
    backslashes double first, then quotes. The other order lets a value ending
    in a backslash escape the closing quote and end the literal where the
    caller chose. A scope id is caller data.
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
    """
    Whether a record's metadata satisfies every filter. A filter naming a key
    the record lacks excludes it: the caller asked for records where that key
    holds a value, and a record without it is not one.
    """
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
