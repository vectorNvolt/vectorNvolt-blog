"""Normalization of composed text (USS-008 "Normalization rules", SSR-AGG-011).

LF line endings, no trailing whitespace, no leading or trailing blank lines per block, exactly
one blank line between blocks, one final newline. Idempotent.
"""

from typing import Iterable


def normalize_block(text: str) -> str:
    """One block without surrounding blank lines and without a final newline."""
    text = text.replace("\r\n", "\n").replace("\r", "\n")
    lines = [ln.rstrip() for ln in text.split("\n")]
    while lines and not lines[0]:
        lines.pop(0)
    while lines and not lines[-1]:
        lines.pop()
    return "\n".join(lines)


def join_blocks(blocks: Iterable[str]) -> str:
    """Blocks separated by one blank line, one final newline; empty blocks are skipped.
    Returns "" when nothing is left."""
    parts = [p for p in (normalize_block(b) for b in blocks) if p]
    return "\n\n".join(parts) + "\n" if parts else ""
