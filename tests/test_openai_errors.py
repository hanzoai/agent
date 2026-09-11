"""An OpenAI-compatible server can answer HTTP 200 with an error object in place of a result."""

from __future__ import annotations

import pytest
from openai import APIError, AsyncOpenAI

from agents import Agent, OpenAIChatCompletionsModel, OpenAIResponsesModel, Runner

from .server import Server


@pytest.mark.allow_call_model_methods
@pytest.mark.asyncio
@pytest.mark.parametrize(
    ("model_class", "path"),
    [
        (OpenAIChatCompletionsModel, "/v1/chat/completions"),
        (OpenAIResponsesModel, "/v1/responses"),
    ],
    ids=["chat_completions", "responses"],
)
async def test_error_body_with_status_200_raises_api_error(model_class: type, path: str) -> None:
    error = {"message": "upstream failed", "type": "server_error"}
    with Server({path: {"error": error}}) as server:
        async with AsyncOpenAI(base_url=f"{server.url}/v1", api_key="k", max_retries=0) as client:
            agent = Agent(name="test", model=model_class(model="m", openai_client=client))
            with pytest.raises(APIError, match="upstream failed") as raised:
                await Runner.run(agent, input="hi")

    assert raised.value.body == error
    assert str(raised.value.request.url) == f"{server.url}{path}"
    assert server.requests == [("POST", path)]
