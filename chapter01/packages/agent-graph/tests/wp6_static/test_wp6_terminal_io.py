"""SSR-AGG-001: only the gateway, render and main touch the terminal."""

import re
from pathlib import Path

import agent_graph

SRC = Path(agent_graph.__file__).resolve().parent
ALLOWED = {SRC / "services" / "gateway.py", SRC / "services" / "render.py", SRC / "main.py"}
# a call of print / input (not a method or helper that merely ends in these names) or sys.stdout
PATTERN = re.compile(r"(?<![\w.])print\(|(?<![\w.])input\(|sys\.stdout")


def test_no_terminal_io_outside_gateway_render_main():
    offenders = []
    for path in sorted(SRC.rglob("*.py")):
        if path in ALLOWED:
            continue
        for no, line in enumerate(path.read_text(encoding="utf-8").splitlines(), start=1):
            if PATTERN.search(line):
                offenders.append(f"{path.relative_to(SRC)}:{no}: {line.strip()}")
    assert not offenders, "terminal I/O outside gateway/render/main:\n" + "\n".join(offenders)


def test_render_does_not_use_terminal_streams():
    text = (SRC / "services" / "render.py").read_text(encoding="utf-8")
    assert "sys.stdout" not in text and not re.search(r"(?<![\w.])print\(", text)
