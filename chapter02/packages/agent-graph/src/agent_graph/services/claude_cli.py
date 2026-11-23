"""ClaudePort implementation: one ``claude -p`` child process per turn.

Covers: SFR-AGG-019/020 (session id, usage, compactions from stream-json), SFR-AGG-029
(login/network/process failure -> LlmUnavailable), SFR-AGG-036 (own process group, killed with
os.killpg on cancel), SFR-AGG-040 / SSR-AGG-010 (invariant flags from Launch), SSR-AGG-009
(overlong line -> ClaudeTurnError), SSR-AGG-012 (environment scrub: no API key, no base URL).

argv: ``[*cmd, "-p", *launch.invariant_argv(), ("--allowedTools", "A,B")?, ("--resume", sid)?]``.
The prompt is written to the child's stdin (no ``-p <prompt>`` argument: no argv length limit,
no confusion with a prompt that starts with "-"). The child is started with an argument list,
never a shell, with ``start_new_session=True``.

Environment: copy of os.environ, plus ``env`` (constructor), plus TMPDIR (if ``tmpdir`` given),
plus ``req.env_extra``; then scrubbed: every ``ANTHROPIC_*`` and ``CLAUDE_CODE_USE_*`` variable is
removed, and ``CLAUDE_CONFIG_DIR`` is set to config.CLAUDE_CONFIG_DIR when absent.

Failure mapping (stderr is never the answer; its tail only goes into reasons):
  cancel token / timeout_s          -> TurnCancelled(cause | "timeout")
  executable missing                -> LlmUnavailable
  result is_error, auth/network text-> LlmUnavailable; other is_error -> ClaudeTurnError
  non-zero exit or no result event  -> LlmUnavailable
  line over NDJSON_LINE_CAP         -> ClaudeTurnError
"""

import json
import os
import re
import signal
import subprocess
import threading
import time
from collections import deque
from pathlib import Path
from typing import Mapping

from .. import config
from ..errors import ClaudeTurnError, LlmUnavailable, TurnCancelled
from ..ports import CancelToken, TurnRequest, TurnResult
from .stream_json import TurnAccumulator, iter_ndjson

_UNAVAILABLE_RE = re.compile(
    r"not logged in|/login|log in|invalid api key|authentication|unauthori[sz]ed|oauth|"
    r"credential|forbidden|ENOTFOUND|ECONNREFUSED|ECONNRESET|ETIMEDOUT|EAI_AGAIN|"
    r"network|unable to connect|could not resolve|connection (error|refused|reset|timed out)|"
    r"rate limit|overloaded|service unavailable|\b(401|403|429|502|503|529)\b",
    re.IGNORECASE)
_KILL_GRACE_S = 2.0
_POLL_S = 0.05


def scrub_env(env: Mapping[str, str]) -> dict[str, str]:
    """SSR-AGG-012: drop every ANTHROPIC_* and CLAUDE_CODE_USE_* variable; keep CLAUDE_CONFIG_DIR."""
    out = {k: v for k, v in env.items()
           if not k.startswith("ANTHROPIC_") and not k.startswith("CLAUDE_CODE_USE_")}
    out.setdefault("CLAUDE_CONFIG_DIR", str(config.CLAUDE_CONFIG_DIR))
    return out


def _kill_group(proc: subprocess.Popen) -> None:
    """SIGTERM the child's process group, SIGKILL after a grace period."""
    for sig in (signal.SIGTERM, signal.SIGKILL):
        try:
            os.killpg(proc.pid, sig)
        except (ProcessLookupError, PermissionError):
            return
        try:
            proc.wait(timeout=_KILL_GRACE_S)
            return
        except subprocess.TimeoutExpired:
            continue


class ClaudeCli:
    def __init__(self, cmd: tuple[str, ...] = config.CLAUDE_CMD, env: Mapping[str, str] | None = None,
                 tmpdir: Path | None = None):
        self.cmd = tuple(cmd)
        self.env = dict(env or {})
        self.tmpdir = Path(tmpdir) if tmpdir is not None else None

    # ---------------------------------------------------------------- building blocks
    def build_argv(self, req: TurnRequest) -> list[str]:
        argv = [*self.cmd, "-p", *req.launch.invariant_argv()]
        if req.events is not None:              # live rendering needs the partial stream_event deltas
            argv.append("--include-partial-messages")
        if req.allowed_tools:
            argv += ["--allowedTools", ",".join(req.allowed_tools)]
        if req.resume:
            argv += ["--resume", req.resume]
        return argv

    def build_env(self, req: TurnRequest) -> dict[str, str]:
        env = dict(os.environ)
        env.update(self.env)
        if self.tmpdir is not None:
            env["TMPDIR"] = str(self.tmpdir)
        env.update(req.env_extra)
        return scrub_env(env)

    # ---------------------------------------------------------------- run_turn
    def run_turn(self, req: TurnRequest, cancel: CancelToken | None = None) -> TurnResult:
        if cancel is not None and cancel.cancelled:
            raise TurnCancelled(cancel.cause or "cancelled")
        argv = self.build_argv(req)
        env = self.build_env(req)
        if self.tmpdir is not None:
            self.tmpdir.mkdir(parents=True, exist_ok=True)
        req.transcript.parent.mkdir(parents=True, exist_ok=True)
        try:
            proc = subprocess.Popen(
                argv, cwd=str(req.cwd), env=env, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                stderr=subprocess.PIPE, start_new_session=True, shell=False)
        except (FileNotFoundError, PermissionError) as exc:
            raise LlmUnavailable(f"cannot start the claude client: {exc}") from exc

        done = threading.Event()
        killed_cause: list[str] = []
        stderr_tail: deque[str] = deque(maxlen=20)

        def feed_stdin() -> None:
            try:
                proc.stdin.write(req.prompt.encode("utf-8"))
                proc.stdin.close()
            except (BrokenPipeError, OSError, ValueError):
                pass

        def drain_stderr() -> None:
            for raw in iter(proc.stderr.readline, b""):
                stderr_tail.append(raw.decode("utf-8", "replace").rstrip())

        def watch() -> None:
            deadline = None if req.timeout_s is None else time.monotonic() + req.timeout_s
            while not done.is_set():
                if cancel is not None and cancel.wait(_POLL_S):
                    killed_cause.append(cancel.cause or "cancelled")
                    break
                if cancel is None:
                    done.wait(_POLL_S)
                if deadline is not None and time.monotonic() >= deadline:
                    killed_cause.append("timeout")
                    break
            if killed_cause and not done.is_set():
                _kill_group(proc)

        threads = [threading.Thread(target=t, daemon=True) for t in (feed_stdin, drain_stderr, watch)]
        for t in threads:
            t.start()

        acc = TurnAccumulator(sink=req.events)
        try:
            with open(req.transcript, "ab") as tf:
                for ev in iter_ndjson(proc.stdout, config.NDJSON_LINE_CAP):
                    tf.write((json.dumps(ev, ensure_ascii=False) + "\n").encode("utf-8"))
                    tf.flush()
                    acc.feed(ev)
                tf.flush()
                os.fsync(tf.fileno())
            proc.wait()
        finally:
            done.set()
            if proc.poll() is None:
                _kill_group(proc)
            for t in threads:
                t.join(timeout=_KILL_GRACE_S * 2)
            for stream in (proc.stdout, proc.stderr):
                try:
                    stream.close()
                except OSError:
                    pass

        if killed_cause:
            raise TurnCancelled(killed_cause[0])
        tail = "\n".join(stderr_tail)
        return self._result(acc, proc.returncode, argv, tail)

    @staticmethod
    def _result(acc: TurnAccumulator, exit_code: int, argv: list[str], stderr_tail: str) -> TurnResult:
        if acc.is_error:
            reason = acc.text or "claude reported an error"
            if _UNAVAILABLE_RE.search(reason):
                raise LlmUnavailable(reason)
            raise ClaudeTurnError(reason)
        if acc.result is None or exit_code != 0:
            what = "no result event" if acc.result is None else f"exit status {exit_code}"
            raise LlmUnavailable(f"claude client failed ({what}): {stderr_tail or 'no stderr'}")
        return TurnResult(session_id=acc.session_id, text=acc.text, usages=tuple(acc.usages),
                          compactions=tuple(acc.compactions), exit_code=exit_code, argv=tuple(argv))
