"""Process file -> ProcessDef with line numbers (SFR-AGG-005, SSR-AGG-013).

Structure is checked against ``schemas/process_v1.schema.yml``; an unknown ``apiVersion``
(for example the prototype ``v0``) yields only the ``api_version`` finding. Durations
(``10m``) become seconds and token counts (``40k``) become tokens. ``run`` is split with
``shlex`` into an argument list (never handed to a shell). Semantic checks are in
``validate.py``, ``transitions.py`` and ``dataflow.py``.
"""

import shlex
from pathlib import Path
from typing import Any

from .. import config
from ..errors import Finding
from ..services.runfs import WorkspacePaths, sha256_bytes
from ..services.yaml_schema import YamlDoc, load_schema, load_yaml_with_lines, validate
from ..state import (
    Budgets, Edge, FileRef, InputSource, OutputTarget, ProcessDef, ProductDecl, RuntimeCfg, StepDef)
from . import refs


def rel_path(ws: WorkspacePaths, path: Path) -> str:
    try:
        return ws.rel(path)
    except ValueError:
        return str(path)


def parse_duration(v: Any) -> "int | None":
    """``600`` or ``"10m"`` / ``"30s"`` / ``"2h"`` -> seconds."""
    if v is None:
        return None
    if isinstance(v, int):
        return v
    return int(v[:-1]) * {"s": 1, "m": 60, "h": 3600}[v[-1]]


def parse_tokens(v: Any) -> "int | None":
    """``40000`` or ``"40k"`` / ``"40"`` -> tokens."""
    if v is None:
        return None
    if isinstance(v, int):
        return v
    s = str(v)
    return int(s[:-1]) * 1000 if s[-1] in "kK" else int(s)


def _schema_finding(path: tuple, msg: str, rel: str, doc: YamlDoc, data: Any) -> Finding:
    code = "schema"
    step = None
    if path == ("apiVersion",) or (not path and "'apiVersion'" in msg):
        code = "api_version"
    elif len(path) >= 2 and path[0] == "steps" and isinstance(path[1], int):
        try:
            step = data["steps"][path[1]].get("name")
        except (KeyError, IndexError, AttributeError, TypeError):
            step = None
        if (len(path) == 3 and path[2] == "type") or (len(path) == 2 and "'type'" in msg):
            code = "step_type"
    where = ".".join(str(p) for p in path) or "<root>"
    return Finding(code=code, message=f"{where}: {msg}", file=rel, line=doc.line_of(path),
                   step=step if isinstance(step, str) else None)


def load_process(path: Path, ws: WorkspacePaths) -> "tuple[ProcessDef | None, list[Finding]]":
    """Load and structurally validate one process file. (None, findings) when invalid."""
    path = Path(path)
    rel = rel_path(ws, path)
    try:
        raw = path.read_bytes()
        text = raw.decode("utf-8")
    except (OSError, UnicodeDecodeError) as e:
        return None, [Finding(code="yaml", message=f"cannot read process file: {e}", file=rel, line=None)]
    doc, fs = load_yaml_with_lines(text, rel)
    if doc is None:
        return None, fs
    data = doc.data
    if not isinstance(data, dict):
        return None, [Finding(code="schema", message="process file must be a mapping", file=rel, line=1)]

    errors = validate(data, load_schema("process_v1"))
    findings = [_schema_finding(p, m, rel, doc, data) for p, m in errors]
    api = [f for f in findings if f.code == "api_version"]
    if api:
        return None, api
    if findings:
        return None, findings

    proc, findings = _build(data, doc, rel, sha256_bytes(raw))
    if findings:
        return None, findings
    return proc, []


def _build(data: dict, doc: YamlDoc, rel: str, sha: str) -> "tuple[ProcessDef | None, list[Finding]]":
    findings: list[Finding] = []
    L = doc.line_of

    inp = data.get("input") or {}
    sources = tuple(
        InputSource(name=s["name"], path=s.get("path"), note=s.get("note"), schema=s.get("schema"),
                    required=s.get("required", True), line=L(("input", "source", i)))
        for i, s in enumerate(inp.get("source") or ()))
    templates = tuple(
        FileRef(name=s["name"], path=s["path"], required=s.get("required", True),
                line=L(("input", "output-template-ref", i)))
        for i, s in enumerate(inp.get("output-template-ref") or ()))
    rulesets = tuple(
        FileRef(name=s["name"], path=s["path"], required=s.get("required", True),
                line=L(("input", "ruleset", i)))
        for i, s in enumerate(inp.get("ruleset") or ()))
    targets = tuple(
        OutputTarget(name=t["name"], path=t["path"], template=t.get("template"),
                     checks=tuple(t.get("checks") or ()), required=t.get("required", True),
                     line=L(("output", "target", i)))
        for i, t in enumerate((data.get("output") or {}).get("target") or ()))
    target_paths = {t.name: t.path for t in targets}

    rt = data.get("runtime") or {}
    bud = rt.get("budgets") or {}
    runtime = RuntimeCfg(
        tools=tuple(rt["tools"]) if "tools" in rt else config.DEFAULT_TOOLS,
        snippets=tuple(rt.get("snippets") or ()), skills=tuple(rt.get("skills") or ()),
        max_skills=rt.get("max_skills", config.DEFAULT_MAX_SKILLS),
        retry=rt.get("retry", config.DEFAULT_RETRY),
        timeout_s=parse_duration(rt.get("timeout")) or config.DEFAULT_STEP_TIMEOUT_S,
        budgets=Budgets(max_steps=bud.get("max_steps", config.DEFAULT_MAX_STEPS),
                        max_llm_calls=bud.get("max_llm_calls", config.DEFAULT_MAX_LLM_CALLS),
                        timeout_s=parse_duration(bud.get("timeout")) or config.DEFAULT_RUN_TIMEOUT_S),
        work_products=rt.get("work_products", config.DEFAULT_WORK_PRODUCTS))

    steps = []
    for si, s in enumerate(data["steps"]):
        name = s["name"]
        reads = []
        for ri, raw in enumerate(s.get("reads") or ()):
            ref = refs.parse_read(raw)
            if ref is None:
                findings.append(Finding(
                    code="bad_ref", message=f"reads entry {raw!r} does not match the reference grammar",
                    file=rel, line=L(("steps", si, "reads", ri)), step=name, item=str(raw)))
            else:
                reads.append(ref)
        produces = []
        for pi, item in enumerate(s.get("produces") or ()):
            pline = L(("steps", si, "produces", pi))
            if isinstance(item, str):
                t = refs.parse_target_ref(item)
                if t is not None:
                    produces.append(ProductDecl(name=t[0], file=target_paths.get(t[0], f"{t[0]}.json"),
                                                schema=None, output_target=t[0], line=pline))
                elif item.startswith("output."):
                    findings.append(Finding(
                        code="bad_ref", message=f"produces entry {item!r} does not match the reference grammar",
                        file=rel, line=pline, step=name, item=item))
                else:
                    produces.append(ProductDecl(name=item, file=f"{item}.json", schema=None,
                                                output_target=None, line=pline))
            else:
                produces.append(ProductDecl(name=item["name"], file=item.get("file") or f"{item['name']}.json",
                                            schema=item.get("schema"), output_target=None, line=pline))
        run = None
        if "run" in s:
            try:
                run = tuple(shlex.split(s["run"]))
            except ValueError as e:
                findings.append(Finding(code="run_syntax", message=f"run command cannot be split: {e}",
                                        file=rel, line=L(("steps", si, "run")), step=name))
        steps.append(StepDef(
            id=s["id"], name=name, type=s["type"], instruction=s["instruction"], reads=tuple(reads),
            produces=tuple(produces), outcomes=tuple(s.get("outcomes") or ()),
            success=s.get("success"), failure=s.get("failure"), warning=s.get("warning"),
            retry=s.get("retry"), timeout_s=parse_duration(s.get("timeout")),
            run=run, isolate=bool(s.get("isolate", False)),
            snippets=tuple(s.get("snippets") or ()), skills=tuple(s.get("skills") or ()),
            exclude_skills=tuple(s.get("exclude_skills") or ()), max_skills=s.get("max_skills"),
            tools=tuple(s["tools"]) if "tools" in s else None, budget=parse_tokens(s.get("budget")),
            line=L(("steps", si)), profile=s.get("profile")))

    tr = data["transitions"]
    edges = []
    for ei, e in enumerate(tr.get("edges") or ()):
        w = refs.parse_when(e.get("when"))
        eline = L(("transitions", "edges", ei))
        if w is None:
            findings.append(Finding(
                code="when_invalid",
                message=f"edge condition {e.get('when')!r} must be done, hil_ok, hil_nok or 'outcome == <value>'",
                file=rel, line=L(("transitions", "edges", ei, "when")) or eline, step=e["from"]))
            continue
        edges.append(Edge(src=e["from"], dst=e["to"], when=w[0], outcome=w[1], line=eline))
    if findings:
        return None, findings

    proc = ProcessDef(
        api_version=data["apiVersion"], id=data["id"], title=data["title"],
        description=str(data.get("description") or "").strip(),
        sources=sources, templates=templates, rulesets=rulesets, targets=targets,
        runtime=runtime, steps=tuple(steps), entry=tr["entry"], edges=tuple(edges),
        source_file=rel, source_sha256=sha)
    return proc, []
