"""Tests for the ``src/agents`` web3 tree — wallet + MPC client.

Covers:
  * ``call_contract`` decodes calldata correctly and forwards through
    a fake provider (no live RPC needed).
  * ``AgentWallet.sign_message`` / ``verify_signature`` round-trips
    through EIP-191 against a real ``eth_account`` key.
  * ``MpcClient`` is consulted only when wired; without it the local
    fallback still works.
  * ``MpcClient`` keygen / sign / verify happy paths with a stubbed
    ``aiohttp.ClientSession``.
  * Retry-on-5xx and hard-fail-on-4xx semantics.

These tests deliberately avoid a live blockchain (use ``MockWallet`` +
``eth_abi`` calldata encoding) and a live MPC bridge (patch
``aiohttp.ClientSession``). Same conventions as
``sdk/python/tests/test_web3_mpc_client.py`` introduced in PR #4.
"""

from __future__ import annotations

import json
from unittest.mock import MagicMock, patch

import pytest
from eth_account import Account

from agents.extensions.web3.mpc_client import (
    KeygenResult,
    MpcClient,
    MpcConfig,
    MpcError,
    SignResult,
    VerifyResult,
)
from agents.extensions.web3.wallet import (
    AgentWallet,
    MockWallet,
    WalletConfig,
    _minimal_abi_from_signature,
)

# ----------------------------------------------------- aiohttp test scaffolding


class _FakeResponse:
    def __init__(self, status: int, body):
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
    iterator = iter(responses)

    class _Session:
        def __init__(self, *_, **__):
            pass

        def post(self, *_, **__):
            try:
                return next(iterator)
            except StopIteration as e:  # pragma: no cover - safety net
                raise AssertionError("MpcClient made more HTTP calls than test queued") from e

        async def close(self):
            return None

    return _Session


# ============================================================ MpcClient itself


def test_mpc_config_defaults_are_sane():
    cfg = MpcConfig()
    assert cfg.endpoint.startswith("http")
    assert cfg.timeout_seconds > 0
    assert cfg.threshold <= cfg.parties


def test_mpc_config_rejects_zero_timeout():
    from pydantic import ValidationError

    with pytest.raises(ValidationError):
        MpcConfig(timeout_seconds=0)


@pytest.mark.asyncio
async def test_mpc_keygen_happy_path():
    fake = _patched_session([_FakeResponse(200, {"public_key": "0xabc", "key_id": "k1"})])
    with patch("aiohttp.ClientSession", fake):
        client = MpcClient(MpcConfig(max_retries=0))
        out = await client.keygen()
    assert isinstance(out, KeygenResult)
    assert out.public_key == "0xabc"
    assert out.key_id == "k1"


@pytest.mark.asyncio
async def test_mpc_sign_happy_path():
    fake = _patched_session([_FakeResponse(200, {"signature": "0xdead", "scheme": "ecdsa"})])
    with patch("aiohttp.ClientSession", fake):
        client = MpcClient(MpcConfig(max_retries=0))
        out = await client.sign("0xdeadbeef", key_id="k1")
    assert isinstance(out, SignResult)
    assert out.signature == "0xdead"


@pytest.mark.asyncio
async def test_mpc_verify_happy_path():
    fake = _patched_session([_FakeResponse(200, {"valid": True})])
    with patch("aiohttp.ClientSession", fake):
        client = MpcClient(MpcConfig(max_retries=0))
        out = await client.verify("0xdead", "0xbeef", "0xabc")
    assert isinstance(out, VerifyResult)
    assert out.valid is True


@pytest.mark.asyncio
async def test_mpc_sign_rejects_non_hex():
    client = MpcClient(MpcConfig(max_retries=0))
    with pytest.raises(MpcError):
        await client.sign("not-hex", key_id="k1")


@pytest.mark.asyncio
async def test_mpc_4xx_does_not_retry_and_raises():
    fake = _patched_session([_FakeResponse(404, "key not found")])
    with patch("aiohttp.ClientSession", fake):
        client = MpcClient(MpcConfig(max_retries=3))
        with pytest.raises(MpcError) as exc:
            await client.sign("0xdeadbeef", key_id="missing")
    assert "404" in str(exc.value)


@pytest.mark.asyncio
async def test_mpc_5xx_retries_then_succeeds():
    fake = _patched_session([
        _FakeResponse(503, "overloaded"),
        _FakeResponse(200, {"public_key": "0xabc", "key_id": "k1"}),
    ])
    with patch("aiohttp.ClientSession", fake):
        client = MpcClient(MpcConfig(max_retries=2, backoff_seconds=0.0))
        out = await client.keygen()
    assert out.key_id == "k1"


@pytest.mark.asyncio
async def test_mpc_5xx_exhausts_retries_and_raises():
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
async def test_mpc_api_key_sets_authorization_header():
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


# ================================================================= wallet side


def _mock_wallet() -> AgentWallet:
    """Force a MockWallet by using a non-http RPC scheme."""
    cfg = WalletConfig(network_rpc="mock://test")
    return AgentWallet(cfg)


def test_sign_verify_round_trip_local():
    wallet = _mock_wallet()
    sig = wallet.sign_message("hello, mpc")
    assert sig.startswith("0x") or len(sig) == 130
    assert wallet.verify_signature("hello, mpc", sig, wallet.address) is True


def test_verify_rejects_wrong_address():
    wallet = _mock_wallet()
    sig = wallet.sign_message("hello")
    other = Account.create().address
    assert wallet.verify_signature("hello", sig, other) is False


def test_verify_rejects_tampered_message():
    wallet = _mock_wallet()
    sig = wallet.sign_message("hello")
    assert wallet.verify_signature("HELLO", sig, wallet.address) is False


def test_verify_handles_garbage_input():
    wallet = _mock_wallet()
    # All defensive paths should return False, not raise.
    assert wallet.verify_signature("", "", "") is False
    assert wallet.verify_signature("m", "0xnothex", wallet.address) is False
    assert wallet.verify_signature("m", "0x" + "00" * 65, wallet.address) is False


def test_agent_wallet_without_mpc_raises_on_sign_via_mpc():
    import asyncio

    wallet = _mock_wallet()
    assert wallet.has_mpc is False
    with pytest.raises(RuntimeError):
        asyncio.get_event_loop().run_until_complete(
            wallet.sign_via_mpc("0xdeadbeef", key_id="k1")
        )


@pytest.mark.asyncio
async def test_agent_wallet_routes_through_mpc_when_configured():
    fake = _patched_session([_FakeResponse(200, {"signature": "0xc0ffee", "scheme": "ecdsa"})])
    with patch("aiohttp.ClientSession", fake):
        client = MpcClient(MpcConfig(max_retries=0))
        wallet = AgentWallet(WalletConfig(network_rpc="mock://t"), mpc_client=client)
        assert wallet.has_mpc is True
        sig = await wallet.sign_via_mpc("0xdeadbeef", key_id="k1")
    assert sig == "0xc0ffee"


# ====================================================== call_contract calldata


def test_minimal_abi_from_signature_parses_args():
    abi = _minimal_abi_from_signature("transfer(address,uint256)")
    assert abi["name"] == "transfer"
    assert [i["type"] for i in abi["inputs"]] == ["address", "uint256"]


def test_minimal_abi_from_signature_handles_no_args():
    abi = _minimal_abi_from_signature("totalSupply()")
    assert abi["name"] == "totalSupply"
    assert abi["inputs"] == []


def test_mock_wallet_call_contract_does_not_raise():
    # The legacy MockWallet stub stays trivial — confirm it no longer
    # raises NotImplementedError, which was the pre-PR behaviour for
    # Web3Wallet and the symptom of issue #3.
    cfg = WalletConfig(network_rpc="mock://t")
    mw = MockWallet(cfg)
    result = mw.call_contract("0x0000000000000000000000000000000000000000", "balanceOf")
    assert "balanceOf" in result


def test_web3_wallet_call_contract_builds_calldata():
    """call_contract on Web3Wallet should hand the request off to web3.py.

    We patch ``Web3.eth.contract`` so no live RPC is needed; the
    assertion is that the right function name + args reach the
    contract, and the return is forwarded straight back.
    """
    from agents.extensions.web3 import wallet as wallet_mod

    if not wallet_mod.WEB3_AVAILABLE:  # pragma: no cover - guarded import
        pytest.skip("web3 not installed")

    # Build a Web3Wallet but skip the live-RPC connectivity check by
    # patching Web3.is_connected.
    with patch.object(wallet_mod.Web3, "HTTPProvider", lambda *_args, **_kw: MagicMock()):
        with patch.object(wallet_mod.Web3, "is_connected", lambda self: True):
            cfg = WalletConfig(network_rpc="http://localhost:8545")
            w = wallet_mod.Web3Wallet(cfg)

    fake_fn = MagicMock()
    fake_fn.call.return_value = 42
    fake_contract = MagicMock()
    fake_contract.functions.balanceOf.return_value = fake_fn

    with patch.object(w.w3.eth, "contract", return_value=fake_contract) as mk:
        out = w.call_contract(
            "0x" + "11" * 20,
            "balanceOf",
            "0x" + "22" * 20,
            abi=[{
                "type": "function",
                "name": "balanceOf",
                "inputs": [{"type": "address", "name": "owner"}],
                "outputs": [{"type": "uint256"}],
                "stateMutability": "view",
            }],
        )

    assert out == 42
    mk.assert_called_once()
    fake_contract.functions.balanceOf.assert_called_once_with("0x" + "22" * 20)
    fake_fn.call.assert_called_once()
