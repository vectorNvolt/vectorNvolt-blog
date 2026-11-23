"""Command/status vocabulary between the S6 engine and the dynamic machine.

Covers: SSR-AGG-004 (the dynamic machine is driven only through these commands and
reports only these statuses), SFR-AGG-013 (notices), SFR-AGG-012 (history records them).

Payload keys::

    Cmd:    RESTART{summary, continue_session}  HIL_NOK{feedback}  ANSWER{text}
            RETRY{reason}  CANCEL{cause}  SUSPEND{reason}
    Status: NODE_STARTED{context_ref}  NODE_DONE{outcome, products:[ProductRef dict], summary}
            WAITING{mode:"HIL", brief, reads:[{ref, path, inline?}]} | WAITING{mode:"INPUT", question}
            NODE_FAILED{kind:FailKind, reason, cause: None|"llm_unavailable"|"cancelled:<cause>", retryable}
            COMPLETED{output_ref}  SUSPENDED{}
"""

from dataclasses import dataclass, field
from typing import Any, Literal, Mapping

CommandName = Literal["START", "RESTART", "HIL_OK", "HIL_NOK", "ANSWER", "RETRY", "CANCEL", "SUSPEND"]
StatusName = Literal["NODE_STARTED", "NODE_DONE", "WAITING", "NODE_FAILED", "COMPLETED", "SUSPENDED"]
FailKind = Literal["schema", "tool", "timeout", "context"]

COMMAND_NAMES = frozenset({"START", "RESTART", "HIL_OK", "HIL_NOK", "ANSWER", "RETRY", "CANCEL", "SUSPEND"})
STATUS_NAMES = frozenset({"NODE_STARTED", "NODE_DONE", "WAITING", "NODE_FAILED", "COMPLETED", "SUSPENDED"})

# Statuses at which the dynamic machine stops and waits for the next command.
PAUSING = frozenset({"WAITING", "NODE_FAILED", "COMPLETED", "SUSPENDED"})


@dataclass(frozen=True)
class Cmd:
    name: CommandName
    node: str | None = None
    payload: Mapping[str, Any] = field(default_factory=dict)

    def to_json(self) -> dict:
        return {"name": self.name, "node": self.node, "payload": dict(self.payload)}

    @staticmethod
    def from_json(d: dict) -> "Cmd":
        name = d["name"]
        if name not in COMMAND_NAMES:
            raise ValueError(f"unknown command: {name!r}")
        return Cmd(name=name, node=d.get("node"), payload=dict(d.get("payload") or {}))


@dataclass(frozen=True)
class Status:
    name: StatusName
    node: str | None = None
    payload: Mapping[str, Any] = field(default_factory=dict)

    def to_json(self) -> dict:
        return {"name": self.name, "node": self.node, "payload": dict(self.payload)}

    @staticmethod
    def from_json(d: dict) -> "Status":
        name = d["name"]
        if name not in STATUS_NAMES:
            raise ValueError(f"unknown status: {name!r}")
        return Status(name=name, node=d.get("node"), payload=dict(d.get("payload") or {}))


@dataclass(frozen=True)
class Notice:
    """Not a status; the gateway prints it. code: "WARNING" | "CACHE_PREFIX_DRIFT" | "PROGRESS"."""
    source: str
    code: str
    text: str


def start() -> Cmd:
    return Cmd("START")


def restart(node: str, summary: str = "", continue_session: bool = False) -> Cmd:
    return Cmd("RESTART", node, {"summary": summary, "continue_session": continue_session})


def hil_ok(node: str) -> Cmd:
    return Cmd("HIL_OK", node)


def hil_nok(node: str, feedback: str) -> Cmd:
    return Cmd("HIL_NOK", node, {"feedback": feedback})


def answer(node: str, text: str) -> Cmd:
    return Cmd("ANSWER", node, {"text": text})


def retry(node: str, reason: str) -> Cmd:
    return Cmd("RETRY", node, {"reason": reason})


def cancel(node: str, cause: str) -> Cmd:
    return Cmd("CANCEL", node, {"cause": cause})


def suspend(reason: str) -> Cmd:
    return Cmd("SUSPEND", None, {"reason": reason})
