#!/usr/bin/env python3
"""Probe Claude Code prompt-cache behaviour for headless `claude -p` node sessions.

Purpose
    Measure how much of a request prefix is served from the prompt cache when
    two headless node sessions differ in a controlled way.

Design verified
    uss-008 "Byte-stable composition for prompt caching": all nodes of a run
    share a cacheable prefix (tool definitions + default system prompt + an
    appended system prompt file `system.md`) and differ only after it
    (CLAUDE.md, skills, turn prompt).

How to run
    python3 design/tools/cache_probe.py --dry-run          # print commands only
    python3 design/tools/cache_probe.py matrix             # all scenarios
    python3 design/tools/cache_probe.py matrix --scenarios baseline,same_node
    python3 design/tools/cache_probe.py ttl --wait 3660    # 1h TTL expiry check

    `matrix` is the default subcommand, so `--dry-run` alone works too.

Where to run
    Where `claude` is installed AND logged in (inside the agent container).

WARNING
    This script calls Claude for real. Every invocation counts against the
    subscription usage. Use --dry-run first to see what would be executed.
"""

from __future__ import annotations

import argparse
import json
import secrets
import shutil
import subprocess
import sys
import tempfile
import time
from dataclasses import asdict, dataclass, field
from pathlib import Path

PROMPT = "Reply with the single word OK. Do not use any tool."
PROMPT_TURN2 = "Reply with the single word OK again."


# --------------------------------------------------------------------------
# Single claude run
# --------------------------------------------------------------------------


@dataclass
class RunResult:
    input_tokens: int = 0
    cache_creation_input_tokens: int = 0
    cache_read_input_tokens: int = 0
    ephemeral_1h_input_tokens: int = 0
    ephemeral_5m_input_tokens: int = 0
    session_id: str = ""
    total_cost_usd: float | None = None

    @property
    def prompt_total(self) -> int:
        return (
            self.input_tokens
            + self.cache_creation_input_tokens
            + self.cache_read_input_tokens
        )


def _int(value: object) -> int:
    return value if isinstance(value, int) else 0


def _tail(text: str, n: int = 20) -> str:
    return "\n".join(text.strip().splitlines()[-n:])


def build_command(flags: list[str], prompt: str, resume: str | None) -> list[str]:
    cmd = [
        "claude", "-p", prompt,
        "--output-format", "stream-json", "--verbose", "--max-turns", "1",
        *flags,
    ]
    if resume:
        cmd += ["--resume", resume]
    return cmd


def run_claude(
    cwd: Path,
    flags: list[str],
    prompt: str,
    resume: str | None = None,
    *,
    timeout: int = 300,
    dry_run: bool = False,
) -> RunResult:
    """Run one headless turn and return the first request's usage."""
    cmd = build_command(flags, prompt, resume)
    if dry_run:
        print(f"[dry-run] cwd={cwd}\n          {' '.join(map(_quote, cmd))}")
        return RunResult(session_id="<session_id>")

    try:
        proc = subprocess.run(
            cmd, cwd=cwd, capture_output=True, text=True, timeout=timeout
        )
    except subprocess.TimeoutExpired as exc:
        raise RuntimeError(f"claude timed out after {timeout}s in {cwd}") from exc

    first_assistant: dict | None = None
    result_event: dict | None = None
    for line in proc.stdout.splitlines():
        line = line.strip()
        if not line.startswith("{"):
            continue
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        if not isinstance(event, dict):
            continue
        if event.get("type") == "assistant" and first_assistant is None:
            first_assistant = event
        elif event.get("type") == "result":
            result_event = event

    if proc.returncode != 0 or first_assistant is None:
        raise RuntimeError(
            f"claude failed (exit {proc.returncode}, assistant event "
            f"{'found' if first_assistant else 'missing'}) in {cwd}\n"
            f"stderr tail:\n{_tail(proc.stderr)}"
        )

    usage = (first_assistant.get("message") or {}).get("usage") or {}
    creation = usage.get("cache_creation") or {}
    result_event = result_event or {}
    cost = result_event.get("total_cost_usd")
    return RunResult(
        input_tokens=_int(usage.get("input_tokens")),
        cache_creation_input_tokens=_int(usage.get("cache_creation_input_tokens")),
        cache_read_input_tokens=_int(usage.get("cache_read_input_tokens")),
        ephemeral_1h_input_tokens=_int(creation.get("ephemeral_1h_input_tokens")),
        ephemeral_5m_input_tokens=_int(creation.get("ephemeral_5m_input_tokens")),
        session_id=str(result_event.get("session_id") or ""),
        total_cost_usd=cost if isinstance(cost, (int, float)) else None,
    )


def _quote(s: str) -> str:
    return json.dumps(s) if (not s or any(c in s for c in " \"'{}")) else s


# --------------------------------------------------------------------------
# Workspace + flags
# --------------------------------------------------------------------------


def make_node_dir(
    base: Path,
    name: str,
    claude_md: str,
    skills: list[str],
    git: bool,
    *,
    dry_run: bool = False,
) -> Path:
    """Create a fresh node workspace under `base` and return its path."""
    if dry_run:
        return base / f"{name}-XXXX"
    base.mkdir(parents=True, exist_ok=True)
    node = Path(tempfile.mkdtemp(prefix=f"{name}-", dir=base))
    (node / "CLAUDE.md").write_text(claude_md, encoding="utf-8")
    for skill in skills:
        skill_dir = node / ".claude" / "skills" / skill
        skill_dir.mkdir(parents=True)
        (skill_dir / "SKILL.md").write_text(
            f"---\nname: {skill}\n"
            f"description: Probe skill {skill}, never use it.\n---\n"
            "Do nothing.\n",
            encoding="utf-8",
        )
    if git:
        subprocess.run(["git", "init", "-q"], cwd=node, check=True)
    return node


def system_text(nonce: str) -> str:
    rules = [f"- Rule {i:02d}: keep answers short." for i in range(1, 61)]
    return "Probe rules:\n" + "\n".join(rules) + f"\nprobe-nonce: {nonce}\n"


def write_system_file(base: Path, name: str, nonce: str, dry_run: bool) -> Path:
    path = base / f"system-{name}-{nonce}.md"
    if not dry_run:
        base.mkdir(parents=True, exist_ok=True)
        path.write_text(system_text(nonce), encoding="utf-8")
    return path


def write_mcp_file(base: Path, dry_run: bool) -> Path:
    path = base / "empty-mcp.json"
    if not dry_run:
        base.mkdir(parents=True, exist_ok=True)
        path.write_text(json.dumps({"mcpServers": {}}), encoding="utf-8")
    return path


def base_flags(
    args: argparse.Namespace,
    sysfile: Path,
    *,
    effort: str | None = None,
    tools: str = "Read,Write",
    allowed: str = "Read",
    exclude: bool = True,
) -> list[str]:
    flags = [
        "--model", args.model,
        "--effort", effort or args.effort,
        "--tools", tools,
        "--allowedTools", allowed,
        "--setting-sources", "project",
        "--strict-mcp-config",
        "--mcp-config", str(args.mcp_file),
    ]
    if exclude:
        flags.append("--exclude-dynamic-system-prompt-sections")
    flags += ["--append-system-prompt-file", str(sysfile)]
    return flags


# --------------------------------------------------------------------------
# Scenarios
# --------------------------------------------------------------------------


@dataclass
class NodeSpec:
    claude_md: str = "# Node A\nYou are node A.\n"
    skills: list[str] = field(default_factory=list)
    git: bool = False
    tools: str = "Read,Write"
    allowed: str = "Read"
    exclude: bool = True
    effort: str | None = None
    other_nonce: bool = False  # use a different system.md nonce


MD_A = "# Node A\nYou are node A. Work on the alpha task.\n"
MD_B = "# Node B\nYou are node B. Work on the beta task, which differs.\n"


@dataclass
class Scenario:
    name: str
    description: str
    b_differs: str
    expectation: str
    a: NodeSpec
    b: NodeSpec
    resume: bool = False  # B = second turn resumed in A's dir
    same_dir: bool = False  # B = fresh session in A's dir (node restart in active/)
    verdict: str = "info"  # "ge_p" | "gt_p" | "info"


def make_scenarios(effort_alt: str) -> list[Scenario]:
    a = NodeSpec(claude_md=MD_A)

    def b(**kw: object) -> NodeSpec:
        return NodeSpec(**{"claude_md": MD_B, **kw})  # type: ignore[arg-type]

    return [
        Scenario("baseline", "two nodes, identical flags and system.md",
                 "CLAUDE.md text", "read ~ P (shared prefix)", a, b(), verdict="ge_p"),
        Scenario("same_node", "same node restarted: fresh session in the same dir",
                 "nothing (new session, same dir)", "read > P (almost whole prompt)",
                 a, NodeSpec(claude_md=MD_A), same_dir=True, verdict="gt_p"),
        Scenario("resume_turn2", "second turn resumed in same session",
                 "turn 2 via --resume", "read ~ A.prompt_total (> P)",
                 a, NodeSpec(claude_md=MD_A), resume=True, verdict="gt_p"),
        Scenario("cwd_no_exclude", "baseline without --exclude-dynamic-system-prompt-sections",
                 "CLAUDE.md, no exclude flag on either run",
                 "read small if cwd is in system prompt",
                 NodeSpec(claude_md=MD_A, exclude=False), b(exclude=False)),
        Scenario("cwd_with_exclude", "control for cwd_no_exclude (exclude flag on)",
                 "CLAUDE.md, different dirs", "read ~ P", a, b(), verdict="ge_p"),
        Scenario("tools_vs_allowed", "B restricts --tools to Read",
                 "--tools Read", "read ~ 0 (tool definitions differ)",
                 a, b(tools="Read")),
        Scenario("allowed_only", "B uses --allowedTools Read,Write",
                 "--allowedTools Read,Write", "read ~ P", a,
                 b(allowed="Read,Write"), verdict="ge_p"),
        Scenario("skills_differ", "different skill sets",
                 "skills [beta,gamma] vs [alpha]", "read ~ P if skills follow the prefix",
                 NodeSpec(claude_md=MD_A, skills=["alpha"]),
                 b(skills=["beta", "gamma"]), verdict="ge_p"),
        Scenario("git_repo", "B dir is a git repo, A is not",
                 "git init in B", "read ~ P", a, b(git=True), verdict="ge_p"),
        Scenario("system_differs", "B has a different system.md nonce",
                 "system.md nonce", "read < P (tools + default system prompt only)",
                 a, b(other_nonce=True)),
        Scenario("effort_differs", "B uses a different --effort",
                 f"--effort {effort_alt}", "lower read",
                 a, b(effort=effort_alt)),
    ]


@dataclass
class Outcome:
    scenario: Scenario
    a: RunResult | None = None
    b: RunResult | None = None
    error: str = ""


def run_scenario(sc: Scenario, args: argparse.Namespace, base: Path) -> Outcome:
    out = Outcome(sc)
    nonce = secrets.token_hex(4)
    sys_a = write_system_file(base, sc.name, nonce, args.dry_run)
    sys_b = sys_a
    if sc.b.other_nonce:
        sys_b = write_system_file(base, sc.name + "-b", secrets.token_hex(4), args.dry_run)

    def node(label: str, spec: NodeSpec) -> Path:
        return make_node_dir(base, f"{sc.name}-{label}", spec.claude_md,
                             spec.skills, spec.git, dry_run=args.dry_run)

    def flags(spec: NodeSpec, sysfile: Path) -> list[str]:
        return base_flags(args, sysfile, effort=spec.effort, tools=spec.tools,
                          allowed=spec.allowed, exclude=spec.exclude)

    kw = {"timeout": args.timeout, "dry_run": args.dry_run}
    try:
        dir_a = node("A", sc.a)
        out.a = run_claude(dir_a, flags(sc.a, sys_a), PROMPT, **kw)
        if sc.resume:
            out.b = run_claude(dir_a, flags(sc.a, sys_a), PROMPT_TURN2,
                               resume=out.a.session_id, **kw)
        elif sc.same_dir:
            out.b = run_claude(dir_a, flags(sc.b, sys_b), PROMPT, **kw)
        else:
            out.b = run_claude(node("B", sc.b), flags(sc.b, sys_b), PROMPT, **kw)
    except (RuntimeError, subprocess.CalledProcessError) as exc:
        out.error = str(exc)
    return out


# --------------------------------------------------------------------------
# Reporting
# --------------------------------------------------------------------------


def verdict_for(o: Outcome, p: int | None) -> str:
    if o.error or o.b is None:
        return "ERROR"
    read = o.b.cache_read_input_tokens
    mode = o.scenario.verdict
    if mode == "info":
        return "INFO"
    if mode == "gt_p":
        return "CHECK" if p is None else ("PASS" if read > p else "CHECK")
    return "CHECK" if p is None else ("PASS" if read >= 0.9 * p else "CHECK")


def print_report(outcomes: list[Outcome]) -> None:
    p: int | None = None
    for o in outcomes:
        if o.scenario.name == "baseline" and o.b:
            p = o.b.cache_read_input_tokens
    head = (f"{'scenario':<18} {'B.input':>8} {'B.creation':>11} {'B.read':>8} "
            f"{'read/P':>7} {'1h':>7} {'5m':>7}  expectation")
    print(head)
    print("-" * len(head))
    for o in outcomes:
        if o.b is None:
            print(f"{o.scenario.name:<18} ERROR: {o.error.splitlines()[0] if o.error else '?'}")
            continue
        r = o.b
        ratio = f"{r.cache_read_input_tokens / p:.2f}" if p else "-"
        print(f"{o.scenario.name:<18} {r.input_tokens:>8} "
              f"{r.cache_creation_input_tokens:>11} {r.cache_read_input_tokens:>8} "
              f"{ratio:>7} {r.ephemeral_1h_input_tokens:>7} "
              f"{r.ephemeral_5m_input_tokens:>7}  {o.scenario.expectation}")
    print()
    print(f"P (shared prefix, baseline B.cache_read) = {p if p is not None else 'unknown'} tokens")
    for o in outcomes:
        extra = ""
        if o.b and p:
            extra = f" (read/P = {o.b.cache_read_input_tokens / p:.2f})"
        print(f"  {o.scenario.name:<18} {verdict_for(o, p)}{extra}")
    created = [r for o in outcomes for r in (o.a, o.b) if r and r.cache_creation_input_tokens]
    if created:
        ok = all(r.ephemeral_5m_input_tokens == 0 and r.ephemeral_1h_input_tokens > 0
                 for r in created)
        print(f"  {'ttl_check':<18} {'PASS' if ok else 'CHECK'} "
              "(all creation tokens in ephemeral_1h)")
    for o in outcomes:
        if o.error:
            print(f"\n[{o.scenario.name}] error:\n{o.error}")


# --------------------------------------------------------------------------
# Subcommands
# --------------------------------------------------------------------------


def prepare(args: argparse.Namespace) -> tuple[Path, bool]:
    """Resolve the work base dir; return (base, created_by_us)."""
    if args.workdir_base:
        base, created = Path(args.workdir_base), False
    elif args.dry_run:
        base, created = Path(tempfile.gettempdir()) / "cache_probe_DRYRUN", False
    else:
        base, created = Path(tempfile.mkdtemp(prefix="cache_probe_")), True
    args.mcp_file = write_mcp_file(base, args.dry_run)
    return base, created


def preflight(args: argparse.Namespace, planned: int) -> None:
    if shutil.which("claude") is None:
        print("error: `claude` not found on PATH", file=sys.stderr)
        sys.exit(2)
    if not args.dry_run:
        ver = subprocess.run(["claude", "--version"], capture_output=True, text=True)
        print(f"claude --version: {(ver.stdout or ver.stderr).strip()}")
        print(f"WARNING: real Claude calls count against subscription usage; "
              f"{planned} claude invocations planned.")


def cmd_matrix(args: argparse.Namespace) -> int:
    effort_alt = args.effort_alt or ("medium" if args.effort == "low" else "low")
    scenarios = make_scenarios(effort_alt)
    if args.scenarios:
        wanted = [s.strip() for s in args.scenarios.split(",") if s.strip()]
        known = {s.name for s in scenarios}
        unknown = [w for w in wanted if w not in known]
        if unknown:
            print(f"error: unknown scenarios {unknown}; known: {sorted(known)}",
                  file=sys.stderr)
            return 2
        scenarios = [s for s in scenarios if s.name in wanted]
    preflight(args, 2 * len(scenarios))

    base, created = prepare(args)
    outcomes: list[Outcome] = []
    try:
        for sc in scenarios:
            print(f"== {sc.name}: {sc.description} (B differs: {sc.b_differs})", flush=True)
            outcomes.append(run_scenario(sc, args, base))
    finally:
        if not args.dry_run and not args.keep and created:
            shutil.rmtree(base, ignore_errors=True)
        elif not args.dry_run and args.keep:
            print(f"kept work dirs under {base}")
    if args.dry_run:
        return 0
    print()
    print_report(outcomes)
    if args.json_out:
        data = [
            {"scenario": o.scenario.name, "a": asdict(o.a) if o.a else None,
             "b": asdict(o.b) if o.b else None, "error": o.error}
            for o in outcomes
        ]
        Path(args.json_out).write_text(json.dumps(data, indent=2), encoding="utf-8")
    return 1 if any(o.error for o in outcomes) else 0


def cmd_ttl(args: argparse.Namespace) -> int:
    preflight(args, 2)
    base, created = prepare(args)
    sysfile = write_system_file(base, "ttl", secrets.token_hex(4), args.dry_run)
    node = make_node_dir(base, "ttl", MD_A, [], False, dry_run=args.dry_run)
    flags = base_flags(args, sysfile)
    kw = {"timeout": args.timeout, "dry_run": args.dry_run}
    try:
        a = run_claude(node, flags, PROMPT, **kw)
        if args.dry_run:
            print(f"[dry-run] would sleep {args.wait}s")
        else:
            print(f"A: creation 1h={a.ephemeral_1h_input_tokens} "
                  f"5m={a.ephemeral_5m_input_tokens}")
            remaining = args.wait
            while remaining > 0:
                step = min(300, remaining)
                time.sleep(step)
                remaining -= step
                print(f"waiting... {remaining}s left", flush=True)
        b = run_claude(node, flags, PROMPT, **kw)
    except RuntimeError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    finally:
        if not args.dry_run and not args.keep and created:
            shutil.rmtree(base, ignore_errors=True)
    if args.dry_run:
        return 0
    print(f"B.read = {b.cache_read_input_tokens} "
          f"({'expired (0)' if b.cache_read_input_tokens == 0 else 'still cached'})")
    ok = a.ephemeral_5m_input_tokens == 0 and a.ephemeral_1h_input_tokens > 0
    print(f"TTL written by A: 1h={a.ephemeral_1h_input_tokens} "
          f"5m={a.ephemeral_5m_input_tokens} -> {'PASS' if ok else 'CHECK'} (all in 1h)")
    return 0


def build_parser() -> argparse.ArgumentParser:
    common = argparse.ArgumentParser(add_help=False)
    common.add_argument("--model", default="opus")
    common.add_argument("--effort", default="medium")
    common.add_argument("--timeout", type=int, default=300, help="per call, seconds")
    common.add_argument("--workdir-base", help="base dir for node workspaces")
    common.add_argument("--keep", action="store_true", help="keep temp dirs")
    common.add_argument("--dry-run", action="store_true",
                        help="print commands and exit without calling claude")

    parser = argparse.ArgumentParser(description=__doc__.split("\n")[0], parents=[common])
    parser.add_argument("--scenarios", help="comma list (default: all)")
    parser.add_argument("--effort-alt", help="effort for effort_differs")
    parser.add_argument("--json-out", help="write raw results to this path")
    parser.set_defaults(func=cmd_matrix)

    sub = parser.add_subparsers(dest="cmd")
    m = sub.add_parser("matrix", parents=[common], help="run scenario matrix (default)")
    m.add_argument("--scenarios", help="comma list (default: all)")
    m.add_argument("--effort-alt", help="effort for effort_differs")
    m.add_argument("--json-out", help="write raw results to this path")
    m.set_defaults(func=cmd_matrix)

    t = sub.add_parser("ttl", parents=[common], help="1h cache TTL expiry check")
    t.add_argument("--wait", type=int, default=3660, help="seconds between A and B")
    t.set_defaults(func=cmd_ttl)
    return parser


def main() -> int:
    args = build_parser().parse_args()
    return args.func(args)


if __name__ == "__main__":
    sys.exit(main())
