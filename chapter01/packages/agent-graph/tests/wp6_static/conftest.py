"""Fixtures of the WP6 tests: a master environment (``Env``) over a temporary workspace, with
a fake Claude CLI and a scripted gateway; plus helpers to script dialog replies."""

import json
from datetime import datetime
from types import SimpleNamespace

import pytest

from agent_graph.nodes import Env
from agent_graph.services.claude_cli import ClaudeCli
from agent_graph.services.lock import ActiveRunLock
from agent_graph.services.runfs import WriteGuard
from fakes.workspace import build_workspace


def _dialog_turn(action: str, say: str = "ok", *, process_id=None, proposals=(), **extra) -> dict:
    """A fake Claude turn whose answer is one dialog reply (dialog_reply.schema.yml)."""
    body = {"say": say, "action": action, "process_id": process_id, "proposals": list(proposals)}
    return {"text": json.dumps(body), **extra}


@pytest.fixture
def dialog_turn():
    """``dialog_turn(action, say, process_id=None, proposals=()) -> fake Claude turn``."""
    return _dialog_turn


@pytest.fixture
def make_env(ws_root, claude_dir, fake_claude, scripted_gateway):
    """``make_env(turns=(), replies=(), ws=None, **gateway_kwargs) -> SimpleNamespace(env, gw, fc, ws)``.
    The active-run lock is held, as after S1."""
    made = []

    def make(turns=(), replies=(), ws=None, **gw_kwargs):
        ws = ws if ws is not None else build_workspace(ws_root)
        fc = fake_claude(list(turns))
        gw = scripted_gateway(replies=list(replies), **gw_kwargs)
        env = Env(ws=ws, guard=WriteGuard(ws, claude_dir), gateway=gw,
                  claude=ClaudeCli(cmd=fc.cmd, env=fc.env), lock=ActiveRunLock(ws),
                  claude_dir=claude_dir, clock=lambda: datetime(2026, 1, 2, 3, 4, 5))
        assert env.lock.acquire(None) is None
        made.append(env)
        return SimpleNamespace(env=env, gw=gw, fc=fc, ws=ws)

    yield make
    for env in made:
        env.lock.release()
