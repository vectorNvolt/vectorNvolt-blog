"""Shared fixtures. pytest runs with pythonpath=["src", "tests"] (see pyproject.toml), so
``agent_graph`` and ``fakes`` import directly. No test touches the network or real /workspace."""

import pytest

from agent_graph.services.runfs import WorkspacePaths, WriteGuard
from fakes.fake_claude import FakeClaude
from fakes.scripted_gateway import ScriptedGateway
from fakes.workspace import build_workspace


@pytest.fixture
def ws_root(tmp_path):
    return tmp_path / "workspace"


@pytest.fixture
def workspace(ws_root) -> WorkspacePaths:
    """A valid example workspace (see fakes/workspace.py)."""
    return build_workspace(ws_root)


@pytest.fixture
def empty_workspace(ws_root) -> WorkspacePaths:
    ws_root.mkdir(parents=True, exist_ok=True)
    return WorkspacePaths(ws_root)


@pytest.fixture
def claude_dir(tmp_path):
    d = tmp_path / "claude_home"
    d.mkdir()
    return d


@pytest.fixture
def guard(workspace, claude_dir) -> WriteGuard:
    return WriteGuard(workspace, claude_dir)


@pytest.fixture
def fake_claude(tmp_path):
    """Factory: ``fake_claude([turn, ...], repeat_last=False) -> FakeClaude``."""
    def make(turns, **kw):
        return FakeClaude(tmp_path, turns, **kw)
    return make


@pytest.fixture
def scripted_gateway():
    """Factory: ``scripted_gateway(replies=[...], confirms=[...], choices=[...]) -> ScriptedGateway``."""
    def make(*a, **kw):
        return ScriptedGateway(*a, **kw)
    return make
