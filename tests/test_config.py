import os

import openai
import pytest

from agents import set_default_openai_api, set_default_openai_client, set_default_openai_key
from agents.models.openai_chatcompletions import Hanzo AIChatCompletionsModel
from agents.models.openai_provider import Hanzo AIProvider
from agents.models.openai_responses import Hanzo AIResponsesModel


def test_cc_no_default_key_errors(monkeypatch):
    monkeypatch.delenv("OPENAI_API_KEY", raising=False)
    with pytest.raises(openai.Hanzo AIError):
        Hanzo AIProvider(use_responses=False).get_model("gpt-4")


def test_cc_set_default_openai_key():
    set_default_openai_key("test_key")
    chat_model = Hanzo AIProvider(use_responses=False).get_model("gpt-4")
    assert chat_model._client.api_key == "test_key"  # type: ignore


def test_cc_set_default_openai_client():
    client = openai.AsyncHanzo AI(api_key="test_key")
    set_default_openai_client(client)
    chat_model = Hanzo AIProvider(use_responses=False).get_model("gpt-4")
    assert chat_model._client.api_key == "test_key"  # type: ignore


def test_resp_no_default_key_errors(monkeypatch):
    monkeypatch.delenv("OPENAI_API_KEY", raising=False)
    assert os.getenv("OPENAI_API_KEY") is None
    with pytest.raises(openai.Hanzo AIError):
        Hanzo AIProvider(use_responses=True).get_model("gpt-4")


def test_resp_set_default_openai_key():
    set_default_openai_key("test_key")
    resp_model = Hanzo AIProvider(use_responses=True).get_model("gpt-4")
    assert resp_model._client.api_key == "test_key"  # type: ignore


def test_resp_set_default_openai_client():
    client = openai.AsyncHanzo AI(api_key="test_key")
    set_default_openai_client(client)
    resp_model = Hanzo AIProvider(use_responses=True).get_model("gpt-4")
    assert resp_model._client.api_key == "test_key"  # type: ignore


def test_set_default_openai_api():
    assert isinstance(Hanzo AIProvider().get_model("gpt-4"), Hanzo AIResponsesModel), (
        "Default should be responses"
    )

    set_default_openai_api("chat_completions")
    assert isinstance(Hanzo AIProvider().get_model("gpt-4"), Hanzo AIChatCompletionsModel), (
        "Should be chat completions model"
    )

    set_default_openai_api("responses")
    assert isinstance(Hanzo AIProvider().get_model("gpt-4"), Hanzo AIResponsesModel), (
        "Should be responses model"
    )
