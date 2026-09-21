// Content shown by the two diagram islands of this post, kept apart from the
// components so the text stays in one place and doesn't drift from the prose.
// Everything here is bundled at build time (no runtime fetches).
//
// Block diagram (blocks.entry.jsx), two overlays on the same drawing:
//   V-n = interpreters on the host where sandbox bytes become instructions
//   C-n = choke points where the fix goes
// Interaction diagram (flow.entry.jsx), two variants of one poisoned turn:
//   A-n = the attack as the post-01 code runs it
//   F-n = the same turn with the choke points in place
//
// Chips: `tc` → danger (threat cases from the chapter 02 inventory),
// `code` → info (the file/symbol involved), `fix` → safe (the mitigation).

export const REF_SHORT = {
  // threat cases
  'T1e': 'terminal query → stdin',
  'T7e': 'harness exhaustion',
  'T9a': 'envelope drives state',
  'T9d': 'terminal escape injection',
  'T9e': 'presentation spoofing',
  'T5b': 'writable rootfs · claude on PATH',
  // code
  'main.py': 'input()',
  'n06_exchange.py': 'exchange()',
  'gateway.py': 'send_chat_turn()',
  'render.py': 'TurnRenderer',
  'container.py': 'ExecSession.readline()',
  'rich': 'Text · Markdown · Panel',
  'markdown-it': 'markdown-it-py',
  'pygments': 'Pygments lexers',
  'claude': 'claude -p · stream-json',
  'terminal': 'VT parser',
  // fixes
  'sanitize()': 'ESC/C0/C1 → visible marker',
  'escape()': 'rich.markup.escape()',
  'hyperlinks=False': 'links show their URL',
  'caps': 'block size · head+tail clip',
  'schema': 'known type · typed fields',
  'RecursionError': 'guarded json.loads',
  'line cap': 'max line length',
  'drain': 'drain + reset on timeout',
  'tcflush': 'tcflush(TCIFLUSH) before input()',
  'reply check': 'reject ESC/C1 in replies',
  'reported': 'metadata shown as reported',
};

// --- block diagram, threat overlay -------------------------------------------

export const THREAT_INFO = {
  'V-1': {
    name: 'The operator',
    desc: 'The last interpreter. What the operator believes happened is what the terminal painted: a panel drawn with box characters by the model, a link whose target is hidden, a red "· error" the CLI chose to set — all read as evidence for the next thing they type.',
    tc: ['T9e'],
    code: [],
  },
  'V-2': {
    name: 'Terminal emulator (VT parser)',
    desc: 'Every byte the Console writes is parsed for ESC-introduced sequences: CSI (cursor, clear screen), OSC (title, OSC 8 links, OSC 52 clipboard, OSC 7 cwd), DCS/APC, and 8-bit C1. Query sequences (DSR, DA, OSC 52 "?") are answered on the pty input side — the operator never sees the reply.',
    tc: ['T9d', 'T1e', 'T9e'],
    code: ['terminal'],
  },
  'V-3': {
    name: 'input() reads the pty queue',
    desc: 'Whatever the terminal answered during the turn is waiting in the pty input queue when main.py calls input(). A DSR reply has no newline, so it becomes the invisible prefix of the operator\'s next line — and Command(resume=…) carries it to the model.',
    tc: ['T1e'],
    code: ['main.py'],
  },
  'V-4': {
    name: 'Rich renders sandbox strings',
    desc: 'Text.__init__ strips only BEL/BS/VT/FF/CR; ESC passes. Markdown emits OSC 8 hyperlinks with the URL hidden, and runs markdown-it-py plus a Pygments lexer chosen by the fenced block\'s language tag. The tool name is interpolated into a markup string, so "[link=…]" in it is markup, not text.',
    tc: ['T9d', 'T9e', 'T7e'],
    code: ['render.py', 'rich', 'markdown-it', 'pygments'],
  },
  'V-5': {
    name: 'json.loads + type dispatch',
    desc: 'send_chat_turn() parses each line and ends the turn on the first {"type":"result"}. A deeply nested line raises RecursionError, which the except clause does not catch — the harness dies. The envelope is unvalidated: any shape is dispatched, any field is trusted.',
    tc: ['T7e', 'T9a'],
    code: ['gateway.py'],
  },
  'V-6': {
    name: 'readline() buffer',
    desc: 'ExecSession.readline() appends to _stdout until a newline arrives; a line that never ends grows it without bound. After CHAT_TURN_TIMEOUT the session stays open with events queued, and the next turn reads those first — the previous answer, attributed to the new question.',
    tc: ['T7e', 'T9a'],
    code: ['container.py', 'gateway.py'],
  },
  'V-7': {
    name: 'Untrusted source',
    desc: 'The events come from a claude binary on a writable rootfs, resolved on PATH inside the container. Model text is steered by whatever it read; tool results are file contents and command output; the envelope — type, is_error, duration_ms, tool name — is whatever the process chose to emit.',
    tc: ['T9a', 'T5b'],
    code: ['claude'],
  },
};

// --- block diagram, fix overlay ----------------------------------------------

export const CHOKE_INFO = {
  'C-1': {
    name: 'IP-1 ingress — before input()',
    desc: 'main.py flushes the pty input queue (termios.tcflush(fd, TCIFLUSH)) right before prompting, so nothing the terminal answered during the turn can prefix the operator\'s line, and rejects a reply that still contains ESC or C1 bytes.',
    fix: ['tcflush', 'reply check'],
    code: ['main.py'],
  },
  'C-2': {
    name: 'IP-1 egress — one sanitize() for every string',
    desc: 'Every string extracted from an event passes through sanitize() before Rich sees it: ESC-introduced sequences (CSI, OSC to BEL or ST, DCS/SOS/PM/APC, 2- and 3-byte ESC forms), C0 except \\n and \\t, and C1 are replaced by a visible marker; bidi and zero-width characters are marked. Tool names go through rich.markup.escape(); Markdown runs with hyperlinks=False; text blocks are capped and tool I/O is clipped head and tail.',
    fix: ['sanitize()', 'escape()', 'hyperlinks=False', 'caps'],
    code: ['render.py'],
  },
  'C-3': {
    name: 'IP-3 ingress — the event schema',
    desc: 'send_chat_turn() accepts only known event types with the expected field types and drops everything else; json.loads is guarded against RecursionError as well as JSONDecodeError. The shape of the envelope is now the harness\'s decision, not the sandbox\'s.',
    fix: ['schema', 'RecursionError'],
    code: ['gateway.py'],
  },
  'C-4': {
    name: 'IP-3 ingress — bounded reads',
    desc: 'readline() enforces a maximum line length and closes the session when it is exceeded. After a turn timeout the queue is drained and the session reset, so the next turn never reads the previous one\'s events.',
    fix: ['line cap', 'drain'],
    code: ['container.py', 'gateway.py'],
  },
  'C-5': {
    name: 'Envelope trust — reported, not verified',
    desc: 'is_error, duration_ms, num_turns and usage are rendered as what the CLI reported; no host-side state changes on them beyond "the turn ended". The truth half of T9a cannot be closed on this channel — only kept from driving decisions.',
    fix: ['reported'],
    code: ['render.py', 'n06_exchange.py'],
  },
  'C-6': {
    name: 'Unchanged: the terminal still obeys',
    desc: 'Nothing here hardens the emulator — it will execute whatever reaches it. The fix is that only printable bytes reach it. The shared canvas remains: the operator\'s prompt and the agent\'s output are still one surface, which is what the dashboard changes.',
    fix: [],
    code: ['terminal'],
  },
};

// --- interaction diagram, attack variant -------------------------------------

export const ATTACK_INFO = {
  'A-1': {
    name: 'An ordinary task',
    desc: 'The operator asks the agent to summarise a README. The model is not compromised and nothing is injected into the prompt — the payload is in a file the agent will legitimately read.',
    tc: [],
    code: ['main.py'],
  },
  'A-2': {
    name: 'The tool reads the poisoned file',
    desc: 'Read returns the file\'s bytes: a paragraph of text, an OSC 52 clipboard write terminated with ST (ESC \\), a DSR cursor query (ESC [ 6 n), and a Markdown link whose text says "docs" and whose target does not.',
    tc: ['T9d', 'T1e', 'T9e'],
    code: ['claude'],
  },
  'A-3': {
    name: 'tool_result carries the raw bytes',
    desc: 'The CLI wraps the content in a "user" event with a tool_result block. Nothing on the way out of the sandbox looks at the bytes — and nothing should have to; the sandbox\'s job is where the tool ran, not what it returned.',
    tc: [],
    code: ['claude', 'gateway.py'],
  },
  'A-4': {
    name: 'Panel(Text(body)) — ESC survives',
    desc: '_tool_results() clips the body and wraps it in Text. Rich strips BEL, BS, VT, FF and CR; the ESC byte and everything it introduces are written to the terminal unchanged.',
    tc: ['T9d'],
    code: ['render.py', 'rich'],
  },
  'A-5': {
    name: 'The terminal executes',
    desc: 'OSC 52 writes the clipboard. DSR is answered: the terminal writes ESC [ <row> ; <col> R to the pty input side, where it waits. No terminal shows either.',
    tc: ['T9d', 'T1e'],
    code: ['terminal'],
  },
  'A-6': {
    name: 'A clean-looking panel',
    desc: 'The operator sees a grey "tool result" panel with the README\'s prose. The sequences are invisible by construction — that is what escape sequences are for.',
    tc: ['T9e'],
    code: [],
  },
  'A-7': {
    name: 'Markdown → OSC 8, target hidden',
    desc: 'The model\'s summary repeats the link. Markdown(hyperlinks=True) renders "docs" underlined with the URL inside an OSC 8 sequence; the terminal shows the text, the target is a hover away in a good emulator and invisible in the rest.',
    tc: ['T9e'],
    code: ['render.py', 'rich'],
  },
  'A-8': {
    name: 'result · is_error = false',
    desc: 'The footer says 4.2s · 2 turn(s). Both numbers, and the absence of "· error", are claims by the CLI process; the renderer prints them as facts.',
    tc: ['T9a'],
    code: ['render.py'],
  },
  'A-9': {
    name: 'The reply prefixes the operator\'s line',
    desc: 'exchange() interrupts; main.py calls input(). The DSR reply is already in the pty queue and has no newline, so the line input() returns is ESC [ 24 ; 80 R followed by whatever the operator typed.',
    tc: ['T1e'],
    code: ['main.py', 'n06_exchange.py'],
  },
  'A-10': {
    name: 'Host → sandbox',
    desc: 'Command(resume=…) stores the line in transcript; send_chat_turn() sends it as the next user message; the model reads it. Cursor position is a proof of channel — an OSC 52 read, where the emulator allows it, sends the clipboard down the same path, and no egress policy ever sees it.',
    tc: ['T1e'],
    code: ['gateway.py', 'claude'],
  },
};

// --- interaction diagram, fix variant ----------------------------------------

export const FIX_INFO = {
  'F-1': {
    name: 'Schema and bounds at IP-3',
    desc: 'The line is under the length cap; json.loads is guarded; the event is a known type with a tool_result block whose content is a string or a list of text blocks. Anything else would be dropped here, before the renderer.',
    fix: ['line cap', 'schema', 'RecursionError'],
    code: ['gateway.py', 'container.py'],
  },
  'F-2': {
    name: 'sanitize() before Rich',
    desc: 'The body goes through the sequence parser: the OSC 52 sequence, whichever terminator it uses, and the DSR query become visible markers (␛]52;c;…␛\\ and ␛[6n); C1 bytes and bidi controls are marked the same way. The tool name is markup-escaped before it is interpolated into the title.',
    fix: ['sanitize()', 'escape()'],
    code: ['render.py'],
  },
  'F-3': {
    name: 'The attempt is visible',
    desc: 'The panel shows the README with the sequences printed as glyphs. The terminal receives only printable characters and has nothing to execute or answer; the operator sees that the file tried something.',
    fix: ['sanitize()'],
    code: ['terminal'],
  },
  'F-4': {
    name: 'Links show their URL',
    desc: 'Markdown(hyperlinks=False) renders the summary\'s link as "docs (https://…)": the target is on screen, not in an escape sequence.',
    fix: ['hyperlinks=False'],
    code: ['render.py'],
  },
  'F-5': {
    name: 'Footer as reported',
    desc: 'The footer still prints duration and turn count, labelled as reported by the CLI. is_error changes a colour, never a decision.',
    fix: ['reported'],
    code: ['render.py'],
  },
  'F-6': {
    name: 'tcflush before input()',
    desc: 'main.py empties the pty input queue immediately before prompting. Had a query reached the terminal anyway, its reply would be discarded here instead of prefixing the operator\'s line.',
    fix: ['tcflush'],
    code: ['main.py'],
  },
  'F-7': {
    name: 'Only the operator\'s text goes back',
    desc: 'The reply is checked for ESC and C1 bytes and then sent as the next user message. The model receives what the operator typed — the host → sandbox path needs both a query that reaches the terminal and a reply that reaches input(), and neither survives.',
    fix: ['reply check'],
    code: ['main.py', 'gateway.py'],
  },
};
