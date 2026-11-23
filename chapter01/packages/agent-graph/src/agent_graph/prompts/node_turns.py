"""Turn prompts of a cognitive node (the user messages of its session).

Covers: SFR-AGG-019 (result and handoff protocol, decision Q6), SFR-AGG-022 (answer turn),
SFR-AGG-026 (retry turn carries the failure reason), SSR-AGG-010 (the prompts carry no
instructions of their own beyond the protocol; the step instructions live in CLAUDE.md).
"""


def first_turn(node: str, work_rel: str, outcomes: "tuple[str, ...]") -> str:
    outcome_line = (
        f'Set "outcome" in result.json to exactly one of: {", ".join(outcomes)}.'
        if outcomes else 'Set "outcome" in result.json to null.')
    return (
        f"Execute step '{node}' as described in CLAUDE.md. The context pack is context.md.\n"
        f"Write every product to {work_rel}/<file> (the paths are given in CLAUDE.md).\n"
        "When finished, write result.json in the current directory:\n"
        '{"status": "done", "outcome": <string|null>, "question": null, '
        '"handoff": {"done": "<what you did>", "decisions": [], "open_points": []}, "warnings": []}\n'
        f"{outcome_line}\n"
        'If you cannot continue without the developer, write result.json with "status": "question" '
        'and put one question in "question"; you will receive the answer as the next message.'
    )


def retry_turn(reason: str) -> str:
    return (
        "Your previous attempt failed.\n"
        f"Reason: {reason}\n"
        "Fix it, rewrite the affected products and result.json, and finish as before."
    )


def answer_turn(text: str) -> str:
    return (
        f"The developer answered: {text}\n"
        "Continue the step, write the products and result.json (status done) as before."
    )
