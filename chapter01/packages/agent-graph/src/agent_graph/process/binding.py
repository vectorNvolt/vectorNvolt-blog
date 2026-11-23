"""Input binding: candidate sets, binding record, input checks (SFR-AGG-009, SSR-AGG-008).

Candidate sets (plan decision 13): the files matched by a source's ``path`` glob (relative to
``.input/``) are grouped by parent folder; each group is one candidate set. Several sets mean the
developer chooses (the runtime never picks); a single set can be taken with
``single_choices``. All paths in sets and bindings are workspace-relative (``.input/...``).
Template and ruleset paths are relative to the workspace and are bound as they are.
"""

import json
from collections.abc import Mapping
from pathlib import Path

from ..errors import Finding
from ..services.runfs import WorkspacePaths, sha256_file
from ..services.yaml_schema import load_schema, load_yaml_with_lines, validate
from ..state import Binding, BoundFile, BoundSource, ProcessDef


def _unsafe_glob(pattern: str) -> bool:
    return not pattern or Path(pattern).is_absolute() or ".." in pattern.replace("\\", "/").split("/")


def find_candidates(proc: ProcessDef, ws: WorkspacePaths) -> dict[str, list[tuple[str, ...]]]:
    """source name -> candidate sets (sorted), for every source that has a ``path``."""
    out: dict[str, list[tuple[str, ...]]] = {}
    for s in proc.sources:
        if s.path is None:
            continue
        groups: dict[str, list[str]] = {}
        if ws.input_dir.is_dir() and not _unsafe_glob(s.path):
            for p in sorted(ws.input_dir.glob(s.path)):
                if p.is_file():
                    groups.setdefault(ws.rel(p.parent), []).append(ws.rel(p))
        out[s.name] = [tuple(groups[k]) for k in sorted(groups)]
    return out


def single_choices(candidates: Mapping[str, list[tuple[str, ...]]]) -> dict[str, tuple[str, ...]]:
    """Sources with exactly one candidate set: that set (no choice is made for several sets)."""
    return {k: v[0] for k, v in candidates.items() if len(v) == 1}


def _bound_file(ws: WorkspacePaths, rel: str) -> BoundFile:
    p = ws.root / rel
    return BoundFile(path=rel, sha256=sha256_file(p) if p.is_file() else "")


def make_binding(proc: ProcessDef, ws: WorkspacePaths, chosen: Mapping[str, tuple[str, ...]],
                 notes: Mapping[str, str]) -> Binding:
    """Build the binding from the developer's choices (no validation: see ``check_binding``).
    A file that does not exist is bound with an empty hash."""
    sources = {}
    for s in proc.sources:
        files = tuple(_bound_file(ws, rel) for rel in chosen.get(s.name, ())) if s.path is not None else ()
        sources[s.name] = BoundSource(name=s.name, files=files, note_text=notes.get(s.name))
    return Binding(
        process_id=proc.id, process_file=proc.source_file, process_sha256=proc.source_sha256,
        sources=sources,
        templates={t.name: _bound_file(ws, t.path) for t in proc.templates},
        rulesets={r.name: _bound_file(ws, r.path) for r in proc.rulesets})


def _inside(ws: WorkspacePaths, rel: str) -> bool:
    import os
    root = os.path.realpath(ws.root)
    real = os.path.realpath(ws.root / rel)
    return real == root or real.startswith(root + os.sep)


def _check_schema(ws: WorkspacePaths, schema_ref: str, rel: str, text_path: Path,
                  fd) -> None:
    try:
        schema = load_schema(str(ws.root / schema_ref))
    except FileNotFoundError:
        fd("input_schema", f"schema file {schema_ref!r} not found", rel)
        return
    except Exception as e:
        fd("input_schema", f"schema file {schema_ref!r} is invalid: {e}", rel)
        return
    suffix = text_path.suffix.lower()
    try:
        text = text_path.read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError) as e:
        fd("input_schema", f"cannot read as text: {e}", rel)
        return
    if suffix == ".json":
        try:
            data = json.loads(text)
        except ValueError as e:
            fd("input_schema", f"not valid JSON: {e}", rel)
            return
    elif suffix in (".yml", ".yaml"):
        doc, fs = load_yaml_with_lines(text, rel)
        if doc is None:
            fd("input_schema", fs[0].message, rel)
            return
        data = doc.data
    else:
        fd("input_schema", f"a schema is declared but {suffix or 'this file type'} cannot be validated", rel)
        return
    for p, m in validate(data, schema):
        fd("input_schema", f"{'.'.join(str(x) for x in p) or '<root>'}: {m}", rel)


def check_binding(proc: ProcessDef, binding: Binding, ws: WorkspacePaths) -> list[Finding]:
    out: list[Finding] = []
    cands = find_candidates(proc, ws)

    def mk(line, item):
        def fd(code, msg, file=None):
            out.append(Finding(code=code, message=msg, file=file or proc.source_file, line=line if file is None else None,
                               item=item))
        return fd

    if binding.process_sha256 != proc.source_sha256 or binding.process_id != proc.id:
        out.append(Finding(code="binding_stale", message="binding was made for another version of the process",
                           file=proc.source_file))

    for s in proc.sources:
        fd = mk(s.line, s.name)
        bound = binding.sources.get(s.name)
        files = bound.files if bound else ()
        if s.path is not None:
            if not files:
                sets = cands.get(s.name, [])
                if len(sets) > 1:
                    fd("set_unchosen", f"source {s.name!r}: {len(sets)} candidate sets match "
                                       f"{s.path!r}; none was chosen")
                elif s.required and s.note is None:
                    fd("input_missing", f"source {s.name!r}: no file matches {s.path!r} in .input/")
            for f in files:
                if not f.path.startswith(".input/") or not _inside(ws, f.path):
                    fd("input_missing", f"source {s.name!r}: {f.path!r} is not a file of .input/", f.path)
                    continue
                p = ws.root / f.path
                if not p.is_file():
                    fd("input_missing", f"source {s.name!r}: bound file {f.path!r} does not exist", f.path)
                    continue
                if f.sha256 != sha256_file(p):
                    fd("input_missing", f"source {s.name!r}: bound file {f.path!r} changed after binding", f.path)
                    continue
                if s.schema:
                    _check_schema(ws, s.schema, f.path, p, fd)
        if s.note is not None:
            if s.required and not (bound and bound.note_text and bound.note_text.strip()):
                if not (s.path is not None and files):
                    fd("input_missing", f"source {s.name!r}: no note was supplied")

    for kind, decls, bmap in (("output-template-ref", proc.templates, binding.templates),
                              ("ruleset", proc.rulesets, binding.rulesets)):
        for d in decls:
            fd = mk(d.line, d.name)
            b = bmap.get(d.name)
            if b is None:
                if d.required:
                    fd("input_missing", f"{kind} {d.name!r} is not bound")
                continue
            if not _inside(ws, b.path) or not (ws.root / b.path).is_file():
                if d.required:
                    fd("input_missing", f"{kind} {d.name!r}: file {b.path!r} does not exist", b.path)
                continue
            if b.sha256 != sha256_file(ws.root / b.path):
                fd("input_missing", f"{kind} {d.name!r}: file {b.path!r} changed after binding", b.path)
    return out
