"""HTTP client for a hanzoai/mpc (or luxfi/mpc) threshold-signing cluster.

Minimal, pydantic-typed, HTTP-only (no NATS/Consul deps). Designed for
``Web3Agent`` to confirm signing through a real MPC cluster instead of
the placeholder ``return True``.

Wire protocol assumed (matches the HTTP bridge documented in
``hanzoai/mpc`` ``docs/BRIDGE-INTEGRATION.md``):

::

    POST  {endpoint}/v1/keygen        ->  {"public_key": "<hex>", "key_id": "<id>"}
    POST  {endpoint}/v1/sign          {"key_id": "<id>", "digest": "<hex>"}
                                      ->  {"signature": "<hex>", "scheme": "ecdsa|ed25519"}
    POST  {endpoint}/v1/verify        {"signature": "<hex>", "digest": "<hex>",
                                       "public_key": "<hex>"}
                                      ->  {"valid": true|false}

Each call retries transient failures (5xx, network) up to ``max_retries``
with exponential backoff. ``timeout`` is per-attempt.
"""

from __future__ import annotations

import asyncio
import logging
from typing import Optional

try:
    import aiohttp
    from aiohttp import ClientError, ClientResponseError
    AIOHTTP_AVAILABLE = True
except ImportError:  # pragma: no cover - dependency declared in pyproject
    AIOHTTP_AVAILABLE = False
    aiohttp = None  # type: ignore[assignment]
    ClientError = Exception  # type: ignore[assignment,misc]
    ClientResponseError = Exception  # type: ignore[assignment,misc]

from pydantic import BaseModel, Field

logger = logging.getLogger(__name__)


class MpcError(RuntimeError):
    """Raised when the MPC cluster cannot satisfy a request."""


class MpcConfig(BaseModel):
    """Configuration for :class:`MpcClient`.

    ``endpoint`` is the base URL of the HTTP bridge in front of the MPC
    cluster (no trailing slash). ``threshold`` and ``parties`` are
    advisory values used at keygen time; the cluster is the source of
    truth for the actual quorum.
    """

    endpoint: str = Field(default="http://127.0.0.1:8080", description="HTTP bridge base URL")
    timeout_seconds: float = Field(default=10.0, ge=0.1, le=120.0)
    max_retries: int = Field(default=3, ge=0, le=10)
    backoff_seconds: float = Field(default=0.5, ge=0.0, le=10.0)
    parties: int = Field(default=3, ge=1, le=21, description="Total signing parties (n)")
    threshold: int = Field(default=2, ge=1, le=21, description="Quorum threshold (t)")
    api_key: Optional[str] = Field(default=None, description="Bearer token, if cluster requires one")


class KeygenResult(BaseModel):
    public_key: str
    key_id: str


class SignResult(BaseModel):
    signature: str
    scheme: str = "ecdsa"


class VerifyResult(BaseModel):
    valid: bool


class MpcClient:
    """Async HTTP client for a threshold-signing MPC cluster.

    The client is stateless across calls; each call opens its own
    short-lived :class:`aiohttp.ClientSession`. For high-throughput
    callers, pass an existing session via ``session=``.
    """

    def __init__(
        self,
        config: Optional[MpcConfig] = None,
        *,
        session: "Optional[aiohttp.ClientSession]" = None,
    ) -> None:
        if not AIOHTTP_AVAILABLE:
            raise ImportError(
                "MpcClient requires aiohttp. Install with: pip install aiohttp"
            )
        self.config = config or MpcConfig()
        self._external_session = session

    # ---------------------------------------------------------------- internals

    def _headers(self) -> dict:
        h = {"Accept": "application/json", "Content-Type": "application/json"}
        if self.config.api_key:
            h["Authorization"] = f"Bearer {self.config.api_key}"
        return h

    async def _request(self, path: str, payload: dict) -> dict:
        url = f"{self.config.endpoint.rstrip('/')}{path}"
        timeout = aiohttp.ClientTimeout(total=self.config.timeout_seconds)
        last_err: Optional[Exception] = None

        for attempt in range(self.config.max_retries + 1):
            session = self._external_session
            owns_session = session is None
            if owns_session:
                session = aiohttp.ClientSession(timeout=timeout)
            try:
                async with session.post(url, json=payload, headers=self._headers()) as resp:
                    body = await resp.text()
                    if resp.status >= 500:
                        raise ClientResponseError(
                            resp.request_info, resp.history,
                            status=resp.status, message=body,
                        )
                    if resp.status >= 400:
                        # Client errors are not retried.
                        raise MpcError(f"MPC bridge {resp.status}: {body[:200]}")
                    return await _parse_json(body, resp.headers.get("Content-Type", ""))
            except (ClientError, asyncio.TimeoutError, ClientResponseError) as exc:
                last_err = exc
                if attempt >= self.config.max_retries:
                    break
                delay = self.config.backoff_seconds * (2 ** attempt)
                logger.warning(
                    "MPC request to %s failed (%s). Retrying in %.2fs (attempt %d/%d)",
                    url, exc, delay, attempt + 1, self.config.max_retries,
                )
                await asyncio.sleep(delay)
            finally:
                if owns_session and session is not None:
                    await session.close()

        raise MpcError(f"MPC bridge unreachable after {self.config.max_retries + 1} attempts: {last_err}")

    # ------------------------------------------------------------------- public

    async def keygen(self) -> KeygenResult:
        """Generate a new threshold key. Returns the public key and a key id."""
        body = await self._request(
            "/v1/keygen",
            {"parties": self.config.parties, "threshold": self.config.threshold},
        )
        return KeygenResult(**body)

    async def sign(self, digest_hex: str, *, key_id: str) -> SignResult:
        """Threshold-sign ``digest_hex`` (32-byte hex string) under ``key_id``."""
        if not _looks_like_hex(digest_hex):
            raise MpcError("digest_hex must be a hex-encoded byte string")
        body = await self._request("/v1/sign", {"key_id": key_id, "digest": digest_hex})
        return SignResult(**body)

    async def verify(self, signature_hex: str, digest_hex: str, public_key_hex: str) -> VerifyResult:
        """Ask the cluster to verify a signature. Useful for cross-check
        before trusting a payment ack signed by another party."""
        if not _looks_like_hex(signature_hex) or not _looks_like_hex(digest_hex) or not _looks_like_hex(public_key_hex):
            raise MpcError("signature, digest, and public_key must be hex-encoded")
        body = await self._request(
            "/v1/verify",
            {"signature": signature_hex, "digest": digest_hex, "public_key": public_key_hex},
        )
        return VerifyResult(**body)


def _looks_like_hex(s: str) -> bool:
    s = s[2:] if s.startswith(("0x", "0X")) else s
    if not s:
        return False
    try:
        int(s, 16)
    except ValueError:
        return False
    return True


async def _parse_json(body: str, content_type: str) -> dict:
    import json
    try:
        return json.loads(body)
    except json.JSONDecodeError as exc:
        raise MpcError(f"MPC bridge returned non-JSON ({content_type}): {body[:200]}") from exc


__all__ = [
    "MpcClient",
    "MpcConfig",
    "MpcError",
    "KeygenResult",
    "SignResult",
    "VerifyResult",
]
