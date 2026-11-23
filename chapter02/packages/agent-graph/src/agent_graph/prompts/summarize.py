"""Prompt of the summary cache call (SFR-AGG-039, WP3).

The summarizer Claude instance reads exactly one file with the Read tool and answers with the
summary text only. The prompt is a fixed template: the same (file, budget) gives the same prompt.
"""

CHARS_PER_TOKEN_TARGET = 3          # aim below the budget: 3 chars per budgeted token

SUMMARY_PROMPT = (
    "Read the file `{path}` with the Read tool and write a summary of it.\n"
    "\n"
    "Rules:\n"
    "- Read only that one file. Do not use any other tool and do not ask questions.\n"
    "- The summary must stay within {budget_tokens} tokens (about {chars} characters).\n"
    "- Keep names, identifiers, numbers and headings exact; drop repetition and detail.\n"
    "- Answer with the summary text only: no preamble, no closing remarks.\n"
)


def build_summary_prompt(path: str, budget_tokens: int) -> str:
    """``path`` is relative to the summarizer's working directory."""
    return SUMMARY_PROMPT.format(path=path, budget_tokens=budget_tokens,
                                 chars=budget_tokens * CHARS_PER_TOKEN_TARGET)
