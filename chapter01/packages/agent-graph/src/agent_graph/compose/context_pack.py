"""context.md: the context pack of a node (SFR-AGG-017, SFR-AGG-039, SFR-AGG-028).

Fixed section order (stable items before variable ones):
  1 goal and output schema   (mandatory)
  2 reads                    (items cited in ``reads`` in their listed order)
  3 handoff notes            (direct upstream nodes)
  4 run facts                (facts that apply to the node)
  Left out                   (items not included, listed by path: pointer over payload)

Budget (plan decision 8): context.md tokens, ``process.budget_of(step)``; a small reserve is kept
for the left-out list. Mandatory = goal, output schema and the non-optional reads. An item over
its share is replaced by a schema sketch plus sample (JSON/YAML) or a cached summary, never
truncated. When the mandatory part does not fit after that, ContextOverBudget is raised.
Share of a read item = remaining space / items still to place, at least SUMMARY_SHARE_MIN_TOKENS.
"""

import json
import os
import re
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Mapping, Sequence

import yaml

from .. import config
from ..errors import ContextOverBudget, InputReadDenied
from ..ports import SummarizerPort
from ..services.runfs import InputReadGuard, WorkspacePaths
from ..state import Binding, ProcessDef, ProductRef, RunFact, StepDef
from .normalize import join_blocks
from .snippets import FACTS_POINTER_PATH, fact_line
from .step_contract import product_file, product_path
from .tokens import estimate_tokens

STRUCTURED_SUFFIXES = {".json": "json", ".yml": "yaml", ".yaml": "yaml"}


@dataclass(frozen=True)
class ContextPack:
    text: str
    tokens: int
    items: tuple            # dicts: kind, ref, path, tokens, mode
    omitted: tuple          # dicts: ref/path, reason, listed
    products: tuple         # dicts: name, path, sha256 of the products consumed


def ctx_path(ws: WorkspacePaths, active_dir: Path, ws_rel: str) -> str:
    """Path of a workspace-relative file as seen from the node directory."""
    return Path(os.path.relpath(ws.root / ws_rel, active_dir)).as_posix()


def _fence(text: str, lang: str = "") -> str:
    longest = max((len(m) for m in re.findall(r"`+", text)), default=0)
    f = "`" * max(3, longest + 1)
    return f"{f}{lang}\n{text.rstrip()}\n{f}"


# ------------------------------------------------------------------ structured data

def _sketch(v: Any, depth: int = 0) -> Any:
    if depth > 6:
        return "..."
    if isinstance(v, dict):
        return {str(k): _sketch(x, depth + 1) for k, x in v.items()}
    if isinstance(v, list):
        return [f"<{len(v)} items>", _sketch(v[0], depth + 1)] if v else []
    if isinstance(v, bool):
        return "boolean"
    if isinstance(v, int):
        return "integer"
    if isinstance(v, float):
        return "number"
    if v is None:
        return "null"
    return "string"


def _sample(v: Any, n_items: int, max_str: int, depth: int = 0) -> Any:
    if depth > 4:
        return "..."
    if isinstance(v, dict):
        return {str(k): _sample(x, n_items, max_str, depth + 1) for k, x in list(v.items())[:20]}
    if isinstance(v, list):
        return [_sample(x, n_items, max_str, depth + 1) for x in v[:n_items]]
    if isinstance(v, str) and len(v) > max_str:
        return v[:max_str] + "..."
    return v


def _dump(v: Any) -> str:
    return json.dumps(v, indent=2, ensure_ascii=False, default=str)


def structured_text(raw: str, suffix: str, limit_tokens: int) -> "str | None":
    """Schema sketch plus sample of a JSON/YAML text that fits ``limit_tokens``; None when the
    text is not structured data or nothing fits."""
    kind = STRUCTURED_SUFFIXES.get(suffix.lower())
    if kind is None:
        return None
    try:
        data = json.loads(raw) if kind == "json" else yaml.safe_load(raw)
    except (ValueError, yaml.YAMLError):
        return None
    if not isinstance(data, (dict, list)):
        return None
    sketch = "Schema sketch:\n\n" + _fence(_dump(_sketch(data)), "json")
    for n_items, max_str in ((2, 80), (1, 40), (0, 0)):
        text = sketch
        if n_items:
            text += "\n\nSample:\n\n" + _fence(_dump(_sample(data, n_items, max_str)), "json")
        if estimate_tokens(text) <= limit_tokens:
            return text
    return None


# ------------------------------------------------------------------ reads

@dataclass
class _Read:
    label: str
    ws_rel: "str | None"            # workspace-relative file; None for a note
    sha256: "str | None"
    text: "str | None"
    mandatory: bool
    reason: "str | None" = None     # set when the item cannot be read
    product: "ProductRef | None" = None


def _load(reader: InputReadGuard, ws_rel: str) -> tuple:
    try:
        return reader.open_text(Path(ws_rel)), None
    except InputReadDenied:
        return None, "not_bound"
    except FileNotFoundError:
        return None, "missing"
    except UnicodeDecodeError:
        return None, "not_text"
    except OSError:
        return None, "unreadable"


def _resolve_reads(step: StepDef, binding: Binding, products: Mapping[str, ProductRef],
                   reader: InputReadGuard) -> list:
    out: list = []
    for ref in step.reads:
        label = ref.raw.rstrip("?")
        mandatory = not ref.optional
        files: list = []                       # (ws_rel, sha, product|None)
        if ref.kind == "product":
            pr = products.get(ref.name)
            if pr is None:
                out.append(_Read(label, None, None, None, mandatory,
                                 reason="absent" if ref.optional else "missing"))
                continue
            files.append((pr.path, pr.sha256, pr))
        elif ref.kind == "input.source":
            bs = binding.sources.get(ref.name)
            if bs is None:
                out.append(_Read(label, None, None, None, mandatory,
                                 reason="absent" if ref.optional else "missing"))
                continue
            if bs.note_text is not None:
                out.append(_Read(f"{label} (note)", None, None, bs.note_text, mandatory))
            files += [(f.path, f.sha256, None) for f in bs.files]
        else:
            table = binding.rulesets if ref.kind == "input.ruleset" else binding.templates
            bf = table.get(ref.name)
            if bf is None:
                out.append(_Read(label, None, None, None, mandatory,
                                 reason="absent" if ref.optional else "missing"))
                continue
            files.append((bf.path, bf.sha256, None))
        for ws_rel, sha, pr in files:
            text, reason = _load(reader, ws_rel)
            out.append(_Read(label, ws_rel, sha, text, mandatory, reason=reason, product=pr))
    return out


# ------------------------------------------------------------------ goal section

def goal_section(process: ProcessDef, step: StepDef, ws: WorkspacePaths, active_dir: Path,
                 reader: InputReadGuard) -> str:
    blocks = ["Step `" + step.name + "`.", step.instruction.rstrip()]
    for d in step.produces:
        if not d.schema:
            continue
        head = (f"Output schema of `{d.name}` (file `{product_path(step, product_file(process, d))}`, "
                f"schema file `{ctx_path(ws, active_dir, d.schema)}`):")
        text, reason = _load(reader, d.schema)
        blocks.append(head + "\n\n" + (_fence(text) if text is not None else f"(schema not readable: {reason})"))
    return join_blocks(blocks)


# ------------------------------------------------------------------ assembly

def _render(goal: str, reads: Sequence[str], handoffs: Sequence[str], facts: Sequence[str],
            left_out: Sequence[str]) -> str:
    parts = ["# Context pack", "## 1. Goal and output schema\n\n" + goal,
             "## 2. Reads\n\n" + ("\n\n".join(reads) if reads else "(none)"),
             "## 3. Handoff notes\n\n" + ("\n\n".join(handoffs) if handoffs else "(none)"),
             "## 4. Run facts\n\n" + ("\n".join(facts) if facts else "(none)")]
    if left_out:
        parts.append("## Left out (read on demand)\n\n" + "\n".join(left_out))
    return join_blocks(parts)


def _item_block(label: str, path: "str | None", note: "str | None", body: str) -> str:
    head = [f"### {label}"]
    if path:
        head.append(f"Path: `{path}`")
    if note:
        head.append(note)
    return "\n".join(head) + "\n\n" + body


def _handoff_block(node: str, data: dict) -> str:
    lines = [f"### {node}"]
    if data.get("done"):
        lines.append(f"Done: {str(data['done']).strip()}")
    for key, title in (("decisions", "Decisions"), ("open_points", "Open points")):
        vals = [str(x).strip() for x in (data.get(key) or [])]
        if vals:
            lines.append(f"{title}:")
            lines += [f"- {v}" for v in vals]
    return "\n".join(lines)


def build_context_pack(*, process: ProcessDef, step: StepDef, binding: Binding,
                       products: Mapping[str, ProductRef],
                       upstream_handoffs: Sequence[tuple], facts: Sequence[RunFact],
                       ws: WorkspacePaths, active_dir: Path, summarizer: SummarizerPort,
                       reader: InputReadGuard) -> ContextPack:
    budget = process.budget_of(step)
    usable = budget - max(30, budget // 20)
    goal = goal_section(process, step, ws, active_dir, reader)

    reads: list = []
    handoffs: list = []
    fact_lines: list = []
    items: list = []
    omitted: list = []
    consumed: list = []

    def used() -> int:
        return estimate_tokens(_render(goal, reads, handoffs, fact_lines, ()))

    if used() > usable:
        raise ContextOverBudget(step.name, budget, used())

    resolved = _resolve_reads(step, binding, products, reader)
    n_left = sum(1 for r in resolved if r.text is not None)
    for r in resolved:
        cpath = ctx_path(ws, active_dir, r.ws_rel) if r.ws_rel else None
        if r.text is None:
            omitted.append({"ref": r.label, "path": cpath, "reason": r.reason,
                            "listed": r.reason != "absent"})
            continue
        n_here = max(1, n_left)
        n_left -= 1
        now = used()
        remaining = usable - now
        share = max(config.SUMMARY_SHARE_MIN_TOKENS, remaining // n_here)
        limit = min(share, remaining)
        inline = _item_block(r.label, cpath, None, _fence(r.text))
        inline_cost = estimate_tokens(inline) + 2
        block, mode, cost = None, "", inline_cost
        if inline_cost <= limit:
            block, mode = inline, "inline"
        elif r.ws_rel is not None and limit > 0:
            sk = structured_text(r.text, Path(r.ws_rel).suffix, limit - 40)
            if sk is not None:
                block, mode = _item_block(r.label, cpath, f"Structure of the file ({estimate_tokens(r.text)} tokens); "
                                          "read it for the full content.", sk), "sketch"
            else:
                summary = summarizer.summary(ws.root / r.ws_rel, r.sha256 or "", share)
                sblock = _item_block(r.label, cpath, f"Summary of the file ({estimate_tokens(r.text)} tokens); "
                                     "read it for the full content.", summary.strip())
                cost = estimate_tokens(sblock) + 2
                if cost <= remaining:
                    block, mode = sblock, "summary"
        if block is None:
            if r.mandatory:
                raise ContextOverBudget(step.name, budget, now + cost)
            omitted.append({"ref": r.label, "path": cpath, "reason": "over_budget", "listed": True})
            continue
        reads.append(block)
        items.append({"kind": "read", "ref": r.label, "path": cpath, "mode": mode,
                      "tokens": estimate_tokens(block)})
        if r.product is not None:
            consumed.append({"name": r.product.name, "path": r.product.path, "sha256": r.product.sha256})

    for node, hp in upstream_handoffs:
        p = Path(hp) if Path(hp).is_absolute() else ws.root / hp
        cpath = Path(os.path.relpath(p, active_dir)).as_posix()
        try:
            data = json.loads(p.read_text(encoding="utf-8"))
        except (OSError, ValueError):
            omitted.append({"ref": f"handoff {node}", "path": cpath, "reason": "missing", "listed": False})
            continue
        if isinstance(data, dict) and isinstance(data.get("handoff"), dict):
            data = data["handoff"]
        if not isinstance(data, dict):
            data = {}
        block = _handoff_block(node, data)
        if estimate_tokens(block) + 2 > usable - used():
            omitted.append({"ref": f"handoff {node}", "path": cpath, "reason": "over_budget", "listed": True})
            continue
        handoffs.append(block)
        items.append({"kind": "handoff", "ref": node, "path": cpath, "mode": "inline",
                      "tokens": estimate_tokens(block)})

    applicable = [f for f in facts if f.node is None or f.node == step.name]
    cut = False
    cut_seqs: list = []
    for f in applicable:
        line = fact_line(f)
        if cut or estimate_tokens(line) + 1 > usable - used():
            cut = True
            cut_seqs.append(f.seq)
            continue
        fact_lines.append(line)
        items.append({"kind": "fact", "ref": f"#{f.seq}", "path": None, "mode": "inline",
                      "tokens": estimate_tokens(line)})
    if cut_seqs:
        omitted.append({"ref": "run facts " + ", ".join(f"#{s}" for s in cut_seqs),
                        "path": FACTS_POINTER_PATH, "reason": "over_budget", "listed": True})

    left_out = [f"- `{o['path']}`: {o['ref']} ({o['reason']})" if o["path"]
                else f"- {o['ref']} ({o['reason']})" for o in omitted if o["listed"]]
    text = _render(goal, reads, handoffs, fact_lines, left_out)
    return ContextPack(text=text, tokens=estimate_tokens(text), items=tuple(items),
                       omitted=tuple(omitted), products=tuple(consumed))
