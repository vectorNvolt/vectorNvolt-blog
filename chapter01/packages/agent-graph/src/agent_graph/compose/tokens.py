"""Local token estimate and goal tokenizer (plan decisions 8, 9).

``estimate_tokens`` is ceil(utf-8 bytes / CHARS_PER_TOKEN). ``goal_tokens`` is the keyword set
used to rank skills and to match auto snippets: deterministic, no model call.
"""

import re

from .. import config

_WORD = re.compile(r"[a-z0-9]+")

STOPWORDS = frozenset({
    "the", "and", "for", "with", "from", "into", "that", "this", "are", "not", "per", "all",
    "any", "its", "via", "then", "than", "when", "each", "can", "has", "have", "was", "were",
})


def estimate_tokens(text: str) -> int:
    n = len(text.encode("utf-8"))
    return -(-n // config.CHARS_PER_TOKEN)


def word_tokens(text: str) -> frozenset:
    """Lower-case alphanumeric words of ``text`` (no stopword filter: used for names and tags)."""
    return frozenset(_WORD.findall(text.lower()))


def goal_tokens(*texts: str) -> frozenset:
    """Words of the node goal (step name plus instruction) without stopwords and 1-letter words."""
    out = set()
    for t in texts:
        for w in _WORD.findall(t.lower()):
            if len(w) >= 2 and w not in STOPWORDS:
                out.add(w)
    return frozenset(out)
