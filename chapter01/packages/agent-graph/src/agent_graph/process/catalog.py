"""List the process files of ``.process/`` (SFR-AGG-004, SFR-AGG-003 listing).

Only id, title and description are read here (full validation is S3). A file whose id is shared
with another file is not selectable and names the other files; a file that is not valid YAML or
has no string id is not selectable either (``error`` says why).
"""

from dataclasses import dataclass

from ..errors import Finding
from ..services.runfs import WorkspacePaths
from ..services.yaml_schema import load_yaml_with_lines


@dataclass(frozen=True)
class CatalogEntry:
    id: str
    title: str
    description: str
    file: str                       # workspace-relative
    selectable: bool
    duplicates: tuple[str, ...]     # other files with the same id
    error: "str | None" = None


def list_processes(ws: WorkspacePaths) -> list[CatalogEntry]:
    raw: list[tuple[str, "str | None", str, str, "str | None"]] = []   # file, id, title, desc, error
    if ws.process_dir.is_dir():
        files = sorted(p for p in ws.process_dir.iterdir()
                       if p.is_file() and p.suffix in (".yml", ".yaml"))
        for p in files:
            rel = ws.rel(p)
            try:
                text = p.read_text(encoding="utf-8")
            except (OSError, UnicodeDecodeError) as e:
                raw.append((rel, None, "", "", f"unreadable: {e}"))
                continue
            doc, fs = load_yaml_with_lines(text, rel)
            if doc is None:
                raw.append((rel, None, "", "", fs[0].message if fs else "invalid YAML"))
                continue
            d = doc.data if isinstance(doc.data, dict) else {}
            pid = d.get("id")
            if not isinstance(pid, str) or not pid:
                raw.append((rel, None, "", "", "no string id"))
                continue
            title = d.get("title") if isinstance(d.get("title"), str) else ""
            desc = d.get("description") if isinstance(d.get("description"), str) else ""
            raw.append((rel, pid, title, desc.strip(), None))

    by_id: dict[str, list[str]] = {}
    for rel, pid, *_ in raw:
        if pid is not None:
            by_id.setdefault(pid, []).append(rel)
    out = []
    for rel, pid, title, desc, err in raw:
        if pid is None:
            out.append(CatalogEntry(id="", title=title, description=desc, file=rel, selectable=False,
                                    duplicates=(), error=err))
            continue
        dups = tuple(f for f in by_id[pid] if f != rel)
        out.append(CatalogEntry(id=pid, title=title, description=desc, file=rel,
                                selectable=not dups, duplicates=dups, error=None))
    return out


def duplicate_findings(entries: "list[CatalogEntry]") -> list[Finding]:
    """One ``dup_process_id`` finding per file with a shared id, naming all files."""
    out = []
    for e in entries:
        if e.duplicates:
            files = ", ".join(sorted((e.file, *e.duplicates)))
            out.append(Finding(code="dup_process_id",
                               message=f"process id {e.id!r} is shared by: {files}",
                               file=e.file, item=e.id))
    return out


def find_entry(entries: "list[CatalogEntry]", process_id: str) -> "CatalogEntry | None":
    """The selectable entry with this id, else None."""
    for e in entries:
        if e.id == process_id and e.selectable:
            return e
    return None
