"""Fixed text of the generated step contract (L2 of CLAUDE.md, SFR-AGG-017/039, WP3).

Everything here is constant text: the contract of a step depends only on the step definition,
so it is byte-stable (no run ids, no timestamps, no absolute paths; SSR-AGG-011).
Field order of the contract is fixed by compose/step_contract.py.
"""

HEADING = "# Step contract"

RESULT_PROTOCOL = """\
## Result

When you finish, write `result.json` in your working directory (the folder that holds this
file), exactly in this shape:

```json
{
  "status": "done",
  "outcome": null,
  "question": null,
  "handoff": {"done": "", "decisions": [], "open_points": []},
  "warnings": []
}
```

- `status`: `"done"` when the step is complete, `"question"` when you need a decision from the
  developer. For a question set `question` to the question text and stop; the answer arrives
  as your next turn.
- `handoff`: a short note for later steps: what you did, decisions taken, open points.
- `warnings`: problems that did not stop the step."""

OUTCOME_RULE = ("- `outcome`: set it to exactly one of the allowed outcomes above when `status` "
                "is `\"done\"`.")
NO_OUTCOME_RULE = "- `outcome`: leave it `null`."

RULES = """\
## Rules

- Read only the files named in `context.md`. Write only the products listed above and `result.json`.
- Never write outside `../work/<step>/` (products) and your working directory (`result.json`).
- Do not address the developer directly; ask through `status: "question"`."""
