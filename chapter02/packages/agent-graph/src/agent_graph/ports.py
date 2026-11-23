"""Protocols and small value types shared by the work packages.

Covers: SSR-AGG-001 (GatewayPort is the only terminal interface), SSR-AGG-011
(Launch: identical invariant argv -> identical launch hash), SSR-AGG-004 (RunGate,
CancelToken: the engine bounds the dynamic machine), SSR-AGG-009 (NodeJournalPort
holds session ids).
"""

import hashlib
import json
import threading
from contextlib import AbstractContextManager
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Mapping, Protocol, Sequence

from .services.run_facts import RunFactsStore
from .services.runfs import RunPaths
from .state import Binding, PoolIndex, ProcessDef


@dataclass(frozen=True)
class Reply:
    """Developer reply. ``command`` is set for explicit commands ("/resolved", "/show", "/quit");
    ``text`` is then the rest of the line (e.g. "2" for "/show 2"). ``quit`` is True only for a
    confirmed quit (the gateway has asked y/N)."""
    text: str
    quit: bool = False
    command: str | None = None


class GatewayPort(Protocol):
    def attach_run(self, transcript_path: Path) -> None: ...          # flushes buffered transcript
    def say(self, source: str, text: str, *, from_llm: bool = False) -> None: ...
    def notice(self, source: str, code: str, text: str) -> None: ...
    # ask/choose/confirm raise TerminalClosed on EOF and, when ``timeout_s`` is given, ReplyTimeout
    # once that many seconds passed without a valid reply (re-asks count against the same deadline).
    def ask(self, source: str, prompt: str, *, allow_quit: bool = False,
            commands: Sequence[str] = (), timeout_s: float | None = None) -> Reply: ...
    def choose(self, source: str, prompt: str, options: Sequence[str], *,
               timeout_s: float | None = None) -> int: ...
    def confirm(self, source: str, prompt: str, *,
                timeout_s: float | None = None) -> bool: ...          # explicit y/n, re-asks
    def conversation(self, source: str) -> AbstractContextManager[None]: ...
    # One streamed Claude turn: yields a TurnSink for TurnRequest.events. ``show_text=False``: spinner,
    # tool/result panels and footer only; the caller prints the assistant text itself (via ``say``).
    def turn(self, source: str, *, show_text: bool = True) -> AbstractContextManager["TurnSink"]: ...


class TurnSink(Protocol):
    """Receives the validated stream-json events of one turn; ``dropped`` counts malformed ones."""
    dropped: int

    def feed(self, ev: dict) -> None: ...


@dataclass(frozen=True)
class Launch:
    model: str
    effort: str
    tools: tuple[str, ...]
    mcp_config: str = "mcp.json"            # relative to the cwd (active/); compose writes it
    system_prompt_file: str | None = "system.md"
    setting_sources: str = "project"
    strict_mcp: bool = True
    exclude_dynamic: bool = True

    def invariant_argv(self) -> list[str]:
        """['--output-format','stream-json','--verbose','--model',m,'--effort',e,'--tools',','.join(tools),
        '--setting-sources','project','--strict-mcp-config','--mcp-config',mcp,
        '--exclude-dynamic-system-prompt-sections','--append-system-prompt-file','system.md']

        Flags guarded by strict_mcp / exclude_dynamic / system_prompt_file are omitted when off."""
        argv = ["--output-format", "stream-json", "--verbose", "--model", self.model,
                "--effort", self.effort, "--tools", ",".join(self.tools),
                "--setting-sources", self.setting_sources]
        if self.strict_mcp:
            argv.append("--strict-mcp-config")
        argv += ["--mcp-config", self.mcp_config]
        if self.exclude_dynamic:
            argv.append("--exclude-dynamic-system-prompt-sections")
        if self.system_prompt_file is not None:
            argv += ["--append-system-prompt-file", self.system_prompt_file]
        return argv

    def launch_hash(self) -> str:
        return hashlib.sha256(json.dumps(self.invariant_argv()).encode("utf-8")).hexdigest()


@dataclass(frozen=True)
class TurnRequest:
    cwd: Path
    prompt: str
    launch: Launch
    allowed_tools: tuple[str, ...]
    resume: str | None
    transcript: Path
    timeout_s: float | None = None
    env_extra: Mapping[str, str] = field(default_factory=dict)
    events: "TurnSink | None" = None        # set: --include-partial-messages, validated events are fed to it


@dataclass(frozen=True)
class Usage:
    input_tokens: int = 0
    cache_creation_input_tokens: int = 0
    cache_read_input_tokens: int = 0
    ephemeral_1h_input_tokens: int = 0
    ephemeral_5m_input_tokens: int = 0

    def to_json(self) -> dict:
        return {"input_tokens": self.input_tokens,
                "cache_creation_input_tokens": self.cache_creation_input_tokens,
                "cache_read_input_tokens": self.cache_read_input_tokens,
                "ephemeral_1h_input_tokens": self.ephemeral_1h_input_tokens,
                "ephemeral_5m_input_tokens": self.ephemeral_5m_input_tokens}


@dataclass(frozen=True)
class TurnResult:
    session_id: str
    text: str
    usages: tuple[Usage, ...]
    compactions: tuple[dict, ...]
    exit_code: int
    argv: tuple[str, ...]


class CancelToken:
    """Thread-safe one-shot cancel flag. The first cause wins."""

    def __init__(self) -> None:
        self._event = threading.Event()
        self._lock = threading.Lock()
        self._cause: str | None = None

    def cancel(self, cause: str) -> None:
        with self._lock:
            if self._cause is None:
                self._cause = cause
        self._event.set()

    @property
    def cancelled(self) -> bool:
        return self._event.is_set()

    @property
    def cause(self) -> str | None:
        return self._cause

    def wait(self, timeout: float) -> bool:
        """Block up to ``timeout`` seconds; True if cancelled."""
        return self._event.wait(timeout)


class ClaudePort(Protocol):
    def run_turn(self, req: TurnRequest, cancel: CancelToken | None = None) -> TurnResult: ...
    # raises LlmUnavailable | ClaudeTurnError | TurnCancelled; appends every stdout event to req.transcript


class SummarizerPort(Protocol):
    def summary(self, path: Path, sha256: str, budget_tokens: int) -> str: ...   # cached, deterministic after 1st


@dataclass(frozen=True)
class ExecResult:
    exit_code: int
    stdout_tail: str
    stderr_tail: str
    cause: str | None = None        # ContainerExec: result.json cause (ok|exit|usage|setup|...)
    exec_dir: str | None = None     # ContainerExec: the tool's exec dir (holds out/)


class ExecBackend(Protocol):
    def run(self, argv: Sequence[str], cwd: Path, env: Mapping[str, str],
            timeout_s: float | None, cancel: CancelToken) -> ExecResult: ...       # raises TurnCancelled


class RunGate(Protocol):                     # provided by the engine (WP5 engine/budgets.py)
    def attempt_token(self, node: str) -> CancelToken: ...      # fresh token per attempt
    def before_node_start(self, node: str) -> str | None: ...   # refusal cause, e.g. "budget:max_steps"
    def before_llm_call(self, node: str) -> str | None: ...     # refusal cause, e.g. "budget:max_llm_calls"


class NodeJournalPort(Protocol):
    def get(self, node: str, visit: int, key: str) -> dict | None: ...
    def put(self, node: str, visit: int, key: str, value: dict) -> None: ...
    def session(self, node: str, visit: int) -> str | None: ...
    def set_session(self, node: str, visit: int, session_id: str) -> None: ...


@dataclass
class DynamicContext:                       # LangGraph context_schema of the dynamic graph
    paths: RunPaths
    process: ProcessDef
    index: PoolIndex
    binding: Binding
    launch: Launch
    claude: ClaudePort
    summarizer: SummarizerPort
    exec_backend: ExecBackend
    gate: RunGate
    journal: NodeJournalPort
    facts: RunFactsStore
