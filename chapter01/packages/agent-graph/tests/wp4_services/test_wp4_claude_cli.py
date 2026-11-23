"""ClaudeCli against fakes/fake_claude.py (SFR-AGG-019/020/029/036/040, SSR-AGG-009/012)."""

import json
import threading
import time

import pytest

from agent_graph import config
from agent_graph.errors import ClaudeTurnError, LlmUnavailable, TurnCancelled
from agent_graph.ports import CancelToken, Launch, TurnRequest, Usage
from agent_graph.services.claude_cli import ClaudeCli, scrub_env

LAUNCH = Launch(model="opus", effort="medium", tools=("Read", "Write"))


@pytest.fixture
def cwd(tmp_path):
    d = tmp_path / "cwd"
    d.mkdir()
    return d


def request(tmp_path, cwd, **kw):
    defaults = dict(cwd=cwd, prompt="do the thing", launch=LAUNCH, allowed_tools=("Read",),
                    resume=None, transcript=tmp_path / "out" / "transcript.jsonl")
    defaults.update(kw)
    return TurnRequest(**defaults)


def cli_for(fc, tmp_path):
    return ClaudeCli(cmd=fc.cmd, env=fc.env, tmpdir=tmp_path / "run_tmp")


def test_argv_order_and_invariant_part(tmp_path, cwd, fake_claude):
    fc = fake_claude([{"text": "hi"}])
    cli_for(fc, tmp_path).run_turn(request(tmp_path, cwd))
    call = fc.calls()[0]
    assert call["argv"] == ["-p", *LAUNCH.invariant_argv(), "--allowedTools", "Read"]
    assert call["prompt"] == "do the thing"
    assert call["cwd"] == str(cwd.resolve())


def test_allowed_tool_pattern_with_spaces_is_one_joined_value(tmp_path, cwd, fake_claude):
    fc = fake_claude([{}])
    pat = "Bash(run_ephemeral_container tool ngspice:*)"
    cli_for(fc, tmp_path).run_turn(request(tmp_path, cwd, allowed_tools=("Read", pat)))
    argv = fc.calls()[0]["argv"]
    assert argv[argv.index("--allowedTools") + 1] == f"Read,{pat}"


def test_resume_flag_and_session_id(tmp_path, cwd, fake_claude):
    fc = fake_claude([{"text": "again"}])
    res = cli_for(fc, tmp_path).run_turn(request(tmp_path, cwd, resume="abc-123"))
    call = fc.calls()[0]
    assert call["argv"][-2:] == ["--resume", "abc-123"]
    assert call["resume"] == "abc-123"
    assert res.session_id == "abc-123"
    assert res.argv[-2:] == ("--resume", "abc-123")


def test_no_allowed_tools_flag_when_empty(tmp_path, cwd, fake_claude):
    fc = fake_claude([{}])
    cli_for(fc, tmp_path).run_turn(request(tmp_path, cwd, allowed_tools=()))
    assert "--allowedTools" not in fc.calls()[0]["argv"]


def test_result_text_session_usage_compaction(tmp_path, cwd, fake_claude):
    fc = fake_claude([{"session_id": "s-1", "text": "the answer", "compact": True,
                       "usages": [{"input_tokens": 5, "cache_read_input_tokens": 70,
                                   "ephemeral_1h_input_tokens": 3},
                                  {"input_tokens": 6}]}])
    res = cli_for(fc, tmp_path).run_turn(request(tmp_path, cwd))
    assert res.session_id == "s-1"
    assert res.text == "the answer"
    assert res.exit_code == 0
    assert len(res.usages) == 2
    assert res.usages[0] == Usage(input_tokens=5, cache_read_input_tokens=70, ephemeral_1h_input_tokens=3)
    assert res.usages[1].input_tokens == 6
    assert len(res.compactions) == 1 and res.compactions[0]["trigger"] == "auto"


def test_garbage_stdout_lines_are_skipped(tmp_path, cwd, fake_claude):
    fc = fake_claude([{"extra_stdout": ["not json at all", "[1, 2]"], "text": "fine"}])
    assert cli_for(fc, tmp_path).run_turn(request(tmp_path, cwd)).text == "fine"


def test_files_written_by_the_child_land_in_cwd(tmp_path, cwd, fake_claude):
    fc = fake_claude([{"write_files": {"result.json": {"status": "done"}}}])
    cli_for(fc, tmp_path).run_turn(request(tmp_path, cwd))
    assert json.loads((cwd / "result.json").read_text()) == {"status": "done"}


def test_transcript_is_appended_across_turns(tmp_path, cwd, fake_claude):
    fc = fake_claude([{"text": "one"}, {"text": "two"}])
    cli = cli_for(fc, tmp_path)
    req = request(tmp_path, cwd)
    cli.run_turn(req)
    n1 = len(req.transcript.read_text().splitlines())
    cli.run_turn(req)
    lines = [json.loads(x) for x in req.transcript.read_text().splitlines()]
    assert n1 >= 3 and len(lines) == 2 * n1
    assert [e["type"] for e in lines[:n1]] == ["system", "assistant", "result"]


def test_env_is_scrubbed_and_merged(tmp_path, cwd, fake_claude, monkeypatch):
    monkeypatch.setenv("ANTHROPIC_API_KEY", "secret")
    monkeypatch.setenv("ANTHROPIC_BASE_URL", "http://evil")
    monkeypatch.setenv("CLAUDE_CODE_USE_BEDROCK", "1")
    monkeypatch.setenv("CLAUDE_CONFIG_DIR", "/home/agent/.claude")
    fc = fake_claude([{}])
    cli_for(fc, tmp_path).run_turn(
        request(tmp_path, cwd, env_extra={"ANTHROPIC_AUTH_TOKEN": "t", "KEEP_ME": "1"}))
    call = fc.calls()[0]
    keys = set(call["env_keys"])
    assert not {"ANTHROPIC_API_KEY", "ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN",
                "CLAUDE_CODE_USE_BEDROCK"} & keys
    assert {"PATH", "HOME", "CLAUDE_CONFIG_DIR", "FAKE_CLAUDE_SCENARIO", "KEEP_ME"} <= keys
    assert call["claude_config_dir"] == "/home/agent/.claude"
    assert call["tmpdir"] == str(tmp_path / "run_tmp")


def test_scrub_env_sets_config_dir_default():
    out = scrub_env({"PATH": "/bin", "ANTHROPIC_API_KEY": "x"})
    assert out == {"PATH": "/bin", "CLAUDE_CONFIG_DIR": str(config.CLAUDE_CONFIG_DIR)}


def test_login_error_is_llm_unavailable(tmp_path, cwd, fake_claude):
    fc = fake_claude([{"login_error": True}])
    with pytest.raises(LlmUnavailable) as ei:
        cli_for(fc, tmp_path).run_turn(request(tmp_path, cwd))
    assert "Not logged in" in ei.value.reason


def test_missing_result_event_is_llm_unavailable(tmp_path, cwd, fake_claude):
    fc = fake_claude([{"no_result": True}])
    with pytest.raises(LlmUnavailable):
        cli_for(fc, tmp_path).run_turn(request(tmp_path, cwd))


def test_nonzero_exit_is_llm_unavailable(tmp_path, cwd, fake_claude):
    fc = fake_claude([{"exit_code": 3, "stderr": "boom"}])
    with pytest.raises(LlmUnavailable) as ei:
        cli_for(fc, tmp_path).run_turn(request(tmp_path, cwd))
    assert "boom" in ei.value.reason


def test_missing_executable_is_llm_unavailable(tmp_path, cwd):
    cli = ClaudeCli(cmd=(str(tmp_path / "no-such-claude"),))
    with pytest.raises(LlmUnavailable):
        cli.run_turn(request(tmp_path, cwd))


def test_is_error_result_is_a_turn_error(tmp_path, cwd, fake_claude):
    fc = fake_claude([{"is_error": True, "text": "tool exploded"}])
    with pytest.raises(ClaudeTurnError) as ei:
        cli_for(fc, tmp_path).run_turn(request(tmp_path, cwd))
    assert "tool exploded" in ei.value.reason


def test_is_error_with_login_text_is_llm_unavailable(tmp_path, cwd, fake_claude):
    fc = fake_claude([{"is_error": True, "text": "Invalid API key - Please run /login"}])
    with pytest.raises(LlmUnavailable):
        cli_for(fc, tmp_path).run_turn(request(tmp_path, cwd))


def test_overlong_line_is_a_turn_error(tmp_path, cwd, fake_claude, monkeypatch):
    monkeypatch.setattr(config, "NDJSON_LINE_CAP", 100)
    fc = fake_claude([{"extra_stdout": ["{" + "x" * 500]}])
    with pytest.raises(ClaudeTurnError):
        cli_for(fc, tmp_path).run_turn(request(tmp_path, cwd))


def test_cancel_kills_the_child_and_raises(tmp_path, cwd, fake_claude):
    fc = fake_claude([{"sleep": 30}])
    token = CancelToken()
    threading.Timer(0.7, token.cancel, args=("budget:max_steps",)).start()
    t0 = time.monotonic()
    with pytest.raises(TurnCancelled) as ei:
        cli_for(fc, tmp_path).run_turn(request(tmp_path, cwd), token)
    assert ei.value.cause == "budget:max_steps"
    assert time.monotonic() - t0 < 15


def test_timeout_kills_the_child(tmp_path, cwd, fake_claude):
    fc = fake_claude([{"sleep": 30}])
    t0 = time.monotonic()
    with pytest.raises(TurnCancelled) as ei:
        cli_for(fc, tmp_path).run_turn(request(tmp_path, cwd, timeout_s=0.7))
    assert ei.value.cause == "timeout"
    assert time.monotonic() - t0 < 15


def test_already_cancelled_token_starts_no_child(tmp_path, cwd, fake_claude):
    fc = fake_claude([{}])
    token = CancelToken()
    token.cancel("timeout")
    with pytest.raises(TurnCancelled):
        cli_for(fc, tmp_path).run_turn(request(tmp_path, cwd), token)
    assert fc.calls() == []
