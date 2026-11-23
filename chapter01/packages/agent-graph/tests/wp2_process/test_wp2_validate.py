"""SFR-AGG-005 / 007, SSR-AGG-013 / 017: loading and validating the process definition."""

import pytest

from agent_graph.process.dataflow import available_before
from agent_graph.process.loader import load_process
from agent_graph.process.transitions import bfs_order
from agent_graph.process.validate import check_run_argv, validate_process
from fakes.workspace import PROCESS_YML, add_process


def mut(old: str, new: str) -> str:
    assert old in PROCESS_YML, old
    return PROCESS_YML.replace(old, new, 1)


def load(ws, text=None):
    if text is not None:
        add_process(ws, "example.yml", text)
    return load_process(ws.process_dir / "example.yml", ws)


def findings_of(ws, text):
    proc, fs = load(ws, text)
    assert proc is not None, fs
    return validate_process(proc, ws)


def codes(fs):
    return {f.code for f in fs}


def test_example_loads_and_validates(workspace):
    proc, fs = load(workspace)
    assert fs == [] and proc is not None
    assert proc.id == "example" and proc.entry == "extract"
    assert [s.name for s in proc.steps] == ["extract", "write-summary", "review"]
    assert proc.source_file == ".process/example.yml"
    extract = proc.step("extract")
    assert extract.run == ("python", ".process/scripts/extract.py", "{input.Source}", "{work}")
    assert proc.step("write-summary").budget == 40000
    assert proc.runtime.timeout_s == 600 and proc.runtime.budgets.timeout_s == 3600
    assert {(e.src, e.dst, e.when) for e in proc.edges} >= {("review", "END", "hil_ok")}
    assert validate_process(proc, workspace) == []


def test_dataflow_and_order_of_example(workspace):
    proc, _ = load(workspace)
    assert bfs_order(proc) == ("extract", "write-summary", "review")
    avail = available_before(proc)
    assert avail["extract"] == frozenset()
    assert avail["write-summary"] == frozenset({"facts"})
    assert avail["review"] == frozenset({"facts", "Report"})


def test_v0_rejected_with_line(workspace):
    proc, fs = load(workspace, PROCESS_YML.replace("agent-graph/v1", "agent-graph/v0", 1))
    assert proc is None
    assert [f.code for f in fs] == ["api_version"] and fs[0].line == 1


def test_invalid_yaml(workspace):
    proc, fs = load(workspace, "id: [unclosed\nsteps: x\n")
    assert proc is None and fs[0].code == "yaml" and fs[0].line is not None


def test_unknown_step_type_names_line(workspace):
    text = mut("type: human", "type: magic")
    expected = next(i for i, l in enumerate(text.splitlines(), 1) if "type: magic" in l)
    proc, fs = load(workspace, text)
    assert proc is None
    f = next(f for f in fs if f.code == "step_type")
    assert f.line == expected and f.step == "review"


def test_bad_when_and_bad_ref_are_load_findings(workspace):
    _, fs = load(workspace, mut("when: hil_nok}", "when: whenever}"))
    assert "when_invalid" in codes(fs) and all(f.line for f in fs)
    _, fs = load(workspace, mut('"review_feedback?"', '"input.bogus[X"'))
    assert "bad_ref" in codes(fs)


def test_missing_done_edge_and_unreachable(workspace):
    fs = findings_of(workspace, mut("    - {from: extract, to: write-summary, when: done}\n", ""))
    assert {"missing_edge", "unreachable"} <= codes(fs)
    f = next(f for f in fs if f.code == "missing_edge")
    assert f.step == "extract" and f.line is not None


def test_duplicate_edge(workspace):
    fs = findings_of(workspace, PROCESS_YML + "    - {from: extract, to: review, when: done}\n")
    assert "dup_edge" in codes(fs)


def test_human_without_hil_nok(workspace):
    fs = findings_of(workspace, mut("    - {from: review, to: write-summary, when: hil_nok}\n", ""))
    f = next(f for f in fs if f.code == "missing_edge")
    assert f.step == "review" and "hil_nok" in f.message


def test_dead_end(workspace):
    fs = findings_of(workspace, mut("{from: review, to: END, when: hil_ok}", "{from: review, to: extract, when: hil_ok}"))
    assert "dead_end" in codes(fs)


def test_unreachable_extra_step(workspace):
    extra = ("  - id: 4\n    name: orphan\n    type: human\n    instruction: x\n\ntransitions:\n")
    fs = findings_of(workspace, mut("\ntransitions:\n", extra))
    assert any(f.code == "unreachable" and f.step == "orphan" for f in fs)


def test_outcomes_need_one_edge_each(workspace):
    fs = findings_of(workspace, mut("    name: extract\n    type: procedure\n",
                                    "    name: extract\n    type: procedure\n    outcomes: [pass, fail]\n"))
    missing = [f.item for f in fs if f.code == "missing_edge"]
    assert sorted(missing) == ["fail", "pass"]
    assert "bad_when" in codes(fs)


def test_read_not_on_all_paths_and_unknown_product(workspace):
    fs = findings_of(workspace, mut('"review_feedback?"', '"review_feedback"'))
    f = next(f for f in fs if f.code == "read_not_on_all_paths")
    assert f.step == "write-summary" and f.item == "review_feedback"
    fs = findings_of(workspace, mut('"review_feedback?"', '"ghost?"'))
    assert any(f.code == "unresolved_ref" and f.item == "ghost" for f in fs)


def test_unknown_input_reference_and_placeholder(workspace):
    fs = findings_of(workspace, mut('"input.source[Source]"', '"input.source[Nope]"'))
    assert any(f.code == "unresolved_ref" and f.item == "Nope" for f in fs)
    fs = findings_of(workspace, mut("{input.Source}", "{input.Nope}"))
    assert any(f.code == "unresolved_ref" and f.item == "Nope" for f in fs)


def test_argv_not_allowed_in_definition(workspace):
    fs = findings_of(workspace, mut("python .process/scripts/extract.py", "bash .process/scripts/extract.py"))
    f = next(f for f in fs if f.code == "argv_not_allowed")
    assert f.step == "extract"


def test_inline_code_refused_with_finding(workspace):
    """SSR-AGG-013 strict: the definition carries no code; python -c is a finding."""
    fs = findings_of(workspace, mut('"python .process/scripts/extract.py {input.Source} {work}"',
                                    """'python -c "print(1)" {input.Source} {work}'"""))
    f = next(f for f in fs if f.code == "argv_not_allowed")
    assert f.step == "extract" and "inline code (-c)" in f.message and f.item == "python"


def test_script_missing_and_escaping_symlink(workspace, tmp_path):
    fs = findings_of(workspace, mut(".process/scripts/extract.py", ".process/scripts/nope.py"))
    f = next(f for f in fs if f.code == "script_missing")
    assert f.step == "extract" and f.item == ".process/scripts/nope.py"
    outside = tmp_path / "outside.py"
    outside.write_text("print(1)\n")
    (workspace.process_dir / "scripts" / "link.py").symlink_to(outside)
    fs = findings_of(workspace, mut(".process/scripts/extract.py", ".process/scripts/link.py"))
    assert any(f.code == "argv_not_allowed" and "outside" in f.message for f in fs)


def test_scripts_dir_is_write_protected(workspace, guard):
    from agent_graph.errors import WriteBoundaryError
    with pytest.raises(WriteBoundaryError):
        guard.check(workspace.process_dir / "scripts" / "extract.py")
    with guard.resolution(approved=True):
        guard.check(workspace.process_dir / "scripts" / "extract.py")


@pytest.mark.parametrize("argv,ok", [
    (["python", "-c", "print(1)"], False),
    (["python", "-Ic", "print(1)"], False),
    (["python3", "-u", ".process/scripts/x.py", "{work}"], True),
    (["/opt/venv/bin/python", ".process/scripts/sub/x.py"], True),
    (["python", "./.process/scripts/x.py"], True),
    (["python", "scripts/x.py"], False),                 # not under .process/scripts/
    (["python", ".process/x.py"], False),
    (["python", ".process/scripts"], False),
    (["python", "-m", "pkg.mod"], False),
    (["python"], False),
    (["python", "-"], False),
    (["python", "/abs/x.py"], False),
    (["python", "../x.py"], False),
    (["python", ".process/scripts/../../x.py"], False),
    (["python", "{work}/x.py"], False),
    (["python", "-c"], False),
    (["bash", "-c", "ls"], False),
    (["./run.sh"], False),
    ([], False),
])
def test_check_run_argv(argv, ok):
    assert (check_run_argv(argv) is None) is ok


@pytest.mark.parametrize("argv,ok", [
    (["bash", "-c", "ls"], False),
    (["sh", "-ec", "ls"], False),
    (["bash", "-e", ".process/scripts/x.sh"], True),
    (["node", "-e", "1"], False),
    (["node", "--eval=1"], False),
    (["node", "-p", "1"], False),
    (["node", ".process/scripts/x.js"], True),
    (["perl", "-e", "1"], False),
    (["perl", "-ne", "print"], False),
    (["perl", "-Mstrict", ".process/scripts/x.pl"], False),
    (["ruby", "-e", "1"], False),
    (["ruby", ".process/scripts/x.rb"], True),
    (["jq", "-c", "."], True),                           # not an interpreter: -c is not code
])
def test_inline_code_flags_refused_for_every_allowlisted_interpreter(monkeypatch, argv, ok):
    from agent_graph import config
    monkeypatch.setattr(config, "PROCEDURE_ALLOWED_EXECUTABLES",
                        ("python", "bash", "sh", "node", "perl", "ruby", "jq"))
    assert (check_run_argv(argv) is None) is ok


def test_resolve_script_against_workspace_root(tmp_path):
    from agent_graph.process.validate import resolve_script
    assert resolve_script(["python", "-u", ".process/scripts/x.py", "a"], tmp_path) == \
        ["python", "-u", str(tmp_path / ".process/scripts/x.py"), "a"]
    assert resolve_script(["jq", "x"], tmp_path) == ["jq", "x"]


def _isolate_extract(workspace, extra):
    return findings_of(workspace, mut("    name: extract\n    type: procedure\n",
                                      "    name: extract\n    type: procedure\n" + extra))


def test_isolate_needs_a_profile_and_profile_needs_isolate(workspace):
    assert "isolate_profile_required" in codes(_isolate_extract(workspace, "    isolate: true\n"))
    assert "isolate_profile_misplaced" in codes(_isolate_extract(workspace, "    profile: py\n"))
    assert "isolate_profile_required" not in codes(_isolate_extract(workspace, "    isolate: true\n    profile: py\n"))


def _no_tool(monkeypatch):
    from agent_graph.services import container
    monkeypatch.delenv(container.ENV_TOOL, raising=False)
    monkeypatch.setenv("PATH", "")
    monkeypatch.setattr(container, "DEFAULT_TOOL", "/nonexistent/run_ephemeral_container")


def _isolated_text(script):
    text = mut("    name: extract\n    type: procedure\n",
               "    name: extract\n    type: procedure\n    isolate: true\n    profile: py\n")
    old = "python .process/scripts/extract.py"
    assert old in text
    return text.replace(old, f"python {script}", 1)


def test_isolate_script_must_lie_under_src(workspace, monkeypatch):
    _no_tool(monkeypatch)
    # the example script is under .process/scripts/: only the isolate finding, not argv_not_allowed
    fs = _isolate_extract(workspace, "    isolate: true\n    profile: py\n")
    assert codes(fs) == {"isolate_script_outside_src"}


def test_isolated_script_under_src_validates_cleanly(workspace, monkeypatch):
    _no_tool(monkeypatch)
    assert codes(findings_of(workspace, _isolated_text("src/job.py"))) == set()


def test_isolated_script_in_process_scripts_yields_only_the_isolate_finding(workspace, monkeypatch):
    _no_tool(monkeypatch)
    assert codes(findings_of(workspace, _isolated_text(".process/scripts/x.py"))) == {"isolate_script_outside_src"}


def test_non_isolated_script_rule_is_unchanged(workspace):
    fs = findings_of(workspace, mut("python .process/scripts/extract.py", "python src/job.py"))
    assert "argv_not_allowed" in codes(fs) and "isolate_script_outside_src" not in codes(fs)


def test_isolate_input_name_must_be_a_valid_tool_name(workspace, monkeypatch):
    from agent_graph.process.validate import tool_input_name
    assert tool_input_name("Source") == "source" and tool_input_name("my-facts") == "my_facts"
    assert tool_input_name("1x") is None and tool_input_name("a.b") is None


def test_declared_skills_over_max(workspace):
    fs = findings_of(workspace, mut("max_skills: 3", "max_skills: 0"))
    f = next(f for f in fs if f.code == "max_skills")
    assert f.step == "write-summary"


def test_exclusion_of_runtime_skill_reduces_declared_count(workspace):
    text = mut("max_skills: 3", "max_skills: 0").replace(
        "    snippets: [step-notes]\n", "    snippets: [step-notes]\n    exclude_skills: [summarize]\n", 1)
    assert "max_skills" not in codes(findings_of(workspace, text))


def test_invalid_output_check_and_template(workspace):
    fs = findings_of(workspace, mut("checks: [non_empty]", "checks: [bogus, max_words]"))
    assert sum(1 for f in fs if f.code == "check_invalid") == 2
    fs = findings_of(workspace, mut("template: Template", "template: Missing"))
    assert any(f.code == "unresolved_ref" and f.item == "Missing" for f in fs)


def test_tools_must_be_subset(workspace):
    text = mut("    snippets: [step-notes]\n", "    snippets: [step-notes]\n    tools: [Bash]\n")
    assert "tools_not_subset" in codes(findings_of(workspace, text))


def _step_tools(entries):
    return mut("    snippets: [step-notes]\n", f"    snippets: [step-notes]\n    tools: {entries}\n")


def test_step_tool_pattern_accepted_when_base_in_runtime(workspace):
    text = _step_tools('[Read, "Write(/workspace/out/*)"]')
    cs = codes(findings_of(workspace, text))
    assert "tools_not_subset" not in cs and "tool_entry_comma" not in cs


def test_step_tool_pattern_rejected_when_base_not_in_runtime(workspace):
    cs = codes(findings_of(workspace, _step_tools('["Bash(ls:*)"]')))
    assert "tools_not_subset" in cs


def test_tool_entry_with_comma_rejected(workspace):
    assert "tool_entry_comma" in codes(findings_of(workspace, _step_tools('["Read,Write"]')))


def test_runtime_tool_pattern_rejected(workspace):
    text = mut("tools: [Read, Write]", 'tools: [Read, Write, "Bash(ls:*)"]')
    assert "runtime_tool_pattern" in codes(findings_of(workspace, text))


def test_product_schema_file_must_exist(workspace):
    text = mut("produces: [{name: facts, file: facts.json}]",
               "produces: [{name: facts, file: facts.json, schema: schemas/none.json}]")
    assert "schema_missing" in codes(findings_of(workspace, text))
