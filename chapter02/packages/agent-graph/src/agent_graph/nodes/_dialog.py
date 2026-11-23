"""Claude dialog helper of the static states S2 and S7 (read-only tools, JSON replies).

A dialog is one Claude session (``--resume`` between turns) whose every reply is one JSON object
matching ``schemas/dialog_reply.schema.yml``. The Claude instance cannot change anything; the
runtime applies approved proposals (SFR-AGG-033). When Claude is unavailable or keeps replying
with invalid JSON, ``DialogUnavailable`` is raised and the state falls back to a text-only menu
on the gateway (SFR-AGG-029).
"""

import json
import re
from dataclasses import dataclass
from pathlib import Path

from .. import config
from ..compose.manifest import EMPTY_MCP_JSON
from ..errors import ClaudeTurnError, LlmUnavailable, TurnCancelled
from ..ports import TurnRequest
from ..services.runfs import write_atomic
from ..services.yaml_schema import load_schema, validate
from . import Env

DIALOG_TIMEOUT_S = 300.0
_INVALID = ("Your last reply was not one valid JSON object of the required format. "
            "Reply again with exactly one JSON object and nothing else.")


class DialogUnavailable(Exception):
    """Claude cannot serve the dialog (login, network, timeout, repeated invalid replies)."""


@dataclass(frozen=True)
class DialogReply:
    say: str
    action: str
    process_id: "str | None"
    proposals: tuple


def _extract_json(text: str) -> "object | None":
    text = text.strip()
    fence = re.match(r"^```[a-zA-Z]*\n(.*)\n```$", text, re.DOTALL)
    candidates = [text] + ([fence.group(1)] if fence else [])
    if "{" in text and "}" in text:
        candidates.append(text[text.index("{"):text.rindex("}") + 1])
    for c in candidates:
        try:
            return json.loads(c)
        except ValueError:
            continue
    return None


def parse_reply(text: str) -> "DialogReply | None":
    obj = _extract_json(text)
    if not isinstance(obj, dict) or validate(obj, load_schema("dialog_reply")):
        return None
    return DialogReply(say=obj["say"], action=obj["action"], process_id=obj.get("process_id"),
                       proposals=tuple(obj.get("proposals") or ()))


class Dialog:
    def __init__(self, env: Env, cwd: Path, transcript: Path, source: "str | None" = None):
        self.env = env
        self.source = source            # set: each turn is rendered live on the gateway under this tag
        self.cwd = Path(cwd)
        self.transcript = Path(transcript)
        self.session: "str | None" = None
        write_atomic(self.cwd / "mcp.json", EMPTY_MCP_JSON, guard=env.guard)

    def _run(self, prompt: str):
        def request(events=None) -> TurnRequest:
            return TurnRequest(
                cwd=self.cwd, prompt=prompt, launch=self.env.dialog_launch(),
                allowed_tools=config.DIALOG_TOOLS, resume=self.session, transcript=self.transcript,
                timeout_s=DIALOG_TIMEOUT_S, env_extra={"TMPDIR": str(self.env.tmp_dir())}, events=events)

        if self.source is None:
            return self.env.claude.run_turn(request())
        # The reply is one JSON object, so the streamed text is not shown (show_text=False): the
        # operator sees spinner, tool panels and footer, and the caller then says ``reply.say`` once.
        with self.env.gateway.turn(self.source, show_text=False) as sink:
            return self.env.claude.run_turn(request(sink))

    def turn(self, prompt: str) -> DialogReply:
        for _ in range(2):
            self.env.tmp_dir().mkdir(parents=True, exist_ok=True)
            try:
                result = self._run(prompt)
            except (LlmUnavailable, ClaudeTurnError) as exc:
                raise DialogUnavailable(str(exc)) from exc
            except TurnCancelled as exc:
                raise DialogUnavailable(f"dialog turn cancelled: {exc.cause}") from exc
            self.session = result.session_id or self.session
            reply = parse_reply(result.text)
            if reply is not None:
                return reply
            prompt = _INVALID
        raise DialogUnavailable("Claude did not return a valid dialog reply")
