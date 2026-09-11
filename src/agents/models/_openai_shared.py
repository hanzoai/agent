from __future__ import annotations

import httpx
from openai import APIError, AsyncOpenAI
from pydantic import BaseModel

_default_openai_key: str | None = None
_default_openai_client: AsyncOpenAI | None = None
_use_responses_by_default: bool = True


def set_default_openai_key(key: str) -> None:
    global _default_openai_key
    _default_openai_key = key


def get_default_openai_key() -> str | None:
    return _default_openai_key


def set_default_openai_client(client: AsyncOpenAI) -> None:
    global _default_openai_client
    _default_openai_client = client


def get_default_openai_client() -> AsyncOpenAI | None:
    return _default_openai_client


def set_use_responses_by_default(use_responses: bool) -> None:
    global _use_responses_by_default
    _use_responses_by_default = use_responses


def get_use_responses_by_default() -> bool:
    return _use_responses_by_default


def raise_for_error(response: object, client: AsyncOpenAI, path: str) -> None:
    """Raise the error object a server returned with HTTP 200 in place of a result, the way the
    openai client raises one that arrives inside a stream."""
    error = getattr(response, "error", None)
    if not error:
        return
    body = error.model_dump(exclude_unset=True) if isinstance(error, BaseModel) else error
    message = body.get("message") if isinstance(body, dict) else None
    raise APIError(
        message if isinstance(message, str) and message else str(body),
        httpx.Request("POST", str(client.base_url.join(path))),
        body=body,
    )
