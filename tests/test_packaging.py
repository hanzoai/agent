"""Packaging checks for hanzo-agent.

0.1.0 reached PyPI without numpy in its METADATA while ``agents/memory/store.py``
imported numpy at module level, so ``import agents`` raised ``ModuleNotFoundError``
in a clean environment. The pin that replaced it, ``numpy>=2.1.0``, cannot install
on 3.9, which requires-python still admits: numpy 2.1.0 dropped 3.9.
"""

from __future__ import annotations

import subprocess
import sys
from importlib.metadata import requires

import pytest
from packaging.requirements import Requirement
from packaging.specifiers import SpecifierSet

from agents.memory import InMemoryMemoryStore, MemoryType

# numpy 2.1.0 dropped Python 3.9 and 2.3.0 dropped 3.10, so these are the newest
# releases those interpreters can install, and they will not move.
LAST_NUMPY = {"3.9": "2.0.2", "3.10": "2.2.6"}


def _numpy_specifiers(python: str) -> list[SpecifierSet]:
    env = {"python_version": python, "python_full_version": f"{python}.0", "extra": ""}
    reqs = [Requirement(r) for r in requires("hanzo-agent") or []]
    return [
        r.specifier
        for r in reqs
        if r.name == "numpy" and (r.marker is None or r.marker.evaluate(env))
    ]


@pytest.mark.parametrize("python", ["3.9", "3.10", "3.11", "3.12", "3.13"])
def test_numpy_is_declared_on_every_supported_python(python: str) -> None:
    assert _numpy_specifiers(python), f"no numpy requirement applies on Python {python}"


@pytest.mark.parametrize(("python", "version"), sorted(LAST_NUMPY.items()))
def test_numpy_floor_admits_the_last_numpy_for_old_pythons(python: str, version: str) -> None:
    for spec in _numpy_specifiers(python):
        assert spec.contains(version), (
            f"numpy{spec} excludes {version}, the newest numpy for Python {python}"
        )


def test_import_agents_does_not_need_numpy() -> None:
    # A None entry in sys.modules makes `import numpy` raise ImportError.
    code = "import sys; sys.modules['numpy'] = None; import agents"
    result = subprocess.run([sys.executable, "-c", code], capture_output=True, text=True)
    assert result.returncode == 0, result.stderr


async def test_search_by_embedding_ranks_by_cosine_similarity() -> None:
    store = InMemoryMemoryStore()
    same = await store.add("same", MemoryType.FACT, embedding=[1.0, 0.0])
    near = await store.add("near", MemoryType.FACT, embedding=[1.0, 1.0])
    await store.add("orthogonal", MemoryType.FACT, embedding=[0.0, 1.0])
    await store.add("no embedding", MemoryType.FACT)

    results = await store.search_by_embedding([2.0, 0.0], threshold=0.5)

    assert [memory.id for memory, _ in results] == [same.id, near.id]
    assert results[0][1] == pytest.approx(1.0)
    assert results[1][1] == pytest.approx(2**-0.5)
