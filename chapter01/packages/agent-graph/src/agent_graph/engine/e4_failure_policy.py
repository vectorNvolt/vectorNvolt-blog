"""E4 failure policy: retry or escalate a failed attempt.

Covers: SFR-AGG-026 (retry while the failed attempts are fewer than the threshold: step retry, else
runtime.retry, default 2; every attempt and reason is recorded in the history and the counters),
SFR-AGG-027 (retry exhaustion escalates with the reasons of each attempt), SFR-AGG-028 (context over
budget escalates at once), SFR-AGG-029 (language model unavailable escalates at once),
SFR-AGG-016 (a refused or cancelled run budget escalates as BUDGET_EXCEEDED with name, limit, count).
"""

from ..contract import Status, retry
from ..errors import ExitState, RunError
from .budgets import EngineRuntime


def _escalate(exit_state: ExitState, message: str, node: "str | None", detail: dict) -> dict:
    err = RunError(exit_state, "S6", message, node=node, detail=detail)
    return {"escalation": err.to_json(), "next_cmd": None, "pending": None}


def e4_failure_policy(rt: EngineRuntime, state: dict) -> dict:
    st = Status.from_json(state["pending"])
    node = st.node
    p = st.payload
    kind, reason = p.get("kind", "tool"), p.get("reason", "")
    cause = p.get("cause")
    counters = rt.counters
    base = {"kind": kind, "reason": reason, "cause": cause}

    if cause == "llm_unavailable":
        return _escalate(ExitState.LLM_UNAVAILABLE, f"language model unavailable at node {node}: {reason}",
                         node, base)
    if cause and str(cause).startswith("cancelled:budget"):
        breach = dict(rt.budgets.breach or {"budget": str(cause).split(":", 2)[-1]})
        return _escalate(ExitState.BUDGET_EXCEEDED,
                         f"run budget {breach.get('budget')} reached at node {node}",
                         node, {**base, **breach})
    if kind == "context":
        return _escalate(ExitState.CONTEXT_OVER_BUDGET,
                         f"node {node}: mandatory context does not fit its budget: {reason}", node,
                         {**base, "budget": p.get("budget"), "mandatory_tokens": p.get("mandatory_tokens")})

    counters.failures[node] = counters.failures.get(node, 0) + 1
    counters.attempts.setdefault(node, []).append(f"{kind}: {reason}")
    rt.persist()
    threshold = rt.proc.retry_of(rt.proc.step(node))
    failed = counters.failures[node]
    if p.get("retryable", True) and failed < threshold:
        return {"next_cmd": retry(node, reason).to_json(), "pending": None, "escalation": None}
    return _escalate(ExitState.NODE_FAILED,
                     f"node {node} failed {failed} time(s) (retry threshold {threshold}): {reason}", node,
                     {**base, "attempts": list(counters.attempts.get(node, [])),
                      "failed": failed, "retry_threshold": threshold, "retryable": p.get("retryable", True)})
