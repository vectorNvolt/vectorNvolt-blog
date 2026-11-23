"""Shared builders and fakes of the WP5 tests (no tests in this module).

Nothing here needs WP2/WP3/WP4: the process is built from the state dataclasses, compose is a fake
function, Claude is ``ScriptedClaude`` (a ClaudePort), the checkpointer is langgraph's InMemorySaver.
"""

import sys
import threading
from datetime import datetime, timezone
from types import SimpleNamespace

from langgraph.checkpoint.memory import InMemorySaver

from agent_graph.contract import Status
from agent_graph.dynamic import DynamicMachine, NodeJournal, build_graph_spec, compile_dynamic
from agent_graph.dynamic.journal import NodeContext
from agent_graph.engine import EngineContext
from agent_graph.errors import ContextOverBudget, TurnCancelled
from agent_graph.ports import CancelToken, Launch, TurnResult, Usage
from agent_graph.services.container import SandboxExec
from agent_graph.services.history import History
from agent_graph.services.run_facts import RunFactsStore
from agent_graph.services.runfs import sha256_file, write_atomic
from agent_graph.state import (Binding, BoundFile, BoundSource, Budgets, Edge, FileRef, OutputTarget,
                               PoolIndex, ProcessDef, ProductDecl, ReadRef, RunRecord, RuntimeCfg, StepDef)

RUN_ID = "pipe-20260101-000000"
REPORT_MD = "# Summary\n\nSome text.\n\n## Findings\n\nThree findings.\n"
PY_EXTRACT = ("import os,pathlib,sys;"
              "pathlib.Path(sys.argv[1],'facts.json').write_text('[1, 2]');"
              "open(os.path.join(os.environ['TMPDIR'],'count.txt'),'a').write('x')")
EXTRACT_SCRIPT = ".process/scripts/pipe_extract.py"    # relative to /workspace (SSR-AGG-013: no -c)


def install_extract_script(workspace) -> None:
    """Ship the extract step's script in the workspace (the step runs it by relative path)."""
    p = workspace.root / EXTRACT_SCRIPT
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(PY_EXTRACT.replace(";", "\n") + "\n", encoding="utf-8")


# ------------------------------------------------------------------ process builders

def decl(name, file, *, schema=None, output_target=None) -> ProductDecl:
    return ProductDecl(name=name, file=file, schema=schema, output_target=output_target, line=None)


def read(raw, kind="product", name=None) -> ReadRef:
    return ReadRef(raw=raw, kind=kind, name=name or raw.rstrip("?"), selector="", optional=raw.endswith("?"))


def step(sid, name, type_, *, reads=(), produces=(), outcomes=(), run=None, retry=None, timeout_s=None,
         tools=None, budget=None, success=None, failure=None, isolate=False, instruction="do it",
         profile=None) -> StepDef:
    return StepDef(
        id=sid, name=name, type=type_, instruction=instruction, reads=tuple(reads), produces=tuple(produces),
        outcomes=tuple(outcomes), success=success, failure=failure, warning=None, retry=retry,
        timeout_s=timeout_s, run=None if run is None else tuple(run), isolate=isolate, snippets=(), skills=(),
        exclude_skills=(), max_skills=None, tools=tools, budget=budget, line=None, profile=profile)


def edge(src, dst, when="done", outcome=None) -> Edge:
    return Edge(src=src, dst=dst, when=when, outcome=outcome, line=None)


def process(steps, edges, entry, *, retry=2, timeout_s=600, budgets=Budgets(50, 20, 3600),
            work_products="archive", targets=(), templates=()) -> ProcessDef:
    return ProcessDef(
        api_version="agent-graph/v1", id="pipe", title="t", description="d", sources=(), templates=tuple(templates),
        rulesets=(), targets=tuple(targets),
        runtime=RuntimeCfg(tools=("Read", "Write"), snippets=(), skills=(), max_skills=3, retry=retry,
                           timeout_s=timeout_s, budgets=budgets, work_products=work_products),
        steps=tuple(steps), entry=entry, edges=tuple(edges), source_file=".process/pipe.yml",
        source_sha256="0" * 64)


def pipeline(*, work_products="archive", checks=("non_empty",), retry=2, budgets=Budgets(50, 20, 3600),
             extract_run=None, with_human=True) -> ProcessDef:
    """extract (procedure) -> write-summary (cognitive) -> review (human) -> END; hil_nok loops back."""
    steps = [
        step(1, "extract", "procedure", reads=(read("input.source[Source]", "input.source", "Source"),),
             produces=(decl("facts", "facts.json"),),
             run=extract_run or (sys.executable, EXTRACT_SCRIPT, "{work}"),
             success="PRINT('STEP 1: DONE')"),
        step(2, "write-summary", "cognitive",
             reads=(read("facts"), read("review_feedback?")),
             produces=(decl("Report", "summary.md", output_target="Report"),),
             success="PRINT('STEP 2: DONE')", failure="STOP\nPRINT('<ERROR_DETAILS>')"),
    ]
    edges = [edge("extract", "write-summary")]
    if with_human:
        steps.append(step(3, "review", "human", reads=(read("output.target[Report]", "product", "Report"),),
                          success="PRINT('STEP 3: DONE')"))
        edges += [edge("write-summary", "review"), edge("review", "END", "hil_ok"),
                  edge("review", "write-summary", "hil_nok")]
    else:
        edges += [edge("write-summary", "END")]
    return process(
        steps, edges, "extract", retry=retry, budgets=budgets, work_products=work_products,
        targets=(OutputTarget("Report", "summary.md", "Template", tuple(checks), True, None),),
        templates=(FileRef("Template", "templates/report.md", True, None),))


# ------------------------------------------------------------------ fakes

class OpenGate:
    """A RunGate without limits."""

    def attempt_token(self, node):
        return CancelToken()

    def before_node_start(self, node):
        return None

    def before_llm_call(self, node):
        return None


def result_json(outcome=None, status="done", question=None, done="did it", warnings=()):
    return {"status": status, "outcome": outcome, "question": question,
            "handoff": {"done": done, "decisions": [], "open_points": []}, "warnings": list(warnings)}


class ScriptedClaude:
    """ClaudePort: turn i of the script is a dict (or a callable(req, cancel) -> dict).

    dict keys: write {relpath-from-cwd: str|obj}, usage {Usage fields}, session_id, text,
    raise (an exception), block (seconds to wait for the cancel token, then TurnCancelled)."""

    def __init__(self, turns):
        self.turns = list(turns)
        self.calls: list[dict] = []
        self._n = 0
        self._lock = threading.Lock()

    def run_turn(self, req, cancel=None):
        with self._lock:
            idx = self._n
            self._n += 1
        self.calls.append({"idx": idx, "cwd": req.cwd, "prompt": req.prompt, "resume": req.resume,
                           "allowed": req.allowed_tools, "timeout": req.timeout_s})
        if idx >= len(self.turns):
            raise AssertionError(f"no scripted turn {idx}")
        t = self.turns[idx]
        if callable(t):
            t = t(req, cancel)
        if t.get("raise") is not None:
            raise t["raise"]
        if t.get("block"):
            if cancel is not None and cancel.wait(t["block"]):
                raise TurnCancelled(cancel.cause or "cancelled")
        for rel, content in (t.get("write") or {}).items():
            p = req.cwd / rel
            p.parent.mkdir(parents=True, exist_ok=True)
            import json
            p.write_text(content if isinstance(content, str) else json.dumps(content), encoding="utf-8")
        usage = Usage(**t.get("usage", {"input_tokens": 1, "ephemeral_1h_input_tokens": 1}))
        sid = t.get("session_id") or req.resume or f"sess-{idx + 1}"
        with open(req.transcript, "a", encoding="utf-8") as f:
            f.write('{"type": "result"}\n')
        return TurnResult(session_id=sid, text=t.get("text", "ok"), usages=(usage,), compactions=(),
                          exit_code=0, argv=("claude",))


class NullSummarizer:
    """SummarizerPort stand-in (the fake compose never summarizes)."""

    def summary(self, path, sha256, budget_tokens):
        return ""


class FakeCompose:
    """compose_context stand-in: writes the four files into active/, records the inputs."""

    def __init__(self, over_budget=()):
        self.calls: list = []
        self.over_budget = set(over_budget)

    def __call__(self, inp, summarizer, guard):
        self.calls.append(inp)
        if inp.step.name in self.over_budget:
            raise ContextOverBudget(inp.step.name, 100, 500)
        for name in ("system.md", "CLAUDE.md", "context.md"):
            write_atomic(inp.active_dir / name, f"# {name} for {inp.step.name}\n", guard=guard)
        write_atomic(inp.active_dir / "context.json", "{}\n", guard=guard)
        return SimpleNamespace(context_tokens=12)


def cognitive_turn(extra_write=None, *, outcome=None, usage=None, text=REPORT_MD, node="write-summary"):
    """A script turn of write-summary: the product and result.json."""
    t = {"write": {f"../work/{node}/summary.md": text, "result.json": result_json(outcome),
                   **(extra_write or {})}}
    if usage is not None:
        t["usage"] = usage
    return t


# ------------------------------------------------------------------ assembly

def bound(workspace) -> Binding:
    src = "example/source.md"
    tpl = "templates/report.md"
    return Binding(
        process_id="pipe", process_file=".process/pipe.yml", process_sha256="0" * 64,
        sources={"Source": BoundSource("Source", (BoundFile(".input/" + src, sha256_file(workspace.input_dir / src)),), None)},
        templates={"Template": BoundFile(tpl, sha256_file(workspace.root / tpl))}, rulesets={})


def new_record(run_id=RUN_ID) -> RunRecord:
    return RunRecord(run_id=run_id, process_id="pipe", process_file=".process/pipe.yml",
                     created=datetime.now(timezone.utc).isoformat(), phase="S6", status="ACTIVE",
                     model="opus", effort="medium")


def dynamic_setup(workspace, guard, proc, claude, compose=None, gate=None, run_id=RUN_ID):
    """(machine, parts) for tests that drive the dynamic machine directly."""
    paths = workspace.run(run_id)
    paths.root.mkdir(parents=True, exist_ok=True)
    install_extract_script(workspace)
    saver = InMemorySaver()
    graph = compile_dynamic(proc, saver)
    history = History(paths.history_jsonl, guard)
    facts = RunFactsStore(paths.run_facts_jsonl, guard)
    journal = NodeJournal(paths.checkpoint_db)
    ctx = NodeContext(
        paths=paths, process=proc, index=PoolIndex({}, {}), binding=bound(workspace),
        launch=Launch("opus", "medium", ("Read", "Write")), claude=claude, summarizer=None,
        exec_backend=SandboxExec(), gate=gate or OpenGate(), journal=journal, facts=facts,
        compose=compose or FakeCompose())

    def machine(thread="dyn-1", carry_from=None):
        return DynamicMachine(graph, thread, ctx, history, carry_from=carry_from)

    return SimpleNamespace(machine=machine, paths=paths, history=history, facts=facts, journal=journal,
                           graph=graph, saver=saver, ctx=ctx)


def drive(machine, cmd):
    """Send ``cmd`` and return (list of all items, last Status)."""
    items = list(machine.send(cmd))
    statuses = [i for i in items if isinstance(i, Status)]
    return items, statuses[-1]


def engine_ctx(workspace, guard, proc, claude, gateway, *, mode="start", restart_node=None,
               restart_summary="", compose=None, record=None, checkpointer=None, run_id=RUN_ID):
    paths = workspace.run(run_id)
    paths.root.mkdir(parents=True, exist_ok=True)
    install_extract_script(workspace)
    return EngineContext(
        paths=paths, process=proc, spec=build_graph_spec(proc), index=PoolIndex({}, {}),
        binding=bound(workspace), record=record or new_record(run_id), gateway=gateway, claude=claude,
        facts=RunFactsStore(paths.run_facts_jsonl, guard), history=History(paths.history_jsonl, guard),
        checkpointer=checkpointer if checkpointer is not None else InMemorySaver(), mode=mode,
        restart_node=restart_node, restart_summary=restart_summary, summarizer=NullSummarizer(),
        compose=compose or FakeCompose())
