"""CLI entry and the states S1-S2 that end a run before a run folder exists
(SFR-AGG-001, 002, 003, 004, 036, 040, SSR-AGG-002, 016)."""

import io
import json
import os
import signal

import pytest

from agent_graph.main import main
from agent_graph.services.lock import ActiveRunLock
from agent_graph.services.preflight import check_preconditions
from fakes.workspace import PROCESS_YML, add_process, build_workspace


def run_dirs(ws):
    if not ws.agent_dir.is_dir():
        return []
    return [p for p in ws.agent_dir.iterdir() if not p.name.startswith(".") and p.name != "active.json"]


def call(ws, claude_dir, gw, fc, argv=("run",), stderr=None):
    from agent_graph.services.claude_cli import ClaudeCli
    return main(list(argv), gateway=gw, claude=ClaudeCli(cmd=fc.cmd, env=fc.env), workspace=ws.root,
                claude_dir=claude_dir, stderr=stderr or io.StringIO())


# ---------------------------------------------------------------- SFR-AGG-001: usage errors
@pytest.mark.parametrize("argv", [(), ("run", "--bogus"), ("run", "extra"), ("start",), ("run", "--resume")])
def test_usage_error_exits_1_and_creates_nothing(workspace, claude_dir, fake_claude, scripted_gateway, argv):
    err = io.StringIO()
    gw = scripted_gateway()
    before = sorted(p.name for p in workspace.agent_dir.iterdir())
    assert call(workspace, claude_dir, gw, fake_claude([]), argv, err) == 1
    assert "usage: agent_graph run [--resume RUNID]" in err.getvalue()
    assert sorted(p.name for p in workspace.agent_dir.iterdir()) == before
    assert gw.events == []                     # no gateway session for a usage error


def test_resume_of_unknown_run_is_a_usage_error(workspace, claude_dir, fake_claude, scripted_gateway):
    err = io.StringIO()
    assert call(workspace, claude_dir, scripted_gateway(), fake_claude([]), ("run", "--resume", "nope"), err) == 1
    assert "nope" in err.getvalue()


def test_resume_of_run_without_checkpoint_or_ended(workspace, claude_dir, fake_claude, scripted_gateway):
    paths = workspace.run("r1-20260101-000000")
    paths.root.mkdir(parents=True)
    paths.run_json.write_text(json.dumps({"status": "ACTIVE"}), encoding="utf-8")
    err = io.StringIO()
    argv = ("run", "--resume", paths.run_id)
    assert call(workspace, claude_dir, scripted_gateway(), fake_claude([]), argv, err) == 1      # no checkpoint
    paths.checkpoint_db.write_bytes(b"")
    paths.run_json.write_text(json.dumps({"status": "ENDED"}), encoding="utf-8")
    err = io.StringIO()
    assert call(workspace, claude_dir, scripted_gateway(), fake_claude([]), argv, err) == 1      # ended
    assert "ended" in err.getvalue()


def test_resume_id_cannot_escape_the_agent_folder(workspace, claude_dir, fake_claude, scripted_gateway):
    err = io.StringIO()
    assert call(workspace, claude_dir, scripted_gateway(), fake_claude([]), ("run", "--resume", "../x"), err) == 1


# ---------------------------------------------------------------- SFR-AGG-002: S1
def test_second_run_is_refused(workspace, claude_dir, fake_claude, scripted_gateway):
    other = ActiveRunLock(workspace)
    assert other.acquire("other-20260101-000000") is None
    try:
        gw = scripted_gateway(choices=[1], confirms=[True])           # S7 menu: Quit
        assert call(workspace, claude_dir, gw, fake_claude([])) == 1  # NOT_STARTED
    finally:
        other.release()
    assert any("other-20260101-000000" in t for t in gw.said())
    assert run_dirs(workspace) == []


def test_no_process_file_is_not_started(ws_root, claude_dir, fake_claude, scripted_gateway):
    ws = build_workspace(ws_root)
    (ws.process_dir / "example.yml").unlink()
    gw = scripted_gateway(choices=[1], confirms=[True])
    assert call(ws, claude_dir, gw, fake_claude([])) == 1
    assert any("no process file" in t for t in gw.said())
    assert run_dirs(ws) == []


def test_s7_before_a_run_can_check_again(ws_root, claude_dir, fake_claude, scripted_gateway):
    ws = build_workspace(ws_root)
    (ws.process_dir / "example.yml").unlink()
    fixed = []

    class Gw(type(scripted_gateway())):
        def choose(self, source, prompt, options):
            idx = super().choose(source, prompt, options)
            if idx == 0 and not fixed:                      # the developer adds the file, then checks again
                add_process(ws, "example.yml", PROCESS_YML)
                fixed.append(1)
            return idx

    gw = Gw(choices=[0, 1], confirms=[True], replies=["/quit"])   # check again -> S2: quit -> exit 3
    assert call(ws, claude_dir, gw, fake_claude([])) == 3
    assert fixed


def test_user_level_instruction_files_are_a_precondition(workspace, claude_dir, fake_claude, scripted_gateway):
    (claude_dir / "CLAUDE.md").write_text("always be rude\n", encoding="utf-8")
    gw = scripted_gateway(choices=[1], confirms=[True])
    assert call(workspace, claude_dir, gw, fake_claude([])) == 1
    assert any("preconditions" in t for t in gw.said())


def test_check_preconditions(workspace, claude_dir):
    assert check_preconditions(workspace, claude_dir) == []
    (claude_dir / "CLAUDE.md").write_text("   \n", encoding="utf-8")           # blank is empty
    assert check_preconditions(workspace, claude_dir) == []
    (claude_dir / "skills").mkdir()
    assert check_preconditions(workspace, claude_dir) == []                    # empty folder
    (claude_dir / "skills" / "x").mkdir()
    (workspace.root / "CLAUDE.md").write_text("x", encoding="utf-8")
    (workspace.agent_dir / ".claude").mkdir()
    messages = " ".join(f.message for f in check_preconditions(workspace, claude_dir))
    assert "skills" in messages and "CLAUDE.md" in messages and ".claude" in messages


# ---------------------------------------------------------------- SFR-AGG-003/004: S2
def test_quit_in_s2_is_intent_rejected_without_run_folder(workspace, claude_dir, fake_claude, scripted_gateway):
    gw = scripted_gateway(replies=["/quit"])
    assert call(workspace, claude_dir, gw, fake_claude([])) == 3
    assert run_dirs(workspace) == []
    assert not list(workspace.output_root.iterdir())
    assert not workspace.active_json.exists()                  # lock released
    assert any("INTENT_REJECTED" in t for t in gw.said())


def test_duplicate_process_ids_are_not_selectable(workspace, claude_dir, fake_claude, scripted_gateway):
    add_process(workspace, "copy.yml", PROCESS_YML)
    gw = scripted_gateway(choices=[1], confirms=[True])            # S7: Quit
    assert call(workspace, claude_dir, gw, fake_claude([])) == 3   # PROCESS_INVALID
    said = "\n".join(gw.said())
    assert "not selectable" in said and "dup_process_id" in said
    assert run_dirs(workspace) == []


# ---------------------------------------------------------------- SFR-AGG-036: interruption
def test_sigint_suspends_with_exit_130(workspace, claude_dir, fake_claude, scripted_gateway):
    def interrupt(source, prompt):
        os.kill(os.getpid(), signal.SIGINT)
        raise AssertionError("the handler must have raised")

    previous = signal.getsignal(signal.SIGINT)
    gw = scripted_gateway(replies=[interrupt])
    assert call(workspace, claude_dir, gw, fake_claude([])) == 130
    assert signal.getsignal(signal.SIGINT) is previous               # handler restored
    assert not workspace.active_json.exists()
    assert run_dirs(workspace) == []


def test_closed_terminal_suspends(workspace, claude_dir, fake_claude, scripted_gateway):
    gw = scripted_gateway(replies=[])                                # EOF at the first question
    assert call(workspace, claude_dir, gw, fake_claude([])) == 130
