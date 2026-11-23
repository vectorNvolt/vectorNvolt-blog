"""Skill selection and staging (SFR-AGG-018, SSR-AGG-017, plan decision 9).

Declared skills (runtime.skills, then step.skills) are always staged. Skills in
``step.exclude_skills`` are never staged and never ranked (a skill the same step declares stays).
The other pool skills are ranked by keyword overlap with the node goal (name 1, description 1,
tag 2 per matched goal word; ties by name; score 0 is never staged) and staged while fewer than
max_skills are staged. Skills go only into ``<active>/.claude/skills/<name>/``, verbatim.
"""

import shutil
from dataclasses import dataclass
from pathlib import Path
from typing import Sequence

from ..services.runfs import WorkspacePaths, WriteGuard, sha256_bytes, write_atomic
from ..state import PoolIndex, ProcessDef, SkillEntry, StepDef
from .tokens import goal_tokens, word_tokens


@dataclass(frozen=True)
class SkillSelection:
    declared: tuple          # always staged, runtime then step order
    excluded: tuple          # step.exclude_skills (minus names the same step declares), sorted
    ranked: tuple            # ((name, score), ...) all candidates with score > 0, best first
    staged: tuple            # sorted names (declared + best ranked within max_skills)
    not_staged_cap: tuple    # ranked names beyond the cap


def score_skill(goal: frozenset, s: SkillEntry) -> int:
    name_t = word_tokens(s.name)
    desc_t = word_tokens(s.description)
    tag_t = frozenset(w for t in s.tags for w in word_tokens(t))
    return sum((w in name_t) + (w in desc_t) + 2 * (w in tag_t) for w in goal)


def select_skills(process: ProcessDef, step: StepDef, index: PoolIndex) -> SkillSelection:
    declared: list = []
    for n in (*process.runtime.skills, *step.skills):
        if n not in declared:
            declared.append(n)
    for n in declared:
        if n not in index.skills:
            raise ValueError(f"declared skill not in the pool index: {n}")
    excluded = sorted(set(step.exclude_skills) - set(step.skills))
    # an exclusion removes a process-declared skill; a step-declared one stays (S3 rejects the clash)
    declared = [n for n in declared if n not in excluded]
    goal = goal_tokens(step.name, step.instruction)
    scored = [(n, score_skill(goal, s)) for n, s in index.skills.items()
              if n not in declared and n not in excluded]
    ranked = tuple(sorted(((n, sc) for n, sc in scored if sc > 0), key=lambda t: (-t[1], t[0])))
    slots = max(0, process.max_skills_of(step) - len(declared))
    added = tuple(n for n, _ in ranked[:slots])
    return SkillSelection(declared=tuple(declared), excluded=tuple(excluded), ranked=ranked,
                          staged=tuple(sorted({*declared, *added})),
                          not_staged_cap=tuple(n for n, _ in ranked[slots:]))


def skills_root(active_dir: Path) -> Path:
    return Path(active_dir) / ".claude" / "skills"


def stage_skills(names: Sequence[str], index: PoolIndex, ws: WorkspacePaths, active_dir: Path,
                 guard: WriteGuard) -> None:
    """Replace ``<active>/.claude/skills`` by verbatim copies of the pool skills ``names``."""
    root = skills_root(active_dir)
    real = guard.check(root)
    if real.exists():
        shutil.rmtree(real)
    for name in sorted(names):
        entry = index.skills[name]
        for rel, sha in entry.files:
            data = (ws.root / entry.dir / rel).read_bytes()
            if sha256_bytes(data) != sha:
                raise ValueError(f"skill file changed since indexing: {entry.dir}/{rel}")
            write_atomic(root / name / rel, data, guard=guard)
