"""SFR-AGG-034: hash snapshots, re-entry table, earliest changed node."""

import dataclasses

import pytest

from agent_graph.process.binding import find_candidates, make_binding, single_choices
from agent_graph.process.loader import load_process
from agent_graph.process.reentry import (
    HashSnapshot, changed_sources, decide_reentry, earliest_changed_node, snapshot)
from agent_graph.state import Edge, GraphNode, GraphSpec

BASE = HashSnapshot(process_listing="l", process_file="f", pools="p", inputs={"source:S": "a"},
                    binding="b", overrides="o", process_id="pid")


def re(failed, **changes):
    choice = changes.pop("process_choice_changed", False)
    return decide_reentry(failed, BASE, dataclasses.replace(BASE, **changes), process_choice_changed=choice)


@pytest.mark.parametrize("failed", ["S2", "S3", "S4", "S5", "S6"])
def test_nothing_or_overrides_only_returns_to_failed_state(failed):
    assert re(failed).state == failed
    r = re(failed, overrides="o2")
    assert r.state == failed and r.restart_node is None and not r.new_run


def test_precondition_fixed_goes_to_s1():
    assert re("S1", process_file="x", inputs={}).state == "S1"


def test_process_choice_goes_to_s2_with_new_run():
    r = re("S6", process_choice_changed=True)
    assert r.state == "S2" and r.new_run


@pytest.mark.parametrize("change", [{"process_file": "f2"}, {"pools": "p2"}, {"process_listing": "l2"},
                                    {"process_id": "other"}, {"scripts": "s2"}])
def test_process_text_or_pools_go_to_s3(change):
    assert re("S6", **change).state == "S3"
    assert re("S5", **change).state == "S3"
    assert re("S4", **change).state == "S3"
    assert re("S3", **change).state == "S3"
    assert re("S2", **change).state == "S2"     # S2 failed before a process was agreed


@pytest.mark.parametrize("change", [{"inputs": {"source:S": "z"}}, {"binding": "b2"}])
def test_inputs_or_binding_go_to_s4(change):
    assert re("S6", **change).state == "S4"
    assert re("S5", **change).state == "S4"
    assert re("S4", **change).state == "S4"
    assert re("S3", **change).state == "S3"     # S3 failed first, S4 follows anyway


def test_process_and_input_changes_together_go_to_s3():
    assert re("S6", process_file="f2", binding="b2").state == "S3"


def test_invalid_failed_state():
    with pytest.raises(ValueError):
        re("S7")


def test_changed_sources_names():
    after = dataclasses.replace(BASE, inputs={"source:S": "z", "template:T": "t"})
    assert changed_sources(BASE, after) == {"S", "T"}
    assert changed_sources(BASE, BASE) == set()


def test_snapshot_detects_each_kind_of_change(workspace):
    from fakes.workspace import write_file
    proc, _ = load_process(workspace.process_dir / "example.yml", workspace)
    binding = make_binding(proc, workspace, single_choices(find_candidates(proc, workspace)), {})
    paths = workspace.run("example-20240101-000000")

    def snap(b=binding):
        return snapshot(workspace, paths, ".process/example.yml", b)

    s0 = snap()
    assert s0.process_id == "example" and snap() == s0

    write_file(workspace.root, "identity/.snippet/step-notes.md",
               "---\nid: step-notes\nlayer: step\ntags: []\nauto: false\n---\nchanged\n")
    s1 = snap()
    assert s1.pools != s0.pools and s1.inputs == s0.inputs and s1.process_file == s0.process_file

    (workspace.root / ".input/example/source.md").write_text("changed\n")
    s2 = snap()
    assert s2.inputs != s1.inputs and changed_sources(s1, s2) == {"Source"}

    write_file(workspace.root, "templates/report.md", "# Other\n")
    assert changed_sources(s2, snap()) == {"Template"}

    paths.root.mkdir(parents=True)
    paths.overrides_json.write_text('{"retry": 3}')
    s3 = snap()
    assert s3.overrides != s2.overrides and s3.pools == s2.pools

    write_file(workspace.root, ".process/scripts/extract.py", "print('edited')\n")
    s3b = snap()
    assert s3b.scripts != s3.scripts and s3b.process_file == s3.process_file
    assert s3b.process_listing == s3.process_listing              # scripts are not process files
    s3 = s3b

    write_file(workspace.root, ".process/example.yml", "id: example\n# edited\n")
    s4 = snap()
    assert s4.process_file != s3.process_file and s4.process_id == ""

    s5 = snapshot(workspace, None, None, None)
    assert s5.process_file == "" and s5.overrides == "" and dict(s5.inputs) == {}


def _spec(hashes, edges, reads=None, entry="a", order=("a", "b", "c")):
    reads = reads or {}
    nodes = tuple(GraphNode(n, "cognitive", hashes[n], tuple(reads.get(n, ())), ()) for n in order)
    return GraphSpec(process_id="p", process_sha256="x", entry=entry, nodes=nodes,
                     edges=tuple(Edge(s, d, w, o, None) for s, d, w, o in edges), order=tuple(order))


EDGES = [("a", "b", "done", None), ("b", "c", "done", None), ("c", "END", "done", None)]
HASHES = {"a": "h1", "b": "h2", "c": "h3", "d": "h4"}


def test_earliest_changed_node_none_when_identical():
    assert earliest_changed_node(_spec(HASHES, EDGES), _spec(HASHES, EDGES), set()) is None


def test_earliest_by_step_hash():
    new = _spec({**HASHES, "b": "changed", "c": "changed"}, EDGES)
    assert earliest_changed_node(_spec(HASHES, EDGES), new, set()) == "b"


def test_earliest_by_edges():
    edges = [("a", "b", "done", None), ("b", "c", "done", None), ("c", "a", "done", None)]
    assert earliest_changed_node(_spec(HASHES, EDGES), _spec(HASHES, edges), set()) == "c"


def test_earliest_by_changed_input_read():
    reads = {"b": ["input.source[S]"], "c": ["input.ruleset[S]"]}
    old, new = _spec(HASHES, EDGES, reads), _spec(HASHES, EDGES, reads)
    assert earliest_changed_node(old, new, {"S"}) == "b"
    assert earliest_changed_node(old, new, {"Other"}) is None
    # a product read is not an input read
    reads2 = {"b": ["facts"]}
    assert earliest_changed_node(_spec(HASHES, EDGES, reads2), _spec(HASHES, EDGES, reads2), {"facts"}) is None


def test_earliest_new_node_and_entry_change():
    edges = EDGES[:2] + [("c", "d", "done", None), ("d", "END", "done", None)]
    new = _spec(HASHES, edges, order=("a", "b", "c", "d"))
    assert earliest_changed_node(_spec(HASHES, EDGES), new, set()) == "c"      # c's edges changed first
    new2 = _spec(HASHES, EDGES, entry="b", order=("b", "c"))
    assert earliest_changed_node(_spec(HASHES, EDGES), new2, set()) == "b"
