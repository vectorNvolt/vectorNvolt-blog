import dataclasses
import json
from pathlib import Path

import pytest

from agent_graph.compose import compose_context, compose_system_md, record_compaction, record_turn
from agent_graph.compose.step_contract import build_step_contract
from agent_graph.errors import ContextOverBudget
from agent_graph.ports import Usage
from agent_graph.services.runfs import canonical_json
from agent_graph.state import ReadRef, RunFact
from fakes.workspace import add_skill, add_snippet, write_file
from test_wp3_support import (FACTS_READ, RUN_ID, CountingSummarizer, add_product, make_index,
                              make_inputs, make_process, write_summary_step)


def active(ws) -> Path:
    return ws.run(RUN_ID).active


def read(ws, name: str) -> str:
    return (active(ws) / name).read_text(encoding="utf-8")


def manifest(ws) -> dict:
    return json.loads(read(ws, "context.json"))


# ----------------------------------------------------------------------- system.md / hashes

def test_system_md_byte_identical_across_steps(workspace, guard):
    s = CountingSummarizer()
    r1 = compose_context(make_inputs(workspace, step_name="write-summary"), s, guard)
    sys1 = (active(workspace) / "system.md").read_bytes()
    m1 = manifest(workspace)
    r2 = compose_context(make_inputs(workspace, step_name="polish"), s, guard)
    sys2 = (active(workspace) / "system.md").read_bytes()
    m2 = manifest(workspace)
    assert sys1 == sys2
    assert r1.system_md_sha256 == r2.system_md_sha256
    assert r1.launch_sha256 == r2.launch_sha256
    assert m1["hashes"]["system_md_sha256"] == m2["hashes"]["system_md_sha256"]
    assert m1["hashes"]["launch_sha256"] == m2["hashes"]["launch_sha256"]
    assert r1.claude_md_sha256 != r2.claude_md_sha256          # L2 differs per step
    assert sys1.decode() == ("You are a careful assistant.\nStay inside the workspace.\n\n"
                             "Cite the source for every claim.\n")
    assert compose_system_md(make_process(), make_index(workspace)) == sys1.decode()


def test_no_run_id_or_absolute_path_in_system_and_claude_md(workspace, guard):
    compose_context(make_inputs(workspace), CountingSummarizer(), guard)
    for name in ("system.md", "CLAUDE.md"):
        text = read(workspace, name)
        assert RUN_ID not in text
        assert str(workspace.root) not in text


def test_recompose_gives_same_bytes(workspace, guard):
    s = CountingSummarizer()
    r1 = compose_context(make_inputs(workspace), s, guard)
    files1 = {n: (active(workspace) / n).read_bytes() for n in ("system.md", "CLAUDE.md", "context.md")}
    r2 = compose_context(make_inputs(workspace), s, guard)
    files2 = {n: (active(workspace) / n).read_bytes() for n in ("system.md", "CLAUDE.md", "context.md")}
    assert r1.claude_md_sha256 == r2.claude_md_sha256
    assert files1 == files2


def test_mcp_json_fixed_bytes(workspace, guard):
    compose_context(make_inputs(workspace), CountingSummarizer(), guard)
    assert (active(workspace) / "mcp.json").read_bytes() == b'{"mcpServers": {}}\n'


# ----------------------------------------------------------------------- CLAUDE.md layers

def test_l2_declared_then_auto_sorted_then_contract(workspace, guard):
    add_snippet(workspace, "zzz-auto", "step", "ZZZ body\n", tags=["summary"], auto=True)
    add_snippet(workspace, "aaa-auto", "step", "AAA body\n", tags=["summary"], auto=True)
    add_snippet(workspace, "net-auto", "step", "NET body\n", tags=["network"], auto=True)
    compose_context(make_inputs(workspace), CountingSummarizer(), guard)
    t = read(workspace, "CLAUDE.md")
    pos = [t.index(x) for x in ("Keep each section under 150 words.",   # declared step-notes
                                "AAA body", "Prefer short sentences.", "ZZZ body", "# Step contract")]
    assert pos == sorted(pos)
    assert "NET body" not in t                                   # tags do not match the goal
    m = manifest(workspace)
    assert [s["id"] for s in m["layers"]["L2"]["snippets"]] == ["step-notes"]
    assert [s["id"] for s in m["layers"]["L2"]["auto"]] == ["aaa-auto", "step-auto", "zzz-auto"]
    assert [s["id"] for s in m["layers"]["L0"]["snippets"]] == ["agent-identity"]
    assert [s["id"] for s in m["layers"]["L1"]["snippets"]] == ["process-rules"]


def test_l3_facts_in_order_after_contract_and_context_md_only_applicable(workspace, guard):
    facts = [RunFact(1, "answer", "write-summary", "Use bullet points."),
             RunFact(2, "hil_nok", "review", "Too long.")]
    compose_context(make_inputs(workspace, facts=facts), CountingSummarizer(), guard)
    t = read(workspace, "CLAUDE.md")
    assert t.index("# Step contract") < t.index("# Run facts") < t.index("#1 answer") < t.index("#2 hil_nok")
    ctx = read(workspace, "context.md")
    assert "Use bullet points." in ctx and "Too long." not in ctx
    assert manifest(workspace)["layers"]["L3"]["facts"] == [1, 2]


def test_no_facts_no_run_facts_heading(workspace, guard):
    compose_context(make_inputs(workspace), CountingSummarizer(), guard)
    assert "# Run facts" not in read(workspace, "CLAUDE.md")


def test_l3_over_cap_keeps_newest(workspace, guard):
    facts = [RunFact(i, "answer", None, f"fact{i} " + "w" * 800) for i in range(1, 61)]
    compose_context(make_inputs(workspace, facts=facts), CountingSummarizer(), guard)
    t = read(workspace, "CLAUDE.md")
    assert "fact60 " in t and "fact1 " not in t
    assert "omitted, see `../run_facts.jsonl`" in t
    l3 = manifest(workspace)["layers"]["L3"]
    assert l3["facts"][-1] == 60 and l3["omitted_facts"][0] == 1


def test_l2_over_cap_drops_auto_snippets_first(workspace, guard):
    add_snippet(workspace, "step-notes", "step", "note " * 4800)              # 6000 tokens
    add_snippet(workspace, "big-auto", "step", "auto " * 2400, tags=["summary"], auto=True)
    compose_context(make_inputs(workspace), CountingSummarizer(), guard)
    t = read(workspace, "CLAUDE.md")
    assert "auto auto" not in t and "note note" in t
    l2 = manifest(workspace)["layers"]["L2"]
    assert "big-auto" in l2["dropped_auto"] and l2["auto"] == []


def test_l2_over_cap_after_dropping_auto_raises_and_writes_nothing(workspace, guard):
    add_snippet(workspace, "step-notes", "step", "note " * 8000)              # 10000 tokens
    with pytest.raises(ContextOverBudget) as e:
        compose_context(make_inputs(workspace), CountingSummarizer(), guard)
    assert e.value.budget == 8000
    assert not (active(workspace) / "system.md").exists()


def test_step_contract_content(workspace):
    process = make_process()
    step = process.step("write-summary")
    c = build_step_contract(process, step, lambda p: "../" + p)
    assert c.startswith("# Step contract\n")
    assert "`../work/write-summary/summary.md`" in c
    assert "output target `Report`, template `Template`" in c
    assert "- Tools: Read, Write" in c and "- Context budget: 40000 tokens" in c
    assert "leave it `null`" in c and "Allowed outcomes" not in c
    step2 = dataclasses.replace(step, outcomes=("ok", "retry"), tools=("Read",))
    c2 = build_step_contract(process, step2, lambda p: p)
    assert "## Allowed outcomes\n\n- `ok`\n- `retry`" in c2
    assert "- Tools: Read\n" in c2 and "leave it `null`" not in c2
    assert c == build_step_contract(process, step, lambda p: "../" + p)


# ----------------------------------------------------------------------- skills

def test_declared_skill_staged_verbatim_and_unranked_not_staged(workspace, guard):
    r = compose_context(make_inputs(workspace), CountingSummarizer(), guard)
    root = active(workspace) / ".claude" / "skills"
    assert r.staged_skills == ("summarize",)
    assert sorted(p.name for p in root.iterdir()) == ["summarize"]
    pool = workspace.skills_dir / "summarize"
    for rel in ("SKILL.md", "notes.md"):
        assert (root / "summarize" / rel).read_bytes() == (pool / rel).read_bytes()
    assert not (root / "translate").exists()


def test_ranking_fills_up_to_max_skills(workspace, guard):
    add_skill(workspace, "report-tools", "Build report layouts", tags=["report"])      # score 4
    add_skill(workspace, "summary-helper", "Helps summarize", tags=["summary"])        # score 3
    r = compose_context(make_inputs(workspace), CountingSummarizer(), guard)
    assert r.staged_skills == ("report-tools", "summarize", "summary-helper")
    process = make_process()
    step = dataclasses.replace(process.step("write-summary"), max_skills=2)
    r = compose_context(make_inputs(workspace, process=process, step=step), CountingSummarizer(), guard)
    assert r.staged_skills == ("report-tools", "summarize")
    sk = manifest(workspace)["skills"]
    assert sk["ranked"] == [{"name": "report-tools", "score": 4}, {"name": "summary-helper", "score": 3}]
    assert sk["not_staged_cap"] == ["summary-helper"] and sk["max_skills"] == 2
    assert not (active(workspace) / ".claude" / "skills" / "summary-helper").exists()


def test_declared_skills_kept_even_when_over_max(workspace, guard):
    process = make_process(skills=("summarize", "translate"))
    step = dataclasses.replace(process.step("write-summary"), max_skills=1)
    add_skill(workspace, "report-tools", "Build report layouts", tags=["report"])
    r = compose_context(make_inputs(workspace, process=process, step=step), CountingSummarizer(), guard)
    assert r.staged_skills == ("summarize", "translate")


def test_excluded_skills_not_staged_not_ranked_but_recorded(workspace, guard):
    add_skill(workspace, "report-tools", "Build report layouts", tags=["report"])
    add_skill(workspace, "summary-helper", "Helps summarize", tags=["summary"])
    compose_context(make_inputs(workspace), CountingSummarizer(), guard)
    assert (active(workspace) / ".claude" / "skills" / "report-tools").is_dir()
    process = make_process()
    step = dataclasses.replace(process.step("write-summary"),
                               exclude_skills=("summarize", "report-tools"))
    r = compose_context(make_inputs(workspace, process=process, step=step), CountingSummarizer(), guard)
    assert r.excluded_skills == ("report-tools", "summarize")
    assert r.staged_skills == ("summary-helper",)
    root = active(workspace) / ".claude" / "skills"
    assert sorted(p.name for p in root.iterdir()) == ["summary-helper"]       # stale copy removed
    sk = manifest(workspace)["skills"]
    assert sk["excluded"] == ["report-tools", "summarize"]
    assert [x["name"] for x in sk["ranked"]] == ["summary-helper"]


# ----------------------------------------------------------------------- context.md

def sections_in_order(ctx: str) -> bool:
    pos = [ctx.index(h) for h in ("## 1. Goal and output schema", "## 2. Reads", "## 3. Handoff notes",
                                  "## 4. Run facts")]
    return pos == sorted(pos)


def test_context_md_fixed_sections_inline_reads_and_relative_paths(workspace, guard):
    hand = workspace.root / ".agent" / RUN_ID / "nodes" / "extract" / "handoff.json"
    hand.parent.mkdir(parents=True, exist_ok=True)
    hand.write_text(json.dumps({"done": "Counted lines", "decisions": ["use all"], "open_points": []}))
    r = compose_context(make_inputs(workspace, handoffs=(("extract", hand),)), CountingSummarizer(), guard)
    ctx = read(workspace, "context.md")
    assert sections_in_order(ctx)
    assert "INSTANTIATE" in ctx
    assert "### facts\nPath: `../work/extract/facts.json`" in ctx
    assert '"lines": 3' in ctx
    assert "Path: `../../../templates/report.md`" in ctx
    assert "### extract\nDone: Counted lines\nDecisions:\n- use all" in ctx
    m = manifest(workspace)
    assert m["context_tokens"] == r.context_tokens
    assert [i["mode"] for i in m["items"] if i["kind"] == "read"] == ["inline", "inline"]
    assert m["products_consumed"][0]["name"] == "facts"
    reasons = {o["ref"]: o["reason"] for o in m["omitted"]}
    assert reasons["review_feedback"] == "absent"                 # optional read not produced yet
    assert "## Left out" not in ctx


def test_mandatory_over_budget_raises_before_any_write(workspace, guard):
    process = make_process()
    step = dataclasses.replace(process.step("write-summary"), budget=50)
    with pytest.raises(ContextOverBudget) as e:
        compose_context(make_inputs(workspace, process=process, step=step), CountingSummarizer(), guard)
    assert e.value.node == "write-summary" and e.value.budget == 50
    assert e.value.mandatory_tokens > 0
    assert not (active(workspace) / "system.md").exists()
    assert not (active(workspace) / "context.md").exists()


SOURCE_READ = ReadRef("input.source[Source]", "input.source", "Source", "", False)


def big_source_inputs(workspace, summarizer_text):
    write_file(workspace.root, ".input/example/source.md", "word " * 40000)      # 50000 tokens
    process = make_process()
    step = dataclasses.replace(process.step("write-summary"), budget=3000, reads=(FACTS_READ, SOURCE_READ))
    return make_inputs(workspace, process=process, step=step)


def test_item_over_share_is_summarized_not_truncated(workspace, guard):
    s = CountingSummarizer("SHORT SUMMARY")
    compose_context(big_source_inputs(workspace, "x"), s, guard)
    assert len(s.calls) == 1
    path, _sha, budget = s.calls[0]
    assert path == workspace.root / ".input/example/source.md" and budget >= 200
    ctx = read(workspace, "context.md")
    assert "SHORT SUMMARY" in ctx and "Path: `../../../.input/example/source.md`" in ctx
    assert len(ctx) < 5000
    modes = {i["ref"]: i["mode"] for i in manifest(workspace)["items"] if i["kind"] == "read"}
    assert modes == {"facts": "inline", "input.source[Source]": "summary"}


def test_mandatory_item_not_fitting_after_summary_raises(workspace, guard):
    s = CountingSummarizer("y" * 40000)                                              # 10000 tokens
    with pytest.raises(ContextOverBudget) as e:
        compose_context(big_source_inputs(workspace, "x"), s, guard)
    assert e.value.mandatory_tokens > e.value.budget == 3000
    assert not (active(workspace) / "CLAUDE.md").exists()


def test_optional_item_left_out_is_listed_by_path(workspace, guard):
    products = {"facts": add_product(workspace, "facts", "extract", "facts.json", '{"lines": 3}'),
                "bignote": add_product(workspace, "bignote", "extract", "bignote.md", "note " * 20000)}
    process = make_process()
    step = dataclasses.replace(process.step("write-summary"), budget=2000, reads=(
        FACTS_READ, ReadRef("bignote?", "product", "bignote", "", True)))
    s = CountingSummarizer("z" * 40000)
    compose_context(make_inputs(workspace, process=process, step=step, products=products), s, guard)
    assert len(s.calls) == 1
    ctx = read(workspace, "context.md")
    left = ctx.split("## Left out (read on demand)")[1]
    assert "`../work/extract/bignote.md`" in left and "over_budget" in left
    assert "zzzz" not in ctx
    om = [o for o in manifest(workspace)["omitted"] if o["reason"] == "over_budget"]
    assert om and om[0]["path"] == "../work/extract/bignote.md"


def test_structured_item_becomes_schema_sketch_without_llm(workspace, guard):
    data = {"rows": [{"id": i, "name": "n" * 50} for i in range(500)]}
    products = {"bigjson": add_product(workspace, "bigjson", "extract", "bigjson.json", json.dumps(data))}
    process = make_process()
    step = dataclasses.replace(process.step("write-summary"), budget=4000,
                               reads=(ReadRef("bigjson", "product", "bigjson", "", False),))
    s = CountingSummarizer()
    compose_context(make_inputs(workspace, process=process, step=step, products=products), s, guard)
    assert s.calls == []
    ctx = read(workspace, "context.md")
    assert "Schema sketch:" in ctx and "<500 items>" in ctx and "Sample:" in ctx
    assert [i["mode"] for i in manifest(workspace)["items"]] == ["sketch"]


def test_missing_handoff_file_is_skipped_and_recorded(workspace, guard):
    gone = workspace.root / ".agent" / RUN_ID / "nodes" / "gone" / "handoff.json"
    compose_context(make_inputs(workspace, handoffs=(("gone", gone),)), CountingSummarizer(), guard)
    assert "### gone" not in read(workspace, "context.md")
    assert any(o["ref"] == "handoff gone" and o["reason"] == "missing" for o in manifest(workspace)["omitted"])


# ----------------------------------------------------------------------- manifest

def test_manifest_hashes_sorted_keys_and_turn_records(workspace, guard):
    r = compose_context(make_inputs(workspace), CountingSummarizer(), guard)
    text = read(workspace, "context.json")
    assert text == canonical_json(json.loads(text))
    m = json.loads(text)
    assert m["hashes"]["claude_md_sha256"] == r.claude_md_sha256
    assert m["hashes"]["system_md_sha256"] == r.system_md_sha256
    assert m["hashes"]["launch_sha256"] == r.launch_sha256 == make_inputs(workspace).launch.launch_hash()
    assert r.manifest == active(workspace) / "context.json"

    record_turn(r.manifest, 2, [Usage(input_tokens=5, cache_read_input_tokens=100),
                                Usage(input_tokens=1, ephemeral_1h_input_tokens=7)], guard)
    record_turn(r.manifest, 1, [Usage(cache_creation_input_tokens=50, ephemeral_1h_input_tokens=50)], guard)
    record_turn(r.manifest, 2, [Usage(input_tokens=9)], guard)                        # idempotent per turn
    record_compaction(r.manifest, {"trigger": "auto", "pre_tokens": 1000}, guard)
    m = json.loads(r.manifest.read_text())
    assert [t["turn"] for t in m["turns"]] == [1, 2]
    assert m["turns"][0]["totals"]["cache_creation_input_tokens"] == 50
    assert m["turns"][1]["totals"]["input_tokens"] == 9
    assert m["compactions"] == [{"trigger": "auto", "pre_tokens": 1000}]

    compose_context(make_inputs(workspace), CountingSummarizer(), guard)              # resume: re-compose
    m2 = json.loads(r.manifest.read_text())
    assert m2["turns"] == m["turns"] and m2["compactions"] == m["compactions"]


def test_write_boundary_only_active_dir(workspace, guard):
    before = {p for p in workspace.root.rglob("*") if p.is_file()}
    compose_context(make_inputs(workspace), CountingSummarizer(), guard)
    new = {p for p in workspace.root.rglob("*") if p.is_file()} - before
    allowed = active(workspace)
    assert new and all(allowed in p.parents or p.parent == allowed.parent / "work" / "extract" for p in new)
