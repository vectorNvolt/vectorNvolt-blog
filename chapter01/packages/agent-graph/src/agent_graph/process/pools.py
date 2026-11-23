"""Skill and snippet pools: parse, index with content hashes, validate declarations
(SFR-AGG-006, SFR-AGG-008, SSR-AGG-017).

``build_index`` reads ``identity/.snippet/**/*.md`` and ``identity/.skills/<name>/`` (SKILL.md plus
every other file, hashed). Broken pool files and duplicate ids are findings of ``build_index``;
``validate_against_pools`` checks what a process declares against the index.

Snippet layers: ``runtime.snippets`` must name ``process`` snippets (L1), a step's ``snippets``
must name ``step`` snippets (L2); ``agent`` snippets (L0) are implicit and at least one must exist.
"""

import math
from collections.abc import Callable
from pathlib import Path

from .. import config
from ..errors import Finding
from ..services.runfs import RunPaths, WorkspacePaths, WriteGuard, canonical_json, sha256_bytes, sha256_file
from ..services.yaml_schema import load_schema, parse_front_matter, validate
from ..state import PoolIndex, ProcessDef, SkillEntry, SnippetEntry


def _front_matter_findings(meta: dict, schema_name: str, rel: str, code: str, item: "str | None") -> list[Finding]:
    return [Finding(code=code, message=f"front matter {'.'.join(str(x) for x in p) or '<root>'}: {m}",
                    file=rel, line=1, item=item)
            for p, m in validate(meta, load_schema(schema_name))]


def build_index(ws: WorkspacePaths) -> "tuple[PoolIndex, list[Finding]]":
    findings: list[Finding] = []
    snippets: dict[str, SnippetEntry] = {}
    skills: dict[str, SkillEntry] = {}

    if ws.snippet_dir.is_dir():
        for p in sorted(x for x in ws.snippet_dir.rglob("*.md") if x.is_file()):
            rel = ws.rel(p)
            raw = p.read_bytes()
            try:
                text = raw.decode("utf-8")
            except UnicodeDecodeError:
                findings.append(Finding(code="snippet_invalid", message="snippet is not UTF-8 text",
                                        file=rel, line=1))
                continue
            meta, body, err = parse_front_matter(text)
            if meta is None:
                findings.append(Finding(code="snippet_invalid", message=err or "invalid front matter",
                                        file=rel, line=1))
                continue
            bad = _front_matter_findings(meta, "snippet_front_matter", rel, "snippet_invalid", meta.get("id"))
            if bad:
                findings.extend(bad)
                continue
            sid = meta["id"]
            if sid in snippets:
                findings.append(Finding(code="snippet_invalid",
                                        message=f"snippet id {sid!r} is also used by {snippets[sid].path}",
                                        file=rel, line=1, item=sid))
                continue
            snippets[sid] = SnippetEntry(
                id=sid, layer=meta["layer"], tags=tuple(meta.get("tags") or ()),
                auto=bool(meta.get("auto", False)), path=rel, sha256=sha256_bytes(raw), body=body)

    if ws.skills_dir.is_dir():
        for d in sorted(x for x in ws.skills_dir.iterdir() if x.is_dir()):
            rel_dir = ws.rel(d)
            skill_md = d / "SKILL.md"
            if not skill_md.is_file():
                findings.append(Finding(code="skill_invalid", message="skill folder has no SKILL.md",
                                        file=rel_dir, item=d.name))
                continue
            rel = ws.rel(skill_md)
            try:
                text = skill_md.read_text(encoding="utf-8")
            except UnicodeDecodeError:
                findings.append(Finding(code="skill_invalid", message="SKILL.md is not UTF-8 text",
                                        file=rel, line=1, item=d.name))
                continue
            meta, _body, err = parse_front_matter(text)
            if meta is None:
                findings.append(Finding(code="skill_invalid", message=err or "invalid front matter",
                                        file=rel, line=1, item=d.name))
                continue
            bad = _front_matter_findings(meta, "skill_front_matter", rel, "skill_invalid", d.name)
            if not bad and meta["name"] != d.name:
                bad = [Finding(code="skill_invalid",
                               message=f"skill name {meta['name']!r} must equal its folder name {d.name!r}",
                               file=rel, line=1, item=d.name)]
            if bad:
                findings.extend(bad)
                continue
            files = tuple(sorted(
                (f.relative_to(d).as_posix(), sha256_file(f)) for f in d.rglob("*") if f.is_file()))
            skills[meta["name"]] = SkillEntry(
                name=meta["name"], description=meta["description"], tags=tuple(meta.get("tags") or ()),
                dir=rel_dir, files=files)
    return PoolIndex(skills=skills, snippets=snippets), findings


def write_index(index: PoolIndex, paths: RunPaths, guard: WriteGuard) -> None:
    """Write ``.agent/<runID>/index.json`` (SHA-256 of every pool file); pool files are not touched."""
    from ..services.runfs import write_atomic
    write_atomic(paths.index_json, canonical_json(index.to_json()), guard=guard)


def validate_against_pools(
        proc: ProcessDef, index: PoolIndex, *,
        system_md: "Callable[[ProcessDef, PoolIndex], str] | None" = None) -> list[Finding]:
    """Declared skills/snippets exist with the right layer; exclusions are valid; an agent snippet exists.

    ``system_md`` (inject ``compose.compose_system_md``) enables the L0+L1 cap check.
    """
    out: list[Finding] = []

    def fd(code, msg, *, step=None, line=None, item=None):
        out.append(Finding(code=code, message=msg, file=proc.source_file, line=line, step=step, item=item))

    if not any(s.layer == "agent" for s in index.snippets.values()):
        fd("no_agent_snippet", "the snippet pool holds no snippet of the agent layer")

    def check_snippets(ids, layer: str, step: "str | None", line: "int | None") -> None:
        for sid in ids:
            e = index.snippets.get(sid)
            where = f"step {step!r}" if step else "runtime"
            if e is None:
                fd("snippet_missing", f"{where}: declared snippet {sid!r} not found (or invalid) in the pool",
                   step=step, line=line, item=sid)
            elif e.layer != layer:
                fd("snippet_layer", f"{where}: snippet {sid!r} has layer {e.layer!r}, expected {layer!r}",
                   step=step, line=line, item=sid)

    def check_skills(names, step: "str | None", line: "int | None") -> None:
        for n in names:
            if n not in index.skills:
                where = f"step {step!r}" if step else "runtime"
                fd("skill_missing", f"{where}: declared skill {n!r} not found (or invalid) in the pool",
                   step=step, line=line, item=n)

    check_snippets(proc.runtime.snippets, "process", None, None)
    check_skills(proc.runtime.skills, None, None)
    for s in proc.steps:
        check_snippets(s.snippets, "step", s.name, s.line)
        check_skills(s.skills, s.name, s.line)
        for n in s.exclude_skills:
            if n not in index.skills:
                fd("exclude_invalid", f"step {s.name!r}: excluded skill {n!r} is not in the pool",
                   step=s.name, line=s.line, item=n)
            elif n in s.skills:
                fd("exclude_invalid", f"step {s.name!r} both declares and excludes skill {n!r}",
                   step=s.name, line=s.line, item=n)

    if system_md is not None:
        try:
            text = system_md(proc, index)
        except Exception as e:  # compose refuses an incoherent pool; report instead of crashing S3
            fd("layer_cap", f"system.md cannot be composed: {e}")
        else:
            tokens = math.ceil(len(text.encode("utf-8")) / config.CHARS_PER_TOKEN)
            cap = config.LAYER_CAPS["L0"] + config.LAYER_CAPS["L1"]
            if tokens > cap:
                fd("layer_cap", f"system.md (L0+L1) is {tokens} tokens, over the cap of {cap}")
    return out
