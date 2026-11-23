"""WP5 tests of the dynamic machine driven directly with commands (real LangGraph graph,
InMemorySaver, sqlite journal, scripted Claude, fake compose)."""

import json
import os
import sys
import threading

from agent_graph.contract import Notice, Status, answer, hil_nok, hil_ok, restart, retry, start, suspend
from agent_graph.errors import ClaudeTurnError, LlmUnavailable

from test_wp5_helpers import (FakeCompose, OpenGate, ScriptedClaude, cognitive_turn, drive, dynamic_setup, edge,
                              pipeline, process, result_json, step)

QUESTION_TURN = {"write": {"result.json": result_json(status="question", question="Which tone?")}}


def names(items):
    return [i.name for i in items if isinstance(i, Status)]


def test_linear_run_reports_statuses_in_order_and_completes(workspace, guard):
    claude = ScriptedClaude([cognitive_turn()])
    s = dynamic_setup(workspace, guard, pipeline(with_human=False), claude)
    m = s.machine()
    items, last = drive(m, start())
    assert names(items) == ["NODE_STARTED", "NODE_DONE", "NODE_STARTED", "NODE_DONE", "COMPLETED"]
    assert last.payload["output_ref"] == f".output/{s.paths.run_id}"
    paths = s.paths
    # cognitive node record: active/ moved to nodes/<node>/, handoff and transcript there
    assert not paths.active.exists()
    assert (paths.node_dir("write-summary") / "handoff.json").is_file()
    assert (paths.node_dir("write-summary") / "transcript.jsonl").is_file()
    assert (paths.output_dir / "summary.md").is_file()               # staged output target
    values = m.values()
    assert set(values["products"]) == {"facts", "Report"}
    assert values["handoffs"] == {"write-summary": "nodes/write-summary/handoff.json"}
    assert values["summaries"]["write-summary"] == "did it"
    assert values["visits"] == {"extract": 1, "write-summary": 1}
    done = [i for i in items if isinstance(i, Status) and i.name == "NODE_DONE"][1]
    assert done.payload["usage"]["first_turn"]["ephemeral_1h_input_tokens"] == 1
    call = claude.calls[0]
    assert call["cwd"] == paths.active and call["resume"] is None and call["allowed"] == ("Read", "Write")


def test_procedure_runs_workspace_script_by_relative_path_from_its_work_folder(workspace, guard):
    """SSR-AGG-013: the step names .process/scripts/<x> relative to /workspace; it runs in work/<node>."""
    workspace.root.joinpath(".process/scripts/where.py").write_text(
        "import os, pathlib, sys\n"
        "pathlib.Path(sys.argv[1], 'facts.json').write_text('\"' + os.getcwd() + '|' + sys.argv[0] + '\"')\n")
    proc = pipeline(with_human=False, extract_run=(sys.executable, ".process/scripts/where.py", "{work}"))
    s = dynamic_setup(workspace, guard, proc, ScriptedClaude([cognitive_turn()]))
    items, last = drive(s.machine(), start())
    assert last.name == "COMPLETED", last
    cwd, script = json.loads((s.paths.work_node("extract") / "facts.json").read_text()).split("|")
    assert cwd == os.path.realpath(s.paths.work_node("extract"))
    assert script == str(workspace.root / ".process/scripts/where.py")


def test_history_records_the_command_before_the_statuses_it_causes(workspace, guard):
    s = dynamic_setup(workspace, guard, pipeline(with_human=False), ScriptedClaude([cognitive_turn()]))
    drive(s.machine(), start())
    recs = s.history.records()
    assert (recs[0]["dir"], recs[0]["name"]) == ("cmd", "START")
    assert [r["name"] for r in recs if r["dir"] == "status"] == [
        "NODE_STARTED", "NODE_DONE", "NODE_STARTED", "NODE_DONE", "COMPLETED"]
    assert any(r["dir"] == "notice" and r["name"] == "PROGRESS" for r in recs)     # success PRINT texts
    assert [r["seq"] for r in recs] == list(range(1, len(recs) + 1))


def test_success_text_of_a_step_is_a_progress_notice(workspace, guard):
    s = dynamic_setup(workspace, guard, pipeline(with_human=False), ScriptedClaude([cognitive_turn()]))
    items, _ = drive(s.machine(), start())
    texts = [i.text for i in items if isinstance(i, Notice)]
    assert texts == ["STEP 1: DONE", "STEP 2: DONE"]
    assert [i.source for i in items if isinstance(i, Notice)] == ["node extract", "node write-summary"]


def test_human_rejection_loops_back_with_feedback_and_a_new_session(workspace, guard):
    claude = ScriptedClaude([cognitive_turn(), cognitive_turn()])
    fc = FakeCompose()
    s = dynamic_setup(workspace, guard, pipeline(), claude, compose=fc)
    m = s.machine()
    _, last = drive(m, start())
    assert last.name == "WAITING" and last.payload["mode"] == "HIL" and last.node == "review"
    assert last.payload["reads"][0]["path"].endswith("summary.md")
    assert "Summary" in last.payload["reads"][0]["inline"]
    _, last = drive(m, hil_nok("review", "needs more detail"))
    assert last.name == "WAITING" and last.node == "review"
    fb = s.paths.work_node("review") / "review_feedback.json"
    assert "needs more detail" in fb.read_text()
    assert "review_feedback" in m.values()["products"]
    assert "review_feedback" in fc.calls[1].products and "review_feedback" not in fc.calls[0].products
    assert m.values()["visits"]["write-summary"] == 2
    assert claude.calls[1]["resume"] is None                         # a repeat is a new session
    assert (s.paths.nodes_dir / "write-summary.1").is_dir()          # the earlier visit's record is kept
    _, last = drive(m, hil_ok("review"))
    assert last.name == "COMPLETED"


def test_question_is_answered_in_the_same_session_without_rerunning_the_turn(workspace, guard):
    claude = ScriptedClaude([QUESTION_TURN, cognitive_turn()])
    s = dynamic_setup(workspace, guard, pipeline(with_human=False), claude)
    m = s.machine()
    _, last = drive(m, start())
    assert (last.name, last.payload["mode"], last.payload["question"]) == ("WAITING", "INPUT", "Which tone?")
    assert len(claude.calls) == 1
    _, last = drive(m, answer("write-summary", "formal"))
    assert last.name == "COMPLETED"
    assert len(claude.calls) == 2                                    # replay did not repeat turn 0
    assert claude.calls[1]["resume"] == "sess-1" and "formal" in claude.calls[1]["prompt"]


def test_failed_attempt_is_retried_in_the_same_session_with_the_reason(workspace, guard):
    claude = ScriptedClaude([{"text": "forgot the files"}, cognitive_turn()])
    fc = FakeCompose()
    s = dynamic_setup(workspace, guard, pipeline(with_human=False), claude, compose=fc)
    m = s.machine()
    _, last = drive(m, start())
    assert last.name == "NODE_FAILED"
    assert (last.payload["kind"], last.payload["retryable"], last.payload["cause"]) == ("schema", True, None)
    assert "result.json" in last.payload["reason"]
    _, last = drive(m, retry("write-summary", last.payload["reason"]))
    assert last.name == "COMPLETED"
    assert claude.calls[1]["resume"] == "sess-1" and "previous attempt failed" in claude.calls[1]["prompt"]
    assert len(fc.calls) == 1                                        # a retry does not recompose
    assert [r["name"] for r in s.history.records() if r["name"] == "RETRY"] == ["RETRY"]


def test_missing_product_and_bad_outcome_are_schema_failures(workspace, guard):
    proc = pipeline(with_human=False)
    claude = ScriptedClaude([{"write": {"result.json": result_json()}}])      # no summary.md
    s = dynamic_setup(workspace, guard, proc, claude)
    _, last = drive(s.machine(), start())
    assert last.name == "NODE_FAILED" and "summary.md" in last.payload["reason"]


def test_llm_unavailable_is_not_retryable_and_suspend_pauses_the_machine(workspace, guard):
    claude = ScriptedClaude([{"raise": LlmUnavailable("not logged in")}])
    s = dynamic_setup(workspace, guard, pipeline(with_human=False), claude)
    m = s.machine()
    _, last = drive(m, start())
    assert (last.name, last.payload["cause"], last.payload["retryable"]) == ("NODE_FAILED", "llm_unavailable", False)
    _, last = drive(m, suspend("developer"))
    assert last.name == "SUSPENDED" and m.pending().name == "SUSPENDED"
    _, again = drive(m, suspend("again"))
    assert again.name == "SUSPENDED"


def test_claude_turn_error_is_a_retryable_tool_failure(workspace, guard):
    claude = ScriptedClaude([{"raise": ClaudeTurnError("tool crashed")}])
    s = dynamic_setup(workspace, guard, pipeline(with_human=False), claude)
    _, last = drive(s.machine(), start())
    assert (last.payload["kind"], last.payload["retryable"]) == ("tool", True)
    assert "tool crashed" in last.payload["reason"]


def test_context_over_budget_fails_without_starting_a_session(workspace, guard):
    claude = ScriptedClaude([])
    s = dynamic_setup(workspace, guard, pipeline(with_human=False), claude,
                      compose=FakeCompose(over_budget={"write-summary"}))
    _, last = drive(s.machine(), start())
    assert last.name == "NODE_FAILED" and last.payload["kind"] == "context"
    assert (last.payload["retryable"], last.payload["budget"], last.payload["mandatory_tokens"]) == (False, 100, 500)
    assert claude.calls == []


def test_cancel_stops_the_running_attempt_as_a_timeout_failure(workspace, guard):
    claude = ScriptedClaude([{"block": 20}])
    s = dynamic_setup(workspace, guard, pipeline(with_human=False), claude)
    m = s.machine()
    threading.Timer(1.5, lambda: m.cancel("write-summary", "timeout")).start()
    _, last = drive(m, start())
    assert last.name == "NODE_FAILED"
    assert (last.payload["kind"], last.payload["cause"], last.payload["retryable"]) == (
        "timeout", "cancelled:timeout", True)
    assert any(r["dir"] == "cmd" and r["name"] == "CANCEL" for r in s.history.records())


def test_gate_refusal_reports_node_failed_without_node_started(workspace, guard):
    class Refuse(OpenGate):
        def before_node_start(self, node):
            return "budget:max_steps"
    s = dynamic_setup(workspace, guard, pipeline(with_human=False), ScriptedClaude([]), gate=Refuse())
    items, last = drive(s.machine(), start())
    assert names(items) == ["NODE_FAILED"]
    assert (last.payload["cause"], last.payload["retryable"]) == ("cancelled:budget:max_steps", False)


def test_llm_call_budget_refusal_is_a_budget_cancellation(workspace, guard):
    class Refuse(OpenGate):
        def before_llm_call(self, node):
            return "budget:max_llm_calls"
    claude = ScriptedClaude([])
    s = dynamic_setup(workspace, guard, pipeline(with_human=False), claude, gate=Refuse())
    _, last = drive(s.machine(), start())
    assert last.payload["cause"] == "cancelled:budget:max_llm_calls" and claude.calls == []


# ------------------------------------------------------------------ procedure steps

LOOP_PY = ("import pathlib,sys;w=pathlib.Path(sys.argv[1]);n=len(list(w.glob('run*')));"
           "(w/f'run{n}').write_text('x');(w/'.outcome').write_text('again' if n < 2 else 'stop')")


def loop_process():
    return process(
        [step(1, "a", "procedure", run=(sys.executable, "-c", LOOP_PY, "{work}"), outcomes=("again", "stop"))],
        [edge("a", "a", "outcome", "again"), edge("a", "END", "outcome", "stop")], "a")


def test_outcome_edges_repeat_a_procedure_step_until_the_outcome_changes(workspace, guard):
    s = dynamic_setup(workspace, guard, loop_process(), ScriptedClaude([]))
    m = s.machine()
    items, last = drive(m, start())
    assert names(items) == ["NODE_STARTED", "NODE_DONE"] * 3 + ["COMPLETED"]
    assert [i.payload["outcome"] for i in items if isinstance(i, Status) and i.name == "NODE_DONE"] == [
        "again", "again", "stop"]
    assert m.values()["visits"] == {"a": 3}                          # braces in program text were left alone


def test_procedure_warning_lines_become_warning_notices(workspace, guard):
    code = "print('WARNING: careful');print('plain')"
    proc = process([step(1, "a", "procedure", run=(sys.executable, "-c", code))], [edge("a", "END")], "a")
    s = dynamic_setup(workspace, guard, proc, ScriptedClaude([]))
    items, last = drive(s.machine(), start())
    warns = [i for i in items if isinstance(i, Notice) and i.code == "WARNING"]
    assert [(w.source, w.text) for w in warns] == [("node a", "careful")]
    assert last.name == "COMPLETED"


def test_procedure_failure_is_retried_by_running_the_command_again(workspace, guard):
    code = ("import os,sys;open(os.path.join(os.environ['TMPDIR'],'runs.txt'),'a').write('x');"
            "sys.stderr.write('bad input');sys.exit(1)")
    proc = process([step(1, "a", "procedure", run=(sys.executable, "-c", code))], [edge("a", "END")], "a")
    s = dynamic_setup(workspace, guard, proc, ScriptedClaude([]))
    m = s.machine()
    _, last = drive(m, start())
    assert (last.name, last.payload["kind"], last.payload["retryable"]) == ("NODE_FAILED", "tool", True)
    assert "bad input" in last.payload["reason"]
    _, last = drive(m, retry("a", "again"))
    assert last.name == "NODE_FAILED"
    assert (s.paths.tmp_dir / "runs.txt").read_text() == "xx"


def test_procedure_outcome_outside_the_declared_set_is_a_schema_failure(workspace, guard):
    code = "import pathlib,sys;(pathlib.Path(sys.argv[1])/'.outcome').write_text('weird')"
    proc = process([step(1, "a", "procedure", run=(sys.executable, "-c", code, "{work}"), outcomes=("x", "y"))],
                   [edge("a", "END", "outcome", "x"), edge("a", "END", "outcome", "y")], "a")
    s = dynamic_setup(workspace, guard, proc, ScriptedClaude([]))
    _, last = drive(s.machine(), start())
    assert last.payload["kind"] == "schema" and "weird" in last.payload["reason"]


def test_isolate_true_without_a_profile_fails(workspace, guard):
    proc = process([step(1, "a", "procedure", run=(sys.executable, "-c", "pass"), isolate=True)],
                   [edge("a", "END")], "a")
    s = dynamic_setup(workspace, guard, proc, ScriptedClaude([]))
    _, last = drive(s.machine(), start())
    assert last.name == "NODE_FAILED" and not last.payload["retryable"] and "profile" in last.payload["reason"]


# ------------------------------------------------------------------ resume on a new thread

def test_new_thread_continues_an_interrupted_node_without_repeating_work(workspace, guard):
    claude = ScriptedClaude([QUESTION_TURN, cognitive_turn()])
    s = dynamic_setup(workspace, guard, pipeline(with_human=False), claude)
    _, last = drive(s.machine("dyn-1"), start())
    assert last.name == "WAITING"                                    # "process killed" here
    assert (s.paths.tmp_dir / "count.txt").read_text() == "x"        # extract ran once
    m2 = s.machine("dyn-2", carry_from="dyn-1")
    items, last = drive(m2, restart("write-summary", "", True))
    assert names(items)[0] == "NODE_STARTED" and last.name == "WAITING" and last.payload["mode"] == "INPUT"
    assert len(claude.calls) == 1                                    # turn 0 was journaled
    _, last = drive(m2, answer("write-summary", "formal"))
    assert last.name == "COMPLETED" and len(claude.calls) == 2
    assert claude.calls[1]["resume"] == "sess-1"                     # the stored session continued
    assert (s.paths.tmp_dir / "count.txt").read_text() == "x"


def test_new_thread_does_not_ask_an_answered_question_again(workspace, guard):
    claude = ScriptedClaude([QUESTION_TURN, {"raise": ClaudeTurnError("died")}, cognitive_turn()])
    s = dynamic_setup(workspace, guard, pipeline(with_human=False), claude)
    m1 = s.machine("dyn-1")
    drive(m1, start())
    _, last = drive(m1, answer("write-summary", "formal"))
    assert last.name == "NODE_FAILED"
    m2 = s.machine("dyn-2", carry_from="dyn-1")
    _, last = drive(m2, restart("write-summary", "", True))
    assert last.name == "NODE_FAILED" and "died" in last.payload["reason"]     # no second WAITING
    assert len(claude.calls) == 2
    _, last = drive(m2, retry("write-summary", "died"))
    assert last.name == "COMPLETED" and claude.calls[2]["resume"] == "sess-1"


def test_restart_from_the_beginning_starts_a_new_visit_and_session(workspace, guard):
    claude = ScriptedClaude([QUESTION_TURN, cognitive_turn()])
    s = dynamic_setup(workspace, guard, pipeline(with_human=False), claude)
    drive(s.machine("dyn-1"), start())
    m2 = s.machine("dyn-2", carry_from="dyn-1")
    _, last = drive(m2, restart("write-summary", "fixed the input", False))
    assert last.name == "COMPLETED"
    assert claude.calls[1]["resume"] is None                         # not the interrupted visit's session
    assert m2.values()["visits"]["write-summary"] == 2
    rec = [r for r in s.history.records() if r["name"] == "RESTART"][0]
    assert rec["payload"] == {"summary": "fixed the input", "continue_session": False}


def test_suspend_at_a_human_step_and_without_pending_work(workspace, guard):
    s = dynamic_setup(workspace, guard, pipeline(), ScriptedClaude([cognitive_turn()]))
    m = s.machine()
    _, last = drive(m, suspend("nothing running"))
    assert last.name == "SUSPENDED"
    m2 = s.machine("dyn-2")
    _, last = drive(m2, start())
    assert last.name == "WAITING" and last.node == "review"
    _, last = drive(m2, suspend("developer left"))
    assert last.name == "SUSPENDED" and last.node == "review"
