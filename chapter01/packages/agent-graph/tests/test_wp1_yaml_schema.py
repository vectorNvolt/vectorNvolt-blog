"""YAML with line numbers, JSON-Schema subset, packaged schemas, front matter (SFR-AGG-005/006)."""

import pytest
import yaml

from agent_graph.services.yaml_schema import (
    SCHEMA_DIR, load_schema, load_yaml_with_lines, parse_front_matter, validate)
from fakes.workspace import PROCESS_YML


def test_line_numbers():
    text = "a: 1\nb:\n  - x\n  - y: 2\n    z: 3\nc:\n  d: 4\n"
    doc, findings = load_yaml_with_lines(text, "f.yml")
    assert findings == [] and doc.data == {"a": 1, "b": ["x", {"y": 2, "z": 3}], "c": {"d": 4}}
    assert doc.line_of(()) == 1
    assert doc.line_of(("a",)) == 1
    assert doc.line_of(("b",)) == 2
    assert doc.line_of(("b", 0)) == 3
    assert doc.line_of(("b", 1, "z")) == 5
    assert doc.line_of(("c", "d")) == 7
    assert doc.line_of(("c", "missing")) == 6          # deepest existing ancestor
    assert doc.line_of(("b", 9)) == 2


def test_yaml_error_finding():
    doc, findings = load_yaml_with_lines("a: [1, 2\nb: 3\n", "bad.yml")
    assert doc is None
    assert len(findings) == 1
    f = findings[0]
    assert f.code == "yaml" and f.file == "bad.yml" and isinstance(f.line, int) and f.line >= 1


def test_empty_document():
    doc, findings = load_yaml_with_lines("", "e.yml")
    assert findings == [] and doc.data is None and doc.line_of(("a",)) is None


S = {
    "type": "object", "required": ["name"], "additionalProperties": False,
    "properties": {
        "name": {"type": "string", "minLength": 2, "maxLength": 5, "pattern": "^[a-z]+$"},
        "n": {"type": "integer", "minimum": 1, "maximum": 3},
        "kind": {"enum": ["a", "b"]},
        "tags": {"type": "array", "items": {"type": "string"}, "minItems": 1, "maxItems": 2},
        "ref": {"$ref": "#/$defs/pos"},
        "any": {"anyOf": [{"type": "integer"}, {"type": "string", "pattern": "^x"}]},
        "one": {"oneOf": [{"type": "integer"}, {"type": "number", "minimum": 0}]},
        "c": {"const": 7},
        "nul": {"type": ["string", "null"]},
        "free": {},
    },
    "$defs": {"pos": {"type": "number", "minimum": 0}},
}


def _errs(inst):
    return validate(inst, S)


def test_validate_valid():
    assert _errs({"name": "abc", "n": 2, "kind": "a", "tags": ["x"], "ref": 1.5, "any": "xy",
                  "c": 7, "nul": None, "free": {"anything": [1]}}) == []


@pytest.mark.parametrize("inst,path", [
    ({}, ()),                                              # required
    ({"name": "a"}, ("name",)),                            # minLength
    ({"name": "abcdef"}, ("name",)),                       # maxLength
    ({"name": "AB"}, ("name",)),                           # pattern
    ({"name": "ab", "n": 0}, ("n",)),                      # minimum
    ({"name": "ab", "n": 4}, ("n",)),                      # maximum
    ({"name": "ab", "n": True}, ("n",)),                   # bool is not integer
    ({"name": "ab", "kind": "z"}, ("kind",)),              # enum
    ({"name": "ab", "tags": []}, ("tags",)),               # minItems
    ({"name": "ab", "tags": ["a", "b", "c"]}, ("tags",)),  # maxItems
    ({"name": "ab", "tags": ["a", 1]}, ("tags", 1)),       # items path index
    ({"name": "ab", "ref": -1}, ("ref",)),                 # $ref
    ({"name": "ab", "any": 1.5}, ("any",)),                # anyOf
    ({"name": "ab", "one": 1}, ("one",)),                  # oneOf matches both
    ({"name": "ab", "c": 8}, ("c",)),                      # const
    ({"name": "ab", "nul": 3}, ("nul",)),                  # type list
    ({"name": "ab", "extra": 1}, ("extra",)),              # additionalProperties false
])
def test_validate_errors(inst, path):
    errs = _errs(inst)
    assert errs, inst
    assert path in [p for p, _m in errs]


def test_validate_prefix_and_additional_schema():
    schema = {"type": "object", "additionalProperties": {"type": "integer"}}
    errs = validate({"a": 1, "b": "x"}, schema, path_prefix=("root", 0))
    assert [p for p, _ in errs] == [("root", 0, "b")]
    assert validate("x", {"type": "object"}) == [((), "expected type object, got string")]


def test_packaged_schemas_load():
    names = {"process_v1", "node_result", "dialog_reply", "snippet_front_matter", "skill_front_matter"}
    assert {p.name.removesuffix(".schema.yml") for p in SCHEMA_DIR.glob("*.schema.yml")} == names
    for n in names:
        assert isinstance(load_schema(n), dict)
        assert load_schema(f"{n}.schema.yml") == load_schema(n)
    with pytest.raises(FileNotFoundError):
        load_schema("nope")


def test_load_schema_from_path(tmp_path):
    (tmp_path / "s.json").write_text('{"type": "integer"}')
    (tmp_path / "s.yml").write_text("type: string\n")
    assert load_schema(str(tmp_path / "s.json")) == {"type": "integer"}
    assert load_schema(str(tmp_path / "s.yml")) == {"type": "string"}


def test_process_schema_accepts_example_process():
    assert validate(yaml.safe_load(PROCESS_YML), load_schema("process_v1")) == []


def test_process_schema_rejections():
    schema = load_schema("process_v1")
    base = yaml.safe_load(PROCESS_YML)

    def errs(mutate):
        d = yaml.safe_load(PROCESS_YML)
        mutate(d)
        return [p for p, _ in validate(d, schema)]

    assert ("apiVersion",) in errs(lambda d: d.update(apiVersion="agent-graph/v0"))
    assert ("steps", 0, "type") in errs(lambda d: d["steps"][0].update(type="exec"))
    assert ("steps", 1, "retry") in errs(lambda d: d["steps"][1].update(retry="two"))
    assert ("steps", 0, "bogus") in errs(lambda d: d["steps"][0].update(bogus=1))
    assert () in errs(lambda d: d.pop("transitions"))
    assert ("runtime", "timeout") in errs(lambda d: d["runtime"].update(timeout="ten minutes"))
    assert ("runtime", "work_products") in errs(lambda d: d["runtime"].update(work_products="zip"))
    assert base["steps"][1]["budget"] == "40k"


def test_dialog_and_result_schemas():
    dr = load_schema("dialog_reply")
    assert validate({"say": "hi", "action": "ask"}, dr) == []
    assert validate({"say": "x", "action": "propose", "process_id": None, "proposals": [
        {"kind": "edit_file", "path": ".process/a.yml", "content": "x", "reason": "fix"},
        {"kind": "override", "key": "retry", "node": None, "value": 3, "reason": "r"}]}, dr) == []
    assert validate({"say": "x", "action": "shout"}, dr)
    assert validate({"say": "x", "action": "propose", "proposals": [{"kind": "edit_file"}]}, dr)

    nr = load_schema("node_result")
    ok = {"status": "done", "outcome": None, "question": None,
          "handoff": {"done": "d", "decisions": [], "open_points": []}, "warnings": []}
    assert validate(ok, nr) == []
    assert validate({"status": "question", "question": "which one?"}, nr) == []
    assert validate({"status": "maybe"}, nr)
    assert validate({**ok, "handoff": {"done": 1}}, nr)


def test_front_matter_schemas():
    sn = load_schema("snippet_front_matter")
    assert validate({"id": "a-b", "layer": "step", "tags": ["x"], "auto": False}, sn) == []
    assert validate({"id": "a", "layer": "user"}, sn)
    assert validate({"layer": "step"}, sn)
    sk = load_schema("skill_front_matter")
    assert validate({"name": "summarize", "description": "d"}, sk) == []
    assert validate({"name": "summarize"}, sk)


def test_parse_front_matter():
    meta, body, err = parse_front_matter("---\nid: a\ntags: [x]\n---\nbody line\n\nmore\n")
    assert err is None and meta == {"id": "a", "tags": ["x"]} and body == "body line\n\nmore\n"
    meta, body, err = parse_front_matter("---\r\nid: a\r\n---\r\nbody\r\n")
    assert err is None and meta == {"id": "a"} and body == "body\r\n"
    assert parse_front_matter("---\n---\nx\n") == ({}, "x\n", None)
    for bad in ("no front matter\n", "---\nid: a\nnever closed\n", "---\n- a\n- b\n---\nx", "---\nid: [\n---\nx"):
        meta, body, err = parse_front_matter(bad)
        assert meta is None and body == bad and err
