"""SSR-AGG-005 state model: JSON round trips, process helpers, pool index, binding, reducers."""

import json

import pytest

from agent_graph import config
from agent_graph.errors import ExitState, RunError
from agent_graph.services.runfs import sha256_file
from agent_graph.state import (
    Binding, BoundFile, BoundSource, Budgets, DynState, Edge, EngineCounters, FileRef, GraphNode,
    GraphSpec, InputSource, OutputTarget, PoolIndex, ProcessDef, ProductDecl, ReadRef, ReEntry,
    RunRecord, RuntimeCfg, SkillEntry, SnippetEntry, StepDef, merge_dict)


def _step(name, typ, **kw):
    base = dict(id=1, name=name, type=typ, instruction="do", reads=(), produces=(), outcomes=(),
                success=None, failure=None, warning=None, retry=None, timeout_s=None, run=None,
                isolate=False, snippets=(), skills=(), exclude_skills=(), max_skills=None,
                tools=None, budget=None, line=None)
    base.update(kw)
    return StepDef(**base)


def _process():
    rt = RuntimeCfg(tools=("Read", "Write"), snippets=("p",), skills=("s",), max_skills=3, retry=2,
                    timeout_s=600, budgets=Budgets(50, 20, 3600), work_products="archive")
    steps = (
        _step("a", "procedure", run=("python", "x.py", "{work}"),
              reads=(ReadRef("input.source[S]", "input.source", "S", "", False),),
              produces=(ProductDecl("facts", "facts.json", None, None, 7),), line=5),
        _step("b", "cognitive", retry=5, max_skills=1, budget=1000, tools=("Read",),
              reads=(ReadRef("review_feedback?", "product", "review_feedback", "", True),)),
        _step("c", "human", timeout_s=60),
        _step("d", "human"),
    )
    return ProcessDef(
        api_version="agent-graph/v1", id="p", title="T", description="D",
        sources=(InputSource("S", "x/*.md", None, None, True, 3),),
        templates=(FileRef("T", "templates/t.md", True, 4),), rulesets=(),
        targets=(OutputTarget("R", "r.md", "T", ("non_empty",), True, 9),),
        runtime=rt, steps=steps, entry="a",
        edges=(Edge("a", "b", "done", None, 1), Edge("b", "END", "done", None, 2)),
        source_file=".process/p.yml", source_sha256="ab" * 32)


def test_process_roundtrip_and_helpers():
    p = _process()
    d = json.loads(json.dumps(p.to_json(), sort_keys=True))
    assert ProcessDef.from_json(d) == p
    assert p.step("b").name == "b"
    with pytest.raises(KeyError):
        p.step("zzz")
    a, b, c, d_ = (p.step(n) for n in "abcd")
    assert p.retry_of(a) == 2 and p.retry_of(b) == 5
    assert p.timeout_of(a) == 600 and p.timeout_of(c) == 60 and p.timeout_of(d_) is None
    assert p.max_skills_of(a) == 3 and p.max_skills_of(b) == 1
    assert p.budget_of(a) == config.DEFAULT_NODE_BUDGET and p.budget_of(b) == 1000


def test_retry_zero_is_respected():
    p = _process()
    s = _step("z", "cognitive", retry=0)
    assert p.retry_of(s) == 0


def test_graph_spec_roundtrip():
    p = _process()
    spec = GraphSpec("p", p.source_sha256, "a", (GraphNode("a", "procedure", "h", ("x",), ("y",)),),
                     p.edges, ("a", "b"), process=p.to_json())
    got = GraphSpec.from_json(json.loads(json.dumps(spec.to_json())))
    assert got == spec and got.engine_entry == "restart_at|entry"


def test_binding_roundtrip_and_allowed_paths():
    b = Binding("p", ".process/p.yml", "h",
                {"S": BoundSource("S", (BoundFile(".input/x/a.md", "h1"), BoundFile(".input/x/b.md", "h2")), None),
                 "N": BoundSource("N", (), "note text")},
                {"T": BoundFile("templates/t.md", "h3")}, {"R": BoundFile("rules/r.md", "h4")})
    assert Binding.from_json(json.loads(json.dumps(b.to_json()))) == b
    assert b.allowed_input_paths() == {".input/x/a.md", ".input/x/b.md", "templates/t.md", "rules/r.md"}


def test_pool_index_roundtrip_verifies_hashes(workspace):
    snip = workspace.snippet_dir / "step-notes.md"
    entry = SnippetEntry("step-notes", "step", ("writing",), False,
                         "identity/.snippet/step-notes.md", sha256_file(snip), "ignored body")
    skill = SkillEntry("summarize", "d", ("t",), "identity/.skills/summarize",
                       (("SKILL.md", "deadbeef"),))
    idx = PoolIndex({"summarize": skill}, {"step-notes": entry})
    j = json.loads(json.dumps(idx.to_json()))
    assert "body" not in j["snippets"]["step-notes"]
    got = PoolIndex.from_json(j, workspace.root)
    assert got.skills == idx.skills
    assert got.snippets["step-notes"].body == "Keep each section under 150 words.\n"
    snip.write_text(snip.read_text() + "changed\n")
    with pytest.raises(ValueError):
        PoolIndex.from_json(j, workspace.root)


def test_run_record_roundtrip():
    rec = RunRecord("p-20250101-000000", "p", ".process/p.yml", "2025-01-01T00:00:00Z", "S6", "SUSPENDED",
                    "opus", "medium",
                    open_error=RunError(ExitState.NODE_FAILED, "S6", "x", node="b"),
                    reentry=ReEntry("S6", "b", False, "summary"),
                    counters=EngineCounters(steps=3, llm_calls=2, elapsed_s=1.5, failures={"b": 1},
                                            attempts={"b": ["schema"]}, p_run=100, cognitive_started=2,
                                            drift=["n"]),
                    resolution_count=1, exit_state="SUSPENDED", exit_code=130)
    got = RunRecord.from_json(json.loads(json.dumps(rec.to_json())))
    assert got == rec
    minimal = RunRecord("r", "p", "f", "c", "S1", "ACTIVE", "m", "e")
    assert RunRecord.from_json(minimal.to_json()) == minimal
    assert minimal.dynamic_thread == "dyn-1"


def test_merge_dict():
    assert merge_dict({"a": 1}, {"b": 2, "a": 3}) == {"a": 3, "b": 2}
    assert merge_dict(None, {"a": 1}) == {"a": 1}


def test_dynstate_reducers_work_in_langgraph():
    langgraph = pytest.importorskip("langgraph.graph")

    def n1(state: DynState):
        return {"visits": {"a": 1}, "last_node": "a"}

    def n2(state: DynState):
        return {"visits": {"b": 1}, "products": {"x": {"name": "x"}}}

    g = langgraph.StateGraph(DynState)
    g.add_node("n1", n1)
    g.add_node("n2", n2)
    g.add_edge(langgraph.START, "n1")
    g.add_edge("n1", "n2")
    g.add_edge("n2", langgraph.END)
    out = g.compile().invoke({"visits": {"z": 9}})
    assert out["visits"] == {"z": 9, "a": 1, "b": 1}
    assert out["last_node"] == "a" and out["products"] == {"x": {"name": "x"}}
