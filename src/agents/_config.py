from openai import AsyncOpenAI
from typing_extensions import Literal

from .models import _openai_shared


def set_default_openai_key(key: str) -> None:
    _openai_shared.set_default_openai_key(key)


def set_default_openai_client(client: AsyncOpenAI) -> None:
    _openai_shared.set_default_openai_client(client)


def set_default_openai_api(api: Literal["chat_completions", "responses"]) -> None:
    if api == "chat_completions":
        _openai_shared.set_use_responses_by_default(False)
    else:
        _openai_shared.set_use_responses_by_default(True)
