"""Start preconditions of a run (SFR-AGG-040, SSR-AGG-010).

No instruction or skill file outside the node working set may reach a Claude session: the
user-level ``CLAUDE.md`` and ``skills/`` of the Claude config folder are empty or absent, and no
``CLAUDE.md``, ``CLAUDE.local.md`` or ``.claude/`` exists in the workspace or in ``.agent/``.
Read-only: nothing is changed.
"""

from pathlib import Path

from ..errors import Finding
from ..services.runfs import WorkspacePaths

CODE = "precondition"


def _finding(message: str, path: Path) -> Finding:
    return Finding(code=CODE, message=message, file=str(path))


def check_preconditions(ws: WorkspacePaths, claude_dir: Path) -> list[Finding]:
    out: list[Finding] = []
    user_md = Path(claude_dir) / "CLAUDE.md"
    if user_md.is_file():
        try:
            nonempty = bool(user_md.read_text(encoding="utf-8", errors="replace").strip())
        except OSError:
            nonempty = True
        if nonempty:
            out.append(_finding("the user-level CLAUDE.md must be empty or absent", user_md))
    skills = Path(claude_dir) / "skills"
    if skills.is_dir():
        try:
            has_entries = any(skills.iterdir())
        except OSError:
            has_entries = True
        if has_entries:
            out.append(_finding("the user-level skills folder must be empty", skills))
    for base in (ws.root, ws.agent_dir):
        for name in ("CLAUDE.md", "CLAUDE.local.md"):
            if (base / name).exists():
                out.append(_finding(f"{name} must not exist in {base}", base / name))
        if (base / ".claude").exists():
            out.append(_finding(f".claude/ must not exist in {base}", base / ".claude"))
    return out
