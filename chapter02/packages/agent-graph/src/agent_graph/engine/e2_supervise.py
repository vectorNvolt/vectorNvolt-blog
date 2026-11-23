"""E2 supervise: sends the next command, watches the dynamic machine until it pauses.

Covers: SFR-AGG-013 (NODE_STARTED / NODE_DONE / notices reach the engine; notices are printed only
through the gateway), SFR-AGG-016 (per-attempt timer armed at NODE_STARTED and when a RETRY or
ANSWER is sent, cancelling the attempt with CANCEL(node); the run-time clock runs only while E2
runs; steps and model calls are counted by the gate), SFR-AGG-021 and SSR-AGG-018 (P_run, drift
notice), SFR-AGG-012 (counters persisted in run.json).
"""

import time

from ..contract import Cmd, Notice, Status
from .budgets import EngineRuntime
from .cache_metrics import on_node_done


def e2_supervise(rt: EngineRuntime, state: dict) -> dict:
    ctx = rt.ctx
    gw = ctx.gateway
    proc = rt.proc
    cmd = Cmd.from_json(state["next_cmd"])
    pending: "Status | None" = None
    rt.budgets.resume_clock()
    try:
        if cmd.name in ("RETRY", "ANSWER") and cmd.node:
            rt.budgets.arm_attempt(cmd.node, proc.timeout_of(proc.step(cmd.node)))
        for item in rt.machine.send(cmd):
            if isinstance(item, Notice):
                gw.notice(item.source, item.code, item.text)
                continue
            node = item.node
            if item.name == "NODE_STARTED" and node:
                rt.current_node = node
                step = proc.step(node)
                if step.type == "human":
                    rt.human_started[node] = time.monotonic()
                else:
                    rt.budgets.arm_attempt(node, proc.timeout_of(step))
                rt.persist()
            elif item.name == "NODE_DONE" and node:
                rt.budgets.disarm_attempt()
                rt.counters.failures.pop(node, None)
                rt.counters.attempts.pop(node, None)
                drift = on_node_done(rt.counters, node, item.payload.get("usage"))
                if drift:
                    n = Notice(f"node {node}", "CACHE_PREFIX_DRIFT", drift)
                    gw.notice(n.source, n.code, n.text)
                    ctx.history.notice(n)
                rt.persist()
            elif item.name in ("WAITING", "NODE_FAILED", "COMPLETED", "SUSPENDED"):
                rt.budgets.disarm_attempt()
                pending = item
    finally:
        rt.budgets.disarm_attempt()
        rt.budgets.pause_clock()
    if pending is None:
        raise RuntimeError("the dynamic machine ended without a pause status")
    rt.persist()
    return {"pending": pending.to_json(), "next_cmd": None}
