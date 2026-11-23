"""The states S2, S3, S4, S5 and S8 one by one, with a fake Claude CLI and a scripted gateway
(SFR-AGG-003, 004, 005-009, 010, 011, 029, 032, 021, SSR-AGG-014)."""

import json
import sys
from types import ModuleType

from agent_graph.contract import Notice
from agent_graph.errors import ExitState
from agent_graph.nodes import n02_clarify, n03_parse_process, n04_bind_input, n05_build_graph, n08_report
from agent_graph.process import load_process
from fakes.workspace import PROCESS_YML, add_process, build_workspace, process_with_id, write_file


def argv_value(call, flag):
    return call["argv"][call["argv"].index(flag) + 1]


# ---------------------------------------------------------------- S2 clarify
def test_s2_agrees_a_process_and_creates_the_run(make_env, dialog_turn):
    h = make_env(turns=[dialog_turn("agree", "The example process fits.", process_id="example")],
                 replies=["I want to summarize my notes"], confirms=[True])
    out = n02_clarify.run(h.env, {})
    assert out["phase"] == "S3" and out["process_id"] == "example"
    paths = h.env.paths
    assert paths.run_id == "example-20260102-030405"
    assert paths.has_checkpoint() and paths.gateway_jsonl.is_file() and paths.mcp_json.is_file()
    assert (paths.dialogs_dir / "S2-1" / "transcript.jsonl").is_file()      # SSR-AGG-014
    rec = json.loads(paths.run_json.read_text(encoding="utf-8"))
    assert rec["process_id"] == "example" and rec["process_file"] == ".process/example.yml"
    assert json.loads(h.ws.active_json.read_text(encoding="utf-8"))["run_id"] == paths.run_id
    call = h.fc.calls()[0]
    assert argv_value(call, "--tools") == "Read" and argv_value(call, "--allowedTools") == "Read"
    assert "summarize my notes" in call["prompt"] and "example" in call["prompt"]
    assert call["tmpdir"] == str(h.ws.static_dir / "tmp")                    # SFR-AGG-041
    assert "The example process fits." in h.gw.said("S2 clarify")


def test_s2_asks_back_and_continues_the_session(make_env, dialog_turn):
    h = make_env(turns=[dialog_turn("ask", "Which notes?"),
                        dialog_turn("agree", "Then it is example.", process_id="example")],
                 replies=["summarize", "the meeting notes"], confirms=[True])
    assert n02_clarify.run(h.env, {})["phase"] == "S3"
    first, second = h.fc.calls()
    assert second["resume"] == "fake-session-1" and "the meeting notes" in second["prompt"]


def test_s2_developer_can_decline_the_proposal(make_env, dialog_turn):
    h = make_env(turns=[dialog_turn("agree", "example?", process_id="example"),
                        dialog_turn("agree", "again: example", process_id="example")],
                 replies=["x", "no, not that"], confirms=[False, True])
    assert n02_clarify.run(h.env, {})["phase"] == "S3"
    assert "no, not that" in h.fc.calls()[1]["prompt"]


def test_s2_unknown_process_id_from_claude_is_not_accepted(make_env, dialog_turn):
    h = make_env(turns=[dialog_turn("agree", "ghost", process_id="ghost"),
                        dialog_turn("agree", "example", process_id="example")], replies=["x"],
                 confirms=[True])
    assert n02_clarify.run(h.env, {})["phase"] == "S3"
    assert h.env.record.process_id == "example"
    assert "ghost" in h.fc.calls()[1]["prompt"]


def test_s2_no_match_is_intent_rejected_without_run(make_env, dialog_turn):
    h = make_env(turns=[dialog_turn("done", "Nothing matches.")], replies=["bake a cake"])
    out = n02_clarify.run(h.env, {})
    assert out["phase"] == "S8" and out["exit_state"] == "INTENT_REJECTED"
    assert h.env.paths is None


def test_s2_text_only_menu_when_claude_is_unavailable(make_env):
    h = make_env(turns=[{"login_error": True}], replies=["anything"], choices=[0])
    out = n02_clarify.run(h.env, {})                                          # SFR-AGG-029
    assert out["phase"] == "S3" and h.env.record.process_id == "example"
    assert [n[1] for n in h.gw.notices] == ["LLM_UNAVAILABLE"]


def test_s2_text_only_menu_after_invalid_replies(make_env):
    h = make_env(turns=[{"text": "not json"}, {"text": "still not json"}], replies=["x"], choices=[0])
    assert n02_clarify.run(h.env, {})["phase"] == "S3"
    assert h.fc.calls()[1]["resume"] == "fake-session-1"
    assert h.gw.notices


def test_s2_menu_quit_is_intent_rejected(make_env):
    h = make_env(turns=[{"login_error": True}], replies=["x"], choices=[1])    # options: example, Quit
    out = n02_clarify.run(h.env, {})
    assert out["exit_state"] == "INTENT_REJECTED" and h.env.paths is None


def test_s2_preselected_process_after_resolution_skips_the_dialog(make_env):
    h = make_env(confirms=[True])
    out = n02_clarify.run(h.env, {"process_id": "example", "reentry": {"state": "S2", "new_run": True}})
    assert out["phase"] == "S3" and h.fc.calls() == []


def test_s2_lists_duplicates_as_not_selectable(ws_root, make_env, dialog_turn):
    ws = build_workspace(ws_root)
    add_process(ws, "twin.yml", process_with_id("twin"))
    add_process(ws, "twin2.yml", process_with_id("twin"))
    h = make_env(turns=[dialog_turn("agree", "example", process_id="example")], replies=["x"], ws=ws,
                 confirms=[True])
    assert n02_clarify.run(h.env, {})["phase"] == "S3"
    listing = h.gw.said("S2 clarify")[0]
    assert "not selectable" in listing and "twin" in listing


# ---------------------------------------------------------------- S3 parse_process
def started(make_env, **kw):
    h = make_env(**kw)
    h.env.create_run("example", ".process/example.yml")
    return h


def test_s3_valid_process_writes_the_index(make_env):
    h = started(make_env)
    out = n03_parse_process.run(h.env, {})
    assert out["phase"] == "S4" and h.env.proc.id == "example"
    index = json.loads(h.env.paths.index_json.read_text(encoding="utf-8"))
    assert "summarize" in index["skills"] and "agent-identity" in index["snippets"]
    assert index["snippets"]["agent-identity"]["sha256"]


def test_s3_findings_go_to_s7_as_process_invalid(make_env):
    h = started(make_env)
    write_file(h.ws.root, ".process/example.yml", PROCESS_YML.replace("skills: [summarize]", "skills: [ghost]"))
    out = n03_parse_process.run(h.env, {})
    assert out["phase"] == "S7" and out["error"]["exit_state"] == "PROCESS_INVALID"
    assert out["error"]["state"] == "S3"
    assert any(f["code"] == "skill_missing" and f["item"] == "ghost" and f["file"] for f in out["error"]["findings"])


def test_s3_invalid_yaml_reports_a_line(make_env):
    h = started(make_env)
    write_file(h.ws.root, ".process/example.yml", "apiVersion: agent-graph/v1\nid: [unclosed\n")
    out = n03_parse_process.run(h.env, {})
    assert out["phase"] == "S7" and out["error"]["exit_state"] == "PROCESS_INVALID"
    assert out["error"]["findings"]


def test_s3_v0_api_version_is_rejected(make_env):
    h = started(make_env)
    write_file(h.ws.root, ".process/example.yml", PROCESS_YML.replace("agent-graph/v1", "agent-graph/v0"))
    out = n03_parse_process.run(h.env, {})
    assert out["phase"] == "S7" and any(f["code"] == "api_version" for f in out["error"]["findings"])


# ---------------------------------------------------------------- S4 bind_input
def validated(make_env, ws=None, **kw):
    h = started(make_env, ws=ws, **kw)
    assert n03_parse_process.run(h.env, {})["phase"] == "S4"
    return h


def test_s4_binds_the_single_candidate_set(make_env):
    h = validated(make_env)
    out = n04_bind_input.run(h.env, {})
    assert out["phase"] == "S5"
    binding = json.loads(h.env.paths.binding_json.read_text(encoding="utf-8"))
    assert [f["path"] for f in binding["sources"]["Source"]["files"]] == [".input/example/source.md"]
    assert binding["templates"]["Template"]["path"] == "templates/report.md"
    assert binding["process_id"] == "example"
    assert ".input/example/source.md" in h.env.binding.allowed_input_paths()


def test_s4_several_candidate_sets_the_developer_chooses(ws_root, make_env):
    ws = build_workspace(ws_root, process_text=PROCESS_YML.replace('path: "example/*.md"', 'path: "*/*.md"'))
    write_file(ws.root, ".input/alt/other.md", "# Other\n")
    h = validated(make_env, ws=ws, choices=[1])               # sorted sets: .input/alt, .input/example
    out = n04_bind_input.run(h.env, {})
    assert out["phase"] == "S5"
    assert [f.path for f in h.env.binding.sources["Source"].files] == [".input/example/source.md"]
    assert [e for e in h.gw.events if e[0] == "choose"]


def test_s4_missing_input_is_input_invalid(make_env):
    h = validated(make_env)
    (h.ws.input_dir / "example" / "source.md").unlink()
    out = n04_bind_input.run(h.env, {})
    assert out["phase"] == "S7" and out["error"]["exit_state"] == "INPUT_INVALID"
    assert out["error"]["state"] == "S4" and not h.env.paths.binding_json.exists()


def test_s4_missing_template_is_input_invalid(make_env):
    h = validated(make_env)
    (h.ws.root / "templates" / "report.md").unlink()
    out = n04_bind_input.run(h.env, {})
    assert out["error"]["exit_state"] == "INPUT_INVALID"


def test_s4_asks_for_a_note_once(ws_root, make_env):
    text = PROCESS_YML.replace('    - {name: Source, path: "example/*.md", required: true}\n',
                               '    - {name: Source, path: "example/*.md", required: true}\n'
                               '    - {name: Audience, note: "Who reads the summary?", required: true}\n')
    ws = build_workspace(ws_root, process_text=text)
    h = started(make_env, ws=ws, replies=["the board"])
    proc, findings = load_process(ws.root / ".process/example.yml", ws)
    assert proc is not None, findings
    h.env.proc = proc
    assert n04_bind_input.run(h.env, {})["phase"] == "S5"
    assert h.env.binding.sources["Audience"].note_text == "the board"
    assert any("Who reads the summary?" in p for _s, p in h.gw.asked)
    assert n04_bind_input.run(h.env, {})["phase"] == "S5"      # re-entry: the note is kept, no question
    assert len(h.gw.asked) == 1


# ---------------------------------------------------------------- S5 build_graph (fake builder)
def fake_builder(monkeypatch, build):
    pkg = ModuleType("agent_graph.dynamic")
    pkg.__path__ = []
    mod = ModuleType("agent_graph.dynamic.builder")
    mod.build_graph_spec = build
    mod.compile_dynamic = lambda proc, checkpointer: object()
    monkeypatch.setitem(sys.modules, "agent_graph.dynamic", pkg)
    monkeypatch.setitem(sys.modules, "agent_graph.dynamic.builder", mod)


def bound(make_env, **kw):
    h = validated(make_env, **kw)
    assert n04_bind_input.run(h.env, {})["phase"] == "S5"
    return h


def test_s5_compile_error_is_build_failed_with_partial_graph(make_env, monkeypatch):
    def boom(proc):
        raise RuntimeError("no edge for outcome")
    fake_builder(monkeypatch, boom)
    h = bound(make_env)
    out = n05_build_graph.run(h.env, {})
    assert out["phase"] == "S7" and out["error"]["exit_state"] == "BUILD_FAILED"
    error = json.loads(h.env.paths.graph_error_json.read_text(encoding="utf-8"))
    assert "no edge for outcome" in error["message"]


def test_s5_missing_builder_is_build_failed(make_env, monkeypatch):
    monkeypatch.setitem(sys.modules, "agent_graph.dynamic", None)           # import raises
    monkeypatch.setitem(sys.modules, "agent_graph.dynamic.builder", None)
    h = bound(make_env)
    out = n05_build_graph.run(h.env, {})
    assert out["error"]["exit_state"] == "BUILD_FAILED" and h.env.paths.graph_error_json.is_file()


def spec_builder(proc):
    from agent_graph.process import bfs_order
    from agent_graph.state import GraphNode, GraphSpec
    nodes = tuple(GraphNode(name=s.name, type=s.type, step_hash=f"h-{s.name}",
                            reads=tuple(r.raw for r in s.reads), produces=tuple(p.name for p in s.produces))
                  for s in proc.steps)
    return GraphSpec(process_id=proc.id, process_sha256=proc.source_sha256, entry=proc.entry, nodes=nodes,
                     edges=proc.edges, order=bfs_order(proc), process=proc.to_json())


def test_s5_shows_the_outline_and_waits_for_the_start(make_env, monkeypatch):
    fake_builder(monkeypatch, spec_builder)
    h = bound(make_env, confirms=[True])
    out = n05_build_graph.run(h.env, {})
    assert out["phase"] == "S6" and h.env.start_mode == "start"
    graph = json.loads(h.env.paths.graph_json.read_text(encoding="utf-8"))
    assert [n["name"] for n in graph["nodes"]] == ["extract", "write-summary", "review"]
    outline = "\n".join(h.gw.said("S5 build_graph"))
    assert "extract (procedure)" in outline and "Human steps: review" in outline
    assert "max_steps 50" in outline
    assert [e for e in h.gw.events if e[0] == "confirm" and "Start the run" in e[2]]


def test_s5_declined_start_can_quit(make_env, monkeypatch):
    fake_builder(monkeypatch, spec_builder)
    h = bound(make_env, replies=["/quit"], confirms=[False])
    out = n05_build_graph.run(h.env, {})
    assert out["phase"] == "S8" and out["exit_state"] == "INTENT_REJECTED"


# ---------------------------------------------------------------- S8 report
def test_s8_completed_writes_report_and_ends_the_run(make_env):
    h = started(make_env)
    rec = h.env.record
    rec.counters.steps, rec.counters.llm_calls = 3, 2
    write_file(h.env.paths.output_dir, "answer.txt", "done\n")
    h.env.history().notice(Notice("engine", "CACHE_PREFIX_DRIFT", "node b read 10 of 100 tokens"))
    out = n08_report.run(h.env, {"exit_state": "COMPLETED", "error": None})
    assert out["exit_state"] == "COMPLETED"
    report = h.env.paths.report_md.read_text(encoding="utf-8")
    assert "COMPLETED" in report and "answer.txt" in report and "CACHE_PREFIX_DRIFT" in report
    saved = json.loads(h.env.paths.run_json.read_text(encoding="utf-8"))
    assert saved["status"] == "ENDED" and saved["exit_code"] == 0 and saved["exit_state"] == "COMPLETED"
    assert any("exit_state: COMPLETED" in t for t in h.gw.said("S8 report"))


def test_s8_without_a_run_only_prints(make_env):
    h = make_env()
    n08_report.run(h.env, {"exit_state": "INTENT_REJECTED", "error": None})
    assert any("exit_code: 3" in t for t in h.gw.said("S8 report"))
    assert not list(h.ws.output_root.iterdir())


def test_exit_codes_per_state():
    codes = {s.value: s.exit_code for s in ExitState}
    assert codes["COMPLETED"] == 0 and codes["NOT_STARTED"] == 1 and codes["INPUT_INVALID"] == 3
    assert codes["NODE_FAILED"] == 2 and codes["SUSPENDED"] == 130
