"""YAML with line numbers, a JSON-Schema subset validator and front matter parsing.

Covers: SFR-AGG-005 (YAML errors and findings with line numbers), SFR-AGG-006
(front matter of snippets and skills), SFR-AGG-015 (product schema checks).

No ``jsonschema`` dependency: the validator below supports exactly this subset:
type, properties, required, additionalProperties (bool|schema), items, enum, const,
minItems, maxItems, minLength, maxLength, pattern, minimum, maximum, anyOf, oneOf,
and ``$ref`` to local ``#/$defs/<name>``. Other keywords are ignored.

Spec (authoritative signatures)::

    @dataclass(frozen=True) class YamlDoc: data: Any; line_of: Callable[[tuple], int|None]
    load_yaml_with_lines(text, file) -> (YamlDoc|None, list[Finding])
    load_schema(name_or_path) -> dict
    validate(instance, schema, *, path_prefix=()) -> list[(path tuple, message)]
    parse_front_matter(text) -> (meta|None, body, error|None)
"""

import copy
import json
import re
from dataclasses import dataclass
from functools import lru_cache
from pathlib import Path
from typing import Any, Callable

import yaml

from ..errors import Finding

SCHEMA_DIR = Path(__file__).resolve().parent.parent / "schemas"


@dataclass(frozen=True)
class YamlDoc:
    data: Any
    line_of: Callable[[tuple], "int | None"]
    # line_of(path): 1-based line of the value at path (keys are str, list indexes are int).
    # For a mapping entry the line of its KEY is returned. A path that does not exist
    # resolves to the line of its deepest existing ancestor (so "missing key" findings
    # point at the enclosing mapping). The empty path is the document root.


def _make_line_of(root: "yaml.Node | None") -> Callable[[tuple], "int | None"]:
    def line_of(path: tuple) -> "int | None":
        if root is None:
            return None
        node = root
        line = node.start_mark.line + 1
        for part in path:
            nxt = None
            if isinstance(node, yaml.MappingNode) and isinstance(part, str):
                for k, v in node.value:
                    if isinstance(k, yaml.ScalarNode) and k.value == part:
                        nxt = v
                        line = k.start_mark.line + 1
                        break
            elif isinstance(node, yaml.SequenceNode) and isinstance(part, int):
                if 0 <= part < len(node.value):
                    nxt = node.value[part]
                    line = nxt.start_mark.line + 1
            if nxt is None:
                return line
            node = nxt
        return line

    return line_of


def load_yaml_with_lines(text: str, file: str) -> "tuple[YamlDoc | None, list[Finding]]":
    """Parse one YAML document. On syntax error returns (None, [Finding(code="yaml", ...)]).
    An empty document gives YamlDoc(data=None)."""
    loader = yaml.SafeLoader(text)
    try:
        node = loader.get_single_node()
        data = loader.construct_document(node) if node is not None else None
    except yaml.YAMLError as e:
        mark = getattr(e, "problem_mark", None) or getattr(e, "context_mark", None)
        line = mark.line + 1 if mark is not None else None
        problem = getattr(e, "problem", None) or str(e)
        return None, [Finding(code="yaml", message=f"invalid YAML: {problem}", file=file, line=line)]
    finally:
        loader.dispose()
    return YamlDoc(data=data, line_of=_make_line_of(node)), []


def load_schema(name_or_path: str) -> dict:
    """Packaged schema by name ("process_v1" -> schemas/process_v1.schema.yml, or
    "process_v1.schema.yml"), else a path (.json or .yml/.yaml)."""
    for cand in (SCHEMA_DIR / f"{name_or_path}.schema.yml", SCHEMA_DIR / f"{name_or_path}.yml",
                 SCHEMA_DIR / name_or_path):
        if cand.is_file():
            return _load_schema_file(cand)
    p = Path(name_or_path)
    if p.is_file():
        return _load_schema_file(p)
    raise FileNotFoundError(f"schema not found: {name_or_path}")


def _load_schema_file(p: Path) -> dict:
    """Parsed schema, cached per (path, mtime, size) so a schema edited during the run (e.g. a
    workspace product schema changed in S7) is re-read. Callers get a deep copy: the cached
    value is never shared, so mutating the result cannot corrupt later loads."""
    st = p.stat()
    return copy.deepcopy(_parse_schema_file(str(p.resolve()), st.st_mtime_ns, st.st_size))


@lru_cache(maxsize=64)
def _parse_schema_file(path: str, mtime_ns: int, size: int) -> dict:
    p = Path(path)
    text = p.read_text(encoding="utf-8")
    data = json.loads(text) if p.suffix == ".json" else yaml.safe_load(text)
    if not isinstance(data, dict):
        raise ValueError(f"schema {p} is not a mapping")
    return data


_TYPE_CHECKS: dict[str, Callable[[Any], bool]] = {
    "object": lambda v: isinstance(v, dict),
    "array": lambda v: isinstance(v, list),
    "string": lambda v: isinstance(v, str),
    "integer": lambda v: isinstance(v, int) and not isinstance(v, bool),
    "number": lambda v: isinstance(v, (int, float)) and not isinstance(v, bool),
    "boolean": lambda v: isinstance(v, bool),
    "null": lambda v: v is None,
}


def validate(instance: Any, schema: dict, *, path_prefix: tuple = ()) -> "list[tuple[tuple, str]]":
    """Validate ``instance``; returns [(path, message)], empty when valid.
    ``path`` is a tuple of mapping keys / list indexes, prefixed with ``path_prefix``."""
    errors: list[tuple[tuple, str]] = []
    _check(instance, schema, schema, tuple(path_prefix), errors)
    return errors


def _resolve_ref(ref: str, root: dict) -> dict:
    if not ref.startswith("#/"):
        raise ValueError(f"unsupported $ref: {ref}")
    node: Any = root
    for part in ref[2:].split("/"):
        node = node[part]
    return node


def _type_name(v: Any) -> str:
    for name in ("null", "boolean", "integer", "number", "string", "array", "object"):
        if _TYPE_CHECKS[name](v):
            return name
    return type(v).__name__


def _check(inst: Any, schema: Any, root: dict, path: tuple, errors: list) -> None:
    if not isinstance(schema, dict):
        return
    if "$ref" in schema:
        _check(inst, _resolve_ref(schema["$ref"], root), root, path, errors)
        return

    if "type" in schema:
        types = schema["type"] if isinstance(schema["type"], list) else [schema["type"]]
        if not any(_TYPE_CHECKS[t](inst) for t in types):
            errors.append((path, f"expected type {' or '.join(types)}, got {_type_name(inst)}"))
            return

    if "const" in schema and inst != schema["const"]:
        errors.append((path, f"must equal {schema['const']!r}"))
    if "enum" in schema and inst not in schema["enum"]:
        errors.append((path, f"must be one of {schema['enum']!r}"))

    if "anyOf" in schema:
        if not any(not _sub_errors(inst, s, root, path) for s in schema["anyOf"]):
            errors.append((path, "does not match any allowed alternative"))
    if "oneOf" in schema:
        n = sum(1 for s in schema["oneOf"] if not _sub_errors(inst, s, root, path))
        if n != 1:
            errors.append((path, f"must match exactly one alternative, matches {n}"))

    if isinstance(inst, str):
        if "minLength" in schema and len(inst) < schema["minLength"]:
            errors.append((path, f"shorter than {schema['minLength']} characters"))
        if "maxLength" in schema and len(inst) > schema["maxLength"]:
            errors.append((path, f"longer than {schema['maxLength']} characters"))
        if "pattern" in schema and re.search(schema["pattern"], inst) is None:
            errors.append((path, f"does not match pattern {schema['pattern']!r}"))

    if _TYPE_CHECKS["number"](inst):
        if "minimum" in schema and inst < schema["minimum"]:
            errors.append((path, f"below minimum {schema['minimum']}"))
        if "maximum" in schema and inst > schema["maximum"]:
            errors.append((path, f"above maximum {schema['maximum']}"))

    if isinstance(inst, list):
        if "minItems" in schema and len(inst) < schema["minItems"]:
            errors.append((path, f"fewer than {schema['minItems']} items"))
        if "maxItems" in schema and len(inst) > schema["maxItems"]:
            errors.append((path, f"more than {schema['maxItems']} items"))
        if "items" in schema:
            for i, item in enumerate(inst):
                _check(item, schema["items"], root, path + (i,), errors)

    if isinstance(inst, dict):
        props = schema.get("properties", {})
        for key in schema.get("required", ()):
            if key not in inst:
                errors.append((path, f"missing required property {key!r}"))
        for key, sub in props.items():
            if key in inst:
                _check(inst[key], sub, root, path + (key,), errors)
        addl = schema.get("additionalProperties", True)
        for key in inst:
            if key in props:
                continue
            if addl is False:
                errors.append((path + (key,), f"additional property {key!r} is not allowed"))
            elif isinstance(addl, dict):
                _check(inst[key], addl, root, path + (key,), errors)


def _sub_errors(inst: Any, schema: Any, root: dict, path: tuple) -> list:
    errs: list = []
    _check(inst, schema, root, path, errs)
    return errs


def parse_front_matter(text: str) -> "tuple[dict | None, str, str | None]":
    """Split ``---`` delimited YAML front matter from the body.
    Returns (meta, body, error); on error meta is None and body is the whole text."""
    src = text[1:] if text.startswith("﻿") else text
    lines = src.splitlines(keepends=True)
    if not lines or lines[0].rstrip("\r\n") != "---":
        return None, text, "missing front matter (file must start with '---')"
    for i in range(1, len(lines)):
        if lines[i].rstrip("\r\n") == "---":
            raw = "".join(lines[1:i])
            body = "".join(lines[i + 1:])
            try:
                meta = yaml.safe_load(raw) if raw.strip() else {}
            except yaml.YAMLError as e:
                return None, text, f"front matter is not valid YAML: {getattr(e, 'problem', None) or e}"
            if not isinstance(meta, dict):
                return None, text, "front matter must be a mapping"
            return meta, body, None
    return None, text, "front matter is not closed (missing second '---')"
