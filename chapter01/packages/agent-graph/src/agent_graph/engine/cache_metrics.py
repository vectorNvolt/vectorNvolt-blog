"""Prompt-cache metrics: P_run, drift notice, acceptance evaluation.

Covers: SFR-AGG-021 (CACHE_PREFIX_DRIFT when, from the second cognitive node on, the first-turn
cache read of a node is below 90 % of P_run, the first-turn cache read of the second cognitive
node; a notice, never an error), SSR-AGG-018 (the usage of every turn is recorded so that the
acceptance thresholds can be checked: ``evaluate``).

A cognitive NODE_DONE carries ``payload["usage"] = {"first_turn": <usage>, "turns": [[<usage>, ...], ...]}``
(a usage is ``ports.Usage.to_json()``; the first usage of the first turn is the first API call).
``EngineCounters.cognitive_started`` counts the cognitive nodes done that carried usage.
"""

from typing import Iterable, Mapping

from .. import config
from ..state import EngineCounters


def first_turn_cache_read(usage: "Mapping | None") -> "int | None":
    if not usage:
        return None
    first = usage.get("first_turn")
    if not first:
        return None
    return int(first.get("cache_read_input_tokens", 0))


def on_node_done(counters: EngineCounters, node: str, usage: "Mapping | None") -> "str | None":
    """Update P_run / the node count; returns the drift message when the node drifted."""
    reads = first_turn_cache_read(usage)
    if reads is None:
        return None
    counters.cognitive_started += 1
    if counters.cognitive_started == 2 and counters.p_run is None:
        counters.p_run = reads
        return None
    if counters.cognitive_started > 2 and counters.p_run:
        if reads < config.CACHE_DRIFT_RATIO * counters.p_run:
            msg = (f"node {node}: first-turn cache read {reads} is below "
                   f"{int(config.CACHE_DRIFT_RATIO * 100)} % of P_run {counters.p_run}")
            counters.drift.append(msg)
            return msg
    return None


def usage_from_history(records: Iterable[dict]) -> "list[dict]":
    """[{"node", "usage"}] of the cognitive NODE_DONE statuses in a history, in order."""
    out = []
    for r in records:
        if r.get("dir") == "status" and r.get("name") == "NODE_DONE" and (r.get("payload") or {}).get("usage"):
            out.append({"node": r.get("node"), "usage": r["payload"]["usage"]})
    return out


def evaluate(node_usages: "list[dict]") -> dict:
    """SSR-AGG-018 acceptance over [{"node", "usage"}] (see ``usage_from_history``).

    first_turn_ok: first-turn cache read of every cognitive node from the second on >= 90 % of P_run.
    within_node_ok: within a node the cache read of a turn >= 90 % of the previous turn
    (first usage of each turn). ephemeral_ok: ephemeral_1h > 0 and ephemeral_5m == 0 over all usages.
    """
    ratio = config.CACHE_DRIFT_RATIO
    violations: list[str] = []
    p_run = None
    first_ok = within_ok = True
    e1h = e5m = 0
    for i, item in enumerate(node_usages):
        usage = item["usage"]
        reads = first_turn_cache_read(usage)
        if i == 1:
            p_run = reads
        elif i > 1 and p_run and reads is not None and reads < ratio * p_run:
            first_ok = False
            violations.append(f"{item['node']}: first-turn cache read {reads} < {ratio:.0%} of P_run {p_run}")
        prev = None
        for turn in usage.get("turns", []):
            for u in turn:
                e1h += int(u.get("ephemeral_1h_input_tokens", 0))
                e5m += int(u.get("ephemeral_5m_input_tokens", 0))
            if turn:
                cur = int(turn[0].get("cache_read_input_tokens", 0))
                if prev and cur < ratio * prev:
                    within_ok = False
                    violations.append(f"{item['node']}: turn cache read {cur} < {ratio:.0%} of previous {prev}")
                prev = cur
    ephemeral_ok = e1h > 0 and e5m == 0
    if not ephemeral_ok:
        violations.append(f"ephemeral_1h={e1h} (must be > 0), ephemeral_5m={e5m} (must be 0)")
    return {"p_run": p_run, "first_turn_ok": first_ok, "within_node_ok": within_ok,
            "ephemeral_ok": ephemeral_ok, "violations": violations,
            "applicable": len(node_usages) >= 2}
