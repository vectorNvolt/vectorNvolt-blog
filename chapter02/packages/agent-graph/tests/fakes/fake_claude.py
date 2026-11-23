"""Fake ``claude`` CLI for tests (no network, no login).

Two uses in one module:

1. As a script: ``[sys.executable, fake_claude.py, <claude args...>]``. Every invocation is
   one turn. It prints stream-json events to stdout, one per line.
2. As a library: ``FakeClaude`` (below) builds the scenario/log files and the command.

Environment of the script
-------------------------
FAKE_CLAUDE_SCENARIO  path to a JSON file: a list of turns, or {"turns": [...], "repeat_last": bool}.
                      The turn index lives in ``<scenario>.state`` (file-locked, so concurrent
                      calls get distinct turns). When the turns are exhausted: with
                      ``repeat_last`` the last turn is reused, else exit code 97 and stderr
                      "fake_claude: scenario exhausted".
FAKE_CLAUDE_LOG       path to a JSONL file; one line per call:
                      {"turn", "argv", "cwd", "env_keys", "prompt", "resume", "tmpdir", "claude_config_dir"}

Turn fields (all optional)
--------------------------
session_id      default: the value after ``--resume`` if given, else "fake-session-<turn+1>"
text            answer text (default "ok")
usage / usages  one usage dict, or a list (one ``assistant`` event each). Keys: input_tokens,
                output_tokens, cache_creation_input_tokens, cache_read_input_tokens,
                ephemeral_1h_input_tokens, ephemeral_5m_input_tokens (flat or under "cache_creation")
write_files     {relpath: str | obj}: written relative to the cwd BEFORE any event (str as is,
                other values as JSON). Simulates tool use, e.g. result.json or ../work/<node>/x.json
compact         true: emit a ``system``/``compact_boundary`` event after the assistant event
sleep           seconds to sleep before the result (cancel/timeout tests; the process is killable)
exit_code       process exit code (default 0); non-zero without ``result`` simulates a crash
is_error        true: the ``result`` event has is_error=true (subtype error_during_execution)
no_result       true: do not emit the ``result`` event
login_error     true: print "Not logged in" to stderr, no events, exit 1
stderr          text for stderr
extra_stdout    list of raw lines printed to stdout before the events (garbage-line tests)

Events (order): [system/init, assistant(s) with message.usage, system/compact_boundary?, result].
"""

import fcntl
import json
import os
import select
import sys
import time
from pathlib import Path

DEFAULT_USAGE = {"input_tokens": 10, "output_tokens": 5, "cache_creation_input_tokens": 0,
                 "cache_read_input_tokens": 0}


def _emit(obj) -> None:
    sys.stdout.write(json.dumps(obj) + "\n")
    sys.stdout.flush()


def _norm_usage(u: dict) -> dict:
    out = {k: v for k, v in u.items() if not k.startswith("ephemeral_") and k != "cache_creation"}
    nested = dict(u.get("cache_creation") or {})
    for k in ("ephemeral_1h_input_tokens", "ephemeral_5m_input_tokens"):
        if k in u:
            nested[k] = u[k]
    out["cache_creation"] = {"ephemeral_1h_input_tokens": nested.get("ephemeral_1h_input_tokens", 0),
                             "ephemeral_5m_input_tokens": nested.get("ephemeral_5m_input_tokens", 0)}
    return {**DEFAULT_USAGE, **out}


def _next_turn(scenario_path: str):
    state = scenario_path + ".state"
    with open(scenario_path, encoding="utf-8") as f:
        scenario = json.load(f)
    turns = scenario["turns"] if isinstance(scenario, dict) else scenario
    repeat_last = bool(scenario.get("repeat_last")) if isinstance(scenario, dict) else False
    with open(state, "a+") as sf:
        fcntl.flock(sf, fcntl.LOCK_EX)
        sf.seek(0)
        raw = sf.read().strip()
        idx = int(raw) if raw else 0
        sf.seek(0)
        sf.truncate()
        sf.write(str(idx + 1))
        sf.flush()
        fcntl.flock(sf, fcntl.LOCK_UN)
    if idx < len(turns):
        return idx, turns[idx]
    if repeat_last and turns:
        return idx, turns[-1]
    return idx, None


def _parse_args(argv: list[str]):
    prompt = None
    resume = None
    for i, a in enumerate(argv):
        if a in ("-p", "--print") and i + 1 < len(argv) and not argv[i + 1].startswith("-"):
            prompt = argv[i + 1]
        if a in ("--resume", "-r") and i + 1 < len(argv):
            resume = argv[i + 1]
    if prompt is None:
        try:
            ready, _, _ = select.select([sys.stdin], [], [], 0.5)
            if ready:
                prompt = sys.stdin.read()
        except (OSError, ValueError):
            pass
    return prompt, resume


def main(argv: list[str] | None = None) -> int:
    argv = list(sys.argv[1:] if argv is None else argv)
    prompt, resume = _parse_args(argv)
    scenario_path = os.environ.get("FAKE_CLAUDE_SCENARIO")
    if not scenario_path:
        sys.stderr.write("fake_claude: FAKE_CLAUDE_SCENARIO not set\n")
        return 96
    idx, turn = _next_turn(scenario_path)

    log = os.environ.get("FAKE_CLAUDE_LOG")
    if log:
        with open(log, "a", encoding="utf-8") as f:
            f.write(json.dumps({
                "turn": idx, "argv": argv, "cwd": os.getcwd(), "env_keys": sorted(os.environ),
                "prompt": prompt, "resume": resume, "tmpdir": os.environ.get("TMPDIR"),
                "claude_config_dir": os.environ.get("CLAUDE_CONFIG_DIR"),
            }) + "\n")

    if turn is None:
        sys.stderr.write("fake_claude: scenario exhausted\n")
        return 97

    if turn.get("login_error"):
        sys.stderr.write("Not logged in. Please run /login\n")
        return 1

    session_id = turn.get("session_id") or resume or f"fake-session-{idx + 1}"
    text = turn.get("text", "ok")

    for rel, content in (turn.get("write_files") or {}).items():
        p = Path(rel)
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_text(content if isinstance(content, str) else json.dumps(content), encoding="utf-8")

    for line in turn.get("extra_stdout") or []:
        sys.stdout.write(line + "\n")
        sys.stdout.flush()

    usages = turn.get("usages") or [turn.get("usage") or {}]
    usages = [_norm_usage(u) for u in usages]

    _emit({"type": "system", "subtype": "init", "session_id": session_id, "model": "fake",
           "cwd": os.getcwd(), "tools": []})
    for u in usages:
        _emit({"type": "assistant", "session_id": session_id,
               "message": {"role": "assistant", "content": [{"type": "text", "text": text}], "usage": u}})
    if turn.get("compact"):
        _emit({"type": "system", "subtype": "compact_boundary", "session_id": session_id,
               "compact_metadata": {"trigger": "auto", "pre_tokens": 1000}})

    if turn.get("stderr"):
        sys.stderr.write(turn["stderr"] + "\n")
        sys.stderr.flush()

    if turn.get("sleep"):
        time.sleep(float(turn["sleep"]))

    if not turn.get("no_result"):
        is_error = bool(turn.get("is_error"))
        _emit({"type": "result", "subtype": "error_during_execution" if is_error else "success",
               "is_error": is_error, "result": text, "session_id": session_id,
               "num_turns": 1, "usage": usages[-1]})
    return int(turn.get("exit_code", 0))


# ----------------------------------------------------------------------------- library side

SCRIPT = Path(__file__).resolve()


class FakeClaude:
    """Scenario + log files and the command to run the fake. Use in tests::

        fc = FakeClaude(tmp_path, [{"text": "hi"}, {"text": "bye", "compact": True}])
        cli = ClaudeCli(cmd=fc.cmd, env=fc.env)      # or fc.apply(monkeypatch) for env-driven code
        ...
        fc.calls()          # list of the log records (argv, cwd, prompt, resume, ...)
        fc.turns_used()     # how many turns were consumed
    """

    def __init__(self, tmp_path: Path, turns: list[dict], *, repeat_last: bool = False):
        self.scenario = Path(tmp_path) / "fake_claude_scenario.json"
        self.log = Path(tmp_path) / "fake_claude_log.jsonl"
        self.scenario.write_text(json.dumps({"turns": turns, "repeat_last": repeat_last}), encoding="utf-8")
        # A new scenario starts at turn 0: the turn index of an earlier FakeClaude on the same
        # tmp_path must not carry over (it would make the new scenario look exhausted).
        Path(str(self.scenario) + ".state").unlink(missing_ok=True)
        self.cmd: tuple[str, ...] = (sys.executable, str(SCRIPT))
        self.env: dict[str, str] = {"FAKE_CLAUDE_SCENARIO": str(self.scenario),
                                    "FAKE_CLAUDE_LOG": str(self.log)}

    @property
    def cmd_string(self) -> str:
        """Value for AGENT_GRAPH_CLAUDE_CMD."""
        import shlex
        return " ".join(shlex.quote(c) for c in self.cmd)

    def apply(self, monkeypatch) -> None:
        """Set FAKE_CLAUDE_* and AGENT_GRAPH_CLAUDE_CMD in the test process environment."""
        for k, v in self.env.items():
            monkeypatch.setenv(k, v)
        monkeypatch.setenv("AGENT_GRAPH_CLAUDE_CMD", self.cmd_string)

    def calls(self) -> list[dict]:
        if not self.log.is_file():
            return []
        return [json.loads(line) for line in self.log.read_text(encoding="utf-8").splitlines() if line.strip()]

    def turns_used(self) -> int:
        state = Path(str(self.scenario) + ".state")
        return int(state.read_text().strip() or 0) if state.is_file() else 0


if __name__ == "__main__":
    sys.exit(main())
