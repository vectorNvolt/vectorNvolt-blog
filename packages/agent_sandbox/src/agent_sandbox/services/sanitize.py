"""Make agent-controlled text safe to print, and make what was neutralised visible.

Every string the CLI sends (model text, tool input, tool results, tool names, the
result footer, stderr) reaches the operator's terminal through Rich, and Rich only
strips five C0 bytes. Everything else — ESC-introduced sequences, the 8-bit C1
controls, bidi overrides — goes to the terminal emulator as-is. This module is the
single choke point in front of Rich.

Design: a small state machine over the terminal control grammar (ECMA-48 / xterm
ctlseqs), not a regex denylist. The grammar matters where a denylist fails:

  * a string sequence (OSC, DCS, SOS, PM, APC) ends at BEL *or* ST (`ESC \\`), so
    a regex that knows only one terminator leaves the other half open;
  * the 8-bit C1 bytes 0x9B / 0x9D / 0x90 / 0x98 / 0x9E / 0x9F / 0x9C are the same
    introducers and terminator in one byte;
  * `ESC` + one byte (Fs/Fp: `ESC c` resets the terminal) and `ESC` + intermediates
    + final (nF: `ESC ( B`) are sequences with no `[` or `]` in them at all.

Nothing is deleted. A control character becomes its Unicode control picture
(`\\x1b` → `␛`, `\\x07` → `␇`), an 8-bit introducer becomes its 7-bit picture
(0x9B → `␛[`), a format character becomes `⟨U+202E⟩`. The bytes of a sequence stay
on screen after the marked introducer — `␛]52;c;SGVsbG8=␛\\` — clipped if long, so
the operator sees *what* was attempted, and the terminal sees only printable text.
"""
from __future__ import annotations

import unicodedata

# --- what is kept ------------------------------------------------------------
_KEEP = {"\n", "\t"}

# Format characters (category Cf) that carry legitimate text: ZWNJ/ZWJ join emoji
# and Indic/Persian script, the soft hyphen is a layout hint. Every other Cf is
# marked: the bidi controls (U+202A–E, U+2066–9, U+200E/F, U+061C), zero-width
# space / joiner, BOM, interlinear annotation and the Unicode tag block.
_CF_ALLOW = {"‌", "‍", "­"}

# --- the control grammar ------------------------------------------------------
ESC = "\x1b"
_C1_INTRO = {                 # 8-bit C1 introducers → their 7-bit spelling
    "\x9b": "[",              # CSI
    "\x9d": "]",              # OSC
    "\x90": "P",              # DCS
    "\x98": "X",              # SOS
    "\x9e": "^",              # PM
    "\x9f": "_",              # APC
}
_STRING_INTRO = set("]PX^_")  # ESC + one of these opens a string ended by BEL or ST
_ST = "\x9c"                  # 8-bit string terminator; 7-bit is ESC \\

# How much of a sequence body stays on screen after the marker.
SEQ_SHOW_MAX = 48


def _picture(ch: str) -> str:
    """The visible stand-in for one control or format character."""
    o = ord(ch)
    if o < 0x20:
        return chr(0x2400 + o)       # U+2400 block: ␀ … ␟, ␛ for ESC
    if o == 0x7F:
        return "␡"              # ␡
    if ch in _C1_INTRO:
        return "␛" + _C1_INTRO[ch]
    if ch == _ST:
        return "␛\\"
    return f"⟨U+{o:04X}⟩"  # ⟨U+009B⟩, ⟨U+202E⟩


def _is_control(ch: str) -> bool:
    o = ord(ch)
    if o < 0x20 or o == 0x7F or 0x80 <= o <= 0x9F:
        return ch not in _KEEP
    return unicodedata.category(ch) == "Cf" and ch not in _CF_ALLOW


def _seq_end(s: str, i: int) -> int:
    """`s[i]` is ESC or a C1 introducer. Return the index just past the sequence
    it starts; `i + 1` if the ESC is alone at the end of the text."""
    n = len(s)
    ch = s[i]
    if ch == ESC:
        if i + 1 >= n:
            return i + 1
        kind = s[i + 1]
        j = i + 2
    else:
        kind = _C1_INTRO[ch]
        j = i + 1

    if kind == "[":                                   # CSI: params, intermediates, final
        while j < n and 0x30 <= ord(s[j]) <= 0x3F:
            j += 1
        while j < n and 0x20 <= ord(s[j]) <= 0x2F:
            j += 1
        return j + 1 if j < n and 0x40 <= ord(s[j]) <= 0x7E else j

    if kind in _STRING_INTRO:                         # OSC/DCS/SOS/PM/APC: to BEL or ST
        while j < n:
            c = s[j]
            if c == "\x07" or c == _ST:
                return j + 1
            if c == ESC:
                return j + 2 if j + 1 < n and s[j + 1] == "\\" else j   # ST, or a new ESC
            j += 1
        return j                                      # unterminated: runs to the end

    if 0x20 <= ord(kind) <= 0x2F:                     # nF: intermediates then final
        while j < n and 0x20 <= ord(s[j]) <= 0x2F:
            j += 1
        return j + 1 if j < n and 0x30 <= ord(s[j]) <= 0x7E else j

    return j                                          # Fp/Fs/Fe: ESC + one byte


def _mark_chars(s: str) -> str:
    return "".join(_picture(c) if _is_control(c) else c for c in s)


def sanitize(text: str, *, seq_show_max: int = SEQ_SHOW_MAX) -> str:
    """Return `text` with every control sequence and control/format character
    replaced by a visible marker. `\\n` and `\\t` are kept; `\\r\\n` becomes `\\n`."""
    if not text:
        return ""
    text = text.replace("\r\n", "\n")
    out: list[str] = []
    i, n = 0, len(text)
    while i < n:
        ch = text[i]
        if ch == ESC or ch in _C1_INTRO:
            end = _seq_end(text, i)
            body = _mark_chars(text[i + 1:end])
            if len(body) > seq_show_max:
                body = body[:seq_show_max] + f"…({len(body) - seq_show_max} more)"
            out.append(_picture(ch) + body)
            i = end
        elif _is_control(ch):
            out.append(_picture(ch))
            i += 1
        else:
            out.append(ch)
            i += 1
    return "".join(out)


def has_control(text: str) -> bool:
    """True if `text` holds anything `sanitize` would change (used to reject
    operator replies that carry a terminal's answer to a query)."""
    return any(_is_control(c) for c in text)
