"""T9d / T9e / T1e (query half): nothing the sanitiser lets through is a control
sequence, and nothing it neutralises disappears."""
import re

from agent_sandbox.services.sanitize import has_control, sanitize

ESC = "\x1b"
_RAW_CONTROL = re.compile(r"[\x00-\x08\x0b-\x1f\x7f-\x9f]")


def test_post01_probe_marks_every_sequence():
    raw = "A\x1b]52;c;SGVsbG8=\x07B\x1b[6nC\x1b[2JD\rE\x07F"
    assert sanitize(raw) == "A␛]52;c;SGVsbG8=␇B␛[6nC␛[2JD␍E␇F"


def test_no_raw_control_byte_survives():
    raw = "".join(chr(i) for i in range(0, 0xA0)) + ESC + "[" + ESC
    out = sanitize(raw)
    assert not _RAW_CONTROL.search(out)
    assert ESC not in out


def test_both_string_terminators():
    bel = sanitize("x\x1b]0;title\x07y")
    st = sanitize("x\x1b]0;title\x1b\\y")
    assert bel == "x␛]0;title␇y"
    assert st == "x␛]0;title␛\\y"


def test_unterminated_osc_keeps_following_text_visible():
    assert sanitize("x\x1b]52;c;payload then text") == "x␛]52;c;payload then text"


def test_eight_bit_c1_introducers_shown_as_seven_bit():
    assert sanitize("\x9b2J\x9d0;t\x9c!") == "␛[2J␛]0;t␛\\!"


def test_fs_and_nf_forms():
    assert sanitize("\x1bc\x1b(B\x1b#8") == "␛c␛(B␛#8"
    assert sanitize(ESC) == "␛"


def test_osc8_link_target_becomes_visible():
    out = sanitize("\x1b]8;;https://evil.example\x1b\\docs\x1b]8;;\x1b\\")
    assert "https://evil.example" in out and ESC not in out


def test_long_sequence_body_is_clipped():
    out = sanitize("\x1b]52;c;" + "A" * 500 + "\x07")
    assert len(out) < 120 and out.startswith("␛]52;c;AAAA") and "more)" in out


def test_format_characters_marked_joiners_kept():
    assert sanitize("a‮b") == "a⟨U+202E⟩b"          # RLO
    assert sanitize("a​b") == "a⟨U+200B⟩b"          # ZWSP
    assert sanitize("\U000e0041") == "⟨U+E0041⟩"         # tag character
    assert sanitize("👨‍👩") == "👨‍👩"         # ZWJ stays


def test_newline_tab_kept_crlf_normalised():
    assert sanitize("a\r\nb\tc\rd\n") == "a\nb\tc␍d\n"


def test_has_control_for_reply_check():
    assert not has_control("summarise README.md")
    assert not has_control("tab\tseparated")
    assert has_control("\x1b[24;80Rsummarise")           # DSR reply prefix
    assert has_control("\x9b24;80R")
    assert has_control("a‮b")
