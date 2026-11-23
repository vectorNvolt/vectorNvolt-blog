"""Execution backends for procedure steps (``ports.ExecBackend``).

Covers: SFR-AGG-014 (the command runs as an argument list, never through a shell, in the
agent sandbox), SSR-AGG-013 (no shell), SFR-AGG-016 (a cancel or timeout stops the child),
decision 15 (resolved by USS-005 / USC-005: ``isolate: true`` runs the step command in an
ephemeral execution container through the Go tool ``run_ephemeral_container``; ContainerExec).
"""

import json
import os
import re
import shutil
import signal
import subprocess
import threading
import time
from pathlib import Path
from typing import Mapping, Sequence

from ..errors import TurnCancelled
from ..ports import CancelToken, ExecResult

TAIL_CHARS = 200_000
_POLL_S = 0.05
TOOL_NAME = "run_ephemeral_container"
DEFAULT_TOOL = "/usr/local/bin/run_ephemeral_container"
ENV_TOOL = "AGENT_GRAPH_EXEC_TOOL"
LABEL_RUN = "agent-graph.run"
LABEL_NODE = "agent-graph.node"
GRACE_S = 10.0
SWEEP_TIMEOUT_S = 60
_EXEC_DIR_LINE = re.compile(rf"^{TOOL_NAME}: exec_dir=(.+)$")
_EARLY_CAUSE = re.compile(rf"^{TOOL_NAME}: (usage|setup|denied|verify|busy)\b", re.MULTILINE)


class _Tail:
    """Keeps the last TAIL_CHARS characters of a text stream."""

    def __init__(self) -> None:
        self.parts: list[str] = []
        self.size = 0

    def add(self, text: str) -> None:
        self.parts.append(text)
        self.size += len(text)
        while self.size > TAIL_CHARS and len(self.parts) > 1:
            self.size -= len(self.parts.pop(0))

    def text(self) -> str:
        return "".join(self.parts)[-TAIL_CHARS:]


def _drain(pipe, tail: _Tail) -> None:
    try:
        for line in iter(pipe.readline, b""):
            tail.add(line.decode("utf-8", errors="replace"))
    finally:
        pipe.close()


def _kill_group(proc: subprocess.Popen) -> None:
    try:
        os.killpg(proc.pid, signal.SIGKILL)
    except (ProcessLookupError, PermissionError):
        try:
            proc.kill()
        except ProcessLookupError:
            pass


class SandboxExec:
    """Runs the argument list as a child of the runtime (the agent sandbox is the container)."""

    def run(self, argv: Sequence[str], cwd: Path, env: Mapping[str, str],
            timeout_s: "float | None", cancel: CancelToken) -> ExecResult:
        if not argv:
            raise ValueError("empty argument list")
        proc = subprocess.Popen(
            list(argv), cwd=str(cwd), env=dict(env), shell=False, stdin=subprocess.DEVNULL,
            stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True)
        out, err = _Tail(), _Tail()
        readers = [threading.Thread(target=_drain, args=(proc.stdout, out), daemon=True),
                   threading.Thread(target=_drain, args=(proc.stderr, err), daemon=True)]
        for t in readers:
            t.start()
        waited = 0.0
        cause: "str | None" = None
        while True:
            if proc.poll() is not None:
                break
            if cancel.wait(_POLL_S):
                cause = cancel.cause or "cancelled"
                break
            waited += _POLL_S
            if timeout_s is not None and waited >= timeout_s:
                cause = "timeout"
                break
        if cause is not None:
            _kill_group(proc)
            proc.wait()
        for t in readers:
            t.join(timeout=5)
        if cause is not None:
            raise TurnCancelled(cause)
        return ExecResult(exit_code=proc.returncode, stdout_tail=out.text(), stderr_tail=err.text())


class _ErrTail(_Tail):
    """Stderr tail that remembers the tool's first line ``run_ephemeral_container: exec_dir=<path>``."""

    def __init__(self) -> None:
        super().__init__()
        self.exec_dir: "str | None" = None

    def add(self, text: str) -> None:
        if self.exec_dir is None:
            m = _EXEC_DIR_LINE.match(text.rstrip("\r\n"))
            if m:
                self.exec_dir = m.group(1).strip()
        super().add(text)


def resolve_tool(tool: "str | None" = None) -> "str | None":
    """Path of run_ephemeral_container: argument, ``AGENT_GRAPH_EXEC_TOOL``, PATH, default location.
    The first candidate that is an executable file wins; None when there is none."""
    for cand in (tool, os.environ.get(ENV_TOOL), shutil.which(TOOL_NAME), DEFAULT_TOOL):
        if cand and os.path.isfile(cand) and os.access(cand, os.X_OK):
            return cand
    return None


def _read_result(exec_dir: "str | None") -> "dict | None":
    if not exec_dir:
        return None
    try:
        data = json.loads((Path(exec_dir) / "result.json").read_text(encoding="utf-8"))
    except (OSError, ValueError):
        return None
    return data if isinstance(data, dict) else None


class ContainerExec:
    """Backend of ``isolate: true``: one ephemeral execution container per call, through the Go
    tool ``run_ephemeral_container run <profile> ... -- <cmd>`` (USS-005, USC-005).

    ``run`` (plain argv without a profile) is not supported; the step calls ``run_isolated``.
    The tool's ``result.json`` is authoritative for the exit code and the cause. A cancel or a
    timeout sends SIGTERM to the tool (it kills and removes the container, writes cause
    ``cancel`` and exits 130), waits ``grace_s``, then kills the tool. ``timeout_s`` is enforced
    here, not passed as ``--timeout`` (the profile ceiling is unknown to agent-graph).
    """

    def __init__(self, tool: "str | None" = None, *, grace_s: float = GRACE_S) -> None:
        self.tool = tool
        self.grace_s = grace_s

    def run(self, argv: Sequence[str], cwd: Path, env: Mapping[str, str],
            timeout_s: "float | None", cancel: CancelToken) -> ExecResult:
        raise NotImplementedError("ContainerExec needs an exec profile: call run_isolated(profile, ...); "
                                  "a plain argument list without a profile is not supported")

    def run_isolated(self, profile: str, argv: Sequence[str], inputs: Sequence[tuple[str, str]],
                     labels: Mapping[str, str], exec_id: str, timeout_s: "float | None",
                     cancel: CancelToken) -> ExecResult:
        if not argv:
            raise ValueError("empty argument list")
        tool = resolve_tool(self.tool)
        if tool is None:
            raise FileNotFoundError(f"{TOOL_NAME} not found (set {ENV_TOOL}, or put it on PATH)")
        cmd = [tool, "run", profile, "--exec-id", exec_id]
        for name, path in inputs:
            cmd += ["--in", f"{name}={path}"]
        for k, v in labels.items():
            cmd += ["--label", f"{k}={v}"]
        cmd += ["--", *argv]
        proc = subprocess.Popen(cmd, shell=False, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                                stderr=subprocess.PIPE, start_new_session=True)
        out, err = _Tail(), _ErrTail()
        readers = [threading.Thread(target=_drain, args=(proc.stdout, out), daemon=True),
                   threading.Thread(target=_drain, args=(proc.stderr, err), daemon=True)]
        for t in readers:
            t.start()
        started = time.monotonic()
        cause: "str | None" = None
        while True:
            if proc.poll() is not None:
                break
            if cancel.wait(_POLL_S):
                cause = cancel.cause or "cancelled"
                break
            if timeout_s is not None and time.monotonic() - started >= timeout_s:
                cause = "timeout"
                break
        if cause is not None:
            try:
                proc.terminate()                 # the tool, not the group: it removes the container
            except ProcessLookupError:
                pass
            try:
                proc.wait(timeout=self.grace_s)
            except subprocess.TimeoutExpired:
                _kill_group(proc)
                proc.wait()
        for t in readers:
            t.join(timeout=5)
        if cause is not None:
            raise TurnCancelled(cause)
        return self._result(proc.returncode, out.text(), err)

    @staticmethod
    def _result(rc: int, stdout: str, err: _ErrTail) -> ExecResult:
        stderr = err.text()
        res = _read_result(err.exec_dir)
        if res is not None:
            tool_cause = str(res.get("cause") or "") or None
            code = res.get("exit")
            code = code if isinstance(code, int) and not isinstance(code, bool) else rc
        else:
            m = _EARLY_CAUSE.search(stderr)
            tool_cause = m.group(1) if m else ("setup" if rc != 0 else None)
            code = rc
        if tool_cause not in (None, "ok", "exit"):
            stderr = f"{TOOL_NAME}: cause={tool_cause}\n{stderr}"
            if code == 0:
                code = rc or 1
        return ExecResult(exit_code=code, stdout_tail=stdout, stderr_tail=stderr[-TAIL_CHARS:],
                          cause=tool_cause, exec_dir=err.exec_dir)

    def sweep(self, run_id: str) -> None:
        """Remove leftover containers of this run (``sweep --label agent-graph.run=<id>``). Best effort."""
        try:
            tool = resolve_tool(self.tool)
            if tool is None:
                return
            subprocess.run([tool, "sweep", "--label", f"{LABEL_RUN}={run_id}"], shell=False,
                           stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL,
                           stderr=subprocess.DEVNULL, timeout=SWEEP_TIMEOUT_S, check=False)
        except Exception:  # noqa: BLE001 - a failed sweep never stops a run
            pass
