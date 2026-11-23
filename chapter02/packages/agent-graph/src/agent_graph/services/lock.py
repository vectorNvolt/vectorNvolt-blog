"""Active-run lock: at most one active run per agent.

Covers: SSR-AGG-002 (one active run; a suspended run no longer counts), SFR-AGG-002.

``fcntl.flock`` on ``.agent/.active.lock`` (the kernel drops it when the process dies, so a
suspended or killed run is not active) plus ``.agent/active.json`` naming the holder:
``{"run_id": str|null, "pid": int}``.
"""

import fcntl
import json
import os
from pathlib import Path

from .runfs import WorkspacePaths

UNKNOWN_HOLDER = "unknown"


class ActiveRunLock:
    def __init__(self, ws: WorkspacePaths):
        self.ws = ws
        self._fd: int | None = None

    @property
    def held(self) -> bool:
        return self._fd is not None

    def acquire(self, run_id: str | None) -> str | None:
        """Take the lock. Returns None when acquired, else the holder's run id
        (``"unknown"`` when it cannot be read or the holder has no run folder yet)."""
        if self._fd is not None:
            return None
        self.ws.agent_dir.mkdir(parents=True, exist_ok=True)
        fd = os.open(self.ws.lock_file, os.O_RDWR | os.O_CREAT, 0o644)
        try:
            fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            os.close(fd)
            return self._holder()
        self._fd = fd
        self._write_active(run_id)
        return None

    def set_run(self, run_id: str) -> None:
        if self._fd is None:
            raise RuntimeError("active-run lock is not held")
        self._write_active(run_id)

    def release(self) -> None:
        if self._fd is None:
            return
        try:
            self.ws.active_json.unlink()
        except FileNotFoundError:
            pass
        try:
            fcntl.flock(self._fd, fcntl.LOCK_UN)
        finally:
            os.close(self._fd)
            self._fd = None

    def __enter__(self) -> "ActiveRunLock":
        return self

    def __exit__(self, *exc: object) -> None:
        self.release()

    # ------------------------------------------------------------------ internals
    def _write_active(self, run_id: str | None) -> None:
        target = self.ws.active_json
        tmp = target.with_name(f".{target.name}.{os.getpid()}.tmp")
        tmp.write_text(json.dumps({"run_id": run_id, "pid": os.getpid()}), encoding="utf-8")
        os.replace(tmp, target)

    def _holder(self) -> str:
        try:
            data = json.loads(Path(self.ws.active_json).read_text(encoding="utf-8"))
            rid = data.get("run_id")
            return rid if isinstance(rid, str) and rid else UNKNOWN_HOLDER
        except (OSError, ValueError, AttributeError):
            return UNKNOWN_HOLDER
