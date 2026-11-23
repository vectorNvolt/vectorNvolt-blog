"""Run budgets, timers and the shared engine types.

Covers: SFR-AGG-016 (per-attempt timer, step / LLM-call / run-time budgets; overrides.json is the
only way to raise them), SSR-AGG-004 (the engine bounds the dynamic machine: the gate refuses a
node start or a model call at the limit and the timers cancel the running attempt), decision 11
(the run-time budget excludes time spent in E3 dialogs and in S7).

Shared types ``EngineContext``, ``EngineOutcome`` and ``EngineRuntime`` live here so that the
E1..E7 modules and ``engine/graph.py`` can import them without a cycle; ``agent_graph.engine``
re-exports the first two.

overrides.json (written by the resolution store, WP6)::

    {"retry": 3 | {"<node>": 3}, "timeout": 900 | {"<node>": 900},
     "budgets": {"max_steps": 80, "max_llm_calls": 100, "timeout": 7200},
     "step_budget": {"<node>": 60000}}
"""

import dataclasses
import json
import threading
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Callable

from ..ports import CancelToken, ClaudePort, ExecBackend, GatewayPort, Launch, SummarizerPort
from ..services.history import History
from ..services.run_facts import RunFactsStore
from ..services.runfs import RunPaths, WriteGuard, canonical_json, write_atomic
from ..state import (Binding, Budgets, EngineCounters, GraphSpec, PoolIndex, ProcessDef, RunRecord)


# ------------------------------------------------------------------ overrides

def load_overrides(path: Path) -> dict:
    p = Path(path)
    if not p.is_file():
        return {}
    try:
        data = json.loads(p.read_text(encoding="utf-8"))
    except ValueError:
        return {}
    return data if isinstance(data, dict) else {}


def _per_node(value: Any, node: str) -> "int | None":
    if isinstance(value, bool):
        return None
    if isinstance(value, int):
        return value
    if isinstance(value, dict) and isinstance(value.get(node), int):
        return value[node]
    return None


def apply_overrides(proc: ProcessDef, ov: dict) -> ProcessDef:
    """The process with the run overrides applied (retry, timeout, budgets.*, step budget)."""
    if not ov:
        return proc
    steps = []
    for s in proc.steps:
        changes: dict = {}
        r = _per_node(ov.get("retry"), s.name)
        if r is not None:
            changes["retry"] = r
        t = _per_node(ov.get("timeout"), s.name)
        if t is not None and s.type != "human":
            changes["timeout_s"] = t
        b = _per_node(ov.get("step_budget"), s.name)
        if b is not None:
            changes["budget"] = b
        steps.append(dataclasses.replace(s, **changes) if changes else s)
    rt = proc.runtime
    ob = ov.get("budgets") or {}
    budgets = Budgets(
        max_steps=int(ob.get("max_steps", rt.budgets.max_steps)),
        max_llm_calls=int(ob.get("max_llm_calls", rt.budgets.max_llm_calls)),
        timeout_s=int(ob.get("timeout", rt.budgets.timeout_s)))
    runtime = dataclasses.replace(rt, budgets=budgets)
    return dataclasses.replace(proc, steps=tuple(steps), runtime=runtime)


# ------------------------------------------------------------------ gate and timers

class RunBudgets:
    """Implements ``ports.RunGate`` and owns the run-time clock and the per-attempt timer."""

    def __init__(self, counters: EngineCounters, budgets: Budgets, *, clock: Callable[[], float] = time.monotonic):
        self.counters = counters
        self.budgets = budgets
        self._clock = clock
        self._lock = threading.RLock()
        self._base = float(counters.elapsed_s)
        self._started: "float | None" = None
        self._run_timer: "threading.Timer | None" = None
        self._attempt_timer: "threading.Timer | None" = None
        self._cancel: "Callable[[str, str], None] | None" = None
        self.current_node: "str | None" = None
        self.expired = False
        self.breach: "dict | None" = None

    def bind(self, cancel: Callable[[str, str], None]) -> None:
        """``cancel(node, cause)`` is ``DynamicMachine.cancel``."""
        self._cancel = cancel

    # ---- RunGate
    def attempt_token(self, node: str) -> CancelToken:
        tok = CancelToken()
        with self._lock:
            self.current_node = node
            if self.expired:
                tok.cancel("budget:timeout")
        return tok

    def before_node_start(self, node: str) -> "str | None":
        with self._lock:
            self.current_node = node
            if self.expired or (self.budgets.timeout_s and self.elapsed() >= self.budgets.timeout_s):
                self._breach("timeout", self.budgets.timeout_s, self.elapsed())
                return "budget:timeout"
            if self.counters.steps >= self.budgets.max_steps:
                self._breach("max_steps", self.budgets.max_steps, self.counters.steps)
                return "budget:max_steps"
            self.counters.steps += 1
            return None

    def before_llm_call(self, node: str) -> "str | None":
        with self._lock:
            self.current_node = node
            if self.counters.llm_calls >= self.budgets.max_llm_calls:
                self._breach("max_llm_calls", self.budgets.max_llm_calls, self.counters.llm_calls)
                return "budget:max_llm_calls"
            self.counters.llm_calls += 1
            return None

    def _breach(self, name: str, limit: int, count: Any) -> None:
        self.breach = {"budget": name, "limit": limit, "count": count}

    # ---- run clock (paused during E3 dialogs and S7)
    def elapsed(self) -> float:
        with self._lock:
            run = (self._clock() - self._started) if self._started is not None else 0.0
            return self._base + run

    def resume_clock(self) -> None:
        with self._lock:
            if self._started is not None:
                return
            self._started = self._clock()
            limit = self.budgets.timeout_s
            if limit:
                remaining = max(0.0, limit - self._base)
                self._run_timer = threading.Timer(remaining, self._expire)
                self._run_timer.daemon = True
                self._run_timer.start()

    def pause_clock(self) -> None:
        with self._lock:
            if self._started is None:
                return
            self._base += self._clock() - self._started
            self._started = None
            if self._run_timer is not None:
                self._run_timer.cancel()
                self._run_timer = None
            self.counters.elapsed_s = self._base

    def _expire(self) -> None:
        with self._lock:
            if self._started is None:
                return
            self.expired = True
            self._breach("timeout", self.budgets.timeout_s, round(self.elapsed(), 1))
            cancel, node = self._cancel, self.current_node
        if cancel is not None and node:
            cancel(node, "budget:timeout")

    # ---- per-attempt timer
    def arm_attempt(self, node: str, seconds: "float | None") -> None:
        self.disarm_attempt()
        if not seconds or seconds <= 0:
            return

        def fire() -> None:
            cancel = self._cancel
            if cancel is not None:
                cancel(node, "timeout")
        with self._lock:
            self._attempt_timer = threading.Timer(seconds, fire)
            self._attempt_timer.daemon = True
            self._attempt_timer.start()

    def disarm_attempt(self) -> None:
        with self._lock:
            if self._attempt_timer is not None:
                self._attempt_timer.cancel()
                self._attempt_timer = None

    def close(self) -> None:
        self.disarm_attempt()
        self.pause_clock()

    def sync_counters(self) -> None:
        self.counters.elapsed_s = round(self.elapsed(), 3)


# ------------------------------------------------------------------ shared engine types

@dataclass
class EngineContext:
    """Input of ``run_engine`` (built by n06_execution). The fields after ``restart_summary``
    are optional: when None the engine builds the default (see e1_load)."""
    paths: RunPaths
    process: ProcessDef
    spec: GraphSpec
    index: PoolIndex
    binding: Binding
    record: RunRecord
    gateway: GatewayPort
    claude: ClaudePort
    facts: RunFactsStore
    history: History
    checkpointer: Any
    mode: str = "start"                          # "start" | "restart" | "resume"
    restart_node: "str | None" = None
    restart_summary: str = ""
    summarizer: "SummarizerPort | None" = None   # default: services.summarize.SummaryCache (WP3)
    exec_backend: "ExecBackend | None" = None    # default: services.container.SandboxExec
    container: "ExecBackend | None" = None       # isolate: true backend (default: services.container.ContainerExec)
    launch: "Launch | None" = None               # default: Launch(model, effort, runtime tools) of the record
    compose: "Callable | None" = None            # default: compose.compose_context (WP3)
    record_turn: "Callable | None" = None
    record_compaction: "Callable | None" = None
    graph: Any = None                            # precompiled dynamic graph (default: compile_dynamic)


@dataclass(frozen=True)
class EngineOutcome:
    done: bool
    output_ref: "str | None"
    escalation: "Any | None"                     # errors.RunError


@dataclass
class EngineRuntime:
    """Mutable state shared by the E1..E7 nodes of one ``run_engine`` call."""
    ctx: EngineContext
    proc: ProcessDef                             # with the run overrides applied
    guard: WriteGuard
    budgets: RunBudgets
    machine: Any = None                          # DynamicMachine, built by E1
    journal: Any = None                          # NodeJournal, opened by E1
    human_started: dict = field(default_factory=dict)
    current_node: "str | None" = None

    @property
    def record(self) -> RunRecord:
        return self.ctx.record

    @property
    def counters(self) -> EngineCounters:
        return self.ctx.record.counters

    def persist(self) -> None:
        """Write run.json (counters included) atomically."""
        self.budgets.sync_counters()
        write_atomic(self.ctx.paths.run_json, canonical_json(self.ctx.record.to_json()), guard=self.guard)
