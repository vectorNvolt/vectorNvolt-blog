"""Human step: waits for the developer's decision on the items it reads.

Covers: SFR-AGG-023 (WAITING HIL with the brief and the items; HIL_OK follows the hil_ok edge),
SFR-AGG-024 (HIL_NOK(feedback) is written as the product ``<step>_feedback``, the hil_nok edge
leads back; the target step reads it as an optional read), decision 12 (no Claude in the review).
The conversation with the developer is E3's job (engine/e3_hil.py); this node only pauses.
"""

import dataclasses

from ..contract import Status
from ..services.runfs import InputReadGuard, canonical_json, sha256_file, write_atomic
from ..state import ProductRef, ReadRef, StepDef
from .journal import NodeRun, StepFailure, run_attempts
from .products import print_texts

_INLINE_MAX = 4000


def _read_entries(r: ReadRef, state: dict, ctx) -> "list[dict]":
    ws = ctx.paths.ws
    b = ctx.binding
    paths: list[str] = []
    if r.kind == "input.source" and r.name in b.sources:
        paths = [f.path for f in b.sources[r.name].files]
    elif r.kind == "input.ruleset" and r.name in b.rulesets:
        paths = [b.rulesets[r.name].path]
    elif r.kind == "input.output-template-ref" and r.name in b.templates:
        paths = [b.templates[r.name].path]
    else:
        products = state.get("products") or {}
        ref = products.get(r.name)
        if ref is None:                         # an output target read by its target name
            for step in ctx.process.steps:
                for decl in step.produces:
                    if r.name in (decl.name, decl.output_target) and decl.name in products:
                        ref = products[decl.name]
        if ref is not None:
            paths = [ref["path"]]
    if not paths:
        return [{"ref": r.raw, "path": None}]
    guard = InputReadGuard(ws, b)
    out = []
    for p in paths:
        entry = {"ref": r.raw, "path": p}
        try:
            text = guard.open_text(ws.root / p)
            entry["inline"] = text[:_INLINE_MAX]
        except (OSError, UnicodeDecodeError, ValueError):
            pass
        out.append(entry)
    return out


def run_human(step: StepDef, state: dict, ctx) -> dict:
    node = step.name
    visit = (state.get("visits") or {}).get(node, 0) + 1
    run = NodeRun(ctx, node, visit)
    paths = ctx.paths
    guard = ctx.facts.guard
    work = paths.work_node(node)
    guard.check(work)
    fb_name = f"{node}_feedback"

    def body(attempt: int, _reason: "str | None") -> dict:
        if attempt == 1:
            run.begin(paths.ws.rel(paths.node_dir(node)))
        reads = run.once("brief", lambda: {"reads": [e for r in step.reads for e in _read_entries(r, state, ctx)]})["reads"]
        cmd = run.wait_for(
            Status("WAITING", node, {"mode": "HIL", "brief": step.instruction.strip(), "reads": reads}),
            "HIL_OK", "HIL_NOK")
        refs: dict = {}
        if cmd.name == "HIL_OK":
            hil, summary = "hil_ok", f"{node}: approved"
        else:
            feedback = str(cmd.payload.get("feedback", ""))
            fb_file = work / f"{fb_name}.json"

            def write_feedback() -> dict:
                work.mkdir(parents=True, exist_ok=True)
                write_atomic(fb_file, canonical_json({"node": node, "feedback": feedback}), guard=guard)
                return {"written": True}
            run.once("feedback", write_feedback)
            refs[fb_name] = dataclasses.asdict(ProductRef(
                name=fb_name, node=node, path=paths.ws.rel(fb_file), sha256=sha256_file(fb_file)))
            hil, summary = "hil_nok", f"{node}: rejected: {feedback[:200]}"
        for i, text in enumerate(print_texts(step.success) if hil == "hil_ok" else ()):
            run.notice_once(f"ok{i}", "PROGRESS", text)
        return run.done(outcome=None, hil=hil, refs=refs, summary=summary)

    def on_fail(attempt: int, f: StepFailure) -> None:
        for i, text in enumerate(print_texts(step.failure, f.reason)):
            run.notice_once(f"a{attempt}.failnote{i}", "PROGRESS", text)

    return run_attempts(run, body, on_fail)
