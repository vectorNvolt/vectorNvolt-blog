"""Cognitive step: one Claude session per node visit, composed context, result and handoff.

Covers: SFR-AGG-019 (session from active/, session id journaled, same session for later turns,
answers and retries, a repeat of the node is a new visit and so a new session, handoff note
required, active/ moved to nodes/<node>/ when done), SFR-AGG-017 (compose into active/),
SFR-AGG-020 (usage and compaction recorded per turn), SFR-AGG-022 (question -> WAITING INPUT
-> ANSWER as the next turn), SFR-AGG-026 (retry turn carries the reason), SFR-AGG-028
(ContextOverBudget -> NODE_FAILED(context), not retryable), SFR-AGG-029 (LlmUnavailable ->
NODE_FAILED cause llm_unavailable), SSR-AGG-009, SSR-AGG-007 (cwd active/).

Attempt structure (every side effect is journaled, see journal.py):
1 gate.before_node_start  2 NODE_STARTED  3 compose  4 turn loop  5 product check
6 stage output targets  7 move active/ to nodes/<node>/  8 NODE_DONE.
"""

import json
import shutil
from pathlib import Path
from types import SimpleNamespace

from ..contract import Status
from ..errors import ClaudeTurnError, ContextOverBudget, LlmUnavailable, TurnCancelled
from ..ports import TurnRequest
from ..prompts.node_turns import answer_turn, first_turn, retry_turn
from ..services.runfs import canonical_json, write_atomic
from ..services.yaml_schema import load_schema, validate
from ..state import ProductRef, StepDef
from .journal import NodeRun, StepFailure, cancel_failure, run_attempts
from .products import check_products, make_refs, print_texts, stage_outputs

_RESULT_SCHEMA: "dict | None" = None


def _result_schema() -> dict:
    global _RESULT_SCHEMA
    if _RESULT_SCHEMA is None:
        _RESULT_SCHEMA = load_schema("node_result")
    return _RESULT_SCHEMA


def _read_result(active: Path) -> "tuple[dict | None, str | None]":
    p = active / "result.json"
    if not p.is_file():
        return None, "result.json was not written in the working directory"
    try:
        obj = json.loads(p.read_text(encoding="utf-8"))
    except ValueError as exc:
        return None, f"result.json is not valid JSON: {exc}"
    errs = validate(obj, _result_schema())
    if errs:
        where = "/".join(str(x) for x in errs[0][0]) or "(root)"
        return None, f"result.json does not match its schema at {where}: {errs[0][1]}"
    return obj, None


def _compose_fn(ctx):
    fn = getattr(ctx, "compose", None)
    if fn is not None:
        return fn, SimpleNamespace
    from ..compose import ComposeInputs, compose_context      # WP3
    return compose_context, ComposeInputs


def _record_turn_fn(ctx):
    rec = getattr(ctx, "record_turn", None)
    if rec is not None or getattr(ctx, "compose", None) is not None:
        return rec
    from ..compose import record_turn                          # WP3
    return record_turn


def _record_compaction_fn(ctx):
    rec = getattr(ctx, "record_compaction", None)
    if rec is not None or getattr(ctx, "compose", None) is not None:
        return rec
    from ..compose import record_compaction                    # WP3
    return record_compaction


def run_cognitive(step: StepDef, state: dict, ctx) -> dict:
    node = step.name
    visit = (state.get("visits") or {}).get(node, 0) + 1
    run = NodeRun(ctx, node, visit)
    paths, proc, ws = ctx.paths, ctx.process, ctx.paths.ws
    guard = ctx.facts.guard
    active = paths.active
    work = paths.work_node(node)
    guard.check(work)
    node_dir = paths.node_dir(node)
    ctx_ref = ws.rel(node_dir / "context.json")
    allowed = tuple(step.tools if step.tools is not None else proc.runtime.tools)
    work_rel = f"../work/{node}"

    def do_compose() -> dict:
        compose, make_inputs = _compose_fn(ctx)
        guard.check(active)
        if active.exists():
            shutil.rmtree(active)
        active.mkdir(parents=True, exist_ok=True)
        work.mkdir(parents=True, exist_ok=True)
        inp = make_inputs(
            process=proc, step=step, index=ctx.index, binding=ctx.binding,
            products={k: ProductRef(**v) for k, v in (state.get("products") or {}).items()},
            upstream_handoffs=[(n, paths.root / rel) for n, rel in (state.get("handoffs") or {}).items()],
            facts=ctx.facts.all(), ws=ws, active_dir=active, launch=ctx.launch)
        try:
            res = compose(inp, ctx.summarizer, guard)
        except ContextOverBudget as e:
            raise StepFailure("context", str(e), retryable=False,
                              extra={"budget": e.budget, "mandatory_tokens": e.mandatory_tokens}) from e
        return {"context_tokens": int(getattr(res, "context_tokens", 0) or 0)}

    def do_turn(prompt: str) -> dict:
        refusal = ctx.gate.before_llm_call(node)
        if refusal:
            raise StepFailure("tool", f"run budget reached: {refusal}", cause=f"cancelled:{refusal}",
                              retryable=False)
        token = ctx.gate.attempt_token(node)
        (active / "result.json").unlink(missing_ok=True)
        def request(events=None) -> TurnRequest:
            return TurnRequest(
                cwd=active, prompt=prompt, launch=ctx.launch, allowed_tools=allowed,
                resume=ctx.journal.session(node, visit), transcript=active / "transcript.jsonl",
                timeout_s=proc.timeout_of(step), env_extra={"TMPDIR": str(paths.tmp_dir)}, events=events)

        gw = getattr(ctx, "gateway", None)
        try:
            if gw is None:
                tr = ctx.claude.run_turn(request(), token)
            else:
                # The product is result.json plus files, so the streamed text is the model's own prose
                # for the developer: show it (show_text=True). Nothing says it again afterwards; the
                # journal keeps only a 2000-char copy. The turn is released before any WAITING pause.
                with gw.turn(f"node {node}", show_text=True) as sink:
                    tr = ctx.claude.run_turn(request(sink), token)
        except LlmUnavailable as e:
            raise StepFailure("tool", f"language model unavailable: {e.reason}", cause="llm_unavailable",
                              retryable=False) from e
        except ClaudeTurnError as e:
            raise StepFailure("tool", f"language-model turn failed: {e.reason}") from e
        except TurnCancelled as c:
            raise cancel_failure(c.cause) from c
        ctx.journal.set_session(node, visit, tr.session_id)
        usages = [u.to_json() for u in tr.usages]
        log = ctx.journal.get(node, visit, "usage") or {"turns": []}
        turn_no = len(log["turns"])
        log["turns"].append(usages)
        ctx.journal.put(node, visit, "usage", log)
        manifest = active / "context.json"
        rec_turn, rec_comp = _record_turn_fn(ctx), _record_compaction_fn(ctx)
        if rec_turn is not None:
            rec_turn(manifest, turn_no, list(tr.usages), guard)
        if rec_comp is not None:
            for ev in tr.compactions:
                rec_comp(manifest, ev, guard)
        result, err = _read_result(active)
        return {"session_id": tr.session_id, "text": tr.text[:2000], "turn": turn_no,
                "result": result, "result_error": err}

    def do_handoff(handoff: dict) -> dict:
        write_atomic(active / "handoff.json", canonical_json(handoff), guard=guard)
        return {"written": True}

    def do_move() -> dict:
        guard.check(node_dir)
        if active.exists():
            node_dir.parent.mkdir(parents=True, exist_ok=True)
            if node_dir.exists():                       # earlier visit: keep its record
                i = 1
                while (paths.nodes_dir / f"{node}.{i}").exists():
                    i += 1
                node_dir.rename(paths.nodes_dir / f"{node}.{i}")
            shutil.move(str(active), str(node_dir))
        return {"moved": True}

    def body(attempt: int, reason: "str | None") -> dict:
        key = f"a{attempt}"
        if attempt == 1:
            run.begin(ctx_ref)
        run.once("compose", do_compose)
        prompt = first_turn(node, work_rel, step.outcomes) if reason is None else retry_turn(reason)
        turn = 0
        while True:
            t = run.once(f"{key}.t{turn}", lambda: do_turn(prompt))
            if t["result_error"]:
                raise StepFailure("schema", t["result_error"])
            res = t["result"]
            if res["status"] == "question":
                question = (res.get("question") or "").strip()
                if not question:
                    raise StepFailure("schema", 'result.json has status "question" but no question')
                cmd = run.wait_for(Status("WAITING", node, {"mode": "INPUT", "question": question}), "ANSWER")
                prompt = answer_turn(str(cmd.payload.get("text", "")))
                turn += 1
                continue
            break
        for i, w in enumerate(res.get("warnings") or []):
            run.notice_once(f"{key}.warn{turn}.{i}", "WARNING", w)
        outcome = res.get("outcome")
        if step.outcomes:
            if outcome not in step.outcomes:
                raise StepFailure("schema", f"outcome {outcome!r} is not one of {list(step.outcomes)}")
        else:
            outcome = None
        handoff = res.get("handoff") or {}
        if not str(handoff.get("done", "")).strip():
            raise StepFailure("schema", "result.json needs a handoff note with a non-empty 'done'")
        problems = check_products(proc, step, work, ws.root)
        if problems:
            raise StepFailure("schema", "; ".join(problems))
        norm = {"done": handoff["done"], "decisions": list(handoff.get("decisions") or []),
                "open_points": list(handoff.get("open_points") or [])}
        run.once(f"{key}.handoff", lambda: do_handoff(norm))
        run.once(f"{key}.stage", lambda: {"copied": stage_outputs(paths, proc, step, work, guard)})
        refs = make_refs(paths, proc, step, work)
        run.once(f"{key}.move", do_move)
        for i, text in enumerate(print_texts(step.success)):
            run.notice_once(f"{key}.ok{i}", "PROGRESS", text)
        usage = ctx.journal.get(node, visit, "usage") or {"turns": []}
        first = usage["turns"][0][0] if usage["turns"] and usage["turns"][0] else {}
        return run.done(outcome=outcome, refs=refs, summary=norm["done"], handoff=f"nodes/{node}/handoff.json",
                        extra={"usage": {"first_turn": first, "turns": usage["turns"]}})

    def on_fail(attempt: int, f: StepFailure) -> None:
        for i, text in enumerate(print_texts(step.failure, f.reason)):
            run.notice_once(f"a{attempt}.failnote{i}", "PROGRESS", text)

    return run_attempts(run, body, on_fail)
