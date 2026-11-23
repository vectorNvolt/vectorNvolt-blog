"""Definition check of a loaded process (SFR-AGG-005, SSR-AGG-013, SSR-AGG-017).

``validate_process`` aggregates: names and references, step type rules, run argv allowlist,
declared-skill cap, transitions and product reads. Pool checks (skills, snippets, exclusions)
need the index and live in ``pools.validate_against_pools``.

SSR-AGG-013 (strict reading: the definition carries no code): a ``run`` command is an argument
list. ``argv[0]`` must be exactly one of ``config.PROCEDURE_ALLOWED_EXECUTABLES``. For an
interpreter (python, a shell, node, perl, ruby) every inline-code option is refused (``-c``,
``-e``, ``--eval``, ...), as are ``-m``, ``-`` (stdin) and a bare interpreter; the program is a
script file under ``SCRIPTS_DIR`` (``.process/scripts/``), given as a workspace-relative path
without ``..`` and without placeholders. ``.process/`` is protected by the WriteGuard, so the
scripts change only in S7 with the developer's approval. ``validate_process`` also checks that
the script exists and does not leave ``SCRIPTS_DIR`` through a symlink. At run time the path is
resolved against the workspace root (``resolve_script``), not the step's work folder.
``INLINE_CODE_ALLOWED`` stays False; True restores the old ``python -c "<code>"`` exception.
"""

import json
import os
import re
import subprocess
from pathlib import Path

from .. import config
from ..errors import Finding
from ..services.container import resolve_tool
from ..services.runfs import WorkspacePaths
from ..services.yaml_schema import load_schema
from ..state import ProcessDef, StepDef
from . import refs
from .dataflow import check_reads, known_products
from .transitions import check_transitions

INLINE_CODE_ALLOWED = False
SCRIPTS_DIR = ".process/scripts"
_PY_FLAGS = frozenset({"-u", "-B", "-E", "-s", "-S", "-I", "-O", "-OO", "-q", "-b", "-bb"})
_SHELLS = frozenset({"sh", "bash", "dash", "zsh", "ksh", "ash", "fish"})
_NODE_INLINE_LONG = frozenset({"--eval", "--print"})


def _family(exe: str) -> "str | None":
    base = os.path.basename(exe)
    if base.startswith("python"):
        return "python"
    if base in _SHELLS:
        return "shell"
    if base in ("node", "nodejs", "bun"):
        return "node"
    if base.startswith("perl"):
        return "perl"
    if base.startswith("ruby"):
        return "ruby"
    return None


def _inline_flag(family: str, a: str) -> bool:
    """True when option ``a`` makes the interpreter run code given on the command line."""
    if family == "python":
        return a == "-c" or (not a.startswith("--") and len(a) > 2 and "c" in a[1:])
    if family == "shell":                       # -c, also inside a group such as -ec / -xc
        return not a.startswith("--") and "c" in a[1:]
    if family == "node":
        return a.split("=", 1)[0] in _NODE_INLINE_LONG or (
            not a.startswith("--") and any(ch in a[1:] for ch in "ep"))
    if family in ("perl", "ruby"):              # -e / -E, also -ne, -pe, -lne ...
        return not a.startswith("--") and any(ch in a[1:] for ch in "eE")
    return False


def check_script_path(script: str, isolate: bool = False) -> "str | None":
    """Reason why ``script`` is not an allowed script path, or None. For an isolated step the
    ``.process/scripts/`` rule does not apply (the script must be under ``src/``, which
    ``_check_isolate`` reports as ``isolate_script_outside_src``)."""
    norm = script.replace("\\", "/")
    if os.path.isabs(script) or norm.startswith("/") or ".." in norm.split("/"):
        return f"script path {script!r} must be relative and inside the workspace"
    if "{" in script or "}" in script:
        return f"script path {script!r} must not contain placeholders"
    if isolate:
        return None
    parts = [p for p in norm.split("/") if p not in ("", ".")]
    if parts[:2] != SCRIPTS_DIR.split("/") or len(parts) < 3:
        return f"script path {script!r} must be a file under {SCRIPTS_DIR}/"
    return None


def script_index(argv: "tuple[str, ...] | list[str] | None") -> "int | None":
    """Index of the script argument of an interpreter command, or None (no interpreter, or no
    script found). Does not judge whether the command is allowed: see ``check_run_argv``."""
    family = _family(argv[0]) if argv else None
    if family is None:
        return None
    for i in range(1, len(argv)):
        if argv[i] in ("-", "--") or (argv[i].startswith("-") and _inline_flag(family, argv[i])):
            return None
        if not argv[i].startswith("-"):
            return i
    return None


def resolve_script(argv: "list[str]", ws_root: Path) -> "list[str]":
    """``argv`` with the script argument made absolute against the workspace root (the command
    runs with cwd = its work folder, the definition names the script relative to /workspace)."""
    i = script_index(argv)
    if i is None or os.path.isabs(argv[i]):
        return list(argv)
    out = list(argv)
    out[i] = str(Path(ws_root) / argv[i])
    return out


def check_run_argv(argv: "tuple[str, ...] | list[str] | None", isolate: bool = False) -> "str | None":
    """Reason why ``argv`` is not allowed, or None. ``isolate``: the script is not held to
    ``.process/scripts/`` (see ``check_script_path``)."""
    if not argv:
        return "run command is empty"
    exe = argv[0]
    if exe not in config.PROCEDURE_ALLOWED_EXECUTABLES:
        return (f"executable {exe!r} is not allowed (allowed: "
                f"{', '.join(config.PROCEDURE_ALLOWED_EXECUTABLES)})")
    family = _family(exe)
    if family is None:
        return None
    i = 1
    while i < len(argv) and argv[i].startswith("-") and argv[i] != "-":
        a = argv[i]
        if _inline_flag(family, a):
            if family == "python" and a == "-c" and INLINE_CODE_ALLOWED:
                return None if i + 1 < len(argv) else "-c needs a code argument"
            return (f"inline code ({a}) is not allowed: the process definition must not carry code "
                    f"(SSR-AGG-013); put the program in a script under {SCRIPTS_DIR}/")
        if family == "python":
            if a == "-m":
                return "-m (running a module by dotted path) is not allowed"
            if a not in _PY_FLAGS:
                return f"interpreter option {a!r} is not allowed"
        elif family == "node" and a in ("-r", "--require", "--import", "--loader"):
            return f"interpreter option {a!r} is not allowed"
        elif family in ("perl", "ruby") and (a.startswith("-M") or a.startswith("-r")):
            return f"interpreter option {a!r} is not allowed"
        i += 1
    if i >= len(argv) or argv[i] == "-":
        return f"a script under {SCRIPTS_DIR}/ is required (no inline code, no interpreter session, no stdin)"
    return check_script_path(argv[i], isolate)


def _run_args_to_scan(argv: tuple[str, ...]) -> list[str]:
    """Arguments checked for placeholders: everything except legacy inline code after ``-c``."""
    code_at = None
    if INLINE_CODE_ALLOWED:
        for i in range(1, len(argv)):
            if argv[i] == "-c":
                code_at = i + 1
                break
            if not argv[i].startswith("-"):
                break
    return [a for i, a in enumerate(argv) if i != code_at]


def _check_script_file(ws: WorkspacePaths, script: str) -> "tuple[str, str] | None":
    """(code, reason) when the script named by a valid run command is missing or escapes."""
    p = ws.root / script
    if not p.is_file():
        return "script_missing", f"script {script!r} not found"
    real = Path(os.path.realpath(p))
    base = Path(os.path.realpath(ws.root / SCRIPTS_DIR))
    if base not in real.parents:
        return "argv_not_allowed", f"script {script!r} resolves outside {SCRIPTS_DIR}/"
    return None


def _fd(proc: ProcessDef, code: str, msg: str, *, line=None, step=None, item=None) -> Finding:
    return Finding(code=code, message=msg, file=proc.source_file, line=line, step=step, item=item)


_TOOL_INPUT_RE = re.compile(r"[a-z][a-z0-9_]*")
_TOOL_PATTERN = re.compile(r"^([A-Za-z][A-Za-z0-9_]*)\((.+)\)$")


def _tool_allowed(entry: str, rt_tools: set) -> bool:
    """A step tool entry is allowed when it is in runtime.tools or is ``Name(spec)`` with Name in it."""
    if entry in rt_tools:
        return True
    m = _TOOL_PATTERN.match(entry)
    return bool(m) and m.group(1) in rt_tools


def tool_input_name(name: str) -> "str | None":
    """Input name of run_ephemeral_container for a process input / product name (lowercase,
    ``-`` -> ``_``), or None when it does not match ``[a-z][a-z0-9_]*`` (USC-005)."""
    n = name.lower().replace("-", "_")
    return n if _TOOL_INPUT_RE.fullmatch(n) else None


class _ProfileTable:
    """``run_ephemeral_container list --json`` of the workspace, read lazily and once.
    ``rows`` is None when the tool is not available or its answer is unusable (no static check)."""

    def __init__(self, ws: WorkspacePaths):
        self.ws = ws
        self._loaded = False
        self._rows: "dict[str, dict] | None" = None

    @property
    def rows(self) -> "dict[str, dict] | None":
        if not self._loaded:
            self._loaded = True
            self._rows = self._load()
        return self._rows

    def _load(self) -> "dict[str, dict] | None":
        tool = resolve_tool()
        if tool is None:
            return None
        try:
            cp = subprocess.run([tool, "list", "--json"], shell=False, stdin=subprocess.DEVNULL,
                                capture_output=True, text=True, timeout=10, check=False,
                                env={**os.environ, "AGENT_EXEC_WORKSPACE": str(self.ws.root)})
            data = json.loads(cp.stdout)
        except Exception:  # noqa: BLE001 - tool missing, hung or answering garbage: skip the check
            return None
        if cp.returncode != 0 or not isinstance(data, list):
            return None
        return {str(r.get("name")): r for r in data if isinstance(r, dict)}


def _check_isolate(proc: ProcessDef, ws: WorkspacePaths, s: StepDef,
                   profiles: "_ProfileTable | None") -> list[Finding]:
    """USC-005 rules of a procedure step: ``isolate: true`` <-> ``profile``, only ``src/`` is visible
    in the container, input names must be valid tool input names, the profile exists (when the
    tool can be asked; without the tool this static check is skipped)."""
    out: list[Finding] = []

    def err(code: str, msg: str, item: "str | None" = None) -> None:
        out.append(_fd(proc, code, f"step {s.name!r}: {msg}", line=s.line, step=s.name, item=item))

    if not s.isolate:
        if s.profile:
            err("isolate_profile_misplaced", "profile needs isolate: true", s.profile)
        return out
    if not s.profile:
        err("isolate_profile_required", "isolate: true needs a profile (the exec profile name)")
    if s.run:
        si = script_index(s.run)
        if si is not None and check_run_argv(s.run, True) is None:     # else argv_not_allowed already
            real = Path(os.path.realpath(ws.root / s.run[si]))
            if Path(os.path.realpath(ws.root / "src")) not in real.parents:
                err("isolate_script_outside_src",
                    f"script {s.run[si]!r} must lie under src/ (only src/ is visible in the container)",
                    s.run[si])
        own = {d.name for d in s.produces}
        for a in _run_args_to_scan(s.run):
            for ph in refs.placeholders(a):
                if ph == "work" or ph in own:
                    continue
                nm = ph[len("input."):] if ph.startswith("input.") else ph
                if tool_input_name(nm) is None:
                    err("isolate_input_name",
                        f"input {nm!r} cannot be passed to the container: its tool input name must "
                        "match [a-z][a-z0-9_]* (lowercase, '-' becomes '_')", nm)
    if s.profile and profiles is not None and profiles.rows is not None:
        row = profiles.rows.get(s.profile)
        if row is None:
            err("isolate_profile_missing", f"exec profile {s.profile!r} not found", s.profile)
        elif row.get("kind") != "code":
            err("isolate_profile_unknown",
                f"exec profile {s.profile!r} has kind {row.get('kind')!r}, a code profile is needed", s.profile)
        elif not row.get("valid"):
            err("isolate_profile_invalid",
                f"exec profile {s.profile!r} is invalid: {row.get('error') or 'unknown error'}", s.profile)
    return out


def effective_skills(proc: ProcessDef, step: StepDef) -> list[str]:
    """Skills declared for a step: runtime skills minus its exclusions, plus its own; order kept."""
    out = [s for s in proc.runtime.skills if s not in step.exclude_skills]
    out += [s for s in step.skills if s not in out]
    return out


def validate_process(proc: ProcessDef, ws: WorkspacePaths) -> list[Finding]:
    out: list[Finding] = []
    names: dict[str, StepDef] = {}
    for s in proc.steps:
        if s.name in names:
            out.append(_fd(proc, "dup_step", f"duplicate step name {s.name!r}", line=s.line, step=s.name))
        names[s.name] = s
    ids: set[int] = set()
    for s in proc.steps:
        if s.id in ids:
            out.append(_fd(proc, "dup_step", f"duplicate step id {s.id}", line=s.line, step=s.name))
        ids.add(s.id)

    def uniq(kind: str, items) -> None:
        seen: set[str] = set()
        for x in items:
            if x.name in seen:
                out.append(_fd(proc, "dup_name", f"duplicate {kind} name {x.name!r}", line=x.line, item=x.name))
            seen.add(x.name)

    uniq("source", proc.sources)
    uniq("output-template-ref", proc.templates)
    uniq("ruleset", proc.rulesets)
    uniq("target", proc.targets)

    src_names = {x.name for x in proc.sources}
    tpl_names = {x.name for x in proc.templates}
    rs_names = {x.name for x in proc.rulesets}
    tgt_names = {x.name for x in proc.targets}

    for x in proc.sources:
        if x.path is None and x.note is None:
            out.append(_fd(proc, "source_def", f"source {x.name!r} needs a path or a note",
                           line=x.line, item=x.name))
    for t in proc.targets:
        if t.template is not None and t.template not in tpl_names:
            out.append(_fd(proc, "unresolved_ref",
                           f"target {t.name!r} names unknown template {t.template!r}", line=t.line, item=t.template))
        for c in t.checks:
            base, _, arg = c.partition(":")
            if base not in config.OUTPUT_CHECKS:
                out.append(_fd(proc, "check_invalid", f"unknown output check {c!r}", line=t.line, item=c))
            elif base in ("max_words", "min_words") and not arg.strip().isdigit():
                out.append(_fd(proc, "check_invalid", f"output check {base} needs an integer ({base}:<n>)",
                               line=t.line, item=c))

    products = known_products(proc)
    profiles = _ProfileTable(ws) if any(x.isolate and x.profile for x in proc.steps) else None
    rt_tools = set(proc.runtime.tools)
    for t in proc.runtime.tools:
        if "," in t:
            out.append(_fd(proc, "tool_entry_comma", f"runtime.tools entry {t!r} contains a comma"))
        elif _TOOL_PATTERN.match(t):
            out.append(_fd(proc, "runtime_tool_pattern",
                           f"runtime.tools entry {t!r} must be a plain tool name, not a Name(spec) pattern"))
    for s in proc.steps:
        for r in s.reads:
            table = {"input.source": src_names, "input.ruleset": rs_names,
                     "input.output-template-ref": tpl_names}
            if r.kind in table and r.name not in table[r.kind]:
                out.append(_fd(proc, "unresolved_ref", f"step {s.name!r} reads unknown {r.kind} {r.name!r}",
                               line=s.line, step=s.name, item=r.name))
            elif r.kind == "product" and r.raw.strip().startswith("output.target[") and r.name not in tgt_names:
                out.append(_fd(proc, "unresolved_ref", f"step {s.name!r} reads unknown output target {r.name!r}",
                               line=s.line, step=s.name, item=r.name))
        for p in s.produces:
            if p.output_target is not None and p.output_target not in tgt_names:
                out.append(_fd(proc, "unresolved_ref",
                               f"step {s.name!r} produces unknown output target {p.output_target!r}",
                               line=p.line or s.line, step=s.name, item=p.output_target))
            if p.schema:
                try:
                    load_schema(str(ws.root / p.schema))
                except FileNotFoundError:
                    out.append(_fd(proc, "schema_missing", f"schema file {p.schema!r} not found",
                                   line=p.line or s.line, step=s.name, item=p.name))
                except Exception as e:  # unreadable or invalid schema file
                    out.append(_fd(proc, "schema_invalid", f"schema file {p.schema!r} is invalid: {e}",
                                   line=p.line or s.line, step=s.name, item=p.name))

        if s.tools is not None:
            for t in s.tools:
                if "," in t:
                    out.append(_fd(proc, "tool_entry_comma",
                                   f"step {s.name!r} tool entry {t!r} contains a comma", line=s.line, step=s.name))
            extra = sorted(t for t in set(s.tools) if not _tool_allowed(t, rt_tools))
            if extra:
                out.append(_fd(proc, "tools_not_subset",
                               f"step {s.name!r} tools {extra} are not in runtime.tools", line=s.line, step=s.name))

        if s.type == "procedure":
            if s.run is None:
                out.append(_fd(proc, "run_missing", f"procedure step {s.name!r} needs a run command",
                               line=s.line, step=s.name))
            else:
                why = check_run_argv(s.run, s.isolate)
                if why:
                    out.append(_fd(proc, "argv_not_allowed", f"step {s.name!r}: {why}",
                                   line=s.line, step=s.name, item=s.run[0] if s.run else None))
                elif not s.isolate:             # an isolated script lives in src/: see _check_isolate
                    si = script_index(s.run)
                    bad = _check_script_file(ws, s.run[si]) if si is not None else None
                    if bad:
                        out.append(_fd(proc, bad[0], f"step {s.name!r}: {bad[1]}",
                                       line=s.line, step=s.name, item=s.run[si]))
                for a in _run_args_to_scan(s.run):
                    for ph in refs.placeholders(a):
                        if ph == "work":
                            continue
                        if ph.startswith("input."):
                            nm = ph[len("input."):]
                            if nm not in src_names | tpl_names | rs_names:
                                out.append(_fd(proc, "unresolved_ref",
                                               f"step {s.name!r}: placeholder {{{ph}}} names no input",
                                               line=s.line, step=s.name, item=nm))
                        elif ph not in products:
                            out.append(_fd(proc, "unresolved_ref",
                                           f"step {s.name!r}: placeholder {{{ph}}} names no product",
                                           line=s.line, step=s.name, item=ph))
            out.extend(_check_isolate(proc, ws, s, profiles))
        else:
            if s.run is not None:
                out.append(_fd(proc, "run_misplaced", f"{s.type} step {s.name!r} must not have a run command",
                               line=s.line, step=s.name))

        if s.type == "cognitive":
            eff = effective_skills(proc, s)
            cap = proc.max_skills_of(s)
            if len(eff) > cap:
                out.append(_fd(proc, "max_skills",
                               f"step {s.name!r} declares {len(eff)} skills, max_skills is {cap}",
                               line=s.line, step=s.name))

    # same product name from two different steps would make "the" product ambiguous
    owner: dict[str, str] = {}
    for s in proc.steps:
        for p in s.produces:
            if p.name in owner and owner[p.name] != s.name:
                out.append(_fd(proc, "dup_product",
                               f"product {p.name!r} is produced by {owner[p.name]!r} and {s.name!r}",
                               line=p.line or s.line, step=s.name, item=p.name))
            owner.setdefault(p.name, s.name)

    out.extend(check_transitions(proc))
    out.extend(check_reads(proc))
    return out
