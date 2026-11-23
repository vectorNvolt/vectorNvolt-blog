"""SSR-AGG-004 vocabulary, ports value types, exit codes (SFR-AGG-032), and the shared fakes."""

import json
import subprocess
import sys
import threading

import pytest

from agent_graph import contract
from agent_graph import config
from agent_graph.contract import Cmd, Notice, Status, PAUSING
from agent_graph.errors import ExitState, Finding, RunError, TerminalClosed
from agent_graph.ports import CancelToken, Launch, Reply, Usage
from fakes.fake_claude import FakeClaude
from fakes.scripted_gateway import ScriptedGateway


def test_cmd_status_roundtrip():
    for c in (contract.start(), contract.restart("n", "sum", True), contract.hil_ok("n"),
              contract.hil_nok("n", "fix it"), contract.answer("n", "42"), contract.retry("n", "why"),
              contract.cancel("n", "timeout"), contract.suspend("quit")):
        assert Cmd.from_json(json.loads(json.dumps(c.to_json()))) == c
    s = Status("NODE_DONE", "n", {"outcome": None, "products": [], "summary": "x"})
    assert Status.from_json(json.loads(json.dumps(s.to_json()))) == s


def test_helper_payloads():
    assert contract.restart("n").payload == {"summary": "", "continue_session": False}
    assert contract.hil_nok("n", "f").payload == {"feedback": "f"}
    assert contract.answer("n", "t").payload == {"text": "t"}
    assert contract.retry("n", "r").payload == {"reason": "r"}
    assert contract.cancel("n", "budget:max_steps").payload == {"cause": "budget:max_steps"}
    assert contract.suspend("r").node is None and contract.suspend("r").payload == {"reason": "r"}


def test_unknown_names_rejected():
    with pytest.raises(ValueError):
        Cmd.from_json({"name": "EXPLODE"})
    with pytest.raises(ValueError):
        Status.from_json({"name": "BOOM"})


def test_pausing_statuses():
    assert PAUSING == {"WAITING", "NODE_FAILED", "COMPLETED", "SUSPENDED"}


def test_exit_codes():
    assert ExitState.COMPLETED.exit_code == 0
    assert ExitState.NOT_STARTED.exit_code == 1
    for s in ("BUILD_FAILED", "CONTEXT_OVER_BUDGET", "NODE_FAILED", "BUDGET_EXCEEDED",
              "LLM_UNAVAILABLE", "OUTPUT_INVALID"):
        assert ExitState(s).exit_code == 2
    for s in ("INTENT_REJECTED", "PROCESS_INVALID", "INPUT_INVALID"):
        assert ExitState(s).exit_code == 3
    assert ExitState.SUSPENDED.exit_code == config.EXIT_SUSPENDED
    assert {e.name for e in ExitState} == {e.value for e in ExitState}


def test_run_error_roundtrip():
    err = RunError(ExitState.PROCESS_INVALID, "S3", "bad", (Finding("yaml", "m", "a.yml", 3, "s", "i"),),
                   node="n", detail={"k": 1})
    assert RunError.from_json(json.loads(json.dumps(err.to_json()))) == err


def test_launch_argv_and_hash():
    la = Launch(model="opus", effort="medium", tools=("Read", "Write"))
    assert la.invariant_argv() == [
        "--output-format", "stream-json", "--verbose", "--model", "opus", "--effort", "medium",
        "--tools", "Read,Write", "--setting-sources", "project", "--strict-mcp-config",
        "--mcp-config", "mcp.json", "--exclude-dynamic-system-prompt-sections",
        "--append-system-prompt-file", "system.md"]
    assert la.launch_hash() == Launch("opus", "medium", ("Read", "Write")).launch_hash()
    assert la.launch_hash() != Launch("opus", "high", ("Read", "Write")).launch_hash()


def test_usage_json():
    assert Usage(1, 2, 3, 4, 5).to_json() == {
        "input_tokens": 1, "cache_creation_input_tokens": 2, "cache_read_input_tokens": 3,
        "ephemeral_1h_input_tokens": 4, "ephemeral_5m_input_tokens": 5}


def test_cancel_token():
    t = CancelToken()
    assert not t.cancelled and t.cause is None and t.wait(0.01) is False
    threading.Timer(0.05, t.cancel, args=("timeout",)).start()
    assert t.wait(2) is True
    t.cancel("budget:x")                    # first cause wins
    assert t.cancelled and t.cause == "timeout"


# ---- fakes

def test_scripted_gateway():
    gw = ScriptedGateway(["hello", "/show 2", "/quit"], confirms=[False], choices=[1])
    assert gw.ask("S2", "q").text == "hello"
    r = gw.ask("S2", "q", commands=("/show",))
    assert (r.command, r.text) == ("/show", "2")
    assert gw.ask("S2", "q", allow_quit=True) == Reply("", True, "/quit")
    with pytest.raises(TerminalClosed):
        gw.ask("S2", "q")
    assert gw.choose("S2", "p", ["a", "b"]) == 1
    assert gw.confirm("S2", "ok?") is False
    with pytest.raises(TerminalClosed):                  # scripts exhausted, no explicit default
        gw.confirm("S2", "ok?")
    with pytest.raises(TerminalClosed):
        gw.choose("S2", "p", ["a", "b"])
    gw2 = ScriptedGateway(default_confirm=True, default_choice=1)
    assert gw2.confirm("S2", "ok?") is True and gw2.choose("S2", "p", ["a", "b"]) == 1
    gw.say("S2", "hi")
    gw.say("S2", "llm text", from_llm=True)
    gw.notice("n", "WARNING", "w")
    with gw.conversation("S2"):
        pass
    assert gw.said() == ["hi", "llm text"] and gw.said(include_llm=False) == ["hi"]
    assert gw.notices == [("n", "WARNING", "w")]
    assert gw.conversations == [("S2", "enter"), ("S2", "exit")]


def test_scripted_gateway_timeout_sentinel():
    from agent_graph.errors import ReplyTimeout
    gw = ScriptedGateway([ScriptedGateway.TIMEOUT, ScriptedGateway.TIMEOUT],
                         confirms=[ScriptedGateway.TIMEOUT], choices=[ScriptedGateway.TIMEOUT])
    with pytest.raises(ReplyTimeout):
        gw.ask("n", "q", timeout_s=5)
    with pytest.raises(ReplyTimeout):
        gw.confirm("n", "q", timeout_s=1.5)
    with pytest.raises(ReplyTimeout):
        gw.choose("n", "q", ["a"], timeout_s=2)
    with pytest.raises(AssertionError):                  # the real call would wait forever
        gw.ask("n", "q")
    assert gw.timeouts == [("ask", "n", 5), ("confirm", "n", 1.5), ("choose", "n", 2), ("ask", "n", None)]


def test_scripted_gateway_attach_flushes(tmp_path):
    gw = ScriptedGateway()
    gw.say("S1", "before")
    gw.attach_run(tmp_path / "run" / "gateway.jsonl")
    gw.say("S1", "after")
    lines = (tmp_path / "run" / "gateway.jsonl").read_text().splitlines()
    assert [json.loads(x)["text"] for x in lines] == ["before", "after"]


def _call(fc: FakeClaude, *args, cwd=None):
    import os
    env = {**os.environ, **fc.env}
    return subprocess.run([*fc.cmd, *args], capture_output=True, text=True, env=env, cwd=cwd, timeout=30)


def test_fake_claude_turns(tmp_path, fake_claude):
    fc = fake_claude([
        {"text": "first", "usage": {"input_tokens": 7, "ephemeral_1h_input_tokens": 3},
         "write_files": {"out/result.json": {"status": "done"}}},
        {"text": "second", "compact": True},
        {"is_error": True, "text": "boom"},
        {"login_error": True},
    ])
    work = tmp_path / "cwd"
    work.mkdir()
    p = _call(fc, "-p", "do it", "--output-format", "stream-json", cwd=work)
    assert p.returncode == 0
    events = [json.loads(x) for x in p.stdout.splitlines()]
    assert [e["type"] for e in events] == ["system", "assistant", "result"]
    assert events[0]["subtype"] == "init" and events[0]["session_id"] == "fake-session-1"
    assert events[1]["message"]["usage"]["input_tokens"] == 7
    assert events[1]["message"]["usage"]["cache_creation"]["ephemeral_1h_input_tokens"] == 3
    assert events[2]["result"] == "first" and events[2]["is_error"] is False
    assert json.loads((work / "out" / "result.json").read_text()) == {"status": "done"}

    p = _call(fc, "-p", "again", "--resume", "sess-9", cwd=work)
    events = [json.loads(x) for x in p.stdout.splitlines()]
    assert events[0]["session_id"] == "sess-9"
    assert any(e.get("subtype") == "compact_boundary" for e in events)

    p = _call(fc, "-p", "x", cwd=work)
    assert json.loads(p.stdout.splitlines()[-1])["is_error"] is True

    p = _call(fc, "-p", "x", cwd=work)
    assert p.returncode == 1 and p.stdout == "" and "Not logged in" in p.stderr

    p = _call(fc, "-p", "x", cwd=work)          # exhausted
    assert p.returncode == 97

    calls = fc.calls()
    assert len(calls) == 5
    assert calls[0]["prompt"] == "do it"
    assert calls[0]["cwd"] in (str(work), str(work.resolve()))
    assert calls[1]["resume"] == "sess-9"
    assert fc.turns_used() == 5
