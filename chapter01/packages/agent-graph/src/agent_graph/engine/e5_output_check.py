"""E5 output check: accepts the run only when every required target passes its checks and its template.

Covers: SFR-AGG-030 (checks such as non_empty, max_words:N and template conformance against the
targets in .output/<runID>/; a failed check keeps the output, marks it invalid with
OUTPUT_INVALID.json and escalates OUTPUT_INVALID through E7), decision 14.
"""

from ..errors import ExitState, Finding, RunError
from ..dynamic.products import check_output_text, check_template, resolve_files
from ..services.runfs import canonical_json, write_atomic
from .budgets import EngineRuntime


def e5_output_check(rt: EngineRuntime, state: dict) -> dict:
    paths, proc, binding = rt.ctx.paths, rt.proc, rt.ctx.binding
    out_dir = paths.output_dir
    findings: list[Finding] = []
    for t in proc.targets:
        files = resolve_files(out_dir, t.path)
        if not files:
            if t.required:
                findings.append(Finding("output_missing",
                                        f"required output target {t.name} ({t.path}) is missing", item=t.name))
            continue
        tpl_path = tpl_text = None
        if t.template and t.template in binding.templates:
            tpl_path = binding.templates[t.template].path
            try:
                tpl_text = (paths.ws.root / tpl_path).read_text(encoding="utf-8")
            except OSError as exc:
                findings.append(Finding("template_unreadable", f"template {t.template}: {exc}",
                                        file=tpl_path, item=t.name))
        for f in files:
            rel = paths.ws.rel(f)
            text = f.read_text(encoding="utf-8", errors="replace")
            for chk in t.checks:
                msg = check_output_text(chk, text)
                if msg:
                    findings.append(Finding("output_check", f"{t.name}: {chk}: {msg}", file=rel, item=t.name))
            if tpl_text is not None:
                msg = check_template(tpl_path, tpl_text, text)
                if msg:
                    findings.append(Finding("template_conformance", f"{t.name}: {msg}", file=rel, item=t.name))
    invalid = rt.guard.check(paths.output_invalid_json)
    if not findings:
        invalid.unlink(missing_ok=True)
        return {"output_ref": state.get("output_ref") or paths.ws.rel(out_dir), "pending": None,
                "escalation": None}
    out_dir.mkdir(parents=True, exist_ok=True)
    write_atomic(invalid, canonical_json({"run_id": paths.run_id,
                                          "findings": [f.to_json() for f in findings]}), guard=rt.guard)
    err = RunError(ExitState.OUTPUT_INVALID, "S6",
                   f"the output of the run failed {len(findings)} check(s); kept in {paths.ws.rel(out_dir)}",
                   findings=tuple(findings), detail={"output": paths.ws.rel(out_dir)})
    return {"escalation": err.to_json(), "pending": None, "next_cmd": None}
