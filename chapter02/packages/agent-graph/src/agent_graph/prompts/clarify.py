"""Prompts of the S2 clarify dialog (SFR-AGG-003). Pure strings; the dialog code sends them."""

from typing import Sequence

from ..process.catalog import CatalogEntry

REPLY_FORMAT = """\
Reply with exactly one JSON object and nothing else (no code fence, no text around it):
{"say": "<what the developer reads>", "action": "ask" | "agree" | "propose" | "done",
 "process_id": "<id or null>", "proposals": []}
Actions: "ask" = you need an answer from the developer (put the question in "say");
"agree" = exactly one process matches (set "process_id"; the developer confirms next);
"done" with "process_id": null = no listed process matches the developer's intent."""


def format_catalog(entries: Sequence[CatalogEntry]) -> str:
    lines = []
    for e in entries:
        if e.selectable:
            lines.append(f"- {e.id}: {e.title}")
        elif e.error:
            lines.append(f"- {e.file}: not selectable ({e.error})")
        else:
            lines.append(f"- {e.id}: not selectable, id shared with {', '.join(e.duplicates)}")
    return "\n".join(lines) if lines else "(no process files)"


def clarify_first_prompt(entries: Sequence[CatalogEntry], intent: str) -> str:
    listing = []
    for e in entries:
        if e.selectable:
            listing.append(f"id: {e.id}\ntitle: {e.title}\ndescription: {e.description}")
    return (
        "You help a developer choose which process definition to run. Match the developer's "
        "wording and intent against the id, title and description of the selectable processes "
        "below. Ask back until exactly one process matches. Do not invent processes and do not "
        "use tools to look for others.\n\n"
        "Selectable processes:\n\n" + "\n\n".join(listing) + "\n\n"
        f"Developer's request:\n{intent}\n\n" + REPLY_FORMAT)


def clarify_unknown(process_id: str) -> str:
    return (f"The process id {process_id!r} is not selectable. Choose among the listed ids or ask "
            "the developer. " + REPLY_FORMAT)


def clarify_rejected(process_id: str, developer_text: str) -> str:
    return (f"The developer did not confirm process {process_id!r} and said: {developer_text}\n"
            "Continue matching. " + REPLY_FORMAT)


def clarify_followup(developer_text: str) -> str:
    return f"Developer: {developer_text}\n\n" + REPLY_FORMAT
