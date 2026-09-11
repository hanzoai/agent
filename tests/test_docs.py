"""The docs configure the SDK only through variables that code reads."""

from __future__ import annotations

import re
from pathlib import Path

import openai

import agents

ROOT = Path(__file__).resolve().parents[1]


def _names(package: Path) -> set[str]:
    return {
        name
        for path in package.rglob("*.py")
        for name in re.findall(r"\b[A-Z][A-Z0-9_]+\b", path.read_text(encoding="utf-8"))
    }


def test_every_variable_the_docs_export_is_read() -> None:
    # The SDK reads its own variables, and the openai client it builds reads OPENAI_API_KEY and
    # OPENAI_BASE_URL.
    read = _names(Path(agents.__file__).parent) | _names(Path(openai.__file__).parent)
    exported = [
        (str(doc.relative_to(ROOT)), name)
        for doc in [ROOT / "README.md", *sorted(ROOT.glob("docs/**/*.md"))]
        for name in re.findall(
            r"^\s*export ([A-Z][A-Z0-9_]*)=", doc.read_text(encoding="utf-8"), re.MULTILINE
        )
    ]
    assert [item for item in exported if item[1] not in read] == []
