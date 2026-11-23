"""Grammar of ``reads`` / ``produces`` references, ``when`` clauses and ``run`` placeholders.

Covers: SFR-AGG-005 (references that cannot be resolved are rejected: a reference that
does not match this grammar is a finding), SFR-AGG-007 (product names).

Reads (a trailing ``?`` marks an optional read)::

    input.source[Name]                 -> kind "input.source"
    input.ruleset[Name]                -> kind "input.ruleset"
    input.output-template-ref[Name]    -> kind "input.output-template-ref"
    output.target[Name]                -> kind "product", name = target name
    <product>                          -> kind "product"
    <product>.field[*]                 -> kind "product", name = <product>, selector ".field[*]"

Produces entries: ``<name>``, ``{name, file, schema}`` or ``output.target[Name]``.
The human step ``<step>`` yields the product ``<step>_feedback`` on its ``hil_nok`` edge.
"""

import re

from ..state import ReadRef, WhenKind

_INPUT_RE = re.compile(r"^input\.(source|ruleset|output-template-ref)\[([^\]]+)\](.*)$")
_TARGET_RE = re.compile(r"^output\.target\[([^\]]+)\](.*)$")
_PRODUCT_RE = re.compile(r"^([A-Za-z0-9][A-Za-z0-9_-]*)(.*)$")
_SELECTOR_RE = re.compile(r"^(?:\.[A-Za-z0-9_-]+|\[[^\]]*\])*$")
_WHEN_OUTCOME_RE = re.compile(r"^outcome\s*==\s*(.+)$")
PLACEHOLDER_RE = re.compile(r"\{([A-Za-z0-9][A-Za-z0-9_.-]*)\}")


def feedback_name(step_name: str) -> str:
    """Name of the product a human step yields on HIL_NOK."""
    return f"{step_name}_feedback"


def parse_target_ref(s: str) -> "tuple[str, str] | None":
    """``output.target[Name]<selector>`` -> (Name, selector) or None."""
    m = _TARGET_RE.match(s.strip())
    if not m:
        return None
    name, sel = m.group(1).strip(), m.group(2)
    if not name or not _SELECTOR_RE.match(sel):
        return None
    return name, sel


def parse_read(raw: str) -> "ReadRef | None":
    """Parse one ``reads`` entry; None when it does not match the grammar."""
    if not isinstance(raw, str):
        return None
    s = raw.strip()
    optional = s.endswith("?")
    if optional:
        s = s[:-1].rstrip()
    if not s:
        return None
    if s.startswith("input."):
        m = _INPUT_RE.match(s)
        if not m or not m.group(2).strip() or not _SELECTOR_RE.match(m.group(3)):
            return None
        return ReadRef(raw=raw, kind=f"input.{m.group(1)}", name=m.group(2).strip(),
                       selector=m.group(3), optional=optional)
    if s.startswith("output."):
        t = parse_target_ref(s)
        if t is None:
            return None
        return ReadRef(raw=raw, kind="product", name=t[0], selector=t[1], optional=optional)
    m = _PRODUCT_RE.match(s)
    if not m or not _SELECTOR_RE.match(m.group(2)):
        return None
    if m.group(1) in ("input", "output"):
        return None
    return ReadRef(raw=raw, kind="product", name=m.group(1), selector=m.group(2), optional=optional)


def parse_when(when: "str | None") -> "tuple[WhenKind, str | None] | None":
    """Edge ``when``: absent -> done; done | hil_ok | hil_nok | ``outcome == <value>``."""
    if when is None:
        return "done", None
    w = when.strip()
    if w in ("", "done"):
        return "done", None
    if w in ("hil_ok", "hil_nok"):
        return w, None  # type: ignore[return-value]
    m = _WHEN_OUTCOME_RE.match(w)
    if m:
        val = m.group(1).strip()
        if len(val) >= 2 and val[0] == val[-1] and val[0] in "'\"":
            val = val[1:-1]
        if val:
            return "outcome", val
    return None


def placeholders(arg: str) -> list[str]:
    """Names of the ``{...}`` placeholders in one ``run`` argument."""
    return PLACEHOLDER_RE.findall(arg)
