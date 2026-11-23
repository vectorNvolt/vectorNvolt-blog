"""Builds a temporary /workspace for tests: process, input, pools, template, helper script.

``build_workspace(root)`` creates (all valid, see PROCESS_YML)::

    .process/example.yml            process "example": extract (procedure) -> write-summary (cognitive)
                                    -> review (human) -> END; review hil_nok loops to write-summary
    .process/scripts/extract.py     the script of the extract step (no inline code, SSR-AGG-013)
    .input/example/source.md        the bound source ("Source": glob example/*.md)
    templates/report.md             output template ("Template")
    identity/.snippet/agent-identity.md   layer agent  (L0)
    identity/.snippet/process-rules.md    layer process (L1), declared in runtime.snippets
    identity/.snippet/step-notes.md       layer step, declared by write-summary
    identity/.snippet/step-auto.md        layer step, auto: true, tags [summary]
    identity/.skills/summarize/SKILL.md   declared in runtime.skills (+ identity/.skills/summarize/notes.md)
    identity/.skills/translate/SKILL.md   not declared anywhere (candidate for ranking/exclusion)
    .agent/  .output/               empty

Helpers: ``add_process`` (also for scripts: ``add_process(ws, "scripts/x.py", text)``),
``add_snippet``, ``add_skill``, ``write_file``, ``process_with_id``.
Returns ``WorkspacePaths``. Everything lives under ``root``; nothing outside is touched.
"""

from pathlib import Path
from typing import Sequence

from agent_graph.services.runfs import WorkspacePaths

SOURCE_MD = "# Source\n\nAlpha line.\nBeta line.\nGamma line.\n"
TEMPLATE_MD = "# Summary\n\n## Findings\n"
EXTRACT_SCRIPT = "scripts/extract.py"          # under .process/ (SCRIPTS_DIR, SSR-AGG-013)
EXTRACT_PY = """\
import json, pathlib, sys

text = pathlib.Path(sys.argv[1]).read_text()
pathlib.Path(sys.argv[2], "facts.json").write_text(json.dumps({"lines": len(text.splitlines())}))
"""

PROCESS_YML = """\
apiVersion: agent-graph/v1
id: example
title: Example summary process
description: >
  Summarizes the example source into a short report.

input:
  source:
    - {name: Source, path: "example/*.md", required: true}
  output-template-ref:
    - {name: Template, path: "templates/report.md", required: true}

output:
  target:
    - {name: Report, path: "summary.md", template: Template, checks: [non_empty], required: true}

runtime:
  snippets: [process-rules]
  skills: [summarize]
  max_skills: 3
  tools: [Read, Write]
  retry: 2
  timeout: 10m
  budgets: {max_steps: 50, max_llm_calls: 20, timeout: 60m}
  work_products: archive

steps:
  - id: 1
    name: extract
    type: procedure
    reads: ["input.source[Source]"]
    produces: [{name: facts, file: facts.json}]
    instruction: |
      COLLECT("lines", "all") THEN STORE(facts)
    run: "python .process/scripts/extract.py {input.Source} {work}"
    success: |
      PRINT('STEP 1: DONE')

  - id: 2
    name: write-summary
    type: cognitive
    reads: ["facts", "input.output-template-ref[Template]", "review_feedback?"]
    produces: ["output.target[Report]"]
    instruction: |
      INSTANTIATE("Template", "with facts")
      WRITE(output.target[Report], "summary per Template")
    snippets: [step-notes]
    budget: 40k
    success: |
      PRINT('STEP 2: DONE')
    failure: |
      STOP
      PRINT('<ERROR_DETAILS>')

  - id: 3
    name: review
    type: human
    reads: ["output.target[Report]"]
    instruction: |
      REQUEST_REVIEW("summary", "against Template")
    success: |
      PRINT('STEP 3: DONE')

transitions:
  entry: extract
  edges:
    - {from: extract, to: write-summary, when: done}
    - {from: write-summary, to: review, when: done}
    - {from: review, to: END, when: hil_ok}
    - {from: review, to: write-summary, when: hil_nok}
"""


def process_with_id(process_id: str, text: str = PROCESS_YML) -> str:
    """The example process under another id (for duplicate-id and multi-process tests)."""
    return text.replace("id: example\n", f"id: {process_id}\n", 1)


def write_file(root: Path, rel: str, text: str) -> Path:
    p = Path(root) / rel
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(text, encoding="utf-8")
    return p


def add_process(ws: WorkspacePaths, filename: str, text: str) -> Path:
    return write_file(ws.root, f".process/{filename}", text)


def add_snippet(ws: WorkspacePaths, snippet_id: str, layer: str, body: str, *,
                tags: Sequence[str] = (), auto: bool = False, filename: str | None = None) -> Path:
    fm = f"---\nid: {snippet_id}\nlayer: {layer}\ntags: [{', '.join(tags)}]\nauto: {'true' if auto else 'false'}\n---\n"
    return write_file(ws.root, f"identity/.snippet/{filename or snippet_id + '.md'}", fm + body)


def add_skill(ws: WorkspacePaths, name: str, description: str, *, tags: Sequence[str] = (),
              body: str = "Follow these steps.\n", extra_files: "dict[str, str] | None" = None) -> Path:
    fm = f"---\nname: {name}\ndescription: {description}\ntags: [{', '.join(tags)}]\n---\n"
    d = ws.root / "identity" / ".skills" / name
    write_file(ws.root, f"identity/.skills/{name}/SKILL.md", fm + body)
    for rel, text in (extra_files or {}).items():
        write_file(d, rel, text)
    return d


def build_workspace(root: Path, *, process_text: str = PROCESS_YML, process_file: str = "example.yml",
                    with_input: bool = True, with_pools: bool = True) -> WorkspacePaths:
    root = Path(root)
    ws = WorkspacePaths(root)
    for d in (ws.agent_dir, ws.output_root, ws.process_dir, ws.input_dir):
        d.mkdir(parents=True, exist_ok=True)
    add_process(ws, process_file, process_text)
    add_process(ws, EXTRACT_SCRIPT, EXTRACT_PY)
    if with_input:
        write_file(root, ".input/example/source.md", SOURCE_MD)
    write_file(root, "templates/report.md", TEMPLATE_MD)
    if with_pools:
        add_snippet(ws, "agent-identity", "agent", "You are a careful assistant.\nStay inside the workspace.\n",
                    tags=["identity"])
        add_snippet(ws, "process-rules", "process", "Cite the source for every claim.\n", tags=["rules"])
        add_snippet(ws, "step-notes", "step", "Keep each section under 150 words.\n", tags=["writing"])
        add_snippet(ws, "step-auto", "step", "Prefer short sentences.\n", tags=["summary"], auto=True)
        add_skill(ws, "summarize", "Summarize a text into key points", tags=["summary", "writing"],
                  extra_files={"notes.md": "Extra notes for the summarize skill.\n"})
        add_skill(ws, "translate", "Translate text between languages", tags=["translation"])
    return ws
