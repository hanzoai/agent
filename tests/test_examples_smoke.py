"""Smoke tests verifying all Python example scripts under examples/ parse cleanly."""

import ast
from pathlib import Path

import pytest

EXAMPLES_DIR = Path(__file__).resolve().parent.parent / "examples"
EXAMPLE_FILES = sorted(EXAMPLES_DIR.rglob("*.py"))


@pytest.mark.parametrize(
    "example_path",
    EXAMPLE_FILES,
    ids=[str(p.relative_to(EXAMPLES_DIR)) for p in EXAMPLE_FILES],
)
def test_example_parses_without_syntax_errors(example_path: Path) -> None:
    """Every Python example file must be syntactically valid Python."""
    source = example_path.read_text(encoding="utf-8")
    ast.parse(source, filename=str(example_path))
