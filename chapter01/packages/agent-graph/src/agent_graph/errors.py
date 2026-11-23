"""Exit states, run errors, findings and exceptions.

Covers: SFR-AGG-032 (exit state -> exit status), SSR-AGG-004 (vocabulary shared by
the static machine and the engine), SFR-AGG-005..009 (Finding).
"""

from dataclasses import dataclass, field
from enum import Enum
from typing import Literal

from . import config

StaticStateName = Literal["S1", "S2", "S3", "S4", "S5", "S6", "S7", "S8"]


class ExitState(str, Enum):
    COMPLETED = "COMPLETED"
    NOT_STARTED = "NOT_STARTED"
    INTENT_REJECTED = "INTENT_REJECTED"
    PROCESS_INVALID = "PROCESS_INVALID"
    INPUT_INVALID = "INPUT_INVALID"
    BUILD_FAILED = "BUILD_FAILED"
    CONTEXT_OVER_BUDGET = "CONTEXT_OVER_BUDGET"
    NODE_FAILED = "NODE_FAILED"
    BUDGET_EXCEEDED = "BUDGET_EXCEEDED"
    LLM_UNAVAILABLE = "LLM_UNAVAILABLE"
    OUTPUT_INVALID = "OUTPUT_INVALID"
    SUSPENDED = "SUSPENDED"

    @property
    def exit_code(self) -> int:
        """0 COMPLETED; 1 NOT_STARTED; 2 BUILD_FAILED, CONTEXT_OVER_BUDGET, NODE_FAILED,
        BUDGET_EXCEEDED, LLM_UNAVAILABLE, OUTPUT_INVALID; 3 INTENT_REJECTED,
        PROCESS_INVALID, INPUT_INVALID (SFR-AGG-032); SUSPENDED -> config.EXIT_SUSPENDED."""
        return _EXIT_CODES[self]


_EXIT_CODES = {
    ExitState.COMPLETED: 0,
    ExitState.NOT_STARTED: 1,
    ExitState.BUILD_FAILED: 2,
    ExitState.CONTEXT_OVER_BUDGET: 2,
    ExitState.NODE_FAILED: 2,
    ExitState.BUDGET_EXCEEDED: 2,
    ExitState.LLM_UNAVAILABLE: 2,
    ExitState.OUTPUT_INVALID: 2,
    ExitState.INTENT_REJECTED: 3,
    ExitState.PROCESS_INVALID: 3,
    ExitState.INPUT_INVALID: 3,
    ExitState.SUSPENDED: config.EXIT_SUSPENDED,
}


@dataclass(frozen=True)
class Finding:
    code: str               # e.g. "yaml", "api_version", "step_type", "unreachable", "missing_edge",
                            # "dup_edge", "unresolved_ref", "read_not_on_all_paths", "skill_missing",
                            # "snippet_invalid", "no_agent_snippet", "exclude_invalid", "dup_process_id",
                            # "input_missing", "input_schema", "set_unchosen", "argv_not_allowed", ...
    message: str
    file: str | None = None  # workspace-relative
    line: int | None = None  # 1-based
    step: str | None = None
    item: str | None = None  # skill/snippet/product/source name

    def to_json(self) -> dict:
        return {"code": self.code, "message": self.message, "file": self.file,
                "line": self.line, "step": self.step, "item": self.item}

    @staticmethod
    def from_json(d: dict) -> "Finding":
        return Finding(code=d["code"], message=d["message"], file=d.get("file"),
                       line=d.get("line"), step=d.get("step"), item=d.get("item"))


@dataclass(frozen=True)
class RunError:
    exit_state: ExitState
    state: StaticStateName
    message: str
    findings: tuple[Finding, ...] = ()
    node: str | None = None
    detail: dict = field(default_factory=dict)  # budget name/limit/count, attempts[], sizes ...

    def to_json(self) -> dict:
        return {"exit_state": ExitState(self.exit_state).value, "state": self.state,
                "message": self.message, "findings": [f.to_json() for f in self.findings],
                "node": self.node, "detail": dict(self.detail)}

    @staticmethod
    def from_json(d: dict) -> "RunError":
        return RunError(exit_state=ExitState(d["exit_state"]), state=d["state"], message=d["message"],
                        findings=tuple(Finding.from_json(f) for f in d.get("findings", ())),
                        node=d.get("node"), detail=dict(d.get("detail") or {}))


class AgentGraphError(Exception):
    """Base of all agent_graph exceptions."""


class UsageError(AgentGraphError):
    """Invalid command line (SFR-AGG-001): stderr usage, exit status 1."""


class TerminalClosed(AgentGraphError):
    """EOF/SIGHUP on the developer terminal -> SUSPENDED (SFR-AGG-036)."""


class ReplyTimeout(AgentGraphError):
    """The developer gave no reply within the ``timeout_s`` of a gateway ask/choose/confirm
    (SFR-AGG-016: a human step waits at most its timeout; the caller maps it to NODE_FAILED(timeout))."""

    def __init__(self, source: str, timeout_s: float):
        super().__init__(f"no reply at {source!r} within {timeout_s:g}s")
        self.source = source
        self.timeout_s = timeout_s


class Interrupted(AgentGraphError):
    """SIGINT/SIGTERM -> SUSPENDED (SFR-AGG-036)."""


class LlmUnavailable(AgentGraphError):
    def __init__(self, reason: str):
        super().__init__(reason)
        self.reason = reason


class ClaudeTurnError(AgentGraphError):
    """result.is_error, tool failure -> NODE_FAILED(tool)."""

    def __init__(self, reason: str):
        super().__init__(reason)
        self.reason = reason


class TurnCancelled(AgentGraphError):
    def __init__(self, cause: str):
        super().__init__(cause)
        self.cause = cause                  # "timeout" | "budget:<name>"


class ContextOverBudget(AgentGraphError):
    def __init__(self, node: str, budget: int, mandatory_tokens: int):
        super().__init__(f"node {node}: mandatory context {mandatory_tokens} tokens exceeds budget {budget}")
        self.node = node
        self.budget = budget
        self.mandatory_tokens = mandatory_tokens


class WriteBoundaryError(AgentGraphError):
    """A write outside /workspace and the Claude config dir, or into a protected pool (SFR-AGG-041)."""


class InputReadDenied(AgentGraphError):
    """Read of an .input file that is not bound (SSR-AGG-008)."""
