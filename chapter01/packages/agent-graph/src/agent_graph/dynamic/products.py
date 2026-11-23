"""Products of a node: presence and schema checks, references, staging of output targets,
the output check registry and template conformance.

Covers: SFR-AGG-015 (a node is done only when every declared product exists and matches its
schema; a later node gets only the products it reads), SFR-AGG-030 (check registry, template
conformance; used by E5), decision 14 (output targets are written to work/<node>/<target.path>
and copied to .output/<runID>/, ``<placeholder>`` segments are glob wildcards).
"""

import dataclasses
import json
import re
import shutil
from pathlib import Path
from typing import Any

import yaml

from ..services.runfs import RunPaths, WriteGuard, sha256_file
from ..services.yaml_schema import load_schema, validate
from ..state import ProcessDef, ProductDecl, ProductRef, StepDef

_PLACEHOLDER = re.compile(r"<[^<>/]+>")
_PRINT = re.compile(r"PRINT\(\s*(['\"])(.*?)\1\s*\)")


def product_file(proc: ProcessDef, decl: ProductDecl) -> str:
    """Relative file of a product inside work/<node>/ (an output target's path if the declaration has none)."""
    if decl.file:
        return decl.file
    if decl.output_target:
        for t in proc.targets:
            if t.name == decl.output_target:
                return t.path
    return decl.name


def resolve_files(work: Path, rel: str) -> "list[Path]":
    """Files matching ``rel`` under ``work`` (``<placeholder>`` segments become ``*``), sorted."""
    pattern = _PLACEHOLDER.sub("*", rel)
    if pattern == rel:
        p = Path(work) / rel
        return [p] if p.is_file() else []
    return sorted(p for p in Path(work).glob(pattern) if p.is_file())


def _load_data(path: Path) -> Any:
    text = path.read_text(encoding="utf-8")
    if path.suffix == ".json":
        return json.loads(text)
    if path.suffix in (".yml", ".yaml"):
        return yaml.safe_load(text)
    raise ValueError("a schema needs a JSON or YAML product")


def _schema(ws_root: Path, name: str) -> dict:
    p = Path(ws_root) / name
    return load_schema(str(p)) if p.is_file() else load_schema(name)


def check_products(proc: ProcessDef, step: StepDef, work: Path, ws_root: Path) -> "list[str]":
    """Problems (empty when every declared product exists and matches its schema)."""
    problems: list[str] = []
    for decl in step.produces:
        rel = product_file(proc, decl)
        files = resolve_files(work, rel)
        if not files:
            problems.append(f"product '{decl.name}': file {rel} is missing in the work folder")
            continue
        if not decl.schema:
            continue
        try:
            schema = _schema(ws_root, decl.schema)
        except Exception as exc:  # noqa: BLE001
            problems.append(f"product '{decl.name}': schema {decl.schema} cannot be loaded: {exc}")
            continue
        for f in files:
            try:
                data = _load_data(f)
            except Exception as exc:  # noqa: BLE001
                problems.append(f"product '{decl.name}': {f.name} cannot be parsed: {exc}")
                continue
            for path, msg in validate(data, schema):
                where = "/".join(str(p) for p in path) or "(root)"
                problems.append(f"product '{decl.name}': {f.name} at {where}: {msg}")
    return problems


def make_refs(paths: RunPaths, proc: ProcessDef, step: StepDef, work: Path) -> "dict[str, dict]":
    """name -> ProductRef (as dict) of the checked products of ``step``."""
    refs: dict[str, dict] = {}
    for decl in step.produces:
        files = resolve_files(work, product_file(proc, decl))
        if not files:
            continue
        f = files[0]
        refs[decl.name] = dataclasses.asdict(ProductRef(
            name=decl.name, node=step.name, path=paths.ws.rel(f), sha256=sha256_file(f)))
    return refs


def stage_outputs(paths: RunPaths, proc: ProcessDef, step: StepDef, work: Path,
                  guard: WriteGuard) -> "list[str]":
    """Copy the output-target products of ``step`` to .output/<runID>/ (decision 14)."""
    copied: list[str] = []
    for decl in step.produces:
        if not decl.output_target:
            continue
        for f in resolve_files(work, product_file(proc, decl)):
            dest = paths.output_dir / f.relative_to(work)
            guard.check(dest)
            dest.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(f, dest)
            copied.append(paths.ws.rel(dest))
    return copied


# ------------------------------------------------------------------ output checks (E5)

def check_output_text(check: str, text: str) -> "str | None":
    """Run one check (``non_empty``, ``max_words:N``, ``min_words:N``, ``json_valid``,
    ``yaml_valid``). Returns the failure message or None."""
    name, _, arg = check.partition(":")
    name = name.strip()
    if name == "non_empty":
        return None if text.strip() else "output is empty"
    if name in ("max_words", "min_words"):
        try:
            n = int(arg)
        except ValueError:
            return f"check {check!r} needs an integer argument"
        words = len(text.split())
        if name == "max_words":
            return None if words <= n else f"{words} words exceed max_words {n}"
        return None if words >= n else f"{words} words are below min_words {n}"
    if name == "json_valid":
        try:
            json.loads(text)
        except ValueError as exc:
            return f"invalid JSON: {exc}"
        return None
    if name == "yaml_valid":
        try:
            yaml.safe_load(text)
        except yaml.YAMLError as exc:
            return f"invalid YAML: {exc}"
        return None
    return f"unknown check {check!r}"


def _headings(text: str) -> "list[str]":
    return [m.group(1).strip() for m in re.finditer(r"^#{1,6}\s+(.*?)\s*$", text, re.M)]


def check_template(template_path: str, template_text: str, text: str) -> "str | None":
    """Template conformance: markdown needs every heading of the template in order; JSON and
    YAML need the top-level keys of the template."""
    suffix = Path(template_path).suffix
    if suffix in (".json", ".yml", ".yaml"):
        try:
            want = json.loads(template_text) if suffix == ".json" else yaml.safe_load(template_text)
            have = json.loads(text) if suffix == ".json" else yaml.safe_load(text)
        except Exception as exc:  # noqa: BLE001
            return f"cannot parse for the template check: {exc}"
        if not isinstance(want, dict):
            return None
        if not isinstance(have, dict):
            return "output is not a mapping like its template"
        missing = [k for k in want if k not in have]
        return f"missing top-level keys: {', '.join(map(str, missing))}" if missing else None
    have_h = iter(_headings(text))
    missing_h = []
    for h in _headings(template_text):
        if not any(h == x for x in have_h):     # consumes the iterator: order is enforced
            missing_h.append(h)
            have_h = iter(())
    return f"missing or misplaced headings: {', '.join(missing_h)}" if missing_h else None


# ------------------------------------------------------------------ step DSL notices

def print_texts(dsl: "str | None", detail: "str | None" = None) -> "list[str]":
    """Texts of the PRINT('...') lines of a success/failure/warning block; ``<ERROR_DETAILS>``
    becomes ``detail``."""
    if not dsl:
        return []
    out = []
    for m in _PRINT.finditer(dsl):
        text = m.group(2)
        if detail is not None:
            text = text.replace("<ERROR_DETAILS>", detail)
        out.append(text)
    return out
