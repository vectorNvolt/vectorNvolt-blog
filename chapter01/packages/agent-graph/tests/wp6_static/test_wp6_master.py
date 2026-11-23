"""The static master S1-S8 end to end with a fake engine and builder (engine-independent), plus
S6 mapping, routing, and gated tests against the real engine (SFR-AGG-012, 027, 034, 035, 037,
SSR-AGG-003, 014, 015, 016)."""

import io
import json
import sys
from dataclasses import dataclass
from types import ModuleType, SimpleNamespace

import pytest

from agent_graph import graph as master
from agent_graph.contract import start
from agent_graph.errors import ExitState, Interrupted, RunError
from agent_graph.main import main
from agent_graph.nodes import n06_execution
from agent_graph.process import bfs_order
from agent_graph.services.claude_cli import ClaudeCli
from agent_graph.state import GraphNode, GraphSpec
from fakes.workspace import PROCESS_YML, add_process, build_workspace, process_with_id


@dataclass
class Outcome:
    done: bool
    output_ref: "str | None"
    escalation: "RunError | None"


def spec_builder(proc):
    nodes = tuple(GraphNode(name=s.name, type=s.type, step_hash=f"h-{s.name}",
                            reads=tuple(r.raw for r in s.reads), produces=tuple(p.name for p in s.produces))
                  for s in proc.steps)
    return GraphSpec(process_id=proc.id, process_sha256=proc.source_sha256, entry=proc.entry, nodes=nodes,
                     edges=proc.edges, order=bfs_order(proc), process=proc.to_json())


@pytest.fixture
def fake_engine(monkeypatch):
    """Replaces agent_graph.dynamic.builder and agent_graph.engine. ``fx.script`` is a list of
    callables ctx -> Outcome (default: finish); ``fx.ctxs`` records the contexts."""
    fx = SimpleNamespace(script=[], ctxs=[])

    def run_engine(ctx):
        fx.ctxs.append(ctx)
        ctx.history.command(start())                      # the engine has run: history.jsonl exists
        step = fx.script.pop(0) if fx.script else None
        if step is not None:
            return step(ctx)
        out = ctx.paths.output_dir
        out.mkdir(parents=True, exist_ok=True)
        (out / "summary.md").write_text("# Summary\n\n## Findings\n\nx\n", encoding="utf-8")
        return Outcome(True, str(out), None)

    eng = ModuleType("agent_graph.engine")
    eng.EngineContext = lambda **kw: SimpleNamespace(**kw)
    eng.run_engine = run_engine
    dyn = ModuleType("agent_graph.dynamic")
    dyn.__path__ = []
    bld = ModuleType("agent_graph.dynamic.builder")
    bld.build_graph_spec = spec_builder
    bld.compile_dynamic = lambda proc, cp: object()
    for name, mod in (("agent_graph.engine", eng), ("agent_graph.dynamic", dyn),
                      ("agent_graph.dynamic.builder", bld)):
        monkeypatch.setitem(sys.modules, name, mod)
    return fx


def run_main(ws, claude_dir, fc, gw, argv=("run",)):
    return main(list(argv), gateway=gw, claude=ClaudeCli(cmd=fc.cmd, env=fc.env), workspace=ws.root,
                claude_dir=claude_dir, stderr=io.StringIO())


def runs(ws):
    return sorted(p for p in ws.agent_dir.iterdir() if p.is_dir() and not p.name.startswith(".")
                  and p.name != "cache")


def agree(dialog_turn, pid="example"):
    return dialog_turn("agree", f"{pid} fits", process_id=pid)


# ---------------------------------------------------------------- full runs on the fake engine
def test_full_run_completes_with_run_folder_and_report(workspace, claude_dir, fake_claude, scripted_gateway,
                                                      dialog_turn, fake_engine):
    gw = scripted_gateway(replies=["summarize my notes"], confirms=[True, True])      # S2 agree, S5 start
    assert run_main(workspace, claude_dir, fake_claude([agree(dialog_turn)]), gw) == 0
    (run,) = runs(workspace)
    for name in ("run.json", "binding.json", "index.json", "graph.json", "gateway.jsonl", "history.jsonl",
                 "checkpoint.sqlite"):
        assert (run / name).is_file(), name
    assert (run / "dialogs" / "S2-1" / "transcript.jsonl").is_file()
    rec = json.loads((run / "run.json").read_text(encoding="utf-8"))
    assert rec["status"] == "ENDED" and rec["exit_code"] == 0 and rec["exit_state"] == "COMPLETED"
    out = workspace.output_root / run.name
    assert (out / "report.md").is_file() and (out / "summary.md").is_file()
    assert "COMPLETED" in (out / "report.md").read_text(encoding="utf-8")
    ctx = fake_engine.ctxs[0]
    assert ctx.mode == "start" and ctx.restart_node is None and ctx.process.id == "example"
    assert not workspace.active_json.exists()
    gateway_log = (run / "gateway.jsonl").read_text(encoding="utf-8")
    assert "S2 clarify" in gateway_log and "S5 build_graph" in gateway_log


def test_two_processes_run_on_the_same_master(ws_root, claude_dir, fake_claude, scripted_gateway, dialog_turn,
                                              fake_engine):
    ws = build_workspace(ws_root)
    add_process(ws, "other.yml", process_with_id("other"))
    for pid in ("example", "other"):
        gw = scripted_gateway(replies=["go"], confirms=[True, True])
        assert run_main(ws, claude_dir, fake_claude([agree(dialog_turn, pid)]), gw) == 0
    ids = [json.loads((r / "graph.json").read_text(encoding="utf-8"))["process_id"] for r in runs(ws)]
    assert sorted(ids) == ["example", "other"]


def test_escalation_is_resolved_and_the_node_restarts(workspace, claude_dir, fake_claude, scripted_gateway,
                                                      dialog_turn, fake_engine):
    err = RunError(ExitState.NODE_FAILED, "S6", "failed twice", node="write-summary")
    fake_engine.script = [lambda ctx: Outcome(False, None, err)]
    turns = [agree(dialog_turn),
             dialog_turn("propose", "More retries.", proposals=[
                 {"kind": "override", "key": "retry", "node": "write-summary", "value": 4, "reason": "retry more"}]),
             dialog_turn("done", "ok")]
    gw = scripted_gateway(replies=["go"], confirms=[True, True, True, True])  # S2, S5, S7 apply + re-enter
    assert run_main(workspace, claude_dir, fake_claude(turns), gw) == 0
    first, second = fake_engine.ctxs
    assert first.mode == "start"
    assert second.mode == "restart" and second.restart_node == "write-summary"
    assert "override retry" in second.restart_summary
    (run,) = runs(workspace)
    assert json.loads((run / "overrides.json").read_text(encoding="utf-8")) == {"retry": {"write-summary": 4}}
    assert (run / "resolution" / "1" / "changes.jsonl").is_file()


def test_invalid_process_is_fixed_in_s7_then_runs(workspace, claude_dir, fake_claude, scripted_gateway,
                                                  dialog_turn, fake_engine):
    (workspace.process_dir / "example.yml").write_text(
        PROCESS_YML.replace("skills: [summarize]", "skills: [ghost]"), encoding="utf-8")
    turns = [agree(dialog_turn),
             dialog_turn("propose", "Fix.", proposals=[{"kind": "edit_file", "path": ".process/example.yml",
                                                        "content": PROCESS_YML, "reason": "fix skill"}]),
             dialog_turn("done", "ok")]
    gw = scripted_gateway(replies=["go"], confirms=[True, True, True, True])  # S2, S7 apply + re-enter, S5
    assert run_main(workspace, claude_dir, fake_claude(turns), gw) == 0
    assert fake_engine.ctxs[0].mode == "start"              # the engine never ran before: a first start


def test_interrupt_in_s6_suspends_and_resume_continues(workspace, claude_dir, fake_claude, scripted_gateway,
                                                       dialog_turn, fake_engine):
    def interrupted(ctx):
        raise Interrupted("SIGINT")
    fake_engine.script = [interrupted]
    fc = fake_claude([agree(dialog_turn)])
    assert run_main(workspace, claude_dir, fc, scripted_gateway(replies=["go"], confirms=[True, True])) == 130
    (run,) = runs(workspace)
    rec = json.loads((run / "run.json").read_text(encoding="utf-8"))
    assert rec["status"] == "SUSPENDED" and rec["phase"] == "S6"
    assert not workspace.active_json.exists()
    # a new main() call: S1 loads the run and goes straight to S6 (steps 2-5 skipped)
    assert run_main(workspace, claude_dir, fc, scripted_gateway(), ("run", "--resume", run.name)) == 0
    ctx = fake_engine.ctxs[1]
    assert ctx.mode == "resume" and ctx.process.id == "example" and "summarize" in ctx.index.skills
    assert ctx.binding.process_id == "example"
    assert fc.turns_used() == 1                              # no second dialog
    assert json.loads((run / "run.json").read_text(encoding="utf-8"))["status"] == "ENDED"


def test_quit_in_s7_keeps_the_run_and_resume_reenters_s7(workspace, claude_dir, fake_claude, scripted_gateway,
                                                         dialog_turn, fake_engine):
    (workspace.process_dir / "example.yml").write_text(
        PROCESS_YML.replace("skills: [summarize]", "skills: [ghost]"), encoding="utf-8")
    fc = fake_claude([agree(dialog_turn), dialog_turn("ask", "What now?"), dialog_turn("ask", "Again?")])
    # first call: the terminal closes inside S7 -> SUSPENDED
    assert run_main(workspace, claude_dir, fc, scripted_gateway(replies=["go"], confirms=[True])) == 130
    (run,) = runs(workspace)
    rec = json.loads((run / "run.json").read_text(encoding="utf-8"))
    assert rec["status"] == "SUSPENDED" and rec["phase"] == "S7"
    # resume: S7 again; the developer quits -> exit state of the error, checkpoint kept
    gw = scripted_gateway(replies=["/quit"])
    assert run_main(workspace, claude_dir, fc, gw, ("run", "--resume", run.name)) == 3
    assert "Exit state: PROCESS_INVALID" in fc.calls()[-1]["prompt"]
    rec = json.loads((run / "run.json").read_text(encoding="utf-8"))
    assert rec["status"] == "ESCALATED" and rec["exit_code"] == 3
    report = (workspace.output_root / run.name / "report.md").read_text(encoding="utf-8")
    assert "Open error" in report
    # the run is still resumable
    assert run_main(workspace, claude_dir, fake_claude([dialog_turn("ask", "x")]),
                    scripted_gateway(replies=["/quit"]), ("run", "--resume", run.name)) == 3
    assert fake_engine.ctxs == []


def test_developer_quit_in_the_engine_leaves_the_run_suspended(workspace, claude_dir, fake_claude,
                                                              scripted_gateway, dialog_turn, fake_engine):
    err = RunError(ExitState.SUSPENDED, "S6", "the developer quit", node="review")
    fake_engine.script = [lambda ctx: Outcome(False, None, err)]
    assert run_main(workspace, claude_dir, fake_claude([agree(dialog_turn)]),
                    scripted_gateway(replies=["go"], confirms=[True, True])) == 130
    (run,) = runs(workspace)
    assert json.loads((run / "run.json").read_text(encoding="utf-8"))["status"] == "SUSPENDED"


def test_engine_defect_goes_to_resolution_not_a_crash(workspace, claude_dir, fake_claude, scripted_gateway,
                                                      dialog_turn, fake_engine):
    def crash(ctx):
        raise KeyError("boom")
    fake_engine.script = [crash]
    gw = scripted_gateway(replies=["go", "/quit"], confirms=[True, True])
    turns = [agree(dialog_turn), dialog_turn("ask", "An engine error occurred.")]
    assert run_main(workspace, claude_dir, fake_claude(turns), gw) == 2        # NODE_FAILED


# ---------------------------------------------------------------- S6 mapping
def n06_env(make_env):
    h = make_env()
    h.env.create_run("example", ".process/example.yml")
    h.env.proc = h.env.index = h.env.binding = h.env.spec = SimpleNamespace()
    return h


def test_s6_modes(make_env, fake_engine):
    from agent_graph.state import ReEntry
    h = n06_env(make_env)
    assert n06_execution.run(h.env, {})["exit_state"] == "COMPLETED"
    assert fake_engine.ctxs[-1].mode == "start"
    h.env.start_mode = "resume"
    n06_execution.run(h.env, {})
    assert fake_engine.ctxs[-1].mode == "resume"
    h.env.record.reentry = ReEntry(state="S6", restart_node="review", summary="s")
    n06_execution.run(h.env, {})
    ctx = fake_engine.ctxs[-1]
    assert (ctx.mode, ctx.restart_node, ctx.restart_summary) == ("restart", "review", "s")
    assert h.env.record.reentry is None


def test_s6_escalation_goes_to_s7_and_interrupt_propagates(make_env, fake_engine):
    h = n06_env(make_env)
    err = RunError(ExitState.BUDGET_EXCEEDED, "S6", "max_steps", node="x")
    fake_engine.script = [lambda ctx: Outcome(False, None, err)]
    out = n06_execution.run(h.env, {})
    assert out["phase"] == "S7" and out["error"]["exit_state"] == "BUDGET_EXCEEDED"

    def interrupted(ctx):
        raise Interrupted("SIGTERM")
    fake_engine.script = [interrupted]
    with pytest.raises(Interrupted):
        n06_execution.run(h.env, {})


# ---------------------------------------------------------------- routing (SSR-AGG-003)
def test_master_graph_is_the_same_for_every_process(make_env, ws_root):
    h1 = make_env()
    g1 = master.build_master(h1.env).get_graph()
    ws2 = build_workspace(ws_root.parent / "other_ws", process_text=process_with_id("zzz"))
    h2 = make_env(ws=ws2)
    g2 = master.build_master(h2.env).get_graph()
    shape = lambda g: (sorted(g.nodes), sorted((e.source, e.target) for e in g.edges))   # noqa: E731
    assert shape(g1) == shape(g2)
    assert {"S1", "S2", "S3", "S4", "S5", "S6", "S7", "S8"} <= set(g1.nodes)


def test_transitions_outside_the_table_are_refused():
    with pytest.raises(RuntimeError):
        master._route("S3")({"phase": "S6"})
    assert master._route("S7")({"phase": "S3"}) == "S3"


# ---------------------------------------------------------------- real engine (gated)
def test_real_engine_end_to_end(workspace, claude_dir, fake_claude, scripted_gateway, dialog_turn):
    pytest.importorskip("agent_graph.engine")
    result = {"status": "done", "outcome": None, "question": None,
              "handoff": {"done": "wrote it", "decisions": [], "open_points": []}, "warnings": []}
    summary = "# Summary\n\n## Findings\n\nThree lines.\n"
    turns = [agree(dialog_turn),
             {"text": "written", "write_files": {"result.json": result,
                                                  "../work/write-summary/summary.md": summary}}]
    answers = ["summarize my notes"]
    gw = scripted_gateway(replies=answers + ["ok"] * 5,          # the review is answered affirmatively
                          confirms=[True, True])                 # S2 agree, S5 start
    code = run_main(workspace, claude_dir, fake_claude(turns), gw)
    assert code == 0, gw.said()
    (run,) = runs(workspace)
    assert (run / "nodes" / "write-summary" / "system.md").is_file()
    assert (workspace.output_root / run.name / "report.md").is_file()
