"""Command-line entry: ``agent_graph run [--resume <runID>]`` (SFR-AGG-001, 036, 037, SSR-AGG-016).

Checks the invocation first (a usage error goes to stderr with exit status 1; nothing is created
and no gateway session is opened), then runs the static master S1-S8. Interruptions (SIGINT,
SIGTERM, SIGHUP, end of input on the terminal) suspend the run: the state is saved and the exit
status is ``config.EXIT_SUSPENDED``. This module and ``services/gateway.py`` / ``services/render.py``
are the only ones that may use the terminal streams.
"""

import argparse
import json
import re
import signal
import sys
from datetime import datetime
from pathlib import Path

from . import config
from .errors import ExitState, Interrupted, TerminalClosed, UsageError
from .graph import run_master
from .nodes import Env
from .services.claude_cli import ClaudeCli
from .services.gateway import Gateway
from .services.lock import ActiveRunLock
from .services.runfs import WorkspacePaths, WriteGuard

_RUN_ID = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]*$")


class _Parser(argparse.ArgumentParser):
    def error(self, message: str):                      # argparse would exit(2): the spec says 1
        raise UsageError(message)


def _parser() -> argparse.ArgumentParser:
    parser = _Parser(prog="agent_graph",
                     description="Run a developer-defined process as a supervised agent graph.")
    sub = parser.add_subparsers(dest="command", metavar="run")
    sub.required = True
    run = sub.add_parser("run", help="start a run (the process is agreed in the dialog)")
    run.add_argument("--resume", metavar="RUNID", help="continue a suspended or interrupted run")
    return parser


def _check_resume(ws: WorkspacePaths, run_id: str) -> None:
    """Read-only check that the named run exists, has a checkpoint and is not ended."""
    if not _RUN_ID.match(run_id):
        raise UsageError(f"not a run id: {run_id!r}")
    paths = ws.run(run_id)
    if not paths.has_checkpoint():
        raise UsageError(f"no run with a checkpoint named {run_id!r} in {ws.agent_dir}")
    try:
        status = json.loads(paths.run_json.read_text(encoding="utf-8")).get("status")
    except (OSError, ValueError, AttributeError) as exc:
        raise UsageError(f"run {run_id!r} has an unreadable run.json: {exc}") from exc
    if status == "ENDED":
        raise UsageError(f"run {run_id!r} has ended and cannot be resumed")


def _install_signals():
    def handler(signum, frame):
        raise Interrupted(signal.Signals(signum).name)

    saved = {}
    for name in ("SIGINT", "SIGTERM", "SIGHUP"):
        sig = getattr(signal, name, None)
        if sig is None:
            continue
        try:
            saved[sig] = signal.signal(sig, handler)
        except (ValueError, OSError):                      # not the main thread
            pass

    def restore() -> None:
        for sig, old in saved.items():
            try:
                signal.signal(sig, old)
            except (ValueError, OSError):
                pass
    return restore


def _suspend(env: Env, exc: Exception, stderr) -> int:
    rec = env.record
    if rec is not None and env.paths is not None:
        rec.status = "SUSPENDED"
        env.save_record()
    text = (f"suspended ({exc}); resume with: agent_graph run --resume {rec.run_id}" if rec is not None
            else f"suspended ({exc}); no run was created")
    try:
        env.gateway.say("S8 report", text)
    except Exception:                                      # the terminal may be gone
        print(text, file=stderr)
    return ExitState.SUSPENDED.exit_code


def main(argv: "list[str] | None" = None, *, gateway=None, claude=None, workspace=None,
         claude_dir=None, clock=None, stderr=None) -> int:
    """Run the command. The keyword arguments let tests inject the gateway, the Claude client,
    the workspace root, the Claude config folder and a clock; the CLI passes none."""
    stderr = stderr if stderr is not None else sys.stderr
    ws = WorkspacePaths(Path(workspace) if workspace is not None else config.WORKSPACE)
    try:
        args = _parser().parse_args(list(sys.argv[1:] if argv is None else argv))
        if args.resume:
            _check_resume(ws, args.resume)
    except UsageError as exc:
        print(f"agent_graph: {exc}", file=stderr)
        print("usage: agent_graph run [--resume RUNID]", file=stderr)
        return ExitState.NOT_STARTED.exit_code
    except SystemExit as exc:                              # --help
        return int(exc.code or 0)

    cdir = Path(claude_dir) if claude_dir is not None else config.CLAUDE_CONFIG_DIR
    env = Env(ws=ws, guard=WriteGuard(ws, cdir), gateway=gateway if gateway is not None else Gateway(),
              claude=claude if claude is not None else ClaudeCli(), lock=ActiveRunLock(ws),
              claude_dir=cdir, clock=clock if clock is not None else datetime.now)
    restore = _install_signals()
    try:
        final = run_master(env, args.resume)
        return ExitState(final["exit_state"]).exit_code
    except (Interrupted, TerminalClosed) as exc:
        return _suspend(env, exc, stderr)
    finally:
        restore()
        env.lock.release()
