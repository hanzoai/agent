"""Tests for hanzo_agents.extensions.web3.mpc_client.

Mocks aiohttp.ClientSession so the MPC HTTP bridge does not need to be
running. Covers: keygen, sign, verify, retry-on-5xx, hard-fail-on-4xx,
and bad-input validation.
"""

import asyncio
import json
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from hanzo_agents.extensions.web3.mpc_client import (
    KeygenResult,
    MpcClient,
    MpcConfig,
    MpcError,
    SignResult,
    VerifyResult,
)


# --------------------------------------------------------------------- helpers


class _FakeResponse:
    def __init__(self, status: int, body: dict | str):
        self.status = status
        self._body = body if isinstance(body, str) else json.dumps(body)
        self.headers = {"Content-Type": "application/json"}
        self.request_info = MagicMock()
        self.history = ()

    async def text(self):
        return self._body

    async def __aenter__(self):
        return self

    async def __aexit__(self, *exc_info):
        return False


def _patched_session(responses):
    """Return a ClientSession factory whose `.post(...)` yields each
    queued response in order, then raises if asked again."""
    iterator = iter(responses)

    class _Session:
        def __init__(self, *_, **__):
            pass

        def post(self, *_, **__):
            try:
                return next(iterator)
            except StopIteration:
                raise AssertionError("MpcClient made more HTTP calls than test queued")

        async def close(self):
            return None

    return _Session


# ----------------------------------------------------------------------- tests


def test_config_defaults_are_sane():
    cfg = MpcConfig()
    assert cfg.endpoint.startswith("http")
    assert cfg.timeout_seconds > 0
    assert cfg.threshold <= cfg.parties


def test_config_rejects_zero_timeout():
    with pytest.raises(Exception):
        MpcConfig(timeout_seconds=0)


@pytest.mark.asyncio
async def test_keygen_happy_path():
    fake = _patched_session([_FakeResponse(200, {"public_key": "0xabc", "key_id": "k1"})])
    with patch("aiohttp.ClientSession", fake):
        client = MpcClient(MpcConfig(max_retries=0))
        out = await client.keygen()
    assert isinstance(out, KeygenResult)
    assert out.public_key == "0xabc"
    assert out.key_id == "k1"


@pytest.mark.asyncio
async def test_sign_happy_path():
    fake = _patched_session([_FakeResponse(200, {"signature": "0xdead", "scheme": "ecdsa"})])
    with patch("aiohttp.ClientSession", fake):
        client = MpcClient(MpcConfig(max_retries=0))
        out = await client.sign("0xdeadbeef", key_id="k1")
    assert isinstance(out, SignResult)
    assert out.signature == "0xdead"
    assert out.scheme == "ecdsa"


@pytest.mark.asyncio
async def test_verify_happy_path():
    fake = _patched_session([_FakeResponse(200, {"valid": True})])
    with patch("aiohttp.ClientSession", fake):
        client = MpcClient(MpcConfig(max_retries=0))
        out = await client.verify("0xdead", "0xbeef", "0xabc")
    assert isinstance(out, VerifyResult)
    assert out.valid is True


@pytest.mark.asyncio
async def test_sign_rejects_non_hex():
    client = MpcClient(MpcConfig(max_retries=0))
    with pytest.raises(MpcError):
        await client.sign("not-hex", key_id="k1")


@pytest.mark.asyncio
async def test_4xx_does_not_retry_and_raises():
    fake = _patched_session([_FakeResponse(404, "key not found")])
    with patch("aiohttp.ClientSession", fake):
        client = MpcClient(MpcConfig(max_retries=3))
        with pytest.raises(MpcError) as exc:
            await client.sign("0xdeadbeef", key_id="missing")
    assert "404" in str(exc.value)


@pytest.mark.asyncio
async def test_5xx_retries_then_succeeds():
    fake = _patched_session([
        _FakeResponse(503, "overloaded"),
        _FakeResponse(200, {"public_key": "0xabc", "key_id": "k1"}),
    ])
    with patch("aiohttp.ClientSession", fake):
        client = MpcClient(MpcConfig(max_retries=2, backoff_seconds=0.0))
        out = await client.keygen()
    assert out.key_id == "k1"


@pytest.mark.asyncio
async def test_5xx_exhausts_retries_and_raises():
    fake = _patched_session([
        _FakeResponse(500, "boom"),
        _FakeResponse(500, "boom"),
        _FakeResponse(500, "boom"),
    ])
    with patch("aiohttp.ClientSession", fake):
        client = MpcClient(MpcConfig(max_retries=2, backoff_seconds=0.0))
        with pytest.raises(MpcError):
            await client.keygen()


@pytest.mark.asyncio
async def test_api_key_sets_authorization_header():
    captured: dict = {}

    class _Capture:
        def __init__(self, *_, **__):
            pass

        def post(self, *args, **kwargs):
            captured.update(kwargs.get("headers", {}))
            return _FakeResponse(200, {"public_key": "0xabc", "key_id": "k1"})

        async def close(self):
            return None

    with patch("aiohttp.ClientSession", _Capture):
        client = MpcClient(MpcConfig(api_key="secret-token", max_retries=0))
        await client.keygen()
    assert captured.get("Authorization") == "Bearer secret-token"
