"""E6 cleanup: disposes of the work products as runtime.work_products says.

Covers: SFR-AGG-031 (archive to .agent/<runID>/archive/work.tar.gz and remove the folder (default),
delete it, or keep it; nodes/ with the transcripts and context records is always kept).
"""

import os
import shutil
import tarfile

from .budgets import EngineRuntime


def e6_cleanup(rt: EngineRuntime, state: dict) -> dict:
    paths = rt.ctx.paths
    mode = rt.proc.runtime.work_products
    work = rt.guard.check(paths.work_dir)
    if work.is_dir() and mode in ("archive", "delete"):
        if mode == "archive":
            dest = rt.guard.check(paths.archive_tgz)
            dest.parent.mkdir(parents=True, exist_ok=True)
            tmp = dest.with_name(dest.name + ".tmp")
            with tarfile.open(tmp, "w:gz") as tar:
                tar.add(work, arcname="work")
            os.replace(tmp, dest)
        shutil.rmtree(work)
    rt.persist()
    return {"done": True, "output_ref": state.get("output_ref"), "next_cmd": None, "pending": None}
