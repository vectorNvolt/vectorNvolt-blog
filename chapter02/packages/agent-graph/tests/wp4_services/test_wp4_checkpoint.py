"""SQLite checkpoint saver and node journal (SSR-AGG-006, SSR-AGG-009, SFR-AGG-037)."""

import operator
import sqlite3
from typing import Annotated, TypedDict

import pytest
from langgraph.graph import END, START, StateGraph
from langgraph.types import Command, interrupt

from agent_graph.services.checkpoint import (
    SqliteCheckpointSaver,
    open_checkpointer,
    open_journal,
)

CFG = {"configurable": {"thread_id": "dyn-1"}}


class S(TypedDict, total=False):
    answer: str
    log: Annotated[list[str], operator.add]


def node_a(state: S) -> S:
    ans = interrupt("what is the answer?")
    return {"answer": ans, "log": ["a"]}


def node_b(state: S) -> S:
    return {"log": ["b"]}


def build(saver):
    g = StateGraph(S)
    g.add_node("a", node_a)
    g.add_node("b", node_b)
    g.add_edge(START, "a")
    g.add_edge("a", "b")
    g.add_edge("b", END)
    return g.compile(checkpointer=saver)


def test_wal_mode_and_tables(tmp_path):
    db = tmp_path / "run" / "checkpoint.sqlite"
    saver = open_checkpointer(db)
    saver.close()
    conn = sqlite3.connect(db)
    assert conn.execute("PRAGMA journal_mode").fetchone()[0].lower() == "wal"
    tables = {r[0] for r in conn.execute("SELECT name FROM sqlite_master WHERE type='table'")}
    conn.close()
    assert {"checkpoints", "blobs", "writes", "node_journal"} <= tables


def test_two_node_graph_interrupt_reopen_resume(tmp_path):
    db = tmp_path / "checkpoint.sqlite"

    saver = open_checkpointer(db)
    graph = build(saver)
    graph.invoke({"log": []}, CFG)
    state = graph.get_state(CFG)
    assert state.next == ("a",)
    assert state.tasks[0].interrupts[0].value == "what is the answer?"
    saver.close()                                   # "the process dies"

    saver2 = open_checkpointer(db)                  # a new process opens the same file
    graph2 = build(saver2)
    state2 = graph2.get_state(CFG)
    assert state2.next == ("a",)
    assert state2.tasks[0].interrupts[0].value == "what is the answer?"
    out = graph2.invoke(Command(resume="yes"), CFG)
    assert out["answer"] == "yes"
    assert out["log"] == ["a", "b"]
    assert graph2.get_state(CFG).next == ()
    saver2.close()

    saver3 = open_checkpointer(db)                  # finished state survives another reopen
    assert build(saver3).get_state(CFG).values["answer"] == "yes"
    saver3.close()


def test_get_tuple_list_and_delete(tmp_path):
    saver = open_checkpointer(tmp_path / "c.sqlite")
    graph = build(saver)
    graph.invoke({"log": []}, CFG)
    graph.invoke(Command(resume="x"), CFG)

    latest = saver.get_tuple(CFG)
    assert latest is not None
    assert latest.checkpoint["channel_values"]["answer"] == "x"
    assert latest.parent_config is not None

    listed = list(saver.list(CFG))
    ids = [t.config["configurable"]["checkpoint_id"] for t in listed]
    assert len(ids) >= 3 and ids == sorted(ids, reverse=True)
    assert ids[0] == latest.config["configurable"]["checkpoint_id"]
    assert len(list(saver.list(CFG, limit=2))) == 2
    older = list(saver.list(CFG, before=listed[0].config))
    assert [t.config["configurable"]["checkpoint_id"] for t in older] == ids[1:]

    by_id = saver.get_tuple(listed[-1].config)
    assert by_id is not None and by_id.parent_config is None

    assert saver.get_tuple({"configurable": {"thread_id": "other"}}) is None
    saver.delete_thread("dyn-1")
    assert saver.get_tuple(CFG) is None
    assert list(saver.list(CFG)) == []
    saver.close()


def test_saver_is_a_base_checkpoint_saver(tmp_path):
    from langgraph.checkpoint.base import BaseCheckpointSaver
    with SqliteCheckpointSaver(tmp_path / "c.sqlite") as saver:
        assert isinstance(saver, BaseCheckpointSaver)
        with pytest.raises(NotImplementedError):
            import asyncio
            asyncio.run(saver.aget_tuple(CFG))


def test_journal_put_get_upsert_and_threads(tmp_path):
    db = tmp_path / "c.sqlite"
    j = open_journal(db, "dyn-1")
    assert j.get("n", 1, "turn") is None
    j.put("n", 1, "turn", {"v": 1})
    j.put("n", 1, "turn", {"v": 2})
    j.put("n", 2, "turn", {"v": 3})
    assert j.get("n", 1, "turn") == {"v": 2}
    assert j.get("n", 2, "turn") == {"v": 3}
    other = open_journal(db, "dyn-2")
    assert other.get("n", 1, "turn") is None
    j.close()
    other.close()


def test_journal_session_survives_reopen_next_to_a_checkpointer(tmp_path):
    db = tmp_path / "c.sqlite"
    saver = open_checkpointer(db)
    j = open_journal(db, "dyn-1")
    assert j.session("n", 1) is None
    j.set_session("n", 1, "sess-abc")
    graph = build(saver)
    graph.invoke({"log": []}, CFG)                  # both connections write the same WAL database
    j.close()
    saver.close()

    j2 = open_journal(db, "dyn-1")
    assert j2.session("n", 1) == "sess-abc"
    assert j2.session("n", 2) is None
    j2.close()
