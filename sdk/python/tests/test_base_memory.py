"""
BaseMemory held to its contract against a Base standing in for the real one.

Enough of Base to check what this store actually sends: the filter grammar,
the paging it walks, and the create-or-patch decision it makes. No network —
``responses`` intercepts requests, as the rest of this suite does.
"""

import json
import re
from types import SimpleNamespace
from urllib.parse import parse_qs, urlparse

import pytest
import responses

from hanzo_agents.base_memory import BaseMemory, _filter_for, _quote

CLAUSE = re.compile(r"(\w+)='((?:[^'\\]|\\.)*)'")
# The query string is part of the url responses matches against, so the
# pattern has to admit one — a `$` right after the path matches only the
# unparameterised calls, and every read here carries a filter.
RECORDS = re.compile(
    r"^http://base\.test/v1/collections/([^/?]+)/records(?:/([^?]+))?(?:\?.*)?$"
)


def unescape(value):
    """Undo a quoted literal's escaping: a backslash escapes what follows."""
    out = []
    i = 0
    while i < len(value):
        if value[i] == "\\" and i + 1 < len(value):
            i += 1
        out.append(value[i])
        i += 1
    return "".join(out)


class FakeBase:
    """One collection's records, in memory, over the real API's shape."""

    def __init__(self, collection="agent_memory"):
        self.collection = collection
        self.records = {}
        self.before_create = None
        self._next = 0

    def install(self):
        for method in ("GET", "POST", "PATCH", "DELETE"):
            responses.add_callback(
                method, RECORDS, callback=self._answer, content_type=None
            )

    def _answer(self, request):
        found = RECORDS.match(request.url.split("?")[0])
        collection, record_id = found.group(1), found.group(2)

        # The admin SPA a real Base serves for a path it does not route: HTML,
        # at 200. It is here because that is the failure this store must name.
        if collection != self.collection:
            return (200, {"Content-Type": "text/html; charset=utf-8"}, "<!doctype html>")

        body = json.loads(request.body) if request.body else None
        headers = {"Content-Type": "application/json"}

        if request.method == "GET":
            return (200, headers, json.dumps(self._list(request.url)))
        if request.method == "POST":
            # A create under the unique index over scope, scope_id and mkey.
            if self.before_create:
                hook, self.before_create = self.before_create, None
                hook()
            body = body or {}
            if any(
                all(record.get(f) == body.get(f) for f in ("scope", "scope_id", "mkey"))
                for record in self.records.values()
            ):
                refusal = {"message": "Failed to create record.", "data": {"mkey": {"code": "validation_not_unique"}}}
                return (400, headers, json.dumps(refusal))
            return (201, headers, json.dumps(self.insert(body)))
        if request.method == "PATCH":
            record = self.records.get(record_id, {})
            # A merge, which is what PATCH means: an absent field keeps its
            # value, and an explicit null clears it.
            record.update(body or {})
            return (200, headers, json.dumps(record))
        self.records.pop(record_id, None)
        return (204, headers, "")

    def insert(self, body):
        self._next += 1
        record = dict(body, id=f"r{self._next}")
        self.records[record["id"]] = record
        return record

    def _list(self, url):
        query = parse_qs(urlparse(url).query)
        want = {
            field: unescape(value)
            for field, value in CLAUSE.findall(query.get("filter", [""])[0])
        }

        hits = [
            record
            for record in self.records.values()
            if record.get("scope") == want.get("scope")
            and record.get("scope_id") == want.get("scope_id")
            and ("mkey" not in want or record.get("mkey") == want["mkey"])
        ]
        hits.sort(key=lambda record: record.get("mkey", ""))

        per_page = int(query.get("perPage", ["30"])[0])
        page = int(query.get("page", ["1"])[0])
        start = min((page - 1) * per_page, len(hits))
        return {"items": hits[start : start + per_page], "totalItems": len(hits)}


@pytest.fixture
def fake():
    # The conftest already starts the default mock for every non-integration
    # test, so callbacks register on that one. A second RequestsMock here would
    # not be the one intercepting, and every request would reach the network.
    base = FakeBase()
    base.install()
    return base


@pytest.fixture
def memory(fake):
    return BaseMemory("http://base.test", "test-token", "agent_memory")


@pytest.mark.asyncio
async def test_reads_back_what_it_wrote(memory):
    await memory.set("greeting", "hello", scope="session", scope_id="s1")
    assert await memory.get("greeting", scope="session", scope_id="s1") == "hello"


@pytest.mark.asyncio
async def test_keeps_the_shape_of_a_structured_value(memory):
    plan = {"steps": ["read", "write"], "depth": 2}
    await memory.set("plan", plan, scope="workflow", scope_id="w1")
    assert await memory.get("plan", scope="workflow", scope_id="w1") == plan


@pytest.mark.asyncio
async def test_replaces_rather_than_appending(memory, fake):
    await memory.set("tone", "terse", scope="user", scope_id="u1")
    await memory.set("tone", "plain", scope="user", scope_id="u1")

    assert await memory.get("tone", scope="user", scope_id="u1") == "plain"
    assert len(fake.records) == 1


@pytest.mark.asyncio
async def test_answers_the_default_for_a_key_never_written(memory):
    assert await memory.get("never", scope="global") is None
    assert await memory.get("never", default="fallback", scope="global") == "fallback"
    assert await memory.exists("never", scope="global") is False


@pytest.mark.asyncio
async def test_keeps_each_scope_to_itself(memory):
    await memory.set("who", "session", scope="session", scope_id="s1")
    await memory.set("who", "user", scope="user", scope_id="u1")

    assert await memory.get("who", scope="session", scope_id="s1") == "session"
    assert await memory.get("who", scope="user", scope_id="u1") == "user"
    assert await memory.list_keys("session", "s1") == ["who"]


@pytest.mark.asyncio
async def test_deletes_and_absent_is_not_an_error(memory):
    await memory.set("temp", 1, scope="session", scope_id="s1")
    await memory.delete("temp", scope="session", scope_id="s1")

    assert await memory.get("temp", scope="session", scope_id="s1") is None
    await memory.delete("temp", scope="session", scope_id="s1")


@pytest.mark.asyncio
async def test_lists_every_page_not_the_first(memory):
    many = 250  # more than one 200-record page
    for i in range(many):
        await memory.set(f"k{i:03d}", i, scope="global")

    assert len(await memory.list_keys("global")) == many


@pytest.mark.asyncio
async def test_ranks_a_similarity_search(memory):
    where = {"scope": "session", "scope_id": "s1"}
    await memory.set_vector("same", [1, 0, 0], **where)
    await memory.set_vector("near", [0.9, 0.4, 0], **where)
    await memory.set_vector("across", [0, 1, 0], **where)

    hits = await memory.similarity_search([1, 0, 0], **where)
    assert [hit["key"] for hit in hits] == ["same", "near", "across"]
    assert hits[0]["score"] > 0.999
    assert hits[0]["scope_id"] == "s1"


@pytest.mark.asyncio
async def test_honours_a_metadata_filter(memory):
    where = {"scope": "session", "scope_id": "s1"}
    await memory.set_vector("note", [1, 0], {"kind": "note"}, **where)
    await memory.set_vector("task", [1, 0], {"kind": "task"}, **where)

    hits = await memory.similarity_search([1, 0], filters={"kind": "task"}, **where)
    assert [hit["key"] for hit in hits] == ["task"]


@pytest.mark.asyncio
async def test_skips_a_vector_of_another_width(memory):
    where = {"scope": "session", "scope_id": "s1"}
    await memory.set_vector("wide", [1, 0, 0, 0], **where)
    await memory.set_vector("right", [1, 0], **where)

    hits = await memory.similarity_search([1, 0], **where)
    assert [hit["key"] for hit in hits] == ["right"]


@pytest.mark.asyncio
async def test_caps_at_top_k(memory):
    where = {"scope": "session", "scope_id": "s1"}
    await memory.set_vector("same", [1, 0], **where)
    await memory.set_vector("across", [0, 1], **where)

    hits = await memory.similarity_search([1, 0], top_k=1, **where)
    assert [hit["key"] for hit in hits] == ["same"]


@pytest.mark.asyncio
async def test_refuses_a_search_with_no_query_vector(memory):
    with pytest.raises(ValueError, match="query vector"):
        await memory.similarity_search([], scope="session", scope_id="s1")


@pytest.mark.asyncio
async def test_a_value_and_an_embedding_share_a_key(memory):
    where = {"scope": "session", "scope_id": "s1"}

    await memory.set_vector("doc", [1, 0], {"kind": "note"}, **where)
    await memory.set("doc", "the text", **where)
    assert len(await memory.similarity_search([1, 0], **where)) == 1

    await memory.delete_vector("doc", **where)
    assert await memory.similarity_search([1, 0], **where) == []
    assert await memory.get("doc", **where) == "the text"


@pytest.mark.asyncio
async def test_names_a_path_base_does_not_serve(fake):
    memory = BaseMemory("http://base.test", "test-token", "wrong_collection")
    with pytest.raises(RuntimeError, match="HTML"):
        await memory.get("anything", scope="session", scope_id="s1")


@pytest.mark.asyncio
async def test_quotes_a_scope_id_so_a_quote_cannot_end_the_clause(memory):
    odd = "tenant' && scope='global"
    await memory.set("k", "mine", scope="user", scope_id=odd)
    await memory.set("k", "not mine", scope="global")

    assert await memory.get("k", scope="user", scope_id=odd) == "mine"


@pytest.mark.asyncio
async def test_escapes_a_backslash_before_a_quote(memory):
    for odd in ["ends-with-backslash\\", "has'quote", "both\\' together", "\\\\"]:
        await memory.set("k", "mine", scope="user", scope_id=odd)
        assert await memory.get("k", scope="user", scope_id=odd) == "mine"


def test_the_filter_clause_is_what_bases_grammar_parses():
    assert (
        _filter_for("session", "s1", "k")
        == "scope='session' && scope_id='s1' && mkey='k'"
    )
    # No key means the whole scope, so the clause must not pin one.
    assert _filter_for("global", "", None) == "scope='global' && scope_id=''"
    assert _quote("a\\") == "'a\\\\'"


@pytest.mark.asyncio
async def test_an_agent_reads_and_writes_through_the_store_it_was_given(fake):
    """
    The wiring, not just the attribute: a handler's memory calls have to land
    in the store the agent was constructed with.
    """
    from hanzo_agents import Agent

    agent = Agent(
        node_id="n1",
        store=BaseMemory("http://base.test", "test-token", "agent_memory"),
        auto_register=False,
    )
    agent._current_execution_context = SimpleNamespace(
        to_headers=lambda: {},
        workflow_id="w1",
        session_id=None,
        actor_id=None,
    )

    memory = agent.memory
    await memory.set("tone", "plain")
    assert await memory.get("tone") == "plain"
    assert any(record.get("mkey") == "tone" for record in fake.records.values())


def execution(workflow_id=None, session_id=None, actor_id=None):
    return SimpleNamespace(
        run_id=workflow_id, workflow_id=workflow_id, session_id=session_id, actor_id=actor_id
    )


@pytest.mark.asyncio
async def test_takes_scope_ids_from_the_execution_it_is_bound_to(memory):
    one, two = memory.bind(execution("w1")), memory.bind(execution("w2"))
    await one.set("step", "one")
    await two.set("step", "two")

    assert await one.get("step") == "one"
    assert await two.get("step") == "two"


@pytest.mark.asyncio
async def test_files_global_under_the_id_every_sdk_uses(memory, fake):
    await memory.set("shared", True, scope="global")
    assert [record["scope_id"] for record in fake.records.values()] == ["global"]


@pytest.mark.asyncio
async def test_a_read_without_a_scope_falls_back_through_the_hierarchy(memory):
    bound = memory.bind(execution("w1", session_id="s1", actor_id="a1"))
    await bound.set("tone", "plain", scope="session")

    assert await bound.get("tone") == "plain"
    assert await bound.exists("tone")


@pytest.mark.asyncio
async def test_keeps_the_last_write_when_two_writers_create_one_key(memory, fake):
    fake.before_create = lambda: fake.insert(
        {"scope": "session", "scope_id": "s1", "mkey": "k", "value": json.dumps("theirs")}
    )
    await memory.set("k", "ours", scope="session", scope_id="s1")

    assert await memory.get("k", scope="session", scope_id="s1") == "ours"
    assert len(fake.records) == 1
