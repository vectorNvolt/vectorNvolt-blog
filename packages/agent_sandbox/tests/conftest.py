import io

import pytest
from rich.console import Console

from agent_sandbox.services import render


@pytest.fixture
def screen(monkeypatch):
    """Swap the renderer's console for one that writes the raw byte stream a
    terminal would receive into a buffer. `force_terminal` keeps Rich's own
    escape codes on, so the test sees exactly what the terminal would."""
    buf = io.StringIO()
    console = Console(file=buf, force_terminal=True, width=100, theme=render.THEME)
    monkeypatch.setattr(render, "console", console)
    return buf
