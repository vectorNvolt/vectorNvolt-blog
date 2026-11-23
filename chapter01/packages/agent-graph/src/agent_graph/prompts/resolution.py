"""Prompts of the S7 resolution dialog (SFR-AGG-033). Pure strings.

The Claude instance has the Read tool only. It cannot change anything: every fix is a proposal
that the runtime applies after the developer approved it.
"""

import json
from typing import Sequence

from ..errors import RunError
from ..services.render import format_findings

REPLY_FORMAT = """\
Reply with exactly one JSON object and nothing else (no code fence, no text around it):
{"say": "<what the developer reads: explanation, question or summary>",
 "action": "ask" | "propose" | "done",
 "process_id": null,
 "proposals": [<change>, ...]}
Actions: "ask" = you need an answer; "propose" = the changes in "proposals" are for the developer
to approve; "done" = the cause is fixed and the run can continue.
A <change> is one of:
 {"kind": "edit_file", "path": "<workspace-relative path>", "content": "<complete new file text, or null to delete the file>", "reason": "..."}
 {"kind": "override", "key": "retry|timeout|budgets.max_steps|budgets.max_llm_calls|budgets.timeout|budget", "node": "<step name or null>", "value": <number or "10m" or "60k">, "reason": "..."}
 {"kind": "choose_process", "value": "<process id>", "reason": "..."}
Per-node overrides (node set) accept the keys retry, timeout and budget only.
Only these paths may be changed: .process/, .input/ and identity/ (identity/.skills/, identity/.snippet/).
Give the complete file text for edit_file. Never propose a change you have not read the file for."""


def resolution_first_prompt(err: RunError, process_file: "str | None") -> str:
    detail = json.dumps(err.detail, sort_keys=True, ensure_ascii=False) if err.detail else "{}"
    findings = format_findings(err.findings) or "(none)"
    return (
        "A run of a developer-defined process stopped with an error. Explain it to the developer "
        "in plain words, propose fixes and, once the developer approved them, tell the runtime "
        "that you are done. You may read files in the workspace with the Read tool (the process "
        f"file is {process_file or 'not chosen yet'}; paths are relative to /workspace, which is "
        "your read root: use absolute paths starting with /workspace).\n\n"
        f"Exit state: {err.exit_state.value}\nFailed state: {err.state}\n"
        f"Node: {err.node or '-'}\nMessage: {err.message}\nDetail: {detail}\n"
        f"Findings:\n{findings}\n\n" + REPLY_FORMAT)


def resolution_followup(results: Sequence[str], developer_text: str) -> str:
    parts = []
    if results:
        parts.append("Outcome of your proposals:\n" + "\n".join(f"- {r}" for r in results))
    if developer_text:
        parts.append(f"Developer: {developer_text}")
    if not parts:
        parts.append("Continue.")
    return "\n\n".join(parts) + "\n\n" + REPLY_FORMAT
