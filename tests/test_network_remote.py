"""Tests for the cross-process ZAP agent proxy (network/remote.py).

Loads `remote.py` directly via importlib to avoid the heavyweight
`agents.__init__` import chain (numpy / griffe transitive deps), mirroring
the pattern used in `test_zap.py`.
"""

import importlib.util
import os
import sys


def _load_module(name: str, path: str):
    spec = importlib.util.spec_from_file_location(name, path)
    mod = importlib.util.module_from_spec(spec)  # type: ignore
    sys.modules[name] = mod
    spec.loader.exec_module(mod)  # type: ignore
    return mod


_here = os.path.dirname(os.path.abspath(__file__))
_base = os.path.join(_here, "..", "src", "agents", "network")
_remote = _load_module("agents.network.remote", os.path.join(_base, "remote.py"))


RemoteAgentProxy = _remote.RemoteAgentProxy
is_zap_wire_enabled = _remote.is_zap_wire_enabled


def test_feature_flag_default_disabled(monkeypatch):
    """Default (no env) keeps the JSON/local-agent path — OpenAI-compatible."""
    monkeypatch.delenv("HANZO_AGENT_WIRE", raising=False)
    assert is_zap_wire_enabled() is False


def test_feature_flag_enabled_with_zap(monkeypatch):
    """`HANZO_AGENT_WIRE=zap` enables the cross-process wire."""
    monkeypatch.setenv("HANZO_AGENT_WIRE", "zap")
    assert is_zap_wire_enabled() is True


def test_feature_flag_case_insensitive(monkeypatch):
    """Uppercase / mixed-case values still flip the flag (operator friendliness)."""
    monkeypatch.setenv("HANZO_AGENT_WIRE", "ZAP")
    assert is_zap_wire_enabled() is True
    monkeypatch.setenv("HANZO_AGENT_WIRE", "Zap")
    assert is_zap_wire_enabled() is True


def test_feature_flag_other_values_disabled(monkeypatch):
    """Anything other than literal 'zap' keeps the flag off — fail-closed default."""
    monkeypatch.setenv("HANZO_AGENT_WIRE", "json")
    assert is_zap_wire_enabled() is False
    monkeypatch.setenv("HANZO_AGENT_WIRE", "1")
    assert is_zap_wire_enabled() is False
    monkeypatch.setenv("HANZO_AGENT_WIRE", "true")
    assert is_zap_wire_enabled() is False


def test_proxy_construction_defaults():
    """Defaults: empty capabilities/metadata/handoffs/tools, no instructions."""
    p = RemoteAgentProxy(name="peer1", zap_uri="zap://peer-1.cluster:9999")
    assert p.name == "peer1"
    assert p.zap_uri == "zap://peer-1.cluster:9999"
    assert p.handoff_description is None
    assert p.capabilities == []
    assert p.metadata == {}
    assert p.handoffs == []
    assert p.tools == []
    assert p.instructions is None


def test_proxy_clone_preserves_and_overrides():
    """clone() mirrors Agent.clone — fields passed are overridden; rest preserved."""
    base = RemoteAgentProxy(
        name="peer1",
        zap_uri="zap://peer-1.cluster:9999",
        handoff_description="code review agent",
        capabilities=["code", "review"],
        metadata={"region": "us-west"},
    )
    cloned = base.clone(name="peer1-shadow")
    assert cloned.name == "peer1-shadow"
    assert cloned.zap_uri == base.zap_uri
    assert cloned.handoff_description == base.handoff_description
    assert cloned.capabilities == base.capabilities
    assert cloned.metadata == base.metadata
    # And confirm the override is shallow-copied (mutating the clone doesn't
    # bleed into the original — caps lists are independent).
    cloned.capabilities.append("ops")
    assert base.capabilities == ["code", "review"]


import pytest


@pytest.mark.asyncio
async def test_invoke_errors_without_feature_flag(monkeypatch):
    """invoke() refuses to dispatch when the wire feature-flag is off.

    Fail-loud: avoids silent regression to a misconfigured local-dispatch
    path when the operator forgets to set HANZO_AGENT_WIRE=zap on a node
    that holds remote proxies.
    """
    monkeypatch.delenv("HANZO_AGENT_WIRE", raising=False)
    p = RemoteAgentProxy(name="peer1", zap_uri="zap://peer-1.cluster:9999")
    with pytest.raises(RuntimeError, match="HANZO_AGENT_WIRE"):
        await p.invoke("hello")
