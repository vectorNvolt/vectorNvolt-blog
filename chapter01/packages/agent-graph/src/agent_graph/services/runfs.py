"""Workspace and run-folder layout, atomic/durable writes, write and input-read guards.

Covers: SSR-AGG-014 (run folder content), SSR-AGG-015 (everything under /workspace),
SFR-AGG-041 / SSR-AGG-007 (write boundary), SSR-AGG-008 (only bound input is read),
SFR-AGG-001 (has_checkpoint), SSR-AGG-006 (fsync'd records).

Spec (authoritative signatures)::

    new_run_id(process_id, now) -> f"{id}-{now:%Y%m%d-%H%M%S}"
    WorkspacePaths(root).run(run_id) -> RunPaths
    sha256_file / sha256_bytes / canonical_json / write_atomic / append_jsonl / read_jsonl
    WriteGuard(ws, claude_dir).check(p) / .resolution(approved)
    InputReadGuard(ws, binding).open_text(p)
"""

import contextlib
import hashlib
import json
import os
import threading
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path
from typing import TYPE_CHECKING, Any, ContextManager, Iterator

from ..errors import InputReadDenied, WriteBoundaryError

if TYPE_CHECKING:  # avoid an import cycle (state does not import services at module level)
    from ..state import Binding


def new_run_id(process_id: str, now: datetime) -> str:
    return f"{process_id}-{now:%Y%m%d-%H%M%S}"


@dataclass(frozen=True)
class WorkspacePaths:
    root: Path

    @property
    def process_dir(self) -> Path:
        return self.root / ".process"

    @property
    def input_dir(self) -> Path:
        return self.root / ".input"

    @property
    def identity_dir(self) -> Path:
        return self.root / "identity"

    @property
    def skills_dir(self) -> Path:
        return self.root / "identity" / ".skills"

    @property
    def snippet_dir(self) -> Path:
        return self.root / "identity" / ".snippet"

    @property
    def agent_dir(self) -> Path:
        return self.root / ".agent"

    @property
    def output_root(self) -> Path:
        return self.root / ".output"

    @property
    def cache_summaries(self) -> Path:
        return self.agent_dir / "cache" / "summaries"

    @property
    def static_dir(self) -> Path:
        return self.agent_dir / ".static"

    @property
    def lock_file(self) -> Path:
        return self.agent_dir / ".active.lock"

    @property
    def active_json(self) -> Path:
        return self.agent_dir / "active.json"

    def run(self, run_id: str) -> "RunPaths":
        return RunPaths(self, run_id)

    def rel(self, p: Path) -> str:
        """Workspace-relative posix path of ``p`` (ValueError if outside)."""
        return Path(os.path.abspath(p)).relative_to(os.path.abspath(self.root)).as_posix()


@dataclass(frozen=True)
class RunPaths:
    ws: WorkspacePaths
    run_id: str

    @property
    def root(self) -> Path:
        return self.ws.agent_dir / self.run_id

    @property
    def run_json(self) -> Path:
        return self.root / "run.json"

    @property
    def binding_json(self) -> Path:
        return self.root / "binding.json"

    @property
    def index_json(self) -> Path:
        return self.root / "index.json"

    @property
    def graph_json(self) -> Path:
        return self.root / "graph.json"

    @property
    def graph_error_json(self) -> Path:
        return self.root / "graph.error.json"

    @property
    def gateway_jsonl(self) -> Path:
        return self.root / "gateway.jsonl"

    @property
    def history_jsonl(self) -> Path:
        return self.root / "history.jsonl"

    @property
    def run_facts_jsonl(self) -> Path:
        return self.root / "run_facts.jsonl"

    @property
    def overrides_json(self) -> Path:
        return self.root / "overrides.json"

    @property
    def checkpoint_db(self) -> Path:
        return self.root / "checkpoint.sqlite"

    @property
    def mcp_json(self) -> Path:
        return self.root / "mcp.json"

    @property
    def active(self) -> Path:
        return self.root / "active"

    @property
    def nodes_dir(self) -> Path:
        return self.root / "nodes"

    @property
    def work_dir(self) -> Path:
        return self.root / "work"

    @property
    def archive_tgz(self) -> Path:
        return self.root / "archive" / "work.tar.gz"

    @property
    def dialogs_dir(self) -> Path:
        return self.root / "dialogs"

    @property
    def tmp_dir(self) -> Path:
        return self.root / "tmp"

    @property
    def output_dir(self) -> Path:
        return self.ws.output_root / self.run_id

    @property
    def report_md(self) -> Path:
        return self.output_dir / "report.md"

    @property
    def output_invalid_json(self) -> Path:
        return self.output_dir / "OUTPUT_INVALID.json"

    def node_dir(self, node: str) -> Path:
        return self.nodes_dir / node

    def work_node(self, node: str) -> Path:
        return self.work_dir / node

    def resolution_dir(self, n: int) -> Path:
        return self.root / "resolution" / str(n)

    def has_checkpoint(self) -> bool:
        """run.json and checkpoint.sqlite both exist (SFR-AGG-001 --resume check)."""
        return self.run_json.is_file() and self.checkpoint_db.is_file()


def sha256_bytes(b: bytes) -> str:
    return hashlib.sha256(b).hexdigest()


def sha256_file(p: Path) -> str:
    h = hashlib.sha256()
    with open(p, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def canonical_json(obj: Any) -> str:
    """sort_keys, separators (",", ": "), indent 2, ensure_ascii False, trailing newline."""
    return json.dumps(obj, sort_keys=True, separators=(",", ": "), indent=2, ensure_ascii=False) + "\n"


class WriteGuard:
    """Every write of the runtime goes through ``check`` (SFR-AGG-041, SSR-AGG-007).

    Allowed: paths that resolve (symlinks followed) under ``ws.root`` or ``claude_dir``.
    Protected: .process/, .input/ and identity/ under the workspace; writable only inside
    ``with guard.resolution(approved=True):`` (used only by the resolution store, n07).
    """

    def __init__(self, ws: WorkspacePaths, claude_dir: Path):
        self.ws = ws
        self.claude_dir = Path(claude_dir)
        self._local = threading.local()
        self._roots = (Path(os.path.realpath(ws.root)), Path(os.path.realpath(self.claude_dir)))
        base = self._roots[0]
        self._protected = (base / ".process", base / ".input", base / "identity")

    @staticmethod
    def _under(p: Path, base: Path) -> bool:
        return p == base or base in p.parents

    def check(self, p: Path) -> Path:
        """Return the resolved path or raise WriteBoundaryError."""
        real = Path(os.path.realpath(p))
        if not any(self._under(real, r) for r in self._roots):
            raise WriteBoundaryError(f"write outside the allowed roots: {p}")
        if any(self._under(real, prot) for prot in self._protected) and \
                getattr(self._local, "depth", 0) == 0:
            raise WriteBoundaryError(f"write to a protected folder outside resolution: {p}")
        return real

    def resolution(self, approved: bool) -> ContextManager[None]:
        """Open the protected folders for this thread. ``approved=False`` raises at once."""
        if not approved:
            raise WriteBoundaryError("change to .process/.input/identity needs the developer's approval")
        return self._resolution_cm()

    @contextlib.contextmanager
    def _resolution_cm(self) -> Iterator[None]:
        self._local.depth = getattr(self._local, "depth", 0) + 1
        try:
            yield
        finally:
            self._local.depth -= 1


def _fsync_dir(d: Path) -> None:
    try:
        fd = os.open(d, os.O_RDONLY)
    except OSError:
        return
    try:
        os.fsync(fd)
    except OSError:
        pass
    finally:
        os.close(fd)


def write_atomic(p: Path, data: "str | bytes", *, guard: WriteGuard) -> None:
    """Write via a temp file in the same folder, fsync, then os.replace."""
    target = guard.check(p)
    target.parent.mkdir(parents=True, exist_ok=True)
    raw = data.encode("utf-8") if isinstance(data, str) else data
    tmp = target.with_name(f".{target.name}.{os.getpid()}.{threading.get_ident()}.tmp")
    try:
        with open(tmp, "wb") as f:
            f.write(raw)
            f.flush()
            os.fsync(f.fileno())
        os.replace(tmp, target)
    finally:
        if tmp.exists():
            tmp.unlink()
    _fsync_dir(target.parent)


def append_jsonl(p: Path, obj: dict, *, guard: WriteGuard) -> None:
    """Append one JSON line (sorted keys) with flush + fsync before returning."""
    target = guard.check(p)
    target.parent.mkdir(parents=True, exist_ok=True)
    line = json.dumps(obj, sort_keys=True, ensure_ascii=False, default=str) + "\n"
    with open(target, "ab") as f:
        f.write(line.encode("utf-8"))
        f.flush()
        os.fsync(f.fileno())


def read_jsonl(p: Path) -> list[dict]:
    """All records; [] if the file does not exist. A torn (unparsable) last line is dropped."""
    if not Path(p).is_file():
        return []
    out: list[dict] = []
    lines = Path(p).read_text(encoding="utf-8").splitlines()
    for i, line in enumerate(lines):
        if not line.strip():
            continue
        try:
            out.append(json.loads(line))
        except json.JSONDecodeError:
            if i == len(lines) - 1:
                break
            raise
    return out


class InputReadGuard:
    """Opens text files; refuses any .input file that is not in the binding (SSR-AGG-008)."""

    def __init__(self, ws: WorkspacePaths, binding: "Binding | None"):
        self.ws = ws
        self.binding = binding

    def open_text(self, p: Path) -> str:
        path = Path(p)
        if not path.is_absolute():
            path = self.ws.root / path
        real = Path(os.path.realpath(path))
        input_root = Path(os.path.realpath(self.ws.input_dir))
        if real == input_root or input_root in real.parents:
            rel = real.relative_to(Path(os.path.realpath(self.ws.root))).as_posix()
            allowed = self.binding.allowed_input_paths() if self.binding is not None else frozenset()
            if rel not in allowed:
                raise InputReadDenied(f"input file not bound for this run: {rel}")
        return real.read_text(encoding="utf-8")
