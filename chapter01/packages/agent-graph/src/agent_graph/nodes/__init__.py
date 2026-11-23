"""Static master states S1-S8 (SSR-AGG-003: the same machine for every process).

``Env`` is the runtime environment of the master: services, the run folder and the loaded run
context. The LangGraph state (``StaticState``) holds only references; objects live here
(SSR-AGG-005). Each state is a function ``run(env, state) -> dict`` in its own module. The
returned dict sets ``phase`` to the state that runs next (``"S7"`` together with ``error`` on a
failure). This module must not import the state modules (they import it).
"""

import json
from collections.abc import Callable
from dataclasses import dataclass, field
from datetime import datetime, timedelta
from pathlib import Path

from .. import config
from ..compose.manifest import EMPTY_MCP_JSON
from ..ports import ClaudePort, GatewayPort, Launch
from ..process.reentry import HashSnapshot
from ..services.checkpoint import open_checkpointer
from ..services.history import History
from ..services.lock import ActiveRunLock
from ..services.run_facts import RunFactsStore
from ..services.runfs import (RunPaths, WorkspacePaths, WriteGuard, canonical_json, new_run_id,
                              write_atomic)
from ..state import Binding, GraphSpec, PoolIndex, ProcessDef, RunRecord


@dataclass
class Env:
    ws: WorkspacePaths
    guard: WriteGuard
    gateway: GatewayPort
    claude: ClaudePort
    lock: ActiveRunLock
    claude_dir: Path
    model: str = config.RUN_MODEL
    effort: str = config.RUN_EFFORT
    clock: Callable[[], datetime] = datetime.now
    # run context, set once the run folder exists
    paths: RunPaths | None = None
    record: RunRecord | None = None
    proc: ProcessDef | None = None
    index: PoolIndex | None = None
    binding: Binding | None = None
    spec: GraphSpec | None = None
    snapshot_before: HashSnapshot | None = None
    start_mode: str = "start"                       # "start" | "restart" | "resume" (read by S6)
    _facts: RunFactsStore | None = field(default=None, repr=False)
    _history: History | None = field(default=None, repr=False)

    # ---------------------------------------------------------------- run context
    def reset_run(self) -> None:
        self.paths = self.record = self.proc = self.index = self.binding = self.spec = None
        self.snapshot_before = None
        self._facts = self._history = None
        self.start_mode = "start"

    def facts(self) -> RunFactsStore:
        if self._facts is None:
            self._facts = RunFactsStore(self.paths.run_facts_jsonl, self.guard)
        return self._facts

    def history(self) -> History:
        if self._history is None:
            self._history = History(self.paths.history_jsonl, self.guard)
        return self._history

    def save_record(self) -> None:
        if self.paths is None or self.record is None:
            return
        write_atomic(self.paths.run_json, canonical_json(self.record.to_json()), guard=self.guard)

    def set_phase(self, name: str, status: str = "ACTIVE") -> None:
        if self.record is not None:
            self.record.phase = name            # type: ignore[assignment]
            self.record.status = status         # type: ignore[assignment]
            self.save_record()

    def create_run(self, process_id: str, process_file: str) -> RunPaths:
        """End of S2: runID, run folder, run.json, checkpoint store, gateway transcript."""
        now = self.clock()
        run_id = new_run_id(process_id, now)
        while self.ws.run(run_id).root.exists():
            now += timedelta(seconds=1)
            run_id = new_run_id(process_id, now)
        paths = self.ws.run(run_id)
        self.guard.check(paths.root)
        paths.root.mkdir(parents=True, exist_ok=False)
        open_checkpointer(paths.checkpoint_db).close()      # --resume needs run.json + checkpoint
        self.reset_run()
        self.paths = paths
        self.record = RunRecord(run_id=run_id, process_id=process_id, process_file=process_file,
                                created=now.isoformat(timespec="seconds"), phase="S3", status="ACTIVE",
                                model=self.model, effort=self.effort)
        write_atomic(paths.mcp_json, EMPTY_MCP_JSON, guard=self.guard)
        self.save_record()
        self.lock.set_run(run_id)
        self.gateway.attach_run(paths.gateway_jsonl)
        return paths

    def load_run(self, run_id: str) -> list[str]:
        """Resume: load run.json and what exists of the run context. Returns problems (strings)."""
        paths = self.ws.run(run_id)
        self.reset_run()
        self.paths = paths
        self.record = RunRecord.from_json(json.loads(paths.run_json.read_text(encoding="utf-8")))
        self.model, self.effort = self.record.model, self.record.effort     # SFR-AGG-037: same pins
        self.gateway.attach_run(paths.gateway_jsonl)
        problems: list[str] = []
        try:
            if paths.graph_json.is_file():
                self.spec = GraphSpec.from_json(json.loads(paths.graph_json.read_text(encoding="utf-8")))
                self.proc = ProcessDef.from_json(self.spec.process)
            if paths.index_json.is_file():
                self.index = PoolIndex.from_json(
                    json.loads(paths.index_json.read_text(encoding="utf-8")), self.ws.root)
            if paths.binding_json.is_file():
                self.binding = Binding.from_json(json.loads(paths.binding_json.read_text(encoding="utf-8")))
        except (OSError, ValueError, KeyError, TypeError) as exc:
            problems.append(f"run state cannot be loaded: {exc}")
        return problems

    # ---------------------------------------------------------------- dialogs
    def dialog_launch(self) -> Launch:
        return Launch(model=self.model, effort=self.effort, tools=config.DIALOG_TOOLS,
                      system_prompt_file=None)

    def tmp_dir(self) -> Path:
        """TMPDIR of child processes (SFR-AGG-041): the run's tmp/, else the shared static one."""
        return self.paths.tmp_dir if self.paths is not None else self.ws.static_dir / "tmp"
