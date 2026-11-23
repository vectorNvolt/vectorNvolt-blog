"""SSR-AGG-014 layout, SFR-AGG-041/SSR-AGG-007 write guard, SSR-AGG-008 input read guard."""

import json
import os
import threading
from datetime import datetime

import pytest

from agent_graph.errors import InputReadDenied, WriteBoundaryError
from agent_graph.services.runfs import (
    InputReadGuard, WorkspacePaths, WriteGuard, append_jsonl, canonical_json, new_run_id, read_jsonl,
    sha256_bytes, sha256_file, write_atomic)
from agent_graph.state import Binding, BoundFile, BoundSource


def test_new_run_id():
    assert new_run_id("proc_m0", datetime(2025, 3, 4, 5, 6, 7)) == "proc_m0-20250304-050607"


def test_layout(tmp_path):
    ws = WorkspacePaths(tmp_path)
    assert ws.process_dir == tmp_path / ".process"
    assert ws.input_dir == tmp_path / ".input"
    assert ws.skills_dir == tmp_path / "identity" / ".skills"
    assert ws.snippet_dir == tmp_path / "identity" / ".snippet"
    assert ws.agent_dir == tmp_path / ".agent"
    assert ws.output_root == tmp_path / ".output"
    assert ws.cache_summaries == tmp_path / ".agent" / "cache" / "summaries"
    assert ws.static_dir == tmp_path / ".agent" / ".static"
    assert ws.lock_file == tmp_path / ".agent" / ".active.lock"
    assert ws.active_json == tmp_path / ".agent" / "active.json"
    r = ws.run("p-1")
    base = tmp_path / ".agent" / "p-1"
    assert r.root == base
    for attr, rel in {
        "run_json": "run.json", "binding_json": "binding.json", "index_json": "index.json",
        "graph_json": "graph.json", "graph_error_json": "graph.error.json",
        "gateway_jsonl": "gateway.jsonl", "history_jsonl": "history.jsonl",
        "run_facts_jsonl": "run_facts.jsonl", "overrides_json": "overrides.json",
        "checkpoint_db": "checkpoint.sqlite", "mcp_json": "mcp.json", "active": "active",
        "nodes_dir": "nodes", "work_dir": "work", "archive_tgz": "archive/work.tar.gz",
        "dialogs_dir": "dialogs", "tmp_dir": "tmp"}.items():
        assert getattr(r, attr) == base / rel, attr
    assert r.output_dir == tmp_path / ".output" / "p-1"
    assert r.report_md == tmp_path / ".output" / "p-1" / "report.md"
    assert r.output_invalid_json == tmp_path / ".output" / "p-1" / "OUTPUT_INVALID.json"
    assert r.node_dir("n") == base / "nodes" / "n"
    assert r.work_node("n") == base / "work" / "n"
    assert r.resolution_dir(2) == base / "resolution" / "2"


def test_has_checkpoint(tmp_path):
    r = WorkspacePaths(tmp_path).run("p-1")
    assert not r.has_checkpoint()
    r.root.mkdir(parents=True)
    r.run_json.write_text("{}")
    assert not r.has_checkpoint()
    r.checkpoint_db.write_bytes(b"")
    assert r.has_checkpoint()


def test_hash_and_canonical_json(tmp_path):
    p = tmp_path / "f"
    p.write_bytes(b"abc")
    assert sha256_file(p) == sha256_bytes(b"abc")
    out = canonical_json({"b": 1, "a": {"y": "é", "x": [1]}})
    assert out == '{\n  "a": {\n    "x": [\n      1\n    ],\n    "y": "é"\n  },\n  "b": 1\n}\n'


def test_write_atomic_and_jsonl(workspace, guard):
    r = workspace.run("p-1")
    write_atomic(r.run_json, '{"a": 1}', guard=guard)
    write_atomic(r.run_json, b'{"a": 2}', guard=guard)
    assert json.loads(r.run_json.read_text()) == {"a": 2}
    assert [p.name for p in r.root.iterdir()] == ["run.json"]          # no temp leftovers
    assert read_jsonl(r.history_jsonl) == []
    append_jsonl(r.history_jsonl, {"b": 1, "a": 2}, guard=guard)
    append_jsonl(r.history_jsonl, {"n": 2}, guard=guard)
    assert r.history_jsonl.read_text().splitlines()[0] == '{"a": 2, "b": 1}'
    assert read_jsonl(r.history_jsonl) == [{"a": 2, "b": 1}, {"n": 2}]


def test_read_jsonl_drops_torn_last_line(tmp_path):
    p = tmp_path / "x.jsonl"
    p.write_text('{"a": 1}\n{"a": 2')
    assert read_jsonl(p) == [{"a": 1}]
    p.write_text('{"a": 1\n{"a": 2}\n')
    with pytest.raises(json.JSONDecodeError):
        read_jsonl(p)


def test_guard_allows_workspace_and_claude_dir(workspace, guard, claude_dir):
    guard.check(workspace.root / ".agent" / "x" / "y.json")
    guard.check(workspace.output_root / "r" / "report.md")
    guard.check(claude_dir / "settings.json")


def test_guard_rejects_outside(workspace, guard, tmp_path):
    for bad in (tmp_path / "elsewhere.txt", workspace.root / ".." / "escape.txt", "/etc/passwd", "/tmp/x"):
        with pytest.raises(WriteBoundaryError):
            guard.check(bad)
    with pytest.raises(WriteBoundaryError):
        write_atomic(tmp_path / "elsewhere.txt", "x", guard=guard)
    assert not (tmp_path / "elsewhere.txt").exists()


def test_guard_rejects_symlink_escape(workspace, guard, tmp_path):
    outside = tmp_path / "outside"
    outside.mkdir()
    link = workspace.agent_dir / "link"
    link.symlink_to(outside)
    with pytest.raises(WriteBoundaryError):
        guard.check(link / "f.txt")


def test_guard_protects_pools_outside_resolution(workspace, guard):
    for p in (workspace.process_dir / "example.yml", workspace.input_dir / "example" / "source.md",
              workspace.snippet_dir / "step-notes.md", workspace.skills_dir / "summarize" / "SKILL.md",
              workspace.identity_dir / "new.md"):
        with pytest.raises(WriteBoundaryError):
            guard.check(p)
    with pytest.raises(WriteBoundaryError):
        append_jsonl(workspace.process_dir / "x.jsonl", {"a": 1}, guard=guard)


def test_guard_resolution_opens_pools(workspace, guard):
    target = workspace.process_dir / "example.yml"
    with pytest.raises(WriteBoundaryError):
        guard.resolution(approved=False)
    with guard.resolution(approved=True):
        write_atomic(target, "apiVersion: agent-graph/v1\n", guard=guard)
    assert target.read_text() == "apiVersion: agent-graph/v1\n"
    with pytest.raises(WriteBoundaryError):             # closed again
        guard.check(target)


def test_guard_resolution_is_thread_local(workspace, guard):
    errors = []

    def other():
        try:
            guard.check(workspace.process_dir / "example.yml")
            errors.append("allowed")
        except WriteBoundaryError:
            errors.append("denied")

    with guard.resolution(approved=True):
        t = threading.Thread(target=other)
        t.start()
        t.join()
    assert errors == ["denied"]


def _binding(*paths):
    return Binding("example", ".process/example.yml", "h",
                   {"Source": BoundSource("Source", tuple(BoundFile(p, "h") for p in paths), None)}, {}, {})


def test_input_read_guard(workspace):
    ok = ".input/example/source.md"
    other = workspace.input_dir / "example" / "other.md"
    other.write_text("secret")
    g = InputReadGuard(workspace, _binding(ok))
    assert "Alpha line." in g.open_text(workspace.root / ok)
    assert "Alpha line." in g.open_text(ok)                                  # workspace-relative
    with pytest.raises(InputReadDenied):
        g.open_text(other)
    with pytest.raises(InputReadDenied):                                      # traversal still resolves
        g.open_text(workspace.root / ".input" / "example" / ".." / "example" / "other.md")
    # non-.input files (template, products) are not restricted by this guard
    assert g.open_text("templates/report.md").startswith("# Summary")
    with pytest.raises(InputReadDenied):
        InputReadGuard(workspace, None).open_text(ok)
