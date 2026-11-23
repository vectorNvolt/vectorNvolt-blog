"""Summary cache and summarizer (SFR-AGG-039, SSR-AGG-010).

``SummaryCache`` implements SummarizerPort: a summary is made once per (file content hash,
budget) by a Claude call that reads the file with the Read tool, and stored in
``.agent/cache/summaries/<sha256>-<budget>.md``. Later calls, also in other runs, read the file.

Deviation notes for callers:
- The call counts as an LLM call: ``gate.before_llm_call("summarize")``; a refusal raises
  TurnCancelled(cause).
- The Claude call runs in ``.agent/cache/summarizer/`` with its own launch (tools Read, no
  system prompt file, empty mcp.json); it is not a node and has no ``active/`` folder.
"""

import dataclasses
import os
import threading
from pathlib import Path

from ..compose.manifest import EMPTY_MCP_JSON
from ..compose.normalize import normalize_block
from ..errors import ClaudeTurnError, TurnCancelled
from ..ports import ClaudePort, Launch, RunGate, TurnRequest
from ..prompts.summarize import build_summary_prompt
from .runfs import WorkspacePaths, WriteGuard, sha256_file, write_atomic

SUMMARIZER_NODE = "summarize"
SUMMARY_TIMEOUT_S = 300.0


class SummaryCache:
    def __init__(self, ws: WorkspacePaths, claude: ClaudePort, launch: Launch,
                 gate: "RunGate | None", guard: WriteGuard):
        self.ws = ws
        self.claude = claude
        self.gate = gate
        self.guard = guard
        self.launch = dataclasses.replace(launch, tools=("Read",), system_prompt_file=None)
        self._lock = threading.Lock()

    @property
    def workdir(self) -> Path:
        return self.ws.agent_dir / "cache" / "summarizer"

    def cache_file(self, sha256: str, budget_tokens: int) -> Path:
        return self.ws.cache_summaries / f"{sha256}-{budget_tokens}.md"

    def summary(self, path: Path, sha256: str, budget_tokens: int) -> str:
        p = Path(path)
        if not p.is_absolute():
            p = self.ws.root / p
        key = sha256_file(p)                       # the content decides, not the caller's claim
        cache = self.cache_file(key, budget_tokens)
        with self._lock:
            if cache.is_file():
                return normalize_block(cache.read_text(encoding="utf-8"))
            if self.gate is not None:
                refusal = self.gate.before_llm_call(SUMMARIZER_NODE)
                if refusal:
                    raise TurnCancelled(refusal)
            work = self.workdir
            write_atomic(work / self.launch.mcp_config, EMPTY_MCP_JSON, guard=self.guard)
            rel = Path(os.path.relpath(p, work)).as_posix()
            req = TurnRequest(cwd=work, prompt=build_summary_prompt(rel, budget_tokens),
                              launch=self.launch, allowed_tools=("Read",), resume=None,
                              transcript=work / "transcript.jsonl", timeout_s=SUMMARY_TIMEOUT_S)
            result = self.claude.run_turn(req)
            text = normalize_block(result.text)
            if not text:
                raise ClaudeTurnError(f"empty summary for {rel}")
            write_atomic(cache, text + "\n", guard=self.guard)
            return text
