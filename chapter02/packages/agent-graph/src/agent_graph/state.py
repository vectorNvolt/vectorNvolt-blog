"""Process model (frozen dataclasses) and graph states (TypedDicts holding references only).

Covers: SSR-AGG-005 (state holds references: the TypedDicts below carry only JSON
primitives, ids, paths and hashes, never file content), SFR-AGG-005/006/008/009/010
(process, pool index, binding, graph spec), SFR-AGG-037 (RunRecord = run.json).

Process model fields mirror the process YAML (``apiVersion`` -> ``api_version``,
``transitions.entry/edges`` -> ``entry``/``edges``, durations parsed to seconds, ``40k``
budgets parsed to tokens by the loader in WP2).
"""

import dataclasses
from collections.abc import Mapping
from dataclasses import dataclass, field
from enum import Enum
from pathlib import Path
from typing import Annotated, Literal, TypedDict

from . import config
from .errors import RunError, StaticStateName

WhenKind = Literal["done", "outcome", "hil_ok", "hil_nok"]
ReadKind = Literal["input.source", "input.ruleset", "input.output-template-ref", "product"]


# ---------------------------------------------------------------- JSON helpers

def _jsonable(v):
    """Recursively convert dataclasses/tuples/Mappings/Enums/Paths to JSON primitives."""
    if not isinstance(v, type) and hasattr(v, "to_json"):
        return v.to_json()
    if dataclasses.is_dataclass(v) and not isinstance(v, type):
        return _fields_json(v)
    if isinstance(v, Enum):
        return v.value
    if isinstance(v, Mapping):
        return {str(k): _jsonable(x) for k, x in v.items()}
    if isinstance(v, (set, frozenset)):
        return sorted(_jsonable(x) for x in v)
    if isinstance(v, (tuple, list)):
        return [_jsonable(x) for x in v]
    if isinstance(v, Path):
        return str(v)
    return v


def _fields_json(obj) -> dict:
    return {f.name: _jsonable(getattr(obj, f.name)) for f in dataclasses.fields(obj)}


def _t(x) -> tuple:
    return tuple(x or ())


# ---------------------------------------------------------------- process model

@dataclass(frozen=True)
class InputSource:
    name: str
    path: str | None
    note: str | None
    schema: str | None
    required: bool
    line: int | None


@dataclass(frozen=True)
class FileRef:                      # template / ruleset, path relative to /workspace
    name: str
    path: str
    required: bool
    line: int | None


@dataclass(frozen=True)
class OutputTarget:
    name: str
    path: str
    template: str | None
    checks: tuple[str, ...]
    required: bool
    line: int | None


@dataclass(frozen=True)
class Budgets:
    max_steps: int
    max_llm_calls: int
    timeout_s: int


@dataclass(frozen=True)
class RuntimeCfg:
    tools: tuple[str, ...]
    snippets: tuple[str, ...]
    skills: tuple[str, ...]
    max_skills: int
    retry: int
    timeout_s: int
    budgets: Budgets
    work_products: Literal["archive", "delete", "keep"]


@dataclass(frozen=True)
class ProductDecl:
    name: str
    file: str
    schema: str | None
    output_target: str | None
    line: int | None


@dataclass(frozen=True)
class ReadRef:
    raw: str
    kind: ReadKind
    name: str
    selector: str
    optional: bool


@dataclass(frozen=True)
class StepDef:
    id: int
    name: str
    type: Literal["procedure", "cognitive", "human"]
    instruction: str
    reads: tuple[ReadRef, ...]
    produces: tuple[ProductDecl, ...]
    outcomes: tuple[str, ...]
    success: str | None
    failure: str | None
    warning: str | None
    retry: int | None
    timeout_s: int | None
    run: tuple[str, ...] | None         # procedure (argv template, shlex-split)
    isolate: bool
    snippets: tuple[str, ...]           # cognitive
    skills: tuple[str, ...]
    exclude_skills: tuple[str, ...]
    max_skills: int | None
    tools: tuple[str, ...] | None
    budget: int | None
    line: int | None
    profile: str | None = None          # isolate: true -> exec profile of run_ephemeral_container (USC-005)


@dataclass(frozen=True)
class Edge:
    src: str
    dst: str                            # "END" allowed
    when: WhenKind
    outcome: str | None
    line: int | None


@dataclass(frozen=True)
class ProcessDef:
    api_version: str
    id: str
    title: str
    description: str
    sources: tuple[InputSource, ...]
    templates: tuple[FileRef, ...]
    rulesets: tuple[FileRef, ...]
    targets: tuple[OutputTarget, ...]
    runtime: RuntimeCfg
    steps: tuple[StepDef, ...]
    entry: str
    edges: tuple[Edge, ...]
    source_file: str
    source_sha256: str

    def step(self, name: str) -> StepDef:
        for s in self.steps:
            if s.name == name:
                return s
        raise KeyError(name)

    def retry_of(self, step: StepDef) -> int:
        return step.retry if step.retry is not None else self.runtime.retry

    def timeout_of(self, step: StepDef) -> int | None:
        """Human steps: only their own timeout (None if absent). Others: step or runtime default."""
        if step.type == "human":
            return step.timeout_s
        return step.timeout_s if step.timeout_s is not None else self.runtime.timeout_s

    def max_skills_of(self, step: StepDef) -> int:
        return step.max_skills if step.max_skills is not None else self.runtime.max_skills

    def budget_of(self, step: StepDef) -> int:
        return step.budget if step.budget is not None else config.DEFAULT_NODE_BUDGET

    def to_json(self) -> dict:
        return _fields_json(self)

    @staticmethod
    def from_json(d: dict) -> "ProcessDef":
        rt = d["runtime"]
        runtime = RuntimeCfg(
            tools=_t(rt["tools"]), snippets=_t(rt["snippets"]), skills=_t(rt["skills"]),
            max_skills=rt["max_skills"], retry=rt["retry"], timeout_s=rt["timeout_s"],
            budgets=Budgets(**rt["budgets"]), work_products=rt["work_products"])
        return ProcessDef(
            api_version=d["api_version"], id=d["id"], title=d["title"], description=d["description"],
            sources=tuple(InputSource(**s) for s in d["sources"]),
            templates=tuple(FileRef(**s) for s in d["templates"]),
            rulesets=tuple(FileRef(**s) for s in d["rulesets"]),
            targets=tuple(OutputTarget(**{**t, "checks": _t(t["checks"])}) for t in d["targets"]),
            runtime=runtime,
            steps=tuple(_step_from_json(s) for s in d["steps"]),
            entry=d["entry"], edges=tuple(Edge(**e) for e in d["edges"]),
            source_file=d["source_file"], source_sha256=d["source_sha256"])


def _step_from_json(s: dict) -> StepDef:
    return StepDef(
        id=s["id"], name=s["name"], type=s["type"], instruction=s["instruction"],
        reads=tuple(ReadRef(**r) for r in s["reads"]),
        produces=tuple(ProductDecl(**p) for p in s["produces"]),
        outcomes=_t(s["outcomes"]), success=s["success"], failure=s["failure"], warning=s["warning"],
        retry=s["retry"], timeout_s=s["timeout_s"],
        run=None if s["run"] is None else _t(s["run"]), isolate=s["isolate"],
        snippets=_t(s["snippets"]), skills=_t(s["skills"]), exclude_skills=_t(s["exclude_skills"]),
        max_skills=s["max_skills"], tools=None if s["tools"] is None else _t(s["tools"]),
        budget=s["budget"], line=s["line"], profile=s.get("profile"))


# ---------------------------------------------------------------- pools

@dataclass(frozen=True)
class SkillEntry:
    name: str
    description: str
    tags: tuple[str, ...]
    dir: str                                    # workspace-relative
    files: tuple[tuple[str, str], ...]          # (relpath, sha256)


@dataclass(frozen=True)
class SnippetEntry:
    id: str
    layer: Literal["agent", "process", "step"]
    tags: tuple[str, ...]
    auto: bool
    path: str                                   # workspace-relative
    sha256: str                                 # of the whole file
    body: str                                   # front matter stripped


@dataclass(frozen=True)
class PoolIndex:
    skills: Mapping[str, SkillEntry]
    snippets: Mapping[str, SnippetEntry]

    def to_json(self) -> dict:
        """Bodies are not serialized; hashes are."""
        return {
            "skills": {k: _fields_json(v) for k, v in self.skills.items()},
            "snippets": {k: {f.name: _jsonable(getattr(v, f.name))
                             for f in dataclasses.fields(v) if f.name != "body"}
                         for k, v in self.snippets.items()},
        }

    @staticmethod
    def from_json(d: dict, workspace: Path) -> "PoolIndex":
        """Re-reads snippet bodies; raises ValueError when a file's sha256 differs from the index."""
        from .services.runfs import sha256_bytes
        from .services.yaml_schema import parse_front_matter
        skills = {k: SkillEntry(name=v["name"], description=v["description"], tags=_t(v["tags"]),
                                dir=v["dir"], files=tuple((a, b) for a, b in v["files"]))
                  for k, v in d["skills"].items()}
        snippets = {}
        for k, v in d["snippets"].items():
            raw = (Path(workspace) / v["path"]).read_bytes()
            if sha256_bytes(raw) != v["sha256"]:
                raise ValueError(f"snippet changed since indexing: {v['path']}")
            _meta, body, _err = parse_front_matter(raw.decode("utf-8"))
            snippets[k] = SnippetEntry(id=v["id"], layer=v["layer"], tags=_t(v["tags"]),
                                       auto=v["auto"], path=v["path"], sha256=v["sha256"], body=body)
        return PoolIndex(skills=skills, snippets=snippets)


# ---------------------------------------------------------------- binding

@dataclass(frozen=True)
class BoundFile:
    path: str                                   # workspace-relative
    sha256: str


@dataclass(frozen=True)
class BoundSource:
    name: str
    files: tuple[BoundFile, ...]
    note_text: str | None


@dataclass(frozen=True)
class Binding:
    process_id: str
    process_file: str
    process_sha256: str
    sources: Mapping[str, BoundSource]
    templates: Mapping[str, BoundFile]
    rulesets: Mapping[str, BoundFile]

    def allowed_input_paths(self) -> frozenset[str]:
        paths = {f.path for s in self.sources.values() for f in s.files}
        paths.update(f.path for f in self.templates.values())
        paths.update(f.path for f in self.rulesets.values())
        return frozenset(paths)

    def to_json(self) -> dict:
        return _fields_json(self)

    @staticmethod
    def from_json(d: dict) -> "Binding":
        return Binding(
            process_id=d["process_id"], process_file=d["process_file"], process_sha256=d["process_sha256"],
            sources={k: BoundSource(name=v["name"], files=tuple(BoundFile(**f) for f in v["files"]),
                                    note_text=v["note_text"]) for k, v in d["sources"].items()},
            templates={k: BoundFile(**v) for k, v in d["templates"].items()},
            rulesets={k: BoundFile(**v) for k, v in d["rulesets"].items()})


# ---------------------------------------------------------------- graph spec

@dataclass(frozen=True)
class GraphNode:
    name: str
    type: str
    step_hash: str
    reads: tuple[str, ...]
    produces: tuple[str, ...]


@dataclass(frozen=True)
class GraphSpec:                                # == graph.json
    process_id: str
    process_sha256: str
    entry: str
    nodes: tuple[GraphNode, ...]
    edges: tuple[Edge, ...]
    order: tuple[str, ...]                      # BFS order from entry; defines "earliest"
    engine_entry: str = "restart_at|entry"      # reserved START router, not a process edge
    process: dict = field(default_factory=dict)  # ProcessDef.to_json()

    def to_json(self) -> dict:
        return _fields_json(self)

    @staticmethod
    def from_json(d: dict) -> "GraphSpec":
        return GraphSpec(
            process_id=d["process_id"], process_sha256=d["process_sha256"], entry=d["entry"],
            nodes=tuple(GraphNode(name=n["name"], type=n["type"], step_hash=n["step_hash"],
                                  reads=_t(n["reads"]), produces=_t(n["produces"])) for n in d["nodes"]),
            edges=tuple(Edge(**e) for e in d["edges"]), order=_t(d["order"]),
            engine_entry=d.get("engine_entry", "restart_at|entry"), process=dict(d.get("process") or {}))


# ---------------------------------------------------------------- run facts, re-entry, counters

@dataclass(frozen=True)
class ProductRef:
    name: str
    node: str
    path: str                                   # workspace-relative
    sha256: str


@dataclass(frozen=True)
class RunFact:
    seq: int
    kind: Literal["answer", "hil_ok", "hil_nok", "resolution"]
    node: str | None
    text: str


@dataclass(frozen=True)
class ReEntry:
    state: StaticStateName                      # S1..S6
    restart_node: str | None = None             # for S6 restarts
    new_run: bool = False                       # process choice changed -> S2 + new runID
    summary: str = ""                           # resolution summary -> run_facts + RESTART

    def to_json(self) -> dict:
        return _fields_json(self)

    @staticmethod
    def from_json(d: dict) -> "ReEntry":
        return ReEntry(state=d["state"], restart_node=d.get("restart_node"),
                       new_run=bool(d.get("new_run", False)), summary=d.get("summary", ""))


@dataclass
class EngineCounters:
    steps: int = 0
    llm_calls: int = 0
    elapsed_s: float = 0.0
    failures: dict[str, int] = field(default_factory=dict)
    attempts: dict[str, list[str]] = field(default_factory=dict)   # node -> reasons
    p_run: int | None = None
    cognitive_started: int = 0
    drift: list[str] = field(default_factory=list)

    def to_json(self) -> dict:
        return _fields_json(self)

    @staticmethod
    def from_json(d: dict) -> "EngineCounters":
        return EngineCounters(
            steps=d.get("steps", 0), llm_calls=d.get("llm_calls", 0), elapsed_s=d.get("elapsed_s", 0.0),
            failures=dict(d.get("failures") or {}),
            attempts={k: list(v) for k, v in (d.get("attempts") or {}).items()},
            p_run=d.get("p_run"), cognitive_started=d.get("cognitive_started", 0),
            drift=list(d.get("drift") or []))


@dataclass
class RunRecord:                                # == run.json
    run_id: str
    process_id: str
    process_file: str
    created: str
    phase: StaticStateName
    status: Literal["ACTIVE", "SUSPENDED", "ESCALATED", "ENDED"]
    model: str
    effort: str
    dynamic_thread: str = "dyn-1"
    open_error: RunError | None = None
    reentry: ReEntry | None = None
    counters: EngineCounters = field(default_factory=EngineCounters)
    resolution_count: int = 0
    exit_state: str | None = None
    exit_code: int | None = None

    def to_json(self) -> dict:
        return _fields_json(self)

    @staticmethod
    def from_json(d: dict) -> "RunRecord":
        return RunRecord(
            run_id=d["run_id"], process_id=d["process_id"], process_file=d["process_file"],
            created=d["created"], phase=d["phase"], status=d["status"], model=d["model"],
            effort=d["effort"], dynamic_thread=d.get("dynamic_thread", "dyn-1"),
            open_error=RunError.from_json(d["open_error"]) if d.get("open_error") else None,
            reentry=ReEntry.from_json(d["reentry"]) if d.get("reentry") else None,
            counters=EngineCounters.from_json(d.get("counters") or {}),
            resolution_count=d.get("resolution_count", 0),
            exit_state=d.get("exit_state"), exit_code=d.get("exit_code"))


# ---------------------------------------------------------------- graph states

def merge_dict(a: dict, b: dict) -> dict:
    """Reducer: shallow merge, keys of ``b`` win. None operands count as empty."""
    return {**(a or {}), **(b or {})}


class DynState(TypedDict, total=False):
    restart_at: str | None
    restart_summary: str | None
    continue_session: bool
    last_node: str | None
    last_outcome: str | None
    last_hil: Literal["hil_ok", "hil_nok"] | None
    visits: Annotated[dict[str, int], merge_dict]
    products: Annotated[dict[str, dict], merge_dict]    # name -> ProductRef as dict
    handoffs: Annotated[dict[str, str], merge_dict]     # node -> "nodes/<node>/handoff.json"
    summaries: Annotated[dict[str, str], merge_dict]    # node -> <=500 chars
    output_ref: str | None


class StaticState(TypedDict, total=False):
    resume_run_id: str | None
    run_id: str | None
    process_id: str | None
    process_file: str | None
    phase: StaticStateName
    error: dict | None                                  # RunError.to_json()
    reentry: dict | None
    quit: bool
    exit_state: str | None
    completed_output: str | None


class EngineState(TypedDict, total=False):
    mode: Literal["start", "restart", "resume"]
    restart_node: str | None
    restart_summary: str
    pending: dict | None            # last Status.to_json() needing handling
    next_cmd: dict | None           # Cmd.to_json() to send
    escalation: dict | None         # RunError.to_json()
    output_ref: str | None
    done: bool
