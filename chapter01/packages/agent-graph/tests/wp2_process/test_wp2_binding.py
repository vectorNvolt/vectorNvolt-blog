"""SFR-AGG-009, SSR-AGG-008: candidate sets, binding record, input checks."""

import json

from agent_graph.process.binding import check_binding, find_candidates, make_binding, single_choices
from agent_graph.process.loader import load_process
from agent_graph.state import Binding
from fakes.workspace import PROCESS_YML, add_process, write_file


def mut(old: str, new: str) -> str:
    assert old in PROCESS_YML, old
    return PROCESS_YML.replace(old, new, 1)


def load(ws, text=None):
    if text is not None:
        add_process(ws, "example.yml", text)
    proc, fs = load_process(ws.process_dir / "example.yml", ws)
    assert proc is not None, fs
    return proc


def codes(fs):
    return [f.code for f in fs]


def test_single_set_binds_and_checks_clean(workspace):
    proc = load(workspace)
    cands = find_candidates(proc, workspace)
    assert cands == {"Source": [(".input/example/source.md",)]}
    chosen = single_choices(cands)
    b = make_binding(proc, workspace, chosen, {})
    assert b.process_id == "example" and b.process_sha256 == proc.source_sha256
    assert [f.path for f in b.sources["Source"].files] == [".input/example/source.md"]
    assert b.sources["Source"].files[0].sha256
    assert b.templates["Template"].path == "templates/report.md"
    assert b.allowed_input_paths() == {".input/example/source.md", "templates/report.md"}
    assert check_binding(proc, b, workspace) == []
    assert Binding.from_json(json.loads(json.dumps(b.to_json()))) == b


def test_several_candidate_sets_are_never_chosen_automatically(workspace):
    write_file(workspace.root, ".input/a/x.md", "x")
    write_file(workspace.root, ".input/b/y.md", "y")
    write_file(workspace.root, ".input/b/z.md", "z")
    proc = load(workspace, mut('path: "example/*.md"', 'path: "**/*.md"'))
    cands = find_candidates(proc, workspace)["Source"]
    assert cands == [(".input/a/x.md",), (".input/b/y.md", ".input/b/z.md"), (".input/example/source.md",)]
    assert single_choices({"Source": cands}) == {}
    unchosen = make_binding(proc, workspace, {}, {})
    assert codes(check_binding(proc, unchosen, workspace)) == ["set_unchosen"]
    chosen = make_binding(proc, workspace, {"Source": cands[1]}, {})
    assert check_binding(proc, chosen, workspace) == []
    assert chosen.allowed_input_paths() >= {".input/b/y.md", ".input/b/z.md"}
    assert ".input/a/x.md" not in chosen.allowed_input_paths()


def test_unsafe_glob_matches_nothing(workspace):
    proc = load(workspace, mut('path: "example/*.md"', 'path: "../*.md"'))
    assert find_candidates(proc, workspace) == {"Source": []}


def test_missing_required_source(workspace):
    (workspace.root / ".input/example/source.md").unlink()
    proc = load(workspace)
    b = make_binding(proc, workspace, single_choices(find_candidates(proc, workspace)), {})
    fs = check_binding(proc, b, workspace)
    assert codes(fs) == ["input_missing"] and fs[0].item == "Source" and fs[0].line is not None


def test_file_changed_after_binding(workspace):
    proc = load(workspace)
    b = make_binding(proc, workspace, single_choices(find_candidates(proc, workspace)), {})
    (workspace.root / ".input/example/source.md").write_text("changed\n")
    fs = check_binding(proc, b, workspace)
    assert codes(fs) == ["input_missing"] and "changed" in fs[0].message


def test_missing_template(workspace):
    (workspace.root / "templates/report.md").unlink()
    proc = load(workspace)
    b = make_binding(proc, workspace, single_choices(find_candidates(proc, workspace)), {})
    fs = check_binding(proc, b, workspace)
    assert codes(fs) == ["input_missing"] and fs[0].item == "Template"


def test_note_source_needs_an_answer(workspace):
    text = mut("  output-template-ref:\n",
               '    - {name: Ask, note: "Which goal?", required: true}\n  output-template-ref:\n')
    proc = load(workspace, text)
    chosen = single_choices(find_candidates(proc, workspace))
    assert "Ask" not in find_candidates(proc, workspace)
    fs = check_binding(proc, make_binding(proc, workspace, chosen, {}), workspace)
    assert codes(fs) == ["input_missing"] and fs[0].item == "Ask"
    b = make_binding(proc, workspace, chosen, {"Ask": "summarize it"})
    assert b.sources["Ask"].note_text == "summarize it"
    assert check_binding(proc, b, workspace) == []


def test_source_schema_is_checked(workspace):
    write_file(workspace.root, "schemas/s.json", json.dumps({"type": "object", "required": ["b"]}))
    write_file(workspace.root, ".input/example/data.json", json.dumps({"a": 1}))
    (workspace.root / ".input/example/source.md").unlink()
    text = mut('"example/*.md", required: true}', '"example/*.json", schema: schemas/s.json, required: true}')
    proc = load(workspace, text)
    b = make_binding(proc, workspace, single_choices(find_candidates(proc, workspace)), {})
    fs = check_binding(proc, b, workspace)
    assert codes(fs) == ["input_schema"] and "'b'" in fs[0].message and fs[0].file == ".input/example/data.json"
    write_file(workspace.root, ".input/example/data.json", json.dumps({"b": 1}))
    b = make_binding(proc, workspace, single_choices(find_candidates(proc, workspace)), {})
    assert check_binding(proc, b, workspace) == []


def test_binding_made_for_other_process_version_is_stale(workspace):
    proc = load(workspace)
    b = make_binding(proc, workspace, single_choices(find_candidates(proc, workspace)), {})
    proc2 = load(workspace, PROCESS_YML + "\n# edited\n")
    assert "binding_stale" in codes(check_binding(proc2, b, workspace))
