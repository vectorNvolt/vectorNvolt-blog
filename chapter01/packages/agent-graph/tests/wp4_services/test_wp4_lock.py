"""Active-run lock (SSR-AGG-002, SFR-AGG-002)."""

import json
import os
import subprocess
import sys
from pathlib import Path

import agent_graph
from agent_graph.services.lock import ActiveRunLock
from agent_graph.services.runfs import WorkspacePaths

HOLDER = """
import sys
from pathlib import Path
from agent_graph.services.lock import ActiveRunLock
from agent_graph.services.runfs import WorkspacePaths
lock = ActiveRunLock(WorkspacePaths(Path(sys.argv[1])))
assert lock.acquire("held-run") is None
print("ready", flush=True)
sys.stdin.read()
"""


def start_holder(root: Path) -> subprocess.Popen:
    src = str(Path(agent_graph.__file__).resolve().parent.parent)
    env = dict(os.environ, PYTHONPATH=src)
    proc = subprocess.Popen([sys.executable, "-c", HOLDER, str(root)], stdin=subprocess.PIPE,
                            stdout=subprocess.PIPE, env=env, text=True)
    assert proc.stdout.readline().strip() == "ready"
    return proc


def test_second_lock_sees_the_holder_run(tmp_path):
    ws = WorkspacePaths(tmp_path)
    a, b = ActiveRunLock(ws), ActiveRunLock(ws)
    assert a.acquire("run-1") is None
    assert b.acquire(None) == "run-1"
    assert not b.held
    a.set_run("run-2")
    assert b.acquire(None) == "run-2"
    assert json.loads(ws.active_json.read_text())["run_id"] == "run-2"
    a.release()
    assert not ws.active_json.exists()
    assert b.acquire("run-3") is None
    b.release()


def test_holder_without_run_id_is_unknown(tmp_path):
    ws = WorkspacePaths(tmp_path)
    a, b = ActiveRunLock(ws), ActiveRunLock(ws)
    assert a.acquire(None) is None
    assert b.acquire("x") == "unknown"
    a.set_run("run-9")
    assert b.acquire("x") == "run-9"
    a.release()


def test_lock_held_by_another_process(tmp_path):
    ws = WorkspacePaths(tmp_path)
    proc = start_holder(tmp_path)
    try:
        mine = ActiveRunLock(ws)
        assert mine.acquire("new-run") == "held-run"
    finally:
        proc.stdin.close()
        proc.wait(timeout=10)


def test_suspended_or_killed_run_no_longer_counts_as_active(tmp_path):
    ws = WorkspacePaths(tmp_path)
    proc = start_holder(tmp_path)
    proc.kill()                           # the kernel drops the flock with the process
    proc.wait(timeout=10)
    mine = ActiveRunLock(ws)
    assert mine.acquire("next-run") is None
    assert json.loads(ws.active_json.read_text())["run_id"] == "next-run"
    mine.release()


def test_context_manager_releases(tmp_path):
    ws = WorkspacePaths(tmp_path)
    with ActiveRunLock(ws) as a:
        assert a.acquire("r") is None
        assert a.held
    assert not a.held
    assert ActiveRunLock(ws).acquire("r2") is None
