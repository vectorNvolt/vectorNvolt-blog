"""Make agent-controlled text safe to print, and make what was neutralised visible.

Covers: SFR-AGG-038 (control sequences from Claude never reach the terminal raw).
Pure string functions; no terminal I/O.

Nothing is deleted. A control character becomes its Unicode control picture (ESC -> U+241B,
BEL -> U+2407), an 8-bit introducer becomes its 7-bit picture (0x9B -> ESC + "["), a format
character (bidi override, zero-width, tag block) becomes ``<U+202E>`` in angle brackets. The bytes of
a sequence stay on screen after the marked introducer, clipped if long, so the operator sees what was
attempted and the terminal sees only printable text. The control grammar is ECMA-48 / xterm ctlseqs:
a string sequence (OSC, DCS, SOS, PM, APC) ends at BEL or ST, the C1 bytes are the same introducers
in one byte, and ESC + one byte (``ESC c``) or ESC + intermediates + final are sequences too.
Newline and tab are kept; ``\\r\\n`` becomes ``\\n``.
"""

import unicodedata

_KEEP = {"\n", "\t"}

# Format characters (category Cf) that carry legitimate text: ZWNJ/ZWJ, the soft hyphen.
_CF_ALLOW = {"‌", "‍", "­"}

ESC = "\x1b"
_C1_INTRO = {"\x9b": "[", "\x9d": "]", "\x90": "P", "\x98": "X", "\x9e": "^", "\x9f": "_"}
_STRING_INTRO = set("]PX^_")  # ESC + one of these opens a string ended by BEL or ST
_ST = "\x9c"                  # 8-bit string terminator; 7-bit is ESC \

SEQ_SHOW_MAX = 48             # how much of a sequence body stays on screen after the marker


def _picture(ch: str) -> str:
    o = ord(ch)
    if o < 0x20:
        return chr(0x2400 + o)
    if o == 0x7F:
        return "␡"
    if ch in _C1_INTRO:
        return "␛" + _C1_INTRO[ch]
    if ch == _ST:
        return "␛\\"
    return f"⟨U+{o:04X}⟩"


def _is_control(ch: str) -> bool:
    o = ord(ch)
    if o < 0x20 or o == 0x7F or 0x80 <= o <= 0x9F:
        return ch not in _KEEP
    return unicodedata.category(ch) == "Cf" and ch not in _CF_ALLOW


def _seq_end(s: str, i: int) -> int:
    """``s[i]`` is ESC or a C1 introducer. Index just past the sequence it starts."""
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
                return j + 2 if j + 1 < n and s[j + 1] == "\\" else j
            j += 1
        return j

    if 0x20 <= ord(kind) <= 0x2F:                     # nF: intermediates then final
        while j < n and 0x20 <= ord(s[j]) <= 0x2F:
            j += 1
        return j + 1 if j < n and 0x30 <= ord(s[j]) <= 0x7E else j

    return j                                          # Fp/Fs/Fe: ESC + one byte


def _mark_chars(s: str) -> str:
    return "".join(_picture(c) if _is_control(c) else c for c in s)


def sanitize(text: str, *, seq_show_max: int = SEQ_SHOW_MAX) -> str:
    """``text`` with every control sequence and control/format character replaced by a visible marker."""
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
