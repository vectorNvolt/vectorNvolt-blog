"""WP5 unit tests: builder, router, products, journal, procedure argv, exec backend, budgets, cache metrics."""

import os
import sys
import threading
import time
from types import SimpleNamespace

import pytest

from agent_graph.contract import Status
from agent_graph.dynamic import NodeJournal, build_graph_spec, compile_dynamic
from agent_graph.dynamic.journal import NodeRun, StepFailure
from agent_graph.dynamic.products import (check_output_text, check_products, check_template, print_texts,
                                          resolve_files)
from agent_graph.dynamic.router import RoutingError, next_after, select_edge
from agent_graph.dynamic.step_procedure import expand_argv
from agent_graph.engine.budgets import RunBudgets, apply_overrides
from agent_graph.engine.cache_metrics import evaluate, on_node_done, usage_from_history
from agent_graph.errors import TurnCancelled
from agent_graph.ports import CancelToken
from agent_graph.services.container import ContainerExec, SandboxExec
from agent_graph.state import (Binding, BoundFile, BoundSource, Budgets, EngineCounters, GraphSpec)

from test_wp5_helpers import RUN_ID, decl, edge, pipeline, process, step


# ------------------------------------------------------------------ builder and router

def test_graph_spec_has_one_node_per_step_and_the_process_edges():
    proc = pipeline()
    spec = build_graph_spec(proc)
    assert [n.name for n in spec.nodes] == ["extract", "write-summary", "review"]
    assert spec.order == ("extract", "write-summary", "review")
    assert spec.edges == proc.edges
    assert spec.entry == "extract" and spec.engine_entry == "restart_at|entry"
    assert len({n.step_hash for n in spec.nodes}) == 3
    assert [n.step_hash for n in build_graph_spec(proc).nodes] == [n.step_hash for n in spec.nodes]
    assert GraphSpec.from_json(spec.to_json()).order == spec.order


def test_compile_adds_no_node_the_process_does_not_define():
    graph = compile_dynamic(pipeline(), None)
    assert set(graph.nodes) - {"__start__"} == {"extract", "write-summary", "review"}


def test_select_edge_follows_done_outcome_and_hil_edges():
    proc = process(
        [step(1, "a", "procedure", run=("x",), outcomes=("yes", "no")), step(2, "b", "human"),
         step(3, "c", "cognitive")],
        [edge("a", "b", "outcome", "yes"), edge("a", "a", "outcome", "no"),
         edge("b", "c", "hil_ok"), edge("b", "a", "hil_nok"), edge("c", "END")], "a")
    assert select_edge(proc.edges, proc.step("a"), {"last_outcome": "yes"}) == "b"
    assert select_edge(proc.edges, proc.step("a"), {"last_outcome": "no"}) == "a"
    assert select_edge(proc.edges, proc.step("b"), {"last_hil": "hil_nok"}) == "a"
    assert select_edge(proc.edges, proc.step("b"), {"last_hil": "hil_ok"}) == "c"
    assert select_edge(proc.edges, proc.step("c"), {}) == "END"
    with pytest.raises(RoutingError):
        select_edge(proc.edges, proc.step("a"), {"last_outcome": "maybe"})
    assert next_after(proc, {}) is None
    assert next_after(proc, {"last_node": "c"}) == "END"
    assert next_after(proc, {"last_node": "b", "last_hil": "hil_nok"}) == "a"


# ------------------------------------------------------------------ products

def test_output_checks():
    assert check_output_text("non_empty", "x") is None
    assert check_output_text("non_empty", "  \n") is not None
    assert check_output_text("max_words:2", "a b") is None
    assert "exceed" in check_output_text("max_words:2", "a b c")
    assert check_output_text("min_words:3", "a b") is not None
    assert check_output_text("json_valid", '[1, 2]') is None
    assert check_output_text("json_valid", "[1,") is not None
    assert check_output_text("yaml_valid", "a: 1") is None
    assert check_output_text("yaml_valid", "a: [") is not None
    assert "unknown" in check_output_text("nope", "x")


def test_template_conformance_markdown_and_json():
    tpl = "# Summary\n\n## Findings\n"
    assert check_template("t.md", tpl, "# Summary\n\ntext\n\n## Findings\n\nmore\n") is None
    assert "Findings" in check_template("t.md", tpl, "# Summary\n\ntext\n")
    assert check_template("t.md", tpl, "## Findings\n\n# Summary\n") is not None      # order matters
    assert check_template("t.json", '{"a": 1, "b": 2}', '{"a": 1, "b": 3, "c": 4}') is None
    assert "b" in check_template("t.json", '{"a": 1, "b": 2}', '{"a": 1}')


def test_print_texts_and_placeholder_globs(tmp_path):
    assert print_texts("PRINT('STEP 1: DONE')\nPRINT(\"x\")") == ["STEP 1: DONE", "x"]
    assert print_texts("STOP\nPRINT('<ERROR_DETAILS>')", "boom") == ["boom"]
    assert print_texts(None) == []
    (tmp_path / "a").mkdir()
    (tmp_path / "a" / "x.md").write_text("1")
    (tmp_path / "b").mkdir()
    (tmp_path / "b" / "x.md").write_text("2")
    assert [p.parent.name for p in resolve_files(tmp_path, "<name>/x.md")] == ["a", "b"]
    assert resolve_files(tmp_path, "a/x.md") == [tmp_path / "a" / "x.md"]
    assert resolve_files(tmp_path, "nope.md") == []


def test_check_products_presence_and_schema(tmp_path):
    (tmp_path / "schemas").mkdir()
    (tmp_path / "schemas" / "facts.yml").write_text("type: array\n")
    work = tmp_path / "work"
    work.mkdir()
    proc = process([step(1, "s", "procedure", run=("x",), produces=(decl("facts", "facts.json", schema="schemas/facts.yml"),
                                                                       decl("other", "other.md")))],
                   [edge("s", "END")], "s")
    s = proc.step("s")
    problems = check_products(proc, s, work, tmp_path)
    assert any("facts" in p and "missing" in p for p in problems) and any("other" in p for p in problems)
    (work / "facts.json").write_text("{}")
    (work / "other.md").write_text("x")
    problems = check_products(proc, s, work, tmp_path)
    assert len(problems) == 1 and "facts.json" in problems[0]
    (work / "facts.json").write_text("[1]")
    assert check_products(proc, s, work, tmp_path) == []


# ------------------------------------------------------------------ journal

def test_journal_roundtrip_and_reopen(tmp_path):
    db = tmp_path / "x" / "checkpoint.sqlite"
    j = NodeJournal(db)
    assert j.get("n", 1, "k") is None and j.session("n", 1) is None
    j.put("n", 1, "k", {"a": [1, 2]})
    j.set_session("n", 1, "sess")
    j.close()
    j2 = NodeJournal(db)
    assert j2.get("n", 1, "k") == {"a": [1, 2]}
    assert j2.session("n", 1) == "sess"
    assert j2.get("n", 2, "k") is None            # another visit does not see it


def test_once_runs_a_side_effect_once_and_journals_failures(tmp_path):
    j = NodeJournal(tmp_path / "j.sqlite")
    run = NodeRun(SimpleNamespace(journal=j), "n", 1, thread="t")
    calls = []
    assert run.once("k", lambda: calls.append(1) or {"v": 1}) == {"v": 1}
    assert run.once("k", lambda: calls.append(2) or {"v": 2}) == {"v": 1}
    assert calls == [1]

    def boom():
        calls.append("boom")
        raise StepFailure("schema", "bad", retryable=True)
    for _ in range(2):
        with pytest.raises(StepFailure) as e:
            run.once("f", boom)
        assert e.value.kind == "schema" and e.value.retryable
    assert calls.count("boom") == 1

    def crash():
        raise OSError("disk")
    with pytest.raises(StepFailure) as e:
        run.once("c", crash)
    assert e.value.kind == "tool" and not e.value.retryable and "disk" in e.value.reason


def test_begin_refusal_is_a_non_retryable_failure_with_cancelled_cause(tmp_path):
    j = NodeJournal(tmp_path / "j.sqlite")
    events = []
    gate = SimpleNamespace(before_node_start=lambda n: "budget:max_steps")
    run = NodeRun(SimpleNamespace(journal=j, gate=gate, emit=events.append), "n", 1, thread="t")
    with pytest.raises(StepFailure) as e:
        run.begin("ref")
    assert e.value.cause == "cancelled:budget:max_steps" and not e.value.retryable
    assert events == []                                  # no NODE_STARTED when refused


def test_started_status_carries_context_ref_and_visit(tmp_path):
    j = NodeJournal(tmp_path / "j.sqlite")
    events = []
    gate = SimpleNamespace(before_node_start=lambda n: None)
    run = NodeRun(SimpleNamespace(journal=j, gate=gate, emit=events.append), "n", 3, thread="t")
    run.begin("nodes/n/context.json")
    assert events == [Status("NODE_STARTED", "n", {"context_ref": "nodes/n/context.json", "visit": 3})]


# ------------------------------------------------------------------ procedure argv

def _ctx(workspace, files=2):
    paths = workspace.run(RUN_ID)
    src = BoundSource("Source", tuple(BoundFile(f".input/example/s{i}.md", "0" * 64) for i in range(files)), None)
    binding = Binding("p", "f", "0" * 64, {"Source": src}, {"Template": BoundFile("templates/report.md", "0")}, {})
    return SimpleNamespace(paths=paths, binding=binding, process=pipeline())


def test_expand_argv_substitutes_without_a_shell(workspace):
    ctx = _ctx(workspace)
    work = ctx.paths.work_node("s")
    s = step(1, "s", "procedure", run=("tool", "{input.Source}", "out={work}/y", "{'a': 1}", "{input.Template}",
                                        "{facts}", "{unknown}", "a; echo hi"))
    state = {"products": {"facts": {"path": ".agent/r/work/x/facts.json"}}}
    argv = expand_argv(s, state, ctx, work)
    root = workspace.root
    assert argv == ["tool", str(root / ".input/example/s0.md"), str(root / ".input/example/s1.md"),
                    f"out={work}/y", "{'a': 1}", str(root / "templates/report.md"),
                    str(root / ".agent/r/work/x/facts.json"), "{unknown}", "a; echo hi"]


def test_expand_argv_embedded_placeholder_needs_one_file(workspace):
    ctx = _ctx(workspace, files=2)
    s = step(1, "s", "procedure", run=("tool", "pre{input.Source}"))
    with pytest.raises(StepFailure) as e:
        expand_argv(s, {}, ctx, ctx.paths.work_node("s"))
    assert not e.value.retryable
    ctx1 = _ctx(workspace, files=1)
    assert expand_argv(s, {}, ctx1, ctx1.paths.work_node("s"))[1].startswith("pre")
    bad = step(1, "s", "procedure", run=("tool", "{input.Nope}"))
    with pytest.raises(StepFailure):
        expand_argv(bad, {}, ctx1, ctx1.paths.work_node("s"))


def test_own_product_placeholder_points_into_the_work_folder(workspace):
    ctx = _ctx(workspace)
    work = ctx.paths.work_node("s")
    s = step(1, "s", "procedure", run=("tool", "{facts}"), produces=(decl("facts", "facts.json"),))
    assert expand_argv(s, {}, ctx, work) == ["tool", str(work / "facts.json")]


# ------------------------------------------------------------------ exec backend

def test_sandbox_exec_runs_an_argument_list_not_a_shell(tmp_path):
    res = SandboxExec().run([sys.executable, "-c", "import sys;print(sys.argv[1]);sys.exit(3)", "a; echo injected"],
                            tmp_path, dict(os.environ), 10, CancelToken())
    assert res.exit_code == 3
    assert res.stdout_tail.strip() == "a; echo injected"


def test_sandbox_exec_cancel_and_timeout(tmp_path):
    sleeper = [sys.executable, "-c", "import time;time.sleep(30)"]
    tok = CancelToken()
    threading.Timer(0.2, lambda: tok.cancel("budget:max_steps")).start()
    t0 = time.monotonic()
    with pytest.raises(TurnCancelled) as e:
        SandboxExec().run(sleeper, tmp_path, dict(os.environ), None, tok)
    assert e.value.cause == "budget:max_steps" and time.monotonic() - t0 < 10
    with pytest.raises(TurnCancelled) as e:
        SandboxExec().run(sleeper, tmp_path, dict(os.environ), 0.2, CancelToken())
    assert e.value.cause == "timeout"


def test_container_exec_plain_run_needs_a_profile(tmp_path):
    with pytest.raises(NotImplementedError, match="run_isolated"):
        ContainerExec().run(["x"], tmp_path, {}, None, CancelToken())


# ------------------------------------------------------------------ budgets

def test_gate_counts_steps_and_llm_calls_and_refuses_at_the_limit():
    c = EngineCounters()
    g = RunBudgets(c, Budgets(2, 1, 100))
    assert g.before_node_start("a") is None and g.before_node_start("b") is None
    assert g.before_node_start("c") == "budget:max_steps"
    assert g.breach == {"budget": "max_steps", "limit": 2, "count": 2}
    assert g.before_llm_call("a") is None
    assert g.before_llm_call("a") == "budget:max_llm_calls"
    assert (c.steps, c.llm_calls) == (2, 1)


def test_run_clock_expiry_cancels_the_running_node_and_refuses_new_work():
    c = EngineCounters()
    g = RunBudgets(c, Budgets(10, 10, 0.2))
    cancelled = []
    g.bind(lambda node, cause: cancelled.append((node, cause)))
    g.attempt_token("n")
    g.resume_clock()
    time.sleep(0.6)
    assert g.expired and cancelled == [("n", "budget:timeout")]
    assert g.attempt_token("n").cancelled
    assert g.before_node_start("m") == "budget:timeout"
    assert g.breach["budget"] == "timeout"
    g.close()


def test_paused_clock_does_not_expire():
    g = RunBudgets(EngineCounters(), Budgets(10, 10, 0.3))
    g.resume_clock()
    g.pause_clock()
    time.sleep(0.6)
    assert not g.expired
    assert g.elapsed() < 0.3


def test_attempt_timer_cancels_with_cause_timeout_and_can_be_disarmed():
    g = RunBudgets(EngineCounters(), Budgets(10, 10, 100))
    cancelled = []
    g.bind(lambda node, cause: cancelled.append((node, cause)))
    g.arm_attempt("n", 0.1)
    time.sleep(0.4)
    assert cancelled == [("n", "timeout")]
    g.arm_attempt("m", 0.2)
    g.disarm_attempt()
    time.sleep(0.4)
    assert cancelled == [("n", "timeout")]


def test_overrides_raise_retry_timeout_budgets_and_step_budget():
    proc = pipeline()
    new = apply_overrides(proc, {"retry": {"extract": 5}, "timeout": 99,
                                 "budgets": {"max_steps": 7, "timeout": 5}, "step_budget": {"write-summary": 123}})
    assert new.retry_of(new.step("extract")) == 5
    assert new.retry_of(new.step("review")) == proc.runtime.retry
    assert new.timeout_of(new.step("extract")) == 99
    assert new.timeout_of(new.step("review")) is None            # human steps keep only their own timeout
    assert new.runtime.budgets.max_steps == 7 and new.runtime.budgets.timeout_s == 5
    assert new.runtime.budgets.max_llm_calls == proc.runtime.budgets.max_llm_calls
    assert new.budget_of(new.step("write-summary")) == 123
    assert apply_overrides(proc, {}) is proc


# ------------------------------------------------------------------ cache metrics

def _usage(first_read, turns=None, e1h=1, e5m=0):
    u = {"cache_read_input_tokens": first_read, "ephemeral_1h_input_tokens": e1h,
         "ephemeral_5m_input_tokens": e5m}
    return {"first_turn": u, "turns": turns or [[u]]}


def test_p_run_is_the_second_cognitive_nodes_first_turn_and_drift_is_a_notice():
    c = EngineCounters()
    assert on_node_done(c, "a", _usage(900)) is None and c.p_run is None
    assert on_node_done(c, "b", _usage(1000)) is None and c.p_run == 1000
    assert on_node_done(c, "c", _usage(950)) is None                     # >= 90 %
    msg = on_node_done(c, "d", _usage(899))
    assert msg and "d" in msg and c.drift == [msg]
    assert on_node_done(c, "procedure", None) is None and c.cognitive_started == 4


def test_evaluate_acceptance_thresholds_and_history_extraction():
    ok = [{"node": "a", "usage": _usage(500)}, {"node": "b", "usage": _usage(1000)},
          {"node": "c", "usage": _usage(990, turns=[[{"cache_read_input_tokens": 990, "ephemeral_1h_input_tokens": 1}],
                                                    [{"cache_read_input_tokens": 950}]])}]
    r = evaluate(ok)
    assert r["applicable"] and r["first_turn_ok"] and r["within_node_ok"] and r["ephemeral_ok"] and r["p_run"] == 1000
    bad = ok + [{"node": "d", "usage": _usage(100, e5m=3)}]
    r = evaluate(bad)
    assert not r["first_turn_ok"] and not r["ephemeral_ok"] and r["violations"]
    recs = [{"dir": "status", "name": "NODE_DONE", "node": "a", "payload": {"usage": _usage(5)}},
            {"dir": "status", "name": "NODE_DONE", "node": "p", "payload": {"outcome": None}},
            {"dir": "cmd", "name": "START", "node": None, "payload": {}}]
    assert [u["node"] for u in usage_from_history(recs)] == ["a"]
