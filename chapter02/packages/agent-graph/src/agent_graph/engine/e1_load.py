"""E1 load: builds the dynamic machine and chooses the first command.

Covers: SFR-AGG-012 (loads the compiled graph, the checkpoint and the run overrides; sends START,
or RESTART(node) on a re-entry or resume; the dynamic machine runs in its own checkpoint thread),
SFR-AGG-034 (RESTART(node) after a fix, the resolution summary becomes a run fact), SFR-AGG-037
(resume: RESTART at the node after the last completed one, continuing an interrupted node's session;
a completed procedure step or a node with complete products is never repeated),
SSR-AGG-004 (the machine is driven only by commands).

Thread handling: ``start`` uses ``record.dynamic_thread``; ``restart`` and ``resume`` use the next
thread (dyn-1 -> dyn-2) and carry the finished state of the old one over (DynamicMachine carry_from).
"""

import re

from ..contract import Cmd, Status, restart, start
from ..dynamic.builder import compile_dynamic
from ..dynamic.driver import DynamicMachine
from ..dynamic.journal import NodeContext, NodeJournal
from ..dynamic.router import next_after
from ..ports import Launch
from ..services.container import ContainerExec, SandboxExec
from .budgets import EngineRuntime


def bump_thread(name: str) -> str:
    m = re.fullmatch(r"(.*?)(\d+)", name)
    return f"{m.group(1)}{int(m.group(2)) + 1}" if m else f"{name}-2"


def _summarizer(rt: EngineRuntime, launch: Launch):
    ctx = rt.ctx
    if ctx.summarizer is not None:
        return ctx.summarizer
    from ..services.summarize import SummaryCache               # WP3
    return SummaryCache(ctx.paths.ws, ctx.claude, launch, rt.budgets, rt.guard)


def build_machine(rt: EngineRuntime, thread: str, carry_from: "str | None") -> DynamicMachine:
    ctx = rt.ctx
    rec = ctx.record
    launch = ctx.launch or Launch(model=rec.model, effort=rec.effort, tools=tuple(rt.proc.runtime.tools))
    graph = ctx.graph if ctx.graph is not None else compile_dynamic(rt.proc, ctx.checkpointer)
    rt.journal = NodeJournal(ctx.paths.checkpoint_db)
    container = ctx.container or ContainerExec()
    if any(s.isolate for s in rt.proc.steps) and hasattr(container, "sweep"):
        try:                                    # USC-005 alt-4: containers a dead earlier process left behind
            container.sweep(rec.run_id)
        except Exception:  # noqa: BLE001 - best effort, never stops a run
            pass
    dctx = NodeContext(
        paths=ctx.paths, process=rt.proc, index=ctx.index, binding=ctx.binding, launch=launch,
        claude=ctx.claude, summarizer=_summarizer(rt, launch),
        exec_backend=ctx.exec_backend or SandboxExec(), gate=rt.budgets, journal=rt.journal, facts=ctx.facts,
        compose=ctx.compose, record_turn=ctx.record_turn, record_compaction=ctx.record_compaction,
        container=container, gateway=ctx.gateway)
    machine = DynamicMachine(graph, thread, dctx, ctx.history, carry_from=carry_from)
    rt.budgets.bind(machine.cancel)
    return machine


def e1_load(rt: EngineRuntime, state: dict) -> dict:
    ctx = rt.ctx
    rec = ctx.record
    mode = state.get("mode") or ctx.mode
    if mode == "start":
        thread, carry = rec.dynamic_thread, None
    else:
        carry = rec.dynamic_thread
        thread = bump_thread(carry)
        rec.dynamic_thread = thread
    rt.machine = build_machine(rt, thread, carry)

    pending = None
    if mode == "start":
        cmd: Cmd = start()
    elif mode == "restart":
        node = state.get("restart_node") or ctx.restart_node
        if not node:
            raise ValueError("mode restart needs restart_node")
        summary = (state.get("restart_summary") or ctx.restart_summary or "").strip()
        if summary:
            ctx.facts.append("resolution", node, summary)
        rec.counters.failures.pop(node, None)
        rec.counters.attempts.pop(node, None)
        cmd = restart(node, summary, False)
    elif mode == "resume":
        values = rt.machine.values(carry)
        nxt = next_after(rt.proc, values)
        if nxt is None:
            cmd = start()
        elif nxt == "END":
            cmd = None
            pending = Status("COMPLETED", None, {"output_ref": ctx.paths.ws.rel(ctx.paths.output_dir)})
        else:
            cmd = restart(nxt, "", True)
    else:
        raise ValueError(f"unknown engine mode {mode!r}")

    rec.phase = "S6"
    rec.status = "ACTIVE"
    rec.open_error = None
    rt.persist()
    return {"next_cmd": cmd.to_json() if cmd else None,
            "pending": pending.to_json() if pending else None, "escalation": None}
