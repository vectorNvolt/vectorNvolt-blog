"""SFR-AGG-014 / SFR-AGG-016, USC-005: ``isolate: true`` runs through the Go tool run_ephemeral_container.

The tool is a fake: a Python script in tmp_path that parses the same grammar, prints the exec_dir
line, creates ``<ws>/.output/exec/<id>/out``, writes ``result.json`` and behaves as FAKE_MODE says.
"""

import json
import re
import threading
import time
from types import SimpleNamespace

import pytest

from agent_graph.contract import start
from agent_graph.dynamic.journal import StepFailure
from agent_graph.dynamic.step_procedure import expand_argv_isolated, label_value, make_exec_id
from agent_graph.errors import TurnCancelled
from agent_graph.ports import CancelToken
from agent_graph.process.validate import validate_process
from agent_graph.services import container as container_mod
from agent_graph.services.container import ContainerExec
from agent_graph.state import Binding, BoundFile, BoundSource

from test_wp5_helpers import (RUN_ID, ScriptedClaude, decl, drive, dynamic_setup, edge, pipeline, process,
                              step)

FAKE_TOOL = r'''#!/usr/bin/env python3
import json, os, pathlib, signal, sys, time
a = sys.argv[1:]
mode = os.environ.get("FAKE_MODE", "ok")
logp = os.environ["FAKE_LOG"]
if a[0] == "list":
    print(os.environ.get("FAKE_LIST", "[]"))
    sys.exit(0)
if a[0] == "sweep":
    pathlib.Path(logp + ".sweep").write_text(json.dumps(a))
    sys.exit(0)
i = a.index("--")
head, cmd = a[:i], a[i + 1:]
rec = {"head": head, "profile": head[1], "exec_id": None, "ins": [], "labels": [], "cmd": cmd}
j = 2
while j < len(head):
    if head[j] == "--exec-id":
        rec["exec_id"] = head[j + 1]
    elif head[j] == "--in":
        rec["ins"].append(head[j + 1])
    elif head[j] == "--label":
        rec["labels"].append(head[j + 1])
    j += 2
pathlib.Path(logp).write_text(json.dumps(rec))
if mode == "nodir":
    sys.stderr.write("run_ephemeral_container: usage: bad flag\n")
    sys.exit(125)
ed = pathlib.Path(os.environ["FAKE_WS"]) / ".output" / "exec" / rec["exec_id"]
out = ed / "out"
out.mkdir(parents=True)
sys.stderr.write("run_ephemeral_container: exec_dir=%s\n" % ed)
sys.stderr.flush()


def result(cause, code):
    (ed / "result.json").write_text(json.dumps(
        {"exit": code, "cause": cause, "exec_dir": str(ed), "image_id": "sha256:x", "start": "s", "end": "e"}))


if mode in ("cancel", "hang"):
    def on_term(sig, frm):
        pathlib.Path(logp + ".term").write_text("term")
        result("cancel", 130)
        sys.exit(130)
    signal.signal(signal.SIGTERM, on_term)
    pathlib.Path(logp + ".started").write_text("1")
    time.sleep(60)
if mode in ("setup", "busy"):
    sys.stderr.write("run_ephemeral_container: %s: refused\n" % mode)
    result(mode, 125)
    sys.exit(125)
(out / ".outcome").write_text("good\n")
(out / "facts.json").write_text("[1]")
(out / "sub").mkdir()
(out / "sub" / "n.txt").write_text("n")
os.symlink("/etc/passwd", str(out / "link"))
print("WARNING: from tool")
if mode == "exit":
    sys.stderr.write("boom\n")
    result("exit", 3)
    sys.exit(3)
result("ok", 0)
'''


@pytest.fixture
def fake(tmp_path, workspace, monkeypatch):
    tool = tmp_path / "bin" / "run_ephemeral_container"
    tool.parent.mkdir()
    tool.write_text(FAKE_TOOL, encoding="utf-8")
    tool.chmod(0o755)
    log = tmp_path / "log.json"
    monkeypatch.setenv("FAKE_LOG", str(log))
    monkeypatch.setenv("FAKE_WS", str(workspace.root))
    monkeypatch.setenv("FAKE_MODE", "ok")
    monkeypatch.setenv(container_mod.ENV_TOOL, str(tool))
    return SimpleNamespace(path=str(tool), log=log,
                           rec=lambda: json.loads(log.read_text()),
                           marker=lambda suffix: log.with_name(log.name + suffix))


def _backend(fake):
    return ContainerExec(fake.path, grace_s=3)


def _call(fake, timeout_s=None, cancel=None):
    return _backend(fake).run_isolated(
        "py", ["python", "/workspace/src/a.py", "{in.source}"], [("source", "/x/y")],
        {"agent-graph.run": "r1", "agent-graph.node": "n"}, "ag-n-1-1-abcd1234", timeout_s,
        cancel or CancelToken())


# ------------------------------------------------------------------ ContainerExec

def test_plain_run_is_not_supported(tmp_path):
    with pytest.raises(NotImplementedError, match="run_isolated"):
        ContainerExec().run(["x"], tmp_path, {}, None, CancelToken())


def test_run_isolated_maps_the_arguments_and_returns_the_result(fake):
    res = _call(fake)
    rec = fake.rec()
    assert rec["head"] == ["run", "py", "--exec-id", "ag-n-1-1-abcd1234", "--in", "source=/x/y",
                           "--label", "agent-graph.run=r1", "--label", "agent-graph.node=n"]
    assert rec["cmd"] == ["python", "/workspace/src/a.py", "{in.source}"]
    assert "--timeout" not in rec["head"]
    assert res.exit_code == 0 and res.cause == "ok"
    assert res.exec_dir.endswith(".output/exec/ag-n-1-1-abcd1234")
    assert "WARNING: from tool" in res.stdout_tail


def test_nonzero_exit_has_cause_exit_and_the_command_stderr(fake, monkeypatch):
    monkeypatch.setenv("FAKE_MODE", "exit")
    res = _call(fake)
    assert res.exit_code == 3 and res.cause == "exit"
    assert "boom" in res.stderr_tail and "cause=" not in res.stderr_tail


@pytest.mark.parametrize("mode", ["setup", "busy"])
def test_tool_internal_cause_is_prefixed_to_stderr(fake, monkeypatch, mode):
    monkeypatch.setenv("FAKE_MODE", mode)
    res = _call(fake)
    assert res.exit_code == 125 and res.cause == mode
    assert res.stderr_tail.startswith(f"run_ephemeral_container: cause={mode}\n")


def test_failure_before_the_exec_dir_exists(fake, monkeypatch):
    monkeypatch.setenv("FAKE_MODE", "nodir")
    res = _call(fake)
    assert res.exec_dir is None and res.exit_code == 125 and res.cause == "usage"
    assert "bad flag" in res.stderr_tail


def test_cancel_sends_sigterm_to_the_tool_and_raises(fake, monkeypatch):
    monkeypatch.setenv("FAKE_MODE", "cancel")
    tok = CancelToken()

    def later():
        for _ in range(200):
            if fake.marker(".started").exists():
                break
            time.sleep(0.05)
        tok.cancel("budget:max_steps")
    threading.Thread(target=later, daemon=True).start()
    t0 = time.monotonic()
    with pytest.raises(TurnCancelled) as e:
        _call(fake, cancel=tok)
    assert e.value.cause == "budget:max_steps" and time.monotonic() - t0 < 10
    assert fake.marker(".term").exists()                     # the tool got SIGTERM and cleaned up


def test_timeout_is_enforced_by_us_with_sigterm(fake, monkeypatch):
    monkeypatch.setenv("FAKE_MODE", "hang")
    with pytest.raises(TurnCancelled) as e:
        _call(fake, timeout_s=1.5)
    assert e.value.cause == "timeout" and fake.marker(".term").exists()


def test_missing_tool_raises_and_sweep_never_does(monkeypatch):
    monkeypatch.delenv(container_mod.ENV_TOOL, raising=False)
    monkeypatch.setenv("PATH", "")
    monkeypatch.setattr(container_mod, "DEFAULT_TOOL", "/nonexistent/run_ephemeral_container")
    ce = ContainerExec("/nonexistent/tool")
    with pytest.raises(FileNotFoundError):
        ce.run_isolated("py", ["x"], [], {}, "ag-x", None, CancelToken())
    ce.sweep("r1")


def test_sweep_uses_the_run_label(fake):
    _backend(fake).sweep("r1")
    assert json.loads(fake.marker(".sweep").read_text()) == ["sweep", "--label", "agent-graph.run=r1"]


def test_tool_resolution_order(fake, tmp_path, monkeypatch):
    other = tmp_path / "other"
    other.write_text("#!/bin/sh\n")
    other.chmod(0o755)
    assert container_mod.resolve_tool(str(other)) == str(other)          # argument first
    assert container_mod.resolve_tool() == fake.path                     # then AGENT_GRAPH_EXEC_TOOL


# ------------------------------------------------------------------ argv of an isolated step

def _ctx(workspace):
    src = BoundSource("Source", tuple(BoundFile(f".input/example/s{i}.md", "0" * 64) for i in range(2)), None)
    tpl = {"Template": BoundFile("templates/report.md", "0"), "Lib": BoundFile("src/lib.txt", "0")}
    binding = Binding("p", "f", "0" * 64, {"Source": src}, tpl, {})
    return SimpleNamespace(paths=workspace.run(RUN_ID), binding=binding, process=pipeline())


def test_exec_id_fits_the_tools_grammar():
    for node in ("extract", "My Node_1", "x" * 80, "--"):
        eid = make_exec_id(node, 12, 3)
        assert re.fullmatch(r"[a-z0-9][a-z0-9-]{0,63}", eid), eid
    assert make_exec_id("extract", 1, 2).startswith("ag-extract-1-2-")
    assert make_exec_id("extract", 1, 2) != make_exec_id("extract", 1, 2)
    assert label_value("a b/c") == "a-b-c"


def test_expand_argv_isolated_maps_inputs_and_out_paths(workspace):
    ctx = _ctx(workspace)
    root = workspace.root
    out = root / ".output/exec/ag-e/out"
    s = step(1, "s", "procedure", produces=(decl("facts", "facts.json"),),
             run=("python", "src/job.py", "{input.Source}", "{input.Template}", "{my-facts}", "{facts}",
                  "emb={work}/x", "lib={input.Lib}", "{unknown}"))
    state = {"products": {"my-facts": {"path": ".agent/r/work/x/facts.json"}}}
    argv, inputs = expand_argv_isolated(s, state, ctx, "ag-e")
    assert argv == ["python", "src/job.py", "{in.source}", "{in.template}", "{in.my_facts}",
                    str(out / "facts.json"), f"emb={out}/x", f"lib={root / 'src/lib.txt'}", "{unknown}"]
    assert inputs == [("source", str(root / ".input/example/s0.md")), ("source", str(root / ".input/example/s1.md")),
                      ("template", str(root / "templates/report.md")),
                      ("my_facts", str(root / ".agent/r/work/x/facts.json"))]


def test_embedded_input_outside_src_is_refused(workspace):
    ctx = _ctx(workspace)
    s = step(1, "s", "procedure", run=("python", "pre{input.Template}"))
    with pytest.raises(StepFailure) as e:
        expand_argv_isolated(s, {}, ctx, "ag-e")
    assert not e.value.retryable and "src/" in e.value.reason


def test_input_name_that_is_not_a_tool_name_is_refused(workspace):
    ctx = _ctx(workspace)
    ctx.binding.templates["9bad"] = BoundFile("templates/x.md", "0")
    s = step(1, "s", "procedure", run=("python", "{input.9bad}"))
    with pytest.raises(StepFailure) as e:
        expand_argv_isolated(s, {}, ctx, "ag-e")
    assert not e.value.retryable


# ------------------------------------------------------------------ the procedure step

def _iso_machine(workspace, guard, fake, **kw):
    s = step(1, "iso", "procedure", produces=(decl("facts", "facts.json"),), outcomes=("good",),
             run=("python", "src/job.py", "{input.Source}", "{facts}"), isolate=True, profile="py", **kw)
    proc = process([s], [edge("iso", "END", "outcome", "good")], "iso", retry=0)
    setup = dynamic_setup(workspace, guard, proc, ScriptedClaude([]))
    setup.ctx.container = _backend(fake)
    return setup


def test_isolated_step_copies_out_into_work_and_completes(workspace, guard, fake):
    s = _iso_machine(workspace, guard, fake)
    _, last = drive(s.machine(), start())
    assert last.name == "COMPLETED", last
    work = s.paths.work_node("iso")
    assert (work / "facts.json").read_text() == "[1]"
    assert (work / ".outcome").read_text().strip() == "good"
    assert (work / "sub" / "n.txt").read_text() == "n"
    assert not (work / "link").exists() and not (work / "link").is_symlink()          # symlinks are skipped
    rec = fake.rec()
    root = workspace.root
    eid = rec["exec_id"]
    assert re.fullmatch(r"ag-iso-1-1-[0-9a-f]{8}", eid)
    assert rec["profile"] == "py" and "--timeout" not in rec["head"]
    assert rec["ins"] == [f"source={root / '.input/example/source.md'}"]
    assert rec["cmd"][-2:] == ["{in.source}", str(root / ".output/exec" / eid / "out/facts.json")]
    assert rec["cmd"][1] == str(root / "src/job.py")
    assert f"agent-graph.run={RUN_ID}" in rec["labels"] and "agent-graph.node=iso" in rec["labels"]


def test_isolated_nonzero_exit_is_a_retryable_tool_failure(workspace, guard, fake, monkeypatch):
    monkeypatch.setenv("FAKE_MODE", "exit")
    _, last = drive(_iso_machine(workspace, guard, fake).machine(), start())
    assert last.name == "NODE_FAILED" and last.payload["kind"] == "tool"
    assert "status 3" in last.payload["reason"] and last.payload["retryable"]


def test_isolated_setup_cause_is_not_retryable_and_named(workspace, guard, fake, monkeypatch):
    monkeypatch.setenv("FAKE_MODE", "setup")
    _, last = drive(_iso_machine(workspace, guard, fake).machine(), start())
    assert last.name == "NODE_FAILED" and not last.payload["retryable"]
    assert "cause setup" in last.payload["reason"]


def test_isolated_busy_cause_is_retryable(workspace, guard, fake, monkeypatch):
    monkeypatch.setenv("FAKE_MODE", "busy")
    _, last = drive(_iso_machine(workspace, guard, fake).machine(), start())
    assert last.name == "NODE_FAILED" and last.payload["retryable"] and "cause busy" in last.payload["reason"]


def test_isolated_step_timeout_terminates_the_tool(workspace, guard, fake, monkeypatch):
    monkeypatch.setenv("FAKE_MODE", "hang")
    _, last = drive(_iso_machine(workspace, guard, fake, timeout_s=2).machine(), start())
    assert last.name == "NODE_FAILED" and last.payload["kind"] == "timeout" and last.payload["retryable"]
    assert fake.marker(".term").exists()


# ------------------------------------------------------------------ validation

def _isolate_codes(workspace, *, run=("python", "src/job.py", "{input.Source}"), isolate=True, profile="py"):
    s = step(1, "iso", "procedure", run=run, isolate=isolate, profile=profile)
    proc = process([s], [edge("iso", "END")], "iso")
    return {f.code for f in validate_process(proc, workspace) if f.code.startswith("isolate_")}


def _list(monkeypatch, *rows):
    monkeypatch.setenv("FAKE_LIST", json.dumps(list(rows)))


def test_validate_isolate_needs_profile_and_profile_needs_isolate(workspace, fake, monkeypatch):
    _list(monkeypatch, {"name": "py", "kind": "code", "valid": True})
    assert _isolate_codes(workspace, profile=None) == {"isolate_profile_required"}
    assert _isolate_codes(workspace, isolate=False) == {"isolate_profile_misplaced"}
    assert _isolate_codes(workspace) == set()


def test_validate_script_must_be_under_src(workspace, fake, monkeypatch):
    _list(monkeypatch, {"name": "py", "kind": "code", "valid": True})
    assert "isolate_script_outside_src" in _isolate_codes(workspace, run=("python", ".process/scripts/x.py"))


def test_validate_input_names_must_be_tool_names(workspace, fake, monkeypatch):
    _list(monkeypatch, {"name": "py", "kind": "code", "valid": True})
    assert "isolate_input_name" in _isolate_codes(workspace, run=("python", "src/job.py", "{input.9bad}"))
    assert "isolate_input_name" not in _isolate_codes(workspace, run=("python", "src/job.py", "{input.My-Src}"))


def test_validate_profile_against_the_tools_list(workspace, fake, monkeypatch):
    _list(monkeypatch, {"name": "other", "kind": "code", "valid": True})
    assert _isolate_codes(workspace) == {"isolate_profile_missing"}
    _list(monkeypatch, {"name": "py", "kind": "install", "valid": True})
    assert _isolate_codes(workspace) == {"isolate_profile_unknown"}
    _list(monkeypatch, {"name": "py", "kind": "code", "valid": False, "error": "bad image"})
    assert _isolate_codes(workspace) == {"isolate_profile_invalid"}


def test_validate_skips_the_profile_check_without_the_tool(workspace, monkeypatch):
    monkeypatch.delenv(container_mod.ENV_TOOL, raising=False)
    monkeypatch.setenv("PATH", "")
    monkeypatch.setattr(container_mod, "DEFAULT_TOOL", "/nonexistent/run_ephemeral_container")
    assert _isolate_codes(workspace) == set()
