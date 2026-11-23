"""Helpers for the WP3 tests (no tests here). Builds a PoolIndex, ProcessDef, Binding and
ComposeInputs from the example workspace without using WP2 code."""

import json
from pathlib import Path

from agent_graph.compose import ComposeInputs
from agent_graph.ports import Launch, TurnResult
from agent_graph.services.runfs import WorkspacePaths, sha256_bytes
from agent_graph.services.yaml_schema import parse_front_matter
from agent_graph.state import (Binding, Budgets, BoundFile, BoundSource, OutputTarget, PoolIndex,
                               ProcessDef, ProductDecl, ProductRef, ReadRef, RuntimeCfg,
                               SkillEntry, SnippetEntry, StepDef)

RUN_ID = "example-20260101-000000"
LAUNCH = Launch(model="opus", effort="medium", tools=("Read", "Write"))


def make_index(ws: WorkspacePaths) -> PoolIndex:
    snippets = {}
    for f in sorted(ws.snippet_dir.glob("*.md")):
        raw = f.read_bytes()
        meta, body, err = parse_front_matter(raw.decode("utf-8"))
        assert err is None, err
        snippets[meta["id"]] = SnippetEntry(
            id=meta["id"], layer=meta["layer"], tags=tuple(meta.get("tags") or ()),
            auto=bool(meta.get("auto")), path=f.relative_to(ws.root).as_posix(),
            sha256=sha256_bytes(raw), body=body)
    skills = {}
    for d in sorted(p for p in ws.skills_dir.iterdir() if p.is_dir()):
        meta, _body, err = parse_front_matter((d / "SKILL.md").read_text(encoding="utf-8"))
        assert err is None, err
        files = tuple((p.relative_to(d).as_posix(), sha256_bytes(p.read_bytes()))
                      for p in sorted(d.rglob("*")) if p.is_file())
        skills[meta["name"]] = SkillEntry(
            name=meta["name"], description=meta["description"], tags=tuple(meta.get("tags") or ()),
            dir=d.relative_to(ws.root).as_posix(), files=files)
    return PoolIndex(skills=skills, snippets=snippets)


def mk_step(id: int, name: str, type: str = "cognitive", **kw) -> StepDef:
    base = dict(id=id, name=name, type=type, instruction="", reads=(), produces=(), outcomes=(),
                success=None, failure=None, warning=None, retry=None, timeout_s=None, run=None,
                isolate=False, snippets=(), skills=(), exclude_skills=(), max_skills=None,
                tools=None, budget=None, line=None)
    base.update(kw)
    return StepDef(**base)


INSTRUCTION = ('INSTANTIATE("Template", "with facts")\n'
               'WRITE(output.target[Report], "summary per Template")\n')

FACTS_READ = ReadRef("facts", "product", "facts", "", False)
TEMPLATE_READ = ReadRef("input.output-template-ref[Template]", "input.output-template-ref",
                        "Template", "", False)
FEEDBACK_READ = ReadRef("review_feedback?", "product", "review_feedback", "", True)


def write_summary_step(**kw) -> StepDef:
    base = dict(instruction=INSTRUCTION, reads=(FACTS_READ, TEMPLATE_READ, FEEDBACK_READ),
                produces=(ProductDecl("Report", "summary.md", None, "Report", None),),
                snippets=("step-notes",))
    base.update(kw)
    return mk_step(2, "write-summary", **base)


def make_process(steps=None, **runtime_kw) -> ProcessDef:
    rt = dict(tools=("Read", "Write"), snippets=("process-rules",), skills=("summarize",),
              max_skills=3, retry=2, timeout_s=600, budgets=Budgets(50, 20, 3600),
              work_products="archive")
    rt.update(runtime_kw)
    steps = tuple(steps) if steps is not None else (
        mk_step(1, "extract", "procedure"), write_summary_step(),
        mk_step(3, "polish", "cognitive", instruction="Polish the wording of the report.\n",
                snippets=()))
    return ProcessDef(
        api_version="agent-graph/v1", id="example", title="Example", description="d",
        sources=(), templates=(), rulesets=(),
        targets=(OutputTarget("Report", "summary.md", "Template", ("non_empty",), True, None),),
        runtime=RuntimeCfg(**rt), steps=steps, entry="extract", edges=(),
        source_file=".process/example.yml", source_sha256="0" * 64)


def make_binding(ws: WorkspacePaths) -> Binding:
    def bf(rel: str) -> BoundFile:
        return BoundFile(rel, sha256_bytes((ws.root / rel).read_bytes()))
    return Binding(
        process_id="example", process_file=".process/example.yml", process_sha256="0" * 64,
        sources={"Source": BoundSource("Source", (bf(".input/example/source.md"),), None)},
        templates={"Template": bf("templates/report.md")}, rulesets={})


def add_product(ws: WorkspacePaths, name: str, node: str, file: str, text: str) -> ProductRef:
    rel = f".agent/{RUN_ID}/work/{node}/{file}"
    p = ws.root / rel
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(text, encoding="utf-8")
    return ProductRef(name, node, rel, sha256_bytes(text.encode("utf-8")))


def default_products(ws: WorkspacePaths) -> dict:
    return {"facts": add_product(ws, "facts", "extract", "facts.json", json.dumps({"lines": 3}))}


def make_inputs(ws: WorkspacePaths, *, step_name: str = "write-summary", process=None, step=None,
                products=None, facts=(), handoffs=(), launch=LAUNCH) -> ComposeInputs:
    process = process or make_process()
    step = step or process.step(step_name)
    return ComposeInputs(
        process=process, step=step, index=make_index(ws), binding=make_binding(ws),
        products=default_products(ws) if products is None else products,
        upstream_handoffs=handoffs, facts=facts, ws=ws, active_dir=ws.run(RUN_ID).active,
        launch=launch)


class CountingSummarizer:
    """SummarizerPort fake: counts calls, returns a fixed text."""

    def __init__(self, text: str = "SHORT SUMMARY"):
        self.text = text
        self.calls: list = []

    def summary(self, path: Path, sha256: str, budget_tokens: int) -> str:
        self.calls.append((Path(path), sha256, budget_tokens))
        return self.text


class FakePort:
    """ClaudePort fake for SummaryCache."""

    def __init__(self, text: str = "A cached summary."):
        self.text = text
        self.requests: list = []

    def run_turn(self, req, cancel=None) -> TurnResult:
        self.requests.append(req)
        return TurnResult(session_id="s", text=self.text, usages=(), compactions=(),
                          exit_code=0, argv=())
