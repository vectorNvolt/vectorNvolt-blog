"""Run report (SFR-AGG-032, SFR-AGG-021): ``.output/<runID>/report.md`` and the console summary.

Pure builders over the run folder; the caller (state S8) writes the file and prints the text
through the gateway.
"""

import json
from pathlib import Path

from ..errors import ExitState, RunError
from ..services.render import format_findings
from ..services.runfs import RunPaths, read_jsonl
from ..state import RunRecord

DRIFT_CODE = "CACHE_PREFIX_DRIFT"
MAX_TRANSCRIPT_LINES = 200


def _outputs(paths: RunPaths) -> list[str]:
    if not paths.output_dir.is_dir():
        return []
    return sorted(p.relative_to(paths.output_dir).as_posix()
                  for p in paths.output_dir.rglob("*") if p.is_file() and p.name != "report.md")


def _drift(paths: RunPaths, record: RunRecord) -> list[str]:
    seen = list(record.counters.drift)
    for rec in read_jsonl(paths.history_jsonl):
        if rec.get("dir") == "notice" and rec.get("name") == DRIFT_CODE:
            text = (rec.get("payload") or {}).get("text", "")
            if text and text not in seen:
                seen.append(text)
    return seen


def _resolutions(paths: RunPaths) -> list[tuple[str, list[dict]]]:
    base = paths.root / "resolution"
    out = []
    if base.is_dir():
        for d in sorted((p for p in base.iterdir() if p.is_dir()),
                        key=lambda p: (not p.name.isdigit(), int(p.name) if p.name.isdigit() else 0)):
            out.append((d.name, read_jsonl(d / "changes.jsonl")))
    return out


def _s7_transcript(paths: RunPaths) -> list[str]:
    lines = []
    if paths.gateway_jsonl.is_file():
        for line in paths.gateway_jsonl.read_text(encoding="utf-8").splitlines():
            try:
                rec = json.loads(line)
            except ValueError:
                continue
            if str(rec.get("source", "")).startswith("S7"):
                lines.append(f"[{rec.get('kind', '')}] {rec.get('text', '')}".replace("\n", " "))
    return lines[-MAX_TRANSCRIPT_LINES:]


def build_report(paths: RunPaths, record: RunRecord, exit_state: ExitState,
                 error: "RunError | None") -> str:
    c = record.counters
    out = [f"# Run report: {record.run_id}", "",
           f"- Process: `{record.process_id}` (`{record.process_file}`)",
           f"- Exit state: **{exit_state.value}**, exit code {exit_state.exit_code}",
           f"- Created: {record.created}", f"- Model: {record.model}, effort {record.effort}",
           f"- Steps run: {c.steps}, LLM calls: {c.llm_calls}, elapsed: {c.elapsed_s:.1f}s",
           f"- Resolutions: {record.resolution_count}"]
    if c.p_run is not None:
        out.append(f"- P_run (first-turn cache read of the second cognitive node): {c.p_run} tokens")
    out.append("")
    if c.failures:
        out += ["## Failed attempts", ""] + [f"- {n}: {k}" for n, k in sorted(c.failures.items())] + [""]
    outs = _outputs(paths)
    out += ["## Outputs", ""] + ([f"- `{o}`" for o in outs] or ["- none"]) + [""]
    if error is not None:
        out += ["## Open error", "", f"- Exit state: {error.exit_state.value} (state {error.state}"
                + (f", node {error.node}" if error.node else "") + ")", f"- {error.message}"]
        if error.findings:
            out += ["", "```", format_findings(error.findings), "```"]
        if error.detail:
            out += ["", "```json", json.dumps(error.detail, indent=2, sort_keys=True, default=str), "```"]
        out += [""]
    res = _resolutions(paths)
    if res:
        out += ["## Resolution", ""]
        for name, changes in res:
            out.append(f"### Resolution {name}")
            out += [f"- {ch.get('kind')} {ch.get('path') or ch.get('key') or ch.get('process_id') or ''}: "
                    f"{ch.get('reason', '')}" for ch in changes] or ["- no changes"]
            out.append("")
        transcript = _s7_transcript(paths)
        if transcript:
            out += ["### Dialog", "", "```"] + transcript + ["```", ""]
    drift = _drift(paths, record)
    if drift:
        out += ["## Notices", ""] + [f"- {DRIFT_CODE}: {d}" for d in drift] + [""]
    return "\n".join(out).rstrip("\n") + "\n"


def console_summary(record: "RunRecord | None", exit_state: ExitState, error: "RunError | None",
                    report_path: "Path | None") -> str:
    lines = [f"exit_state: {exit_state.value}", f"exit_code: {exit_state.exit_code}"]
    if record is not None:
        lines.insert(0, f"run: {record.run_id} (process {record.process_id})")
        lines.append(f"steps: {record.counters.steps}, llm_calls: {record.counters.llm_calls}")
    if error is not None:
        lines.append(f"open error: {error.message}")
        if record is not None:
            lines.append(f"resume with: agent_graph run --resume {record.run_id}")
    if report_path is not None:
        lines.append(f"report: {report_path}")
    return "\n".join(lines)
