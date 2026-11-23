"""Procedure step: runs ``run`` as an argument list, without a language model.

Covers: SFR-AGG-014 (no shell, work folder, input and product placeholders, products in
work/<node>/, isolate: true goes to the container backend), SFR-AGG-015, SFR-AGG-013 (success
and warning notices through the gateway, via the engine), SSR-AGG-007 (cwd work/<node>),
SSR-AGG-013 (no shell: ``shell=False`` in the backend, the argv comes from the validated process;
the script path of an interpreter command is resolved against the workspace root, since the
command runs in work/<node>).

Result protocol: the outcome is read from ``<work>/.outcome``; every stdout line that starts
with ``WARNING:`` becomes a notice.

Isolated steps (SFR-AGG-014, USC-005; ``isolate: true`` + ``profile``): the command runs in an
ephemeral execution container through ``ContainerExec.run_isolated``. ``{work}`` and own products
point into the exec's ``out/`` folder (``$OUT`` in the container); inputs and previous products
are staged by the tool (``{in.<name>}``). After the run ``out/`` is copied into ``<work>``, so
the command writes ``$OUT/.outcome`` and its products to ``$OUT``.
"""

import os
import re
import secrets
import shutil
from pathlib import Path

from ..errors import TurnCancelled
from ..process.validate import resolve_script, tool_input_name
from ..services.container import LABEL_NODE, LABEL_RUN, ContainerExec
from ..state import StepDef
from .journal import NodeRun, StepFailure, cancel_failure, run_attempts
from .products import check_products, make_refs, print_texts, product_file, stage_outputs

_TOKEN = re.compile(r"\{(work|input\.[^{}\s]+|[A-Za-z_][\w-]*)\}")
_ENV_KEEP = ("PATH", "HOME", "LANG", "LC_ALL", "VIRTUAL_ENV", "PYTHONPATH")
_STDOUT_KEEP = 20_000
_STDERR_KEEP = 4_000
_NO_RETRY_CAUSES = frozenset({"usage", "setup", "denied", "verify"})    # USC-005: retrying cannot help


def _bad(msg: str) -> StepFailure:
    return StepFailure("tool", msg, retryable=False)


def _values(key: str, step: StepDef, state: dict, ctx, work) -> "list[str] | None":
    """Values of a placeholder; None when ``key`` names nothing (the text then stays literal, so
    braces in program text such as a Python dict literal are not touched)."""
    ws = ctx.paths.ws
    if key == "work":
        return [str(work)]
    if key.startswith("input."):
        name = key[len("input."):]
        b = ctx.binding
        if name in b.sources:
            return [str(ws.root / f.path) for f in b.sources[name].files]
        if name in b.templates:
            return [str(ws.root / b.templates[name].path)]
        if name in b.rulesets:
            return [str(ws.root / b.rulesets[name].path)]
        raise _bad(f"placeholder {{{key}}}: no such bound input")
    for decl in step.produces:
        if decl.name == key:
            return [str(work / product_file(ctx.process, decl))]
    ref = (state.get("products") or {}).get(key)
    if ref:
        return [str(ws.root / ref["path"])]
    return None


def expand_argv(step: StepDef, state: dict, ctx, work) -> "list[str]":
    """A placeholder that is a whole argument expands to N arguments; an embedded one needs one value."""
    out: list[str] = []
    for tok in step.run or ():
        whole = _TOKEN.fullmatch(tok)
        if whole:
            vals = _values(whole.group(1), step, state, ctx, work)
            out.extend([tok] if vals is None else vals)
            continue

        def sub(m):
            vals = _values(m.group(1), step, state, ctx, work)
            if vals is None:
                return m.group(0)
            if len(vals) != 1:
                raise _bad(f"placeholder {{{m.group(1)}}} embedded in an argument needs exactly one file, got {len(vals)}")
            return vals[0]
        out.append(_TOKEN.sub(sub, tok))
    if not out:
        raise _bad("the step has no command")
    return out


def make_exec_id(node: str, visit: int, attempt: int) -> str:
    """``ag-<node>-<visit>-<attempt>-<8 hex>``, within the tool's ``[a-z0-9][a-z0-9-]{0,63}``."""
    base = re.sub(r"[^a-z0-9-]+", "-", node.lower()).strip("-") or "n"
    suffix = f"-{visit}-{attempt}-{secrets.token_hex(4)}"
    return ("ag-" + base)[: 64 - len(suffix)].rstrip("-") + suffix


def label_value(text: str) -> str:
    """A label-safe value (letters, digits, ``._-``)."""
    return re.sub(r"[^A-Za-z0-9._-]+", "-", text)[:63] or "x"


def _is_input(key: str, step: StepDef) -> bool:
    """True for a placeholder that names something staged INTO the container: a bound input or a
    previous step's product. ``{work}`` and the step's own products live in ``out/``."""
    if key == "work":
        return False
    return key.startswith("input.") or not any(d.name == key for d in step.produces)


def expand_argv_isolated(step: StepDef, state: dict, ctx, exec_id: str) -> "tuple[list[str], list[tuple[str, str]]]":
    """(argv for the tool's ``-- <cmd>``, ``--in`` pairs) of an ``isolate: true`` step (USC-005).

    ``{work}`` and the step's own product names become paths in the exec's ``out/`` folder
    (``<ws>/.output/exec/<exec_id>/out``, which the container sees as ``$OUT``). A whole-argument
    input or previous-product placeholder becomes the tool's ``{in.<name>}`` plus one ``--in``
    per file; embedded in a longer argument it is substituted only when the file is under ``src/``
    (visible in the container in place)."""
    ws_root = ctx.paths.ws.root
    out_dir = ws_root / ".output" / "exec" / exec_id / "out"
    src_dir = Path(os.path.realpath(ws_root / "src"))
    inputs: list[tuple[str, str]] = []
    staged: dict[str, list[str]] = {}

    def tool_name(key: str) -> str:
        raw = key[len("input."):] if key.startswith("input.") else key
        n = tool_input_name(raw)
        if n is None:
            raise _bad(f"placeholder {{{key}}}: {raw!r} is not a valid input name for the execution container")
        return n

    out: list[str] = []
    for tok in step.run or ():
        whole = _TOKEN.fullmatch(tok)
        if whole:
            key = whole.group(1)
            vals = _values(key, step, state, ctx, out_dir)
            if vals is None:
                out.append(tok)
            elif _is_input(key, step):
                n = tool_name(key)
                if n in staged and staged[n] != vals:
                    raise _bad(f"placeholder {{{key}}}: input name {n!r} is used for different files")
                if n not in staged:
                    staged[n] = vals
                    inputs.extend((n, v) for v in vals)
                out.append(f"{{in.{n}}}")
            else:
                out.extend(vals)
            continue

        def sub(m):
            key = m.group(1)
            vals = _values(key, step, state, ctx, out_dir)
            if vals is None:
                return m.group(0)
            if len(vals) != 1:
                raise _bad(f"placeholder {{{key}}} embedded in an argument needs exactly one file, got {len(vals)}")
            if _is_input(key, step):
                real = Path(os.path.realpath(vals[0]))
                if src_dir not in real.parents:
                    raise _bad(f"placeholder {{{key}}} embedded in an argument is only supported for files "
                               "under src/ in an isolated step; pass it as a whole argument")
            return vals[0]
        out.append(_TOKEN.sub(sub, tok))
    if not out:
        raise _bad("the step has no command")
    return out, inputs


def copy_exec_out(src: Path, work: Path, guard) -> int:
    """Copy the files of the exec's ``out/`` into the step's work folder (overwriting; symlinks skipped)."""
    n = 0
    if not src.is_dir():
        return 0
    for dirpath, dirnames, filenames in os.walk(src, followlinks=False):
        dirnames[:] = [d for d in dirnames if not os.path.islink(os.path.join(dirpath, d))]
        dest_dir = work / Path(dirpath).relative_to(src)
        guard.check(dest_dir)
        dest_dir.mkdir(parents=True, exist_ok=True)
        for name in filenames:
            sp = Path(dirpath) / name
            if sp.is_symlink() or not sp.is_file():
                continue
            dp = dest_dir / name
            guard.check(dp)
            if dp.is_symlink():
                dp.unlink()
            shutil.copyfile(sp, dp)
            n += 1
    return n


def run_procedure(step: StepDef, state: dict, ctx) -> dict:
    node = step.name
    visit = (state.get("visits") or {}).get(node, 0) + 1
    run = NodeRun(ctx, node, visit)
    paths, proc = ctx.paths, ctx.process
    guard = ctx.facts.guard
    work = paths.work_node(node)
    guard.check(work)
    ctx_ref = paths.ws.rel(paths.node_dir(node))

    def body(attempt: int, _reason: "str | None") -> dict:
        key = f"a{attempt}"
        if attempt == 1:
            run.begin(ctx_ref)
        work.mkdir(parents=True, exist_ok=True)
        paths.tmp_dir.mkdir(parents=True, exist_ok=True)

        def do_exec() -> dict:
            token = ctx.gate.attempt_token(node)
            if step.isolate:
                backend = getattr(ctx, "container", None) or ContainerExec()
                if not step.profile:
                    raise _bad("isolate: true needs a profile")
                if not hasattr(backend, "run_isolated"):
                    raise _bad("the container backend does not support isolated execution")
                exec_id = make_exec_id(node, visit, attempt)
                argv, inputs = expand_argv_isolated(step, state, ctx, exec_id)
                argv = resolve_script(argv, paths.ws.root)
                (work / ".outcome").unlink(missing_ok=True)
                labels = {LABEL_RUN: label_value(paths.run_id), LABEL_NODE: label_value(node)}
                try:
                    res = backend.run_isolated(step.profile, argv, inputs, labels, exec_id,
                                               proc.timeout_of(step), token)
                except TurnCancelled as c:
                    raise cancel_failure(c.cause) from c
                if res.exec_dir:
                    copy_exec_out(Path(res.exec_dir) / "out", work, guard)
            else:
                argv = resolve_script(expand_argv(step, state, ctx, work), paths.ws.root)
                (work / ".outcome").unlink(missing_ok=True)
                env = {k: os.environ[k] for k in _ENV_KEEP if k in os.environ}
                env.update({"AGENT_GRAPH_WORK": str(work), "TMPDIR": str(paths.tmp_dir)})
                try:
                    res = ctx.exec_backend.run(argv, work, env, proc.timeout_of(step), token)
                except TurnCancelled as c:
                    raise cancel_failure(c.cause) from c
            outcome_file = work / ".outcome"
            outcome = outcome_file.read_text(encoding="utf-8").strip() if outcome_file.is_file() else None
            return {"exit_code": res.exit_code, "stdout": res.stdout_tail[-_STDOUT_KEEP:],
                    "stderr": res.stderr_tail[-_STDERR_KEEP:], "outcome": outcome,
                    **({"cause": res.cause} if res.cause else {})}

        res = run.once(f"{key}.exec", do_exec)
        warnings = [ln[len("WARNING:"):].strip() for ln in res["stdout"].splitlines()
                    if ln.startswith("WARNING:")]
        if warnings:
            run.once(f"{key}.warn", lambda: ([run.notice("WARNING", w) for w in warnings], {"n": len(warnings)})[1])
        if res["exit_code"] != 0:
            tail = res["stderr"].strip()[-500:]
            cause = res.get("cause")
            note = f" (cause {cause})" if cause else ""
            raise StepFailure("tool", f"command exited with status {res['exit_code']}{note}: {tail}",
                              retryable=cause not in _NO_RETRY_CAUSES)
        outcome = res["outcome"]
        if step.outcomes and outcome not in step.outcomes:
            where = "$OUT" if step.isolate else "$AGENT_GRAPH_WORK"
            raise StepFailure("schema", f"outcome {outcome!r} is not one of {list(step.outcomes)} "
                                        f"(the command must write it to {where}/.outcome)")
        problems = check_products(proc, step, work, paths.ws.root)
        if problems:
            raise StepFailure("schema", "; ".join(problems))
        run.once(f"{key}.stage", lambda: {"copied": stage_outputs(paths, proc, step, work, guard)})
        refs = make_refs(paths, proc, step, work)
        for i, text in enumerate(print_texts(step.success)):
            run.notice_once(f"{key}.ok{i}", "PROGRESS", text)
        return run.done(outcome=outcome if step.outcomes else None, refs=refs,
                        summary=f"{node}: done" + (f" ({outcome})" if outcome else ""))

    def on_fail(attempt: int, f: StepFailure) -> None:
        for i, text in enumerate(print_texts(step.failure, f.reason)):
            run.notice_once(f"a{attempt}.failnote{i}", "PROGRESS", text)

    return run_attempts(run, body, on_fail)
