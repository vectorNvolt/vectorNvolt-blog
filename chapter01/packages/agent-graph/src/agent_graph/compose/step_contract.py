"""Generated step contract (last part of L2 of CLAUDE.md; SFR-AGG-017/039).

Fixed template, fixed field order: goal, reads, products, outcomes, limits, result protocol,
rules. Depends only on the step and process definition. No run id, no timestamps, no absolute
paths: paths are relative to the node's working directory (``active/``).
"""

from typing import Callable

from ..prompts import step_contract as P
from ..state import ProcessDef, ProductDecl, StepDef
from .normalize import join_blocks


def product_file(process: ProcessDef, decl: ProductDecl) -> str:
    """File name of a product inside ``work/<node>/``. Output-target products use the target
    path (plan decision 14)."""
    if decl.output_target:
        for t in process.targets:
            if t.name == decl.output_target:
                return t.path
    return decl.file


def product_path(step: StepDef, file: str) -> str:
    return f"../work/{step.name}/{file}"


def effective_tools(process: ProcessDef, step: StepDef) -> tuple:
    return tuple(step.tools) if step.tools is not None else tuple(process.runtime.tools)


def build_step_contract(process: ProcessDef, step: StepDef, ctx_path: Callable[[str], str]) -> str:
    """``ctx_path`` maps a workspace-relative path to a path relative to the node directory."""
    blocks = [P.HEADING, f"Step: `{step.name}` (id {step.id}, type {step.type})",
              "## Goal\n\n" + step.instruction.rstrip()]
    reads = "\n".join(f"- `{r.raw}`" for r in step.reads) or "(none)"
    blocks.append("## Reads\n\nThe items are listed in `context.md`; read left-out items on demand.\n\n"
                  + reads)
    prod = []
    for d in step.produces:
        line = f"- `{d.name}`: write `{product_path(step, product_file(process, d))}`"
        if d.output_target:
            tgt = next((t for t in process.targets if t.name == d.output_target), None)
            line += f" (output target `{d.output_target}`"
            if tgt is not None and tgt.template:
                line += f", template `{tgt.template}`"
            line += ")"
        if d.schema:
            line += f"; it must match the schema `{ctx_path(d.schema)}`"
        prod.append(line)
    blocks.append("## Products\n\n" + ("\n".join(prod) if prod else "(none: hand on only the handoff note)"))
    if step.outcomes:
        blocks.append("## Allowed outcomes\n\n" + "\n".join(f"- `{o}`" for o in step.outcomes))
    blocks.append("## Limits\n\n"
                  f"- Context budget: {process.budget_of(step)} tokens\n"
                  f"- Tools: {', '.join(effective_tools(process, step)) or '(none)'}")
    blocks.append(P.RESULT_PROTOCOL + "\n" + (P.OUTCOME_RULE if step.outcomes else P.NO_OUTCOME_RULE))
    blocks.append(P.RULES)
    return join_blocks(blocks)

