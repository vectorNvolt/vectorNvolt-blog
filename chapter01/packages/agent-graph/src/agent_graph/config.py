"""Runtime settings. Every value has an env override or a fixed default.

Covers: SFR-AGG-016 (default budgets), SFR-AGG-017 (layer caps), SSR-AGG-013
(allowed procedure executables), SFR-AGG-032 (exit codes live in errors.py),
SSR-AGG-018 (cache drift ratio).
"""

import os
import shlex
from pathlib import Path

WORKSPACE = Path(os.environ.get("AGENT_GRAPH_WORKSPACE", "/workspace"))
CLAUDE_CONFIG_DIR = Path(os.environ.get("CLAUDE_CONFIG_DIR", "/home/agent/.claude"))
CLAUDE_CMD: tuple[str, ...] = tuple(shlex.split(os.environ.get("AGENT_GRAPH_CLAUDE_CMD", "claude")))
RUN_MODEL = os.environ.get("AGENT_GRAPH_MODEL", "opus")
RUN_EFFORT = os.environ.get("AGENT_GRAPH_EFFORT", "medium")
API_VERSION = "agent-graph/v1"
STEP_TYPES = ("procedure", "cognitive", "human")
DEFAULT_TOOLS = ("Read", "Write")
DEFAULT_MAX_SKILLS = 3
DEFAULT_RETRY = 2
DEFAULT_STEP_TIMEOUT_S = 600            # not applied to human steps (Q11)
DEFAULT_MAX_STEPS = 50
DEFAULT_MAX_LLM_CALLS = 50
DEFAULT_RUN_TIMEOUT_S = 4 * 3600
RECURSION_LIMIT = 1_000_000             # LangGraph superstep cap for every graph here; safety net, step/LLM budgets bind first
DEFAULT_NODE_BUDGET = 40_000            # context.md tokens
DEFAULT_WORK_PRODUCTS = "archive"
LAYER_CAPS = {"L0": 4000, "L1": 4000, "L2": 8000, "L3": 4000}
CHARS_PER_TOKEN = 4                     # local estimate (Q8)
CACHE_DRIFT_RATIO = 0.9
NDJSON_LINE_CAP = 8 * 1024 * 1024
PROCEDURE_ALLOWED_EXECUTABLES = tuple(os.environ.get(
    "AGENT_GRAPH_PROC_EXES", "python,python3,/opt/venv/bin/python").split(","))
OUTPUT_CHECKS = ("non_empty", "max_words", "min_words", "json_valid", "yaml_valid")
DIALOG_TOOLS = ("Read",)                # S2/S4/S7 Claude instances: read-only
EXIT_SUSPENDED = 130                    # proposed (Q4)
SUMMARY_SHARE_MIN_TOKENS = 200
