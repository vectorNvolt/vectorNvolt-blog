"""SSR-AGG-006 history and run facts: durable, ordered, thread-safe."""

import threading

import pytest

from agent_graph import contract
from agent_graph.contract import Notice, Status
from agent_graph.errors import WriteBoundaryError
from agent_graph.services.history import History
from agent_graph.services.run_facts import RunFactsStore
from agent_graph.services.runfs import read_jsonl
from agent_graph.state import RunFact


def test_history_records_in_order(workspace, guard):
    paths = workspace.run("r1")
    h = History(paths.history_jsonl, guard)
    h.command(contract.start())
    h.status(Status("NODE_STARTED", "extract", {"context_ref": None}))
    h.notice(Notice("extract", "WARNING", "careful"))
    h.command(contract.hil_nok("review", "redo"))
    recs = h.records()
    assert [r["seq"] for r in recs] == [1, 2, 3, 4]
    assert [(r["dir"], r["name"], r["node"]) for r in recs] == [
        ("cmd", "START", None), ("status", "NODE_STARTED", "extract"),
        ("notice", "WARNING", None), ("cmd", "HIL_NOK", "review")]
    assert recs[2]["payload"] == {"source": "extract", "text": "careful"}
    assert recs[3]["payload"] == {"feedback": "redo"}
    assert all(isinstance(r["ts"], str) and r["ts"] for r in recs)


def test_history_continues_seq_after_reopen(workspace, guard):
    p = workspace.run("r1").history_jsonl
    History(p, guard).command(contract.start())
    h2 = History(p, guard)
    h2.command(contract.suspend("x"))
    assert [r["seq"] for r in h2.records()] == [1, 2]


def test_history_is_thread_safe(workspace, guard):
    h = History(workspace.run("r1").history_jsonl, guard)

    def work(i):
        for j in range(10):
            h.command(contract.answer(f"n{i}", f"t{j}"))

    threads = [threading.Thread(target=work, args=(i,)) for i in range(8)]
    for t in threads:
        t.start()
    for t in threads:
        t.join()
    recs = h.records()
    assert sorted(r["seq"] for r in recs) == list(range(1, 81))
    assert [r["seq"] for r in recs] == sorted(r["seq"] for r in recs)    # file order == seq order


def test_history_respects_write_guard(workspace, guard, tmp_path):
    with pytest.raises(WriteBoundaryError):
        History(tmp_path / "outside.jsonl", guard).command(contract.start())


def test_run_facts(workspace, guard):
    store = RunFactsStore(workspace.run("r1").run_facts_jsonl, guard)
    assert store.all() == []
    f1 = store.append("answer", "write-summary", "use bullets")
    f2 = store.append("hil_nok", "review", "too long")
    store.append("resolution", None, "fixed the process")
    assert f1 == RunFact(1, "answer", "write-summary", "use bullets")
    assert f2.seq == 2
    got = RunFactsStore(workspace.run("r1").run_facts_jsonl, guard).all()     # new instance reads the file
    assert [(f.seq, f.kind, f.node) for f in got] == [
        (1, "answer", "write-summary"), (2, "hil_nok", "review"), (3, "resolution", None)]
    assert got[1].text == "too long"
    assert len(read_jsonl(workspace.run("r1").run_facts_jsonl)) == 3
    with pytest.raises(ValueError):
        store.append("gossip", None, "x")
