from agent_graph import config
from agent_graph.compose.normalize import join_blocks, normalize_block
from agent_graph.compose.snippets import render_facts
from agent_graph.compose.tokens import estimate_tokens, goal_tokens
from agent_graph.state import RunFact


def test_normalize_block_lf_trailing_ws_and_blank_edges():
    assert normalize_block("\r\n\r\nA  \r\nB\t\r\n\r\n") == "A\nB"
    assert normalize_block("") == ""
    assert normalize_block("  indented\nx") == "  indented\nx"


def test_join_blocks_one_blank_line_one_final_newline():
    assert join_blocks(["A\n\n\n", "", "B  \n"]) == "A\n\nB\n"
    assert join_blocks([]) == ""
    assert join_blocks(["", "  \n"]) == ""


def test_join_blocks_idempotent():
    once = join_blocks(["a  ", "b\r\nc"])
    assert join_blocks([once]) == once


def test_estimate_tokens_is_ceil_bytes_over_chars_per_token():
    assert estimate_tokens("") == 0
    assert estimate_tokens("a" * config.CHARS_PER_TOKEN) == 1
    assert estimate_tokens("a" * (config.CHARS_PER_TOKEN + 1)) == 2
    assert estimate_tokens("ä" * 4) == -(-8 // config.CHARS_PER_TOKEN)


def test_goal_tokens_lowercase_no_stopwords():
    g = goal_tokens("write-summary", 'WRITE(output.target[Report], "with the facts")')
    assert {"write", "summary", "output", "target", "report", "facts"} <= g
    assert "with" not in g and "the" not in g


def test_render_facts_small_keeps_all():
    facts = [RunFact(1, "answer", "n", "a"), RunFact(2, "hil_ok", None, "b\nc")]
    text, kept, omitted = render_facts(facts, 1000)
    assert kept == (1, 2) and omitted == ()
    assert text.startswith("# Run facts\n\n- #1 answer (n): a\n- #2 hil_ok: b\n  c\n")


def test_render_facts_none_is_empty():
    assert render_facts([], 100) == ("", (), ())


def test_render_facts_over_cap_keeps_newest_and_points_to_file():
    facts = [RunFact(i, "answer", None, f"f{i} " + "w" * 400) for i in range(1, 11)]
    text, kept, omitted = render_facts(facts, 300)
    assert kept and omitted
    assert kept == tuple(range(11 - len(kept), 11))             # newest suffix
    assert omitted == tuple(range(1, 1 + len(omitted)))         # oldest prefix
    assert len(kept) + len(omitted) == 10
    assert "omitted, see `../run_facts.jsonl`" in text
    assert "f10 " in text and "f1 " not in text
    assert estimate_tokens(text) <= 300 + 10
