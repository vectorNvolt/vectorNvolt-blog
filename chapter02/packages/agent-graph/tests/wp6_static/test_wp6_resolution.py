"""S7 resolution and the resolution store (SFR-AGG-033, 034, 035, 041, SSR-AGG-016).

Approved changes are applied by the runtime and recorded in resolution/<n>/; the re-entry
state follows the hash comparison; the run ends only on the developer's quit."""

import hashlib
import json
from types import SimpleNamespace

import pytest

from agent_graph.errors import ExitState, RunError, WriteBoundaryError
from agent_graph.nodes import n03_parse_process, n04_bind_input, n07_resolution, n08_report
from agent_graph.services.resolution_store import ResolutionStore
from fakes.workspace import PROCESS_YML, add_process, process_with_id, write_file

BROKEN = PROCESS_YML.replace("skills: [summarize]", "skills: [ghost]")


def sha(text: str) -> str:
    return hashlib.sha256(text.encode("utf-8")).hexdigest()


def edit(path, content, reason="fix"):
    return {"kind": "edit_file", "path": path, "content": content, "reason": reason}


def broken_run(make_env, **kw):
    """A run whose process is invalid: the state S3 failed and S7 is next."""
    h = make_env(**kw)
    h.env.create_run("example", ".process/example.yml")
    write_file(h.ws.root, ".process/example.yml", BROKEN)
    out = n03_parse_process.run(h.env, {})
    assert out["phase"] == "S7"
    return h, out


def calls_after_first(h):
    return h.fc.calls()


# ---------------------------------------------------------------- applying approved changes
def test_approved_edit_is_applied_recorded_and_reenters_at_s3(make_env, dialog_turn):
    h, state = broken_run(make_env, confirms=[True, True], turns=[
        dialog_turn("propose", "The skill name is wrong.",
                    proposals=[edit(".process/example.yml", PROCESS_YML, "restore skill summarize")]),
        dialog_turn("done", "Fixed.")])
    out = n07_resolution.run(h.env, state)
    assert out["phase"] == "S3" and out["reentry"]["state"] == "S3"
    assert (h.ws.process_dir / "example.yml").read_text(encoding="utf-8") == PROCESS_YML

    rdir = h.env.paths.resolution_dir(1)
    changes = [json.loads(x) for x in (rdir / "changes.jsonl").read_text(encoding="utf-8").splitlines()]
    assert len(changes) == 1 and changes[0]["kind"] == "edit_file"
    assert changes[0]["before_sha256"] == sha(BROKEN) and changes[0]["after_sha256"] == sha(PROCESS_YML)
    assert (rdir / "before" / ".process" / "example.yml").read_text(encoding="utf-8") == BROKEN
    assert (rdir / "after" / ".process" / "example.yml").read_text(encoding="utf-8") == PROCESS_YML
    hashes = json.loads((rdir / "hashes.json").read_text(encoding="utf-8"))
    assert hashes["before"]["process_file"] != hashes["after"]["process_file"]
    assert hashes["reentry"]["state"] == "S3"
    assert (rdir / "transcript.jsonl").is_file()

    rec = json.loads(h.env.paths.run_json.read_text(encoding="utf-8"))
    assert rec["resolution_count"] == 1 and rec["status"] == "ACTIVE" and rec["reentry"]["state"] == "S3"
    facts = h.env.facts().all()
    assert [f.kind for f in facts] == ["resolution"] and "restore skill summarize" in facts[0].text
    first, second = h.fc.calls()
    assert argv_tools(first) == "Read"                                      # read-only instance
    assert "edit .process/example.yml: applied" in second["prompt"] and second["resume"]
    assert n03_parse_process.run(h.env, {})["phase"] == "S4"               # the fix works


def argv_tools(call):
    return call["argv"][call["argv"].index("--allowedTools") + 1]


def test_rejected_edit_changes_nothing(make_env, dialog_turn):
    h, state = broken_run(make_env, confirms=[False, True], turns=[
        dialog_turn("propose", "Try this.", proposals=[edit(".process/example.yml", PROCESS_YML)]),
        dialog_turn("done", "Then nothing.")])
    out = n07_resolution.run(h.env, state)
    assert (h.ws.process_dir / "example.yml").read_text(encoding="utf-8") == BROKEN
    assert "rejected by the developer" in h.fc.calls()[1]["prompt"]
    assert out["phase"] == "S3"                                             # nothing changed: the failed state
    assert not (h.env.paths.resolution_dir(1) / "changes.jsonl").exists()


@pytest.mark.parametrize("path", [".agent/x.txt", "../outside.txt", "/etc/passwd", "src/x.py",
                                  "identity/../../outside.txt"])
def test_changes_outside_the_allowed_folders_are_refused(make_env, dialog_turn, path):
    h, state = broken_run(make_env, confirms=[True], turns=[
        dialog_turn("propose", "Write it.", proposals=[edit(path, "x")]),
        dialog_turn("done", "ok")])
    n07_resolution.run(h.env, state)
    asked = [t for k, _s, t in h.gw.events if k == "confirm" and t == "Apply this change?"]
    assert asked == []                                                      # refused before asking
    assert "refused" in h.fc.calls()[1]["prompt"]
    assert not (h.ws.root / ".agent" / "x.txt").exists()
    assert not (h.ws.root.parent / "outside.txt").exists()


def test_new_pool_file_and_delete(make_env, dialog_turn):
    snippet = "---\nid: extra\nlayer: step\ntags: []\nauto: false\n---\nextra\n"
    h, state = broken_run(make_env, confirms=[True, True, True], turns=[
        dialog_turn("propose", "Add and drop.", proposals=[
            edit("identity/.snippet/extra.md", snippet),
            edit("identity/.snippet/step-auto.md", None, "remove")]),
        dialog_turn("done", "ok")])
    out = n07_resolution.run(h.env, state)
    assert (h.ws.snippet_dir / "extra.md").read_text(encoding="utf-8") == snippet
    assert not (h.ws.snippet_dir / "step-auto.md").exists()
    assert out["phase"] == "S3"                                             # pools changed
    changes = h.env.facts().all()
    assert "extra.md" in changes[0].text and "step-auto.md" in changes[0].text


def test_input_change_reenters_at_s4(make_env, dialog_turn):
    h = make_env(confirms=[True, True], turns=[
        dialog_turn("propose", "Fix the input.", proposals=[edit(".input/example/source.md", "# New\n")]),
        dialog_turn("done", "ok")])
    h.env.create_run("example", ".process/example.yml")
    assert n03_parse_process.run(h.env, {})["phase"] == "S4"
    assert n04_bind_input.run(h.env, {})["phase"] == "S5"
    err = RunError(ExitState.INPUT_INVALID, "S4", "the input does not match")
    out = n07_resolution.run(h.env, {"error": err.to_json()})
    assert out["phase"] == "S4"


def test_override_only_reenters_at_the_failed_state_and_restarts_the_node(make_env, dialog_turn):
    h = make_env(confirms=[True, True], turns=[
        dialog_turn("propose", "Allow more retries.", proposals=[
            {"kind": "override", "key": "retry", "node": "write-summary", "value": 5, "reason": "more retries"}]),
        dialog_turn("done", "ok")])
    h.env.create_run("example", ".process/example.yml")
    n03_parse_process.run(h.env, {})
    n04_bind_input.run(h.env, {})
    h.env.spec = SimpleNamespace()                                          # a built graph exists
    err = RunError(ExitState.NODE_FAILED, "S6", "failed twice", node="write-summary")
    out = n07_resolution.run(h.env, {"error": err.to_json()})
    assert out["phase"] == "S6"
    re = h.env.record.reentry
    assert re.restart_node == "write-summary" and "override retry" in re.summary
    overrides = json.loads(h.env.paths.overrides_json.read_text(encoding="utf-8"))
    assert overrides == {"retry": {"write-summary": 5}}


def test_context_not_loaded_falls_back_to_s3(make_env, dialog_turn):
    h = make_env(confirms=[True], turns=[dialog_turn("done", "ok")])
    h.env.create_run("example", ".process/example.yml")
    err = RunError(ExitState.NODE_FAILED, "S6", "failed", node="write-summary")   # e.g. after --resume of S7
    assert n07_resolution.run(h.env, {"error": err.to_json()})["phase"] == "S3"


def test_choosing_another_process_starts_a_new_run(ws_root, make_env, dialog_turn):
    h, state = broken_run(make_env, confirms=[True, True], turns=[
        dialog_turn("propose", "Use the other one.",
                    proposals=[{"kind": "choose_process", "value": "other", "reason": "wrong process"}]),
        dialog_turn("done", "ok")])
    add_process(h.ws, "other.yml", process_with_id("other"))
    old_id = h.env.paths.run_id
    out = n07_resolution.run(h.env, state)
    assert out["phase"] == "S2" and out["process_id"] == "other" and out["reentry"]["new_run"] is True
    assert h.env.paths is None                                              # the old run is kept on disk
    old = json.loads(h.ws.run(old_id).run_json.read_text(encoding="utf-8"))
    assert old["status"] == "ESCALATED" and old["phase"] == "S7"


# ---------------------------------------------------------------- quit and resume
def test_quit_keeps_the_checkpoint_and_ends_with_the_error_exit_state(make_env, dialog_turn):
    h, state = broken_run(make_env, replies=["/quit"], turns=[dialog_turn("ask", "What now?")])
    out = n07_resolution.run(h.env, state)
    assert out["phase"] == "S8" and out["quit"] is True and out["exit_state"] == "PROCESS_INVALID"
    n08_report.run(h.env, out)
    paths = h.env.paths
    rec = json.loads(paths.run_json.read_text(encoding="utf-8"))
    assert rec["status"] == "ESCALATED" and rec["phase"] == "S7" and rec["exit_code"] == 3
    assert rec["open_error"]["exit_state"] == "PROCESS_INVALID"
    report = paths.report_md.read_text(encoding="utf-8")
    assert "Open error" in report and "skill_missing" in report and "## Resolution" in report
    assert paths.has_checkpoint()


def test_text_only_menu_when_claude_is_unavailable(make_env):
    h, state = broken_run(make_env, turns=[{"login_error": True}], choices=[1, 1, 0],
                          replies=["bogus 1", "retry 3"])
    out = n07_resolution.run(h.env, state)
    assert [n[1] for n in h.gw.notices] == ["LLM_UNAVAILABLE"]
    assert out["phase"] == "S3"
    assert json.loads(h.env.paths.overrides_json.read_text(encoding="utf-8")) == {"retry": 3}
    assert any("refused" in t for t in h.gw.said("S7 resolution"))


def test_text_only_menu_quit_needs_confirmation(make_env):
    h, state = broken_run(make_env, turns=[{"login_error": True}], choices=[2, 2], confirms=[False, True])
    out = n07_resolution.run(h.env, state)
    assert out["phase"] == "S8" and out["quit"] is True
    assert len([e for e in h.gw.events if e[0] == "confirm"]) == 2


@pytest.mark.parametrize("choice,phase", [(2, "S8"), (0, "S3")])
def test_turn_limit_is_not_resolved_and_needs_an_explicit_menu_choice(make_env, dialog_turn, monkeypatch,
                                                                     choice, phase):
    """SFR-AGG-033/035: reaching MAX_TURNS does not re-enter the run; the developer is told and
    the text-only menu waits for an explicit choice (quit is confirmed)."""
    monkeypatch.setattr(n07_resolution, "MAX_TURNS", 2)
    h, state = broken_run(make_env, replies=["hm", "still thinking"], choices=[choice], confirms=[True],
                          turns=[dialog_turn("ask", "What now?"), dialog_turn("ask", "And now?")])
    out = n07_resolution.run(h.env, state)
    assert [n[1] for n in h.gw.notices] == ["RESOLUTION_LIMIT"]
    assert len(h.fc.calls()) == 2
    menus = [e for e in h.gw.events if e[0] == "choose"]
    assert len(menus) == 1 and "Re-check now" in menus[0][2]
    assert out["phase"] == phase
    if phase == "S8":
        assert out["quit"] is True and out["exit_state"] == "PROCESS_INVALID"
    else:
        assert out["reentry"]["state"] == "S3"


def test_turn_limit_menu_without_a_choice_suspends_not_resolves(make_env, dialog_turn, monkeypatch):
    from agent_graph.errors import TerminalClosed
    monkeypatch.setattr(n07_resolution, "MAX_TURNS", 1)
    h, state = broken_run(make_env, replies=["hm"], turns=[dialog_turn("ask", "What now?")])
    with pytest.raises(TerminalClosed):                  # no scripted choice: EOF, never "resolved"
        n07_resolution.run(h.env, state)


def test_resolve_command_reenters_after_a_manual_fix(make_env, dialog_turn):
    h, state = broken_run(make_env, replies=["/resolved"], turns=[dialog_turn("ask", "Fix it yourself?")])
    write_file(h.ws.root, ".process/example.yml", PROCESS_YML)             # the developer edits the file
    out = n07_resolution.run(h.env, state)
    assert out["phase"] == "S3" and out["reentry"]["state"] == "S3"


def test_before_a_run_s7_is_a_text_menu_without_claude(make_env):
    h = make_env(choices=[0], turns=[])
    err = RunError(ExitState.NOT_STARTED, "S1", "no process file in /workspace/.process/")
    out = n07_resolution.run(h.env, {"error": err.to_json()})
    assert out == {"phase": "S1", "error": None}
    assert h.fc.calls() == []


# ---------------------------------------------------------------- ResolutionStore
@pytest.fixture
def store(workspace, guard):
    return ResolutionStore(workspace, guard, workspace.agent_dir / "res" / "1",
                           workspace.agent_dir / "res" / "overrides.json")


def test_store_needs_approval(workspace, store):
    before = (workspace.process_dir / "example.yml").read_text(encoding="utf-8")
    with pytest.raises(WriteBoundaryError):
        store.apply_edit(".process/example.yml", "changed", "r", approved=False)
    assert (workspace.process_dir / "example.yml").read_text(encoding="utf-8") == before
    with pytest.raises(WriteBoundaryError):
        store.apply_override("retry", 3, None, "r", approved=False)


@pytest.mark.parametrize("path", [None, "", "x", ".agent/x", "../x", "/abs", "templates/report.md"])
def test_store_rejects_paths_outside_the_pools(store, path):
    with pytest.raises(WriteBoundaryError):
        store.apply_edit(path, "x", "r", approved=True)


@pytest.mark.parametrize("key,value,node", [("nope", 1, None), ("retry", "abc", None), ("retry", -1, None),
                                            ("budgets.max_steps", 0, None), ("budgets.max_steps", 5, "n"),
                                            ("budget", 5, None), ("timeout", "10x", None)])
def test_store_rejects_invalid_overrides(store, key, value, node):
    with pytest.raises(ValueError):
        store.apply_override(key, value, node, "r", approved=True)


def test_store_override_forms(workspace, store):
    store.apply_override("retry", "3", None, "r", approved=True)
    store.apply_override("timeout", "20m", None, "r", approved=True)
    store.apply_override("budgets.max_steps", 80, None, "r", approved=True)
    store.apply_override("budgets.timeout", "2h", None, "r", approved=True)
    store.apply_override("budget", "60k", "write-summary", "r", approved=True)
    data = json.loads((workspace.agent_dir / "res" / "overrides.json").read_text(encoding="utf-8"))
    assert data == {"retry": 3, "timeout": 1200, "budgets": {"max_steps": 80, "timeout": 7200},
                    "step_budget": {"write-summary": 60000}}
    assert [c["seq"] for c in store.changes()] == [1, 2, 3, 4, 5]
    with pytest.raises(ValueError):                       # global and per-node values do not mix
        store.apply_override("retry", 1, "write-summary", "r", approved=True)
