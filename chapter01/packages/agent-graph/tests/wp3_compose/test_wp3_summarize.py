import dataclasses

import pytest

from agent_graph.compose import compose_context
from agent_graph.errors import ClaudeTurnError, TurnCancelled
from agent_graph.services.runfs import sha256_file
from agent_graph.services.summarize import SummaryCache
from agent_graph.state import ReadRef
from fakes.workspace import write_file
from test_wp3_support import (FACTS_READ, LAUNCH, FakePort, make_inputs, make_process)


class Gate:
    def __init__(self, refusal=None):
        self.refusal = refusal
        self.nodes = []

    def attempt_token(self, node):
        raise AssertionError("not used")

    def before_node_start(self, node):
        return None

    def before_llm_call(self, node):
        self.nodes.append(node)
        return self.refusal


def src(workspace):
    return workspace.root / ".input/example/source.md"


def test_first_call_asks_claude_second_hits_cache(workspace, guard):
    port = FakePort("A cached summary.")
    sc = SummaryCache(workspace, port, LAUNCH, None, guard)
    key = sha256_file(src(workspace))
    assert sc.summary(src(workspace), key, 100) == "A cached summary."
    assert len(port.requests) == 1
    assert (workspace.cache_summaries / f"{key}-100.md").read_text() == "A cached summary.\n"
    assert sc.summary(src(workspace), key, 100) == "A cached summary."
    assert len(port.requests) == 1                                           # cache hit
    sc.summary(src(workspace), key, 200)                                     # other budget: new entry
    assert len(port.requests) == 2
    # another cache object (other run, same agent) sees the same files
    port2 = FakePort("different")
    assert SummaryCache(workspace, port2, LAUNCH, None, guard).summary(src(workspace), key, 100) \
        == "A cached summary."
    assert port2.requests == []


def test_request_shape_read_only_own_launch_relative_path(workspace, guard):
    port = FakePort()
    SummaryCache(workspace, port, LAUNCH, None, guard).summary(src(workspace), "ignored", 100)
    req = port.requests[0]
    assert req.allowed_tools == ("Read",) and req.launch.tools == ("Read",)
    assert req.launch.system_prompt_file is None and req.resume is None
    assert req.cwd == workspace.agent_dir / "cache" / "summarizer"
    assert "`../../../.input/example/source.md`" in req.prompt and "100 tokens" in req.prompt
    assert (req.cwd / "mcp.json").read_bytes() == b'{"mcpServers": {}}\n'
    assert str(workspace.root) not in req.prompt


def test_content_hash_decides_the_key_not_the_callers_claim(workspace, guard):
    port = FakePort()
    sc = SummaryCache(workspace, port, LAUNCH, None, guard)
    sc.summary(src(workspace), "0" * 64, 100)
    assert (workspace.cache_summaries / f"{sha256_file(src(workspace))}-100.md").is_file()
    write_file(workspace.root, ".input/example/source.md", "changed\n")
    sc.summary(src(workspace), "0" * 64, 100)
    assert len(port.requests) == 2


def test_gate_refusal_raises_turn_cancelled_without_call(workspace, guard):
    port = FakePort()
    gate = Gate("budget:max_llm_calls")
    sc = SummaryCache(workspace, port, LAUNCH, gate, guard)
    with pytest.raises(TurnCancelled) as e:
        sc.summary(src(workspace), "x", 100)
    assert e.value.cause == "budget:max_llm_calls" and port.requests == []
    assert gate.nodes == ["summarize"]
    assert not any(workspace.cache_summaries.glob("*")) if workspace.cache_summaries.exists() else True


def test_empty_summary_is_an_error_and_not_cached(workspace, guard):
    sc = SummaryCache(workspace, FakePort("  \n"), LAUNCH, None, guard)
    with pytest.raises(ClaudeTurnError):
        sc.summary(src(workspace), "x", 100)
    assert not list(workspace.cache_summaries.glob("*.md")) if workspace.cache_summaries.exists() else True


def test_compose_twice_uses_the_summary_cache(workspace, guard):
    write_file(workspace.root, ".input/example/source.md", "word " * 40000)
    port = FakePort("Condensed source.")
    sc = SummaryCache(workspace, port, LAUNCH, None, guard)
    process = make_process()
    step = dataclasses.replace(
        process.step("write-summary"), budget=3000,
        reads=(FACTS_READ, ReadRef("input.source[Source]", "input.source", "Source", "", False)))
    r1 = compose_context(make_inputs(workspace, process=process, step=step), sc, guard)
    assert len(port.requests) == 1
    r2 = compose_context(make_inputs(workspace, process=process, step=step), sc, guard)
    assert len(port.requests) == 1
    assert r1.claude_md_sha256 == r2.claude_md_sha256
    ctx = (workspace.run("example-20260101-000000").active / "context.md").read_text()
    assert "Condensed source." in ctx
