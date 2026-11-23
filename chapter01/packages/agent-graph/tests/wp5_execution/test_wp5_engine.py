"""WP5 tests of the S6 engine (E1..E7): real dynamic and engine graphs, InMemorySaver, sqlite journal,
scripted Claude and gateway, fake compose. No network, no WP2-WP4 modules."""

import dataclasses
import io
import json
import sys
import tarfile
import time

import pytest
from langgraph.checkpoint.memory import InMemorySaver

from agent_graph.engine import run_engine
from agent_graph.errors import ClaudeTurnError, ExitState, LlmUnavailable, TerminalClosed
from agent_graph.state import Budgets, RunRecord

from test_wp5_helpers import (RUN_ID, FakeCompose, ScriptedClaude, cognitive_turn, decl, edge, engine_ctx,
                              pipeline, process, read, result_json, step)

QUESTION_TURN = {"write": {"result.json": result_json(status="question", question="Which tone?")}}
FAIL = (sys.executable, "-c", "import sys;sys.exit(1)")


def cmd_names(ctx):
    return [r["name"] for r in ctx.history.records() if r["dir"] == "cmd"]


# ------------------------------------------------------------------ the happy path

def test_full_run_completes_checks_output_cleans_up_and_records(workspace, guard, scripted_gateway):
    claude = ScriptedClaude([cognitive_turn()])
    gw = scripted_gateway(replies=["ok"])
    ctx = engine_ctx(workspace, guard, pipeline(), claude, gw)
    out = run_engine(ctx)
    assert out.done and out.escalation is None and out.output_ref == f".output/{RUN_ID}"
    paths = ctx.paths
    assert (paths.output_dir / "summary.md").is_file()
    assert not paths.output_invalid_json.exists()
    assert paths.archive_tgz.is_file() and not paths.work_dir.exists()          # archive is the default
    with tarfile.open(paths.archive_tgz) as tar:
        assert "work/write-summary/summary.md" in tar.getnames()
    assert (paths.node_dir("write-summary") / "handoff.json").is_file()         # node records are kept
    assert (ctx.record.counters.steps, ctx.record.counters.llm_calls) == (3, 1)
    persisted = json.loads(paths.run_json.read_text())
    assert persisted["counters"]["steps"] == 3 and persisted["phase"] == "S6"
    assert [f.kind for f in ctx.facts.all()] == ["hil_ok"]
    assert ("node write-summary", "PROGRESS", "STEP 2: DONE") in gw.notices     # notices only via the gateway
    assert gw.asked and gw.asked[0][0] == "node review"
    assert ("node review", "enter") in gw.conversations
    assert cmd_names(ctx) == ["START", "HIL_OK"]


def test_cognitive_turn_streams_through_the_gateway_turn_renderer(workspace, guard, scripted_gateway):
    event = {"type": "stream_event", "event": {"type": "message_start"}}

    def turn(req, cancel):
        assert req.events is not None                      # the sink of gateway.turn rides on the request
        req.events.feed(event)
        return cognitive_turn()

    gw = scripted_gateway(replies=["ok"])
    ctx = engine_ctx(workspace, guard, pipeline(), ScriptedClaude([turn]), gw)
    assert run_engine(ctx).done
    assert len(gw.turns) == 1                              # one cognitive turn, one streamed turn
    source, show_text, sink = gw.turns[0]
    assert (source, show_text) == ("node write-summary", True)
    assert sink.events == [event]


def test_rejection_goes_back_with_feedback_and_both_decisions_become_facts(workspace, guard, scripted_gateway):
    claude = ScriptedClaude([cognitive_turn(), cognitive_turn()])
    fc = FakeCompose()
    gw = scripted_gateway(replies=["needs more detail", "ok"], confirms=[True])   # send the feedback back
    ctx = engine_ctx(workspace, guard, pipeline(), claude, gw, compose=fc)
    out = run_engine(ctx)
    assert out.done
    assert [(f.kind, f.node, f.text) for f in ctx.facts.all()] == [
        ("hil_nok", "review", "needs more detail"), ("hil_ok", "review", "approved")]
    assert "review_feedback" in fc.calls[1].products
    assert [f.kind for f in fc.calls[1].facts] == ["hil_nok"]                   # L3 input in order of occurrence
    assert (ctx.record.counters.steps, ctx.record.counters.llm_calls) == (5, 2)
    assert cmd_names(ctx) == ["START", "HIL_NOK", "HIL_OK"]


def test_rejection_needs_a_confirmation_and_show_displays_an_item(workspace, guard, scripted_gateway):
    gw = scripted_gateway(replies=["/show 1", "/show 9", "meh", "ok"], confirms=[False])
    ctx = engine_ctx(workspace, guard, pipeline(), ScriptedClaude([cognitive_turn()]), gw)
    assert run_engine(ctx).done
    said = gw.said("node review")
    assert any("# Summary" in t and "Three findings." in t for t in said)       # /show 1
    assert any("/show needs a number" in t for t in said)                       # /show 9
    assert [f.kind for f in ctx.facts.all()] == ["hil_ok"]                      # rejection not confirmed


def test_question_is_relayed_through_the_gateway_and_the_answer_becomes_a_fact(workspace, guard, scripted_gateway):
    claude = ScriptedClaude([QUESTION_TURN, cognitive_turn()])
    gw = scripted_gateway(replies=["formal"])
    ctx = engine_ctx(workspace, guard, pipeline(with_human=False), claude, gw)
    assert run_engine(ctx).done
    assert "Which tone?" in gw.said("node write-summary", include_llm=True)
    assert ("llm", "node write-summary", "Which tone?") in gw.events
    assert [(f.kind, f.node, f.text) for f in ctx.facts.all()] == [("answer", "write-summary", "Which tone?\n-> formal")]
    assert claude.calls[1]["resume"] == "sess-1" and "formal" in claude.calls[1]["prompt"]
    assert cmd_names(ctx) == ["START", "ANSWER"]


# ------------------------------------------------------------------ retry and escalation

def test_retry_exhaustion_escalates_with_every_attempt_and_suspends(workspace, guard, scripted_gateway):
    ctx = engine_ctx(workspace, guard, pipeline(extract_run=FAIL, retry=2), ScriptedClaude([]), scripted_gateway())
    out = run_engine(ctx)
    err = out.escalation
    assert not out.done and err.exit_state == ExitState.NODE_FAILED and err.node == "extract"
    assert err.exit_state.exit_code == 2
    assert err.detail["kind"] == "tool" and len(err.detail["attempts"]) == 2 and err.detail["retry_threshold"] == 2
    assert cmd_names(ctx) == ["START", "RETRY", "SUSPEND"]
    assert ctx.record.status == "ESCALATED" and ctx.record.open_error.exit_state == ExitState.NODE_FAILED
    assert json.loads(ctx.paths.run_json.read_text())["status"] == "ESCALATED"
    assert ctx.record.counters.failures == {"extract": 2}
    names = [r["name"] for r in ctx.history.records() if r["dir"] == "status"]
    assert names[-1] == "SUSPENDED"


def test_step_retry_overrides_runtime_retry_and_zero_escalates_at_once(workspace, guard, scripted_gateway):
    proc = pipeline(extract_run=FAIL, retry=5)
    proc = dataclasses.replace(proc, steps=tuple(
        dataclasses.replace(s, retry=0) if s.name == "extract" else s for s in proc.steps))
    ctx = engine_ctx(workspace, guard, proc, ScriptedClaude([]), scripted_gateway())
    err = run_engine(ctx).escalation
    assert err.exit_state == ExitState.NODE_FAILED and len(err.detail["attempts"]) == 1
    assert "RETRY" not in cmd_names(ctx)


def test_overrides_json_raises_the_retry_threshold(workspace, guard, scripted_gateway):
    ctx = engine_ctx(workspace, guard, pipeline(extract_run=FAIL, retry=2), ScriptedClaude([]), scripted_gateway())
    ctx.paths.overrides_json.write_text(json.dumps({"retry": {"extract": 4}}))
    err = run_engine(ctx).escalation
    assert len(err.detail["attempts"]) == 4 and err.detail["retry_threshold"] == 4


def test_attempt_timeout_is_cancelled_counted_and_escalates_after_the_retries(workspace, guard, scripted_gateway):
    proc = pipeline(extract_run=(sys.executable, "-c", "import time;time.sleep(30)"))
    proc = dataclasses.replace(proc, runtime=dataclasses.replace(proc.runtime, timeout_s=0.3, retry=2))
    ctx = engine_ctx(workspace, guard, proc, ScriptedClaude([]), scripted_gateway())
    t0 = time.monotonic()
    err = run_engine(ctx).escalation
    assert time.monotonic() - t0 < 20
    assert err.exit_state == ExitState.NODE_FAILED and err.detail["kind"] == "timeout"
    assert ctx.record.counters.failures == {"extract": 2}
    assert all(a.startswith("timeout") for a in err.detail["attempts"])


def test_context_over_budget_escalates_at_once_without_a_session(workspace, guard, scripted_gateway):
    claude = ScriptedClaude([])
    ctx = engine_ctx(workspace, guard, pipeline(), claude, scripted_gateway(),
                     compose=FakeCompose(over_budget={"write-summary"}))
    err = run_engine(ctx).escalation
    assert err.exit_state == ExitState.CONTEXT_OVER_BUDGET and err.node == "write-summary"
    assert (err.detail["budget"], err.detail["mandatory_tokens"]) == (100, 500)
    assert claude.calls == [] and "RETRY" not in cmd_names(ctx)


def test_llm_unavailable_escalates_without_retry(workspace, guard, scripted_gateway):
    claude = ScriptedClaude([{"raise": LlmUnavailable("not logged in")}])
    ctx = engine_ctx(workspace, guard, pipeline(), claude, scripted_gateway())
    err = run_engine(ctx).escalation
    assert err.exit_state == ExitState.LLM_UNAVAILABLE and "not logged in" in err.message
    assert len(claude.calls) == 1 and "RETRY" not in cmd_names(ctx)
    assert ctx.record.status == "ESCALATED"


def test_a_failed_turn_is_retried_in_the_same_session(workspace, guard, scripted_gateway):
    claude = ScriptedClaude([{"raise": ClaudeTurnError("tool crashed")}, cognitive_turn()])
    ctx = engine_ctx(workspace, guard, pipeline(), claude, scripted_gateway(replies=["ok"]))
    assert run_engine(ctx).done
    assert claude.calls[1]["resume"] is None                                    # the failed turn stored no session
    assert "previous attempt failed" in claude.calls[1]["prompt"]
    assert cmd_names(ctx)[:2] == ["START", "RETRY"]
    assert ctx.record.counters.failures == {}                                  # reset when the node is done


# ------------------------------------------------------------------ budgets

def looping_process(budgets):
    code = "import pathlib,sys;(pathlib.Path(sys.argv[1])/'.outcome').write_text('again')"
    return process(
        [step(1, "a", "procedure", run=(sys.executable, "-c", code, "{work}"), outcomes=("again", "stop"))],
        [edge("a", "a", "outcome", "again"), edge("a", "END", "outcome", "stop")], "a", budgets=budgets)


def test_loops_are_bounded_by_max_steps(workspace, guard, scripted_gateway):
    ctx = engine_ctx(workspace, guard, looping_process(Budgets(3, 10, 3600)), ScriptedClaude([]), scripted_gateway())
    err = run_engine(ctx).escalation
    assert err.exit_state == ExitState.BUDGET_EXCEEDED
    assert (err.detail["budget"], err.detail["limit"], err.detail["count"]) == ("max_steps", 3, 3)
    assert ctx.record.counters.steps == 3


def test_overrides_json_can_raise_max_steps(workspace, guard, scripted_gateway):
    ctx = engine_ctx(workspace, guard, looping_process(Budgets(3, 10, 3600)), ScriptedClaude([]), scripted_gateway())
    ctx.paths.overrides_json.write_text(json.dumps({"budgets": {"max_steps": 5}}))
    err = run_engine(ctx).escalation
    assert err.detail["limit"] == 5 and ctx.record.counters.steps == 5


def test_llm_call_budget(workspace, guard, scripted_gateway):
    claude = ScriptedClaude([])
    ctx = engine_ctx(workspace, guard, pipeline(budgets=Budgets(50, 0, 3600)), claude, scripted_gateway())
    err = run_engine(ctx).escalation
    assert err.exit_state == ExitState.BUDGET_EXCEEDED and err.detail["budget"] == "max_llm_calls"
    assert claude.calls == []


def test_run_time_budget_cancels_the_running_attempt(workspace, guard, scripted_gateway):
    proc = pipeline(extract_run=(sys.executable, "-c", "import time;time.sleep(30)"), budgets=Budgets(50, 20, 0.4))
    ctx = engine_ctx(workspace, guard, proc, ScriptedClaude([]), scripted_gateway())
    t0 = time.monotonic()
    err = run_engine(ctx).escalation
    assert time.monotonic() - t0 < 20
    assert err.exit_state == ExitState.BUDGET_EXCEEDED and err.detail["budget"] == "timeout"
    assert err.detail["limit"] == 0.4


# ------------------------------------------------------------------ output check and cleanup

def test_failed_output_check_keeps_the_output_marks_it_invalid_and_escalates(workspace, guard, scripted_gateway):
    ctx = engine_ctx(workspace, guard, pipeline(checks=("non_empty", "max_words:3")), ScriptedClaude([cognitive_turn()]),
                     scripted_gateway(replies=["ok"]))
    out = run_engine(ctx)
    err = out.escalation
    assert err.exit_state == ExitState.OUTPUT_INVALID and [f.code for f in err.findings] == ["output_check"]
    assert (ctx.paths.output_dir / "summary.md").is_file()                      # kept for inspection
    invalid = json.loads(ctx.paths.output_invalid_json.read_text())
    assert invalid["findings"][0]["item"] == "Report"
    assert ctx.paths.work_dir.exists() and not ctx.paths.archive_tgz.exists()   # no cleanup of a failed run
    assert ctx.record.status == "ESCALATED" and not out.done


def test_template_nonconformance_is_an_output_failure(workspace, guard, scripted_gateway):
    claude = ScriptedClaude([cognitive_turn(text="# Summary\n\nno findings heading\n")])
    ctx = engine_ctx(workspace, guard, pipeline(), claude, scripted_gateway(replies=["ok"]))
    err = run_engine(ctx).escalation
    assert [f.code for f in err.findings] == ["template_conformance"] and "Findings" in err.findings[0].message


def test_missing_required_target_is_an_output_failure(workspace, guard, scripted_gateway):
    proc = pipeline(with_human=False)
    proc = dataclasses.replace(proc, steps=tuple(
        dataclasses.replace(s, produces=(decl("other", "other.md"),)) if s.name == "write-summary" else s
        for s in proc.steps))
    claude = ScriptedClaude([{"write": {"../work/write-summary/other.md": "x", "result.json": result_json()}}])
    ctx = engine_ctx(workspace, guard, proc, claude, scripted_gateway())
    err = run_engine(ctx).escalation
    assert err.exit_state == ExitState.OUTPUT_INVALID and err.findings[0].code == "output_missing"


@pytest.mark.parametrize("mode,archive,work", [("archive", True, False), ("delete", False, False), ("keep", False, True)])
def test_cleanup_of_work_products_follows_the_process(workspace, guard, scripted_gateway, mode, archive, work):
    ctx = engine_ctx(workspace, guard, pipeline(work_products=mode, with_human=False),
                     ScriptedClaude([cognitive_turn()]), scripted_gateway())
    assert run_engine(ctx).done
    assert ctx.paths.archive_tgz.exists() is archive and ctx.paths.work_dir.exists() is work
    assert ctx.paths.nodes_dir.is_dir() and (ctx.paths.output_dir / "summary.md").is_file()


# ------------------------------------------------------------------ cache drift

def three_cognitive():
    steps = [step(i, f"c{i}", "cognitive") for i in (1, 2, 3)]
    return process(steps, [edge("c1", "c2"), edge("c2", "c3"), edge("c3", "END")], "c1")


def usage_turn(cache_read):
    return {"write": {"result.json": result_json()},
            "usage": {"cache_read_input_tokens": cache_read, "ephemeral_1h_input_tokens": 1}}


def test_cache_prefix_drift_is_a_notice_and_the_run_continues(workspace, guard, scripted_gateway):
    gw = scripted_gateway()
    ctx = engine_ctx(workspace, guard, three_cognitive(), ScriptedClaude([usage_turn(900), usage_turn(1000), usage_turn(500)]), gw)
    out = run_engine(ctx)
    assert out.done and out.escalation is None
    drift = [n for n in gw.notices if n[1] == "CACHE_PREFIX_DRIFT"]
    assert len(drift) == 1 and drift[0][0] == "node c3"
    assert ctx.record.counters.p_run == 1000 and len(ctx.record.counters.drift) == 1
    assert any(r["dir"] == "notice" and r["name"] == "CACHE_PREFIX_DRIFT" for r in ctx.history.records())


def test_no_drift_notice_when_the_cache_read_holds(workspace, guard, scripted_gateway):
    gw = scripted_gateway()
    ctx = engine_ctx(workspace, guard, three_cognitive(), ScriptedClaude([usage_turn(900), usage_turn(1000), usage_turn(950)]), gw)
    assert run_engine(ctx).done
    assert [n for n in gw.notices if n[1] == "CACHE_PREFIX_DRIFT"] == []
    assert ctx.record.counters.drift == []


# ------------------------------------------------------------------ human timeout, quit

def test_human_step_timeout_escalates_when_the_decision_comes_too_late(workspace, guard, scripted_gateway):
    proc = pipeline()
    proc = dataclasses.replace(proc, steps=tuple(
        dataclasses.replace(s, timeout_s=0.05) if s.name == "review" else s for s in proc.steps))
    late = lambda source, prompt: (time.sleep(0.3), "ok")[1]
    ctx = engine_ctx(workspace, guard, proc, ScriptedClaude([cognitive_turn()]), scripted_gateway(replies=[late]))
    err = run_engine(ctx).escalation
    assert err.exit_state == ExitState.NODE_FAILED and err.node == "review" and err.detail["kind"] == "timeout"
    assert ctx.facts.all() == []                                                # the late decision was not applied


def _review_timeout(proc, seconds):
    return dataclasses.replace(proc, steps=tuple(
        dataclasses.replace(s, timeout_s=seconds) if s.name == "review" else s for s in proc.steps))


def test_human_step_wait_is_bounded_and_a_reply_timeout_is_node_failed_timeout(workspace, guard, scripted_gateway):
    """SFR-AGG-016: the gateway call itself gets the time left; ReplyTimeout -> NODE_FAILED(timeout)."""
    from fakes.scripted_gateway import TIMEOUT
    proc = _review_timeout(pipeline(), 30)
    gw = scripted_gateway(replies=["/show 1", TIMEOUT])
    ctx = engine_ctx(workspace, guard, proc, ScriptedClaude([cognitive_turn()]), gw)
    err = run_engine(ctx).escalation
    assert err.exit_state == ExitState.NODE_FAILED and err.node == "review" and err.detail["kind"] == "timeout"
    assert ctx.facts.all() == []
    waits = [t for k, src, t in gw.timeouts if src == "node review"]
    assert len(waits) == 2 and all(t is not None and 0 < t <= 30 for t in waits)
    assert waits[1] <= waits[0]                                                # the time left shrinks


def test_human_step_feedback_confirmation_is_bounded_too(workspace, guard, scripted_gateway):
    from fakes.scripted_gateway import TIMEOUT
    proc = _review_timeout(pipeline(), 30)
    gw = scripted_gateway(replies=["needs work"], confirms=[TIMEOUT])
    ctx = engine_ctx(workspace, guard, proc, ScriptedClaude([cognitive_turn()]), gw)
    err = run_engine(ctx).escalation
    assert err.exit_state == ExitState.NODE_FAILED and err.detail["kind"] == "timeout"
    assert [k for k, src, t in gw.timeouts if src == "node review"] == ["ask", "confirm"]


def test_human_step_without_timeout_waits_unbounded(workspace, guard, scripted_gateway):
    gw = scripted_gateway(replies=["ok"])
    ctx = engine_ctx(workspace, guard, pipeline(), ScriptedClaude([cognitive_turn()]), gw)
    assert run_engine(ctx).done
    assert [t for k, src, t in gw.timeouts if src == "node review"] == [None]


def test_human_step_times_out_on_a_real_gateway_pipe(workspace, guard):
    """End to end with the real Gateway on a pipe that never gets input."""
    import os
    from agent_graph.services.gateway import Gateway
    r, w = os.pipe()
    try:
        with os.fdopen(r, "r", closefd=False) as stdin:
            gw = Gateway(stdin=stdin, stdout=io.StringIO())
            ctx = engine_ctx(workspace, guard, _review_timeout(pipeline(), 0.3),
                             ScriptedClaude([cognitive_turn()]), gw)
            t0 = time.monotonic()
            err = run_engine(ctx).escalation
            assert time.monotonic() - t0 < 10
            assert err.exit_state == ExitState.NODE_FAILED and err.detail["kind"] == "timeout"
    finally:
        os.close(r)
        os.close(w)


def test_developer_quit_at_a_question_suspends_the_run(workspace, guard, scripted_gateway):
    ctx = engine_ctx(workspace, guard, pipeline(with_human=False), ScriptedClaude([QUESTION_TURN]),
                     scripted_gateway(replies=["/quit"]))
    out = run_engine(ctx)
    assert not out.done and out.escalation.exit_state == ExitState.SUSPENDED
    assert ctx.record.status == "SUSPENDED" and cmd_names(ctx)[-1] == "SUSPEND"


# ------------------------------------------------------------------ resume and restart

def test_resume_after_the_process_died_mid_node_repeats_nothing(workspace, guard, scripted_gateway):
    saver = InMemorySaver()
    claude = ScriptedClaude([QUESTION_TURN, cognitive_turn()])
    proc = pipeline(with_human=False)
    ctx1 = engine_ctx(workspace, guard, proc, claude, scripted_gateway(replies=[]), checkpointer=saver)
    with pytest.raises(TerminalClosed):                                         # EOF at the node's question
        run_engine(ctx1)
    paths = ctx1.paths
    assert json.loads(paths.run_json.read_text())["status"] == "SUSPENDED"
    count = paths.tmp_dir / "count.txt"
    assert count.read_text() == "x" and len(claude.calls) == 1

    record = RunRecord.from_json(json.loads(paths.run_json.read_text()))        # a new process loads run.json
    ctx2 = engine_ctx(workspace, guard, proc, claude, scripted_gateway(replies=["formal"]), mode="resume",
                      record=record, checkpointer=saver)
    out = run_engine(ctx2)
    assert out.done
    assert count.read_text() == "x"                                             # the procedure step did not run again
    assert len(claude.calls) == 2 and claude.calls[1]["resume"] == "sess-1"     # the stored session continued
    assert record.dynamic_thread == "dyn-2"
    assert (record.counters.steps, record.counters.llm_calls) == (2, 2)
    assert cmd_names(ctx2) == ["START", "RESTART", "ANSWER"]


def test_resume_of_a_run_that_had_reached_the_end_only_checks_and_cleans_up(workspace, guard, scripted_gateway):
    saver = InMemorySaver()
    claude = ScriptedClaude([cognitive_turn()])
    proc = pipeline(with_human=False, work_products="keep")
    ctx1 = engine_ctx(workspace, guard, proc, claude, scripted_gateway(), checkpointer=saver)
    assert run_engine(ctx1).done
    record = RunRecord.from_json(json.loads(ctx1.paths.run_json.read_text()))
    ctx2 = engine_ctx(workspace, guard, proc, claude, scripted_gateway(), mode="resume", record=record,
                      checkpointer=saver)
    out = run_engine(ctx2)
    assert out.done and len(claude.calls) == 1 and out.output_ref == f".output/{RUN_ID}"


def test_restart_after_resolution_runs_the_node_again_from_its_beginning(workspace, guard, scripted_gateway):
    flag = workspace.root / "fail.flag"
    flag.write_text("x")
    code = ("import pathlib,sys;flag=pathlib.Path(sys.argv[2]);"
            "sys.exit(1) if flag.exists() else pathlib.Path(sys.argv[1],'facts.json').write_text('[1]')")
    proc = pipeline(extract_run=(sys.executable, "-c", code, "{work}", str(flag)), retry=1)
    saver = InMemorySaver()
    claude = ScriptedClaude([cognitive_turn()])
    ctx1 = engine_ctx(workspace, guard, proc, claude, scripted_gateway(replies=["ok"]), checkpointer=saver)
    err = run_engine(ctx1).escalation
    assert err.exit_state == ExitState.NODE_FAILED and ctx1.record.counters.failures == {"extract": 1}

    flag.unlink()                                                               # the developer fixed the cause
    record = RunRecord.from_json(json.loads(ctx1.paths.run_json.read_text()))
    ctx2 = engine_ctx(workspace, guard, proc, claude, scripted_gateway(replies=["ok"]), mode="restart",
                      restart_node="extract", restart_summary="removed the flag", record=record,
                      checkpointer=saver)
    out = run_engine(ctx2)
    assert out.done
    assert [(f.kind, f.node, f.text) for f in ctx2.facts.all()][0] == ("resolution", "extract", "removed the flag")
    assert record.dynamic_thread == "dyn-2" and record.counters.failures == {}
    restart_rec = [r for r in ctx2.history.records() if r["name"] == "RESTART"][0]
    assert restart_rec["node"] == "extract" and restart_rec["payload"]["continue_session"] is False


def test_unknown_mode_is_rejected(workspace, guard, scripted_gateway):
    ctx = engine_ctx(workspace, guard, pipeline(), ScriptedClaude([]), scripted_gateway(), mode="bogus")
    with pytest.raises(ValueError):
        run_engine(ctx)
