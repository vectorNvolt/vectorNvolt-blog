"""DynamicMachine: the only surface of the dynamic state machine the engine sees.

Covers: SSR-AGG-004 (changes only on commands START, RESTART, HIL_OK, HIL_NOK, ANSWER, RETRY,
CANCEL, SUSPEND; every change is reported as one of NODE_STARTED, NODE_DONE, WAITING, NODE_FAILED,
COMPLETED, SUSPENDED; the machine pauses by interrupt and is driven by streaming with resume
commands, in its own checkpoint thread), SFR-AGG-012/013 (command recorded before the status it
causes; one node at a time), SFR-AGG-016 (CANCEL reaches the running attempt through its token),
SFR-AGG-037 (RESTART on a new thread carries the finished state over).

LangGraph delivers custom-stream events of a single running node only after the node ends, so
the nodes emit through ``NodeContext.emit`` into a queue instead; ``graph.stream`` runs in a
worker thread and ``send`` is a real-time iterator over that queue. The final status (a pause or
COMPLETED) is read from the checkpoint after the stream ended.
"""

import dataclasses
import queue
import threading
from typing import Iterator

from langgraph.types import Command

from ..config import RECURSION_LIMIT
from ..contract import Cmd, Notice, Status, cancel as cancel_cmd
from ..ports import CancelToken, DynamicContext, RunGate
from ..services.history import History
from .journal import NodeContext

_CONTEXT_FIELDS = tuple(f.name for f in dataclasses.fields(DynamicContext))
_EXTRAS = ("compose", "record_turn", "record_compaction", "container", "gateway")


class TokenBook:
    """RunGate wrapper that remembers the token of each node's current attempt so that
    ``cancel(node, cause)`` can stop it. A cancel that arrives before the attempt has asked for
    its token is applied to the next token."""

    def __init__(self, inner: RunGate):
        self.inner = inner
        self._lock = threading.Lock()
        self._tokens: dict[str, CancelToken] = {}
        self._pending: dict[str, str] = {}

    def attempt_token(self, node: str) -> CancelToken:
        tok = self.inner.attempt_token(node)
        with self._lock:
            self._tokens[node] = tok
            cause = self._pending.pop(node, None)
        if cause:
            tok.cancel(cause)
        return tok

    def before_node_start(self, node: str) -> "str | None":
        return self.inner.before_node_start(node)

    def before_llm_call(self, node: str) -> "str | None":
        return self.inner.before_llm_call(node)

    def reset(self, node: str) -> None:
        with self._lock:
            self._tokens.pop(node, None)
            self._pending.pop(node, None)

    def cancel(self, node: str, cause: str) -> None:
        with self._lock:
            tok = self._tokens.get(node)
            if tok is None or tok.cancelled:
                self._pending[node] = cause
                return
        tok.cancel(cause)


class DynamicMachine:
    def __init__(self, graph, thread_id: str, ctx: DynamicContext, history: History, *,
                 carry_from: "str | None" = None):
        """``carry_from``: an earlier thread whose finished state (visits, products, handoffs,
        summaries, last node/outcome) a RESTART on this new thread starts from."""
        self.graph = graph
        self.thread_id = thread_id
        self.history = history
        self.carry_from = carry_from
        self.paths = ctx.paths
        self._book = TokenBook(ctx.gate)
        base = {n: getattr(ctx, n) for n in _CONTEXT_FIELDS}
        base["gate"] = self._book
        self._ctx = NodeContext(**base, **{n: getattr(ctx, n, None) for n in _EXTRAS}, emit=self._emit)
        self._config = {"configurable": {"thread_id": thread_id}, "recursion_limit": RECURSION_LIMIT}
        self._events: "queue.Queue" = queue.Queue()
        self._started_seen: set = set()
        self._worker: "threading.Thread | None" = None
        self._node_names = [n for n in graph.nodes if not n.startswith("__")]

    # ------------------------------------------------------------------ public
    def send(self, cmd: Cmd) -> "Iterator[Status | Notice]":
        if cmd.name == "CANCEL":
            self.cancel(cmd.node or "", str(cmd.payload.get("cause", "")))
            return
        self.history.command(cmd)
        if cmd.name in ("START", "RESTART"):
            yield from self._run(self._start_input(cmd))
        elif cmd.name == "SUSPEND":
            yield from self._suspend()
        else:
            pend = self.pending()
            if pend is None:
                raise RuntimeError(f"{cmd.name}: no node is waiting on thread {self.thread_id}")
            if pend.node:
                self._book.reset(pend.node)
            yield from self._run(Command(resume=cmd.to_json()))

    def cancel(self, node: str, cause: str) -> None:
        """Thread-safe: record CANCEL and stop the node's running attempt."""
        self.history.command(cancel_cmd(node, cause))
        self._book.cancel(node, cause)

    def abort(self, cause: str = "interrupted", timeout: float = 10.0) -> None:
        """Stop the running attempt (SIGINT, closed terminal) and wait for the worker to pause."""
        worker = self._worker
        if worker is not None and worker.is_alive():
            for node in self._node_names:
                self._book.cancel(node, cause)
            worker.join(timeout)

    def pending(self) -> "Status | None":
        """The status the machine is paused at, or None."""
        snap = self.graph.get_state(self._config)
        if not snap.next:
            return None
        for task in snap.tasks:
            for itr in task.interrupts:
                return Status.from_json(itr.value)
        for itr in snap.interrupts:
            return Status.from_json(itr.value)
        return None

    def values(self, thread_id: "str | None" = None) -> dict:
        cfg = {"configurable": {"thread_id": thread_id or self.thread_id}}
        return dict(self.graph.get_state(cfg).values or {})

    # ------------------------------------------------------------------ internals
    def _emit(self, item: "Status | Notice") -> None:
        if isinstance(item, Status) and item.name == "NODE_STARTED" and item.node:
            self._book.reset(item.node)
        self._events.put(("event", item))

    def _record(self, item: "Status | Notice") -> None:
        if isinstance(item, Status):
            self.history.status(item)
        else:
            self.history.notice(item)

    def _start_input(self, cmd: Cmd) -> dict:
        if cmd.name == "START":
            return {"restart_at": None, "continue_session": False}
        own_next = self.graph.get_state(self._config).next
        if own_next and self.carry_from is None:
            raise RuntimeError("RESTART needs a new thread when a node is still paused; pass carry_from")
        values = self.values(self.carry_from)
        cont = bool(cmd.payload.get("continue_session"))
        inp: dict = {"restart_at": cmd.node, "restart_summary": cmd.payload.get("summary") or None,
                     "continue_session": cont}
        for k in ("products", "handoffs", "summaries"):
            if values.get(k):
                inp[k] = dict(values[k])
        for k in ("last_node", "last_outcome", "last_hil"):
            if values.get(k) is not None:
                inp[k] = values[k]
        visits = dict(values.get("visits") or {})
        if not cont:        # a restart from its beginning never replays an earlier visit's journal
            for n in self._node_names:
                visits[n] = visits.get(n, 0) + 1
        inp["visits"] = visits
        return inp

    def _suspend(self) -> "Iterator[Status | Notice]":
        pend = self.pending()
        if pend is not None and pend.name == "SUSPENDED":
            yield pend
            return
        if pend is None:
            st = Status("SUSPENDED", None, {})
            self._record(st)
            yield st
            return
        yield from self._run(Command(resume=Cmd("SUSPEND", None, {}).to_json()))

    def _final_status(self) -> Status:
        pend = self.pending()
        if pend is not None:
            return pend
        snap = self.graph.get_state(self._config)
        if snap.next:
            raise RuntimeError("the dynamic machine stopped without a pause status")
        return Status("COMPLETED", None, {"output_ref": self.paths.ws.rel(self.paths.output_dir)})

    def _run(self, inp) -> "Iterator[Status | Notice]":
        q: "queue.Queue" = queue.Queue()
        self._events = q

        def work() -> None:
            try:
                for _ in self.graph.stream(inp, self._config, context=self._ctx, stream_mode="updates"):
                    pass
                q.put(("end", None))
            except BaseException as exc:  # noqa: BLE001 - handed to the caller's thread
                q.put(("error", exc))

        worker = threading.Thread(target=work, name=f"dynamic-{self.thread_id}", daemon=True)
        self._worker = worker
        worker.start()
        while True:
            try:
                kind, val = q.get(timeout=0.25)
            except queue.Empty:
                continue
            if kind == "event":
                if isinstance(val, Status) and val.name == "NODE_STARTED":
                    key = (val.node, val.payload.get("visit"))
                    if key in self._started_seen:       # a replay after a resume re-emits it
                        continue
                    self._started_seen.add(key)
                self._record(val)
                yield val
            elif kind == "error":
                worker.join()
                raise val
            else:
                break
        worker.join()
        final = self._final_status()
        self._record(final)
        yield final
