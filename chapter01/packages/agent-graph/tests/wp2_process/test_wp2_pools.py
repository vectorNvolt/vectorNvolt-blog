"""SFR-AGG-006 / 008, SSR-AGG-017: pool parsing, index with hashes, declaration checks."""

import json

from agent_graph.process.loader import load_process
from agent_graph.process.pools import build_index, validate_against_pools, write_index
from agent_graph.services.runfs import sha256_file
from agent_graph.state import PoolIndex
from fakes.workspace import PROCESS_YML, add_process, add_skill, add_snippet, write_file


def mut(old: str, new: str) -> str:
    assert old in PROCESS_YML, old
    return PROCESS_YML.replace(old, new, 1)


def check(ws, text=None, **kw):
    if text is not None:
        add_process(ws, "example.yml", text)
    proc, fs = load_process(ws.process_dir / "example.yml", ws)
    assert proc is not None, fs
    index, ifs = build_index(ws)
    return validate_against_pools(proc, index, **kw), ifs


def test_index_of_example_pools(workspace):
    index, fs = build_index(workspace)
    assert fs == []
    assert set(index.skills) == {"summarize", "translate"}
    assert set(index.snippets) == {"agent-identity", "process-rules", "step-notes", "step-auto"}
    s = index.skills["summarize"]
    assert [f for f, _ in s.files] == ["SKILL.md", "notes.md"]
    sn = index.snippets["step-auto"]
    assert sn.auto and sn.layer == "step" and sn.tags == ("summary",)
    assert sn.sha256 == sha256_file(workspace.root / sn.path)
    assert "short sentences" in sn.body and "---" not in sn.body
    problems, _ = check(workspace)
    assert problems == []


def test_empty_workspace_has_empty_index(empty_workspace):
    index, fs = build_index(empty_workspace)
    assert not index.skills and not index.snippets and fs == []


def test_missing_skill_and_snippet_name_item_and_step(workspace):
    fs, _ = check(workspace, mut("skills: [summarize]", "skills: [ghost]"))
    assert any(f.code == "skill_missing" and f.item == "ghost" for f in fs)
    fs, _ = check(workspace, mut("snippets: [step-notes]", "snippets: [nope]"))
    f = next(f for f in fs if f.code == "snippet_missing")
    assert f.item == "nope" and f.step == "write-summary" and f.line is not None


def test_snippet_layer_mismatch(workspace):
    fs, _ = check(workspace, mut("snippets: [step-notes]", "snippets: [process-rules]"))
    assert any(f.code == "snippet_layer" and f.item == "process-rules" for f in fs)
    fs, _ = check(workspace, mut("snippets: [process-rules]", "snippets: [agent-identity]"))
    assert any(f.code == "snippet_layer" and f.item == "agent-identity" for f in fs)


def test_exclusions(workspace):
    base = "    snippets: [step-notes]\n"
    fs, _ = check(workspace, mut(base, base + "    exclude_skills: [ghost]\n"))
    assert any(f.code == "exclude_invalid" and f.item == "ghost" for f in fs)
    fs, _ = check(workspace, mut(base, base + "    skills: [translate]\n    exclude_skills: [translate]\n"))
    assert any(f.code == "exclude_invalid" and f.item == "translate" for f in fs)
    fs, _ = check(workspace, mut(base, base + "    exclude_skills: [summarize]\n"))
    assert fs == []


def test_no_agent_snippet(workspace):
    (workspace.snippet_dir / "agent-identity.md").unlink()
    fs, _ = check(workspace)
    assert [f.code for f in fs] == ["no_agent_snippet"]


def test_invalid_pool_files_are_reported_by_build_index(workspace):
    write_file(workspace.root, "identity/.snippet/broken.md", "no front matter here\n")
    add_snippet(workspace, "step-notes", "step", "dup\n", filename="zz-dup.md")
    write_file(workspace.root, "identity/.snippet/badlayer.md", "---\nid: bl\nlayer: nope\n---\nbody\n")
    write_file(workspace.root, "identity/.skills/empty/readme.txt", "x")
    add_skill(workspace, "wrongname", "d")
    (workspace.skills_dir / "wrongname").rename(workspace.skills_dir / "other-dir")
    _, fs = build_index(workspace)
    by = {(f.code, f.file) for f in fs}
    assert ("snippet_invalid", "identity/.snippet/broken.md") in by
    assert ("snippet_invalid", "identity/.snippet/zz-dup.md") in by
    assert ("snippet_invalid", "identity/.snippet/badlayer.md") in by
    assert ("skill_invalid", "identity/.skills/empty") in by
    assert ("skill_invalid", "identity/.skills/other-dir/SKILL.md") in by
    assert all(f.code in ("snippet_invalid", "skill_invalid") for f in fs)


def test_write_index_records_hashes_and_leaves_pools_alone(workspace, guard):
    before = {p: sha256_file(p) for p in workspace.identity_dir.rglob("*") if p.is_file()}
    index, _ = build_index(workspace)
    paths = workspace.run("example-20240101-000000")
    write_index(index, paths, guard)
    after = {p: sha256_file(p) for p in workspace.identity_dir.rglob("*") if p.is_file()}
    assert before == after
    data = json.loads(paths.index_json.read_text())
    assert data["snippets"]["process-rules"]["sha256"] == index.snippets["process-rules"].sha256
    assert "body" not in data["snippets"]["process-rules"]
    assert dict(map(tuple, data["skills"]["summarize"]["files"]))["notes.md"]
    again = PoolIndex.from_json(data, workspace.root)
    assert again.snippets["process-rules"].body == index.snippets["process-rules"].body


def test_system_md_cap_uses_injected_composer(workspace):
    fs, _ = check(workspace, system_md=lambda proc, index: "x" * 4)
    assert fs == []
    fs, _ = check(workspace, system_md=lambda proc, index: "x" * 40_000)
    assert [f.code for f in fs] == ["layer_cap"]
