// Content shown by the two diagram islands of this post, kept apart from the
// components so the text stays in one place and doesn't drift from the prose.
// Everything here is bundled at build time (no runtime fetches).
//
// B-n = boundaries/links in the block diagram, S-n = steps of one turn in the
// interaction diagram. Chips name the code symbol behind each point.

// Short labels for chips, keyed by id.
export const REF_SHORT = {
  'main.py': 'input() ⇄ interrupt()',
  'n06_exchange.py': 'exchange()',
  'gateway.py': 'send_chat_turn()',
  'render.py': 'TurnRenderer',
  'container.py': 'ExecSession(tty=False)',
  'config.py': 'CHAT_CMD',
  'claude': 'stream-json ⇄ stream-json',
  'API': 'Messages API',
  'cache': 'prompt cache',
  'history': 'conversation history',
  'auto': '--permission-mode auto',
};

// Block diagram: one entry per Badge id.
export const BLOCK_INFO = {
  'B-1': {
    name: 'Operator input',
    desc: 'Every question the harness asks is a LangGraph interrupt(); main.py answers it with a bare input() and resumes the graph with Command(resume=text). The text is the only thing that crosses here.',
    chips: ['main.py', 'n06_exchange.py'],
  },
  'B-2': {
    name: 'Rendered output',
    desc: 'render.py owns a rich Console built with THEME. Spinner, live tail, markdown, tool panels and the footer are printed straight to the terminal from inside the node — before the graph has even returned from the turn.',
    chips: ['render.py'],
  },
  'B-3': {
    name: 'Events → renderer',
    desc: 'send_chat_turn() parses each NDJSON line into a dict and calls TurnRenderer.feed(ev). The renderer dispatches on ev["type"]: stream_event, assistant, user, result; anything else (system, rate_limit_event) is ignored.',
    chips: ['gateway.py', 'render.py'],
  },
  'B-4': {
    name: 'Session read/write',
    desc: 'One ExecSession per container, kept in a module-level dict across graph passes and interrupts. write() sends one JSON user message; readline() returns one stdout line, None on timeout, "" on EOF.',
    chips: ['gateway.py', 'container.py'],
  },
  'B-5': {
    name: 'Exec socket (framed)',
    desc: 'exec_create(stdin=True, tty=False) + exec_start(socket=True). Without a TTY, Docker multiplexes stdout and stderr on one socket with 8-byte frame headers; _demux() unpacks them (">BxxxL") and keeps partial frames until the rest arrives.',
    chips: ['container.py'],
  },
  'B-6': {
    name: 'Process stdin/stdout',
    desc: 'The exec instance is a long-lived `claude -p` process: stdin is NDJSON user messages (--input-format stream-json), stdout is NDJSON events (--output-format stream-json --include-partial-messages). It stays alive until the harness closes stdin.',
    chips: ['config.py', 'claude'],
  },
  'B-7': {
    name: 'CLI ⇄ Anthropic API',
    desc: 'The CLI, not the harness, holds the conversation. Each turn it sends the same system prompt, the same tool definitions and the whole history so far, with cache_control breakpoints — so all but a handful of input tokens are prompt-cache reads.',
    chips: ['claude', 'API', 'cache', 'history'],
  },
};

// Interaction diagram: one entry per Badge id (S-n = step n of one turn).
export const STEP_INFO = {
  'S-1': {
    name: 'Operator reply resumes the graph',
    desc: 'The previous pass ended on interrupt("Your reply (or /quit):"). main.py collects the answer with input() and invokes the graph with Command(resume=text); exchange() runs again with turn == "agent" and picks the last user message out of state["transcript"].',
    chips: ['main.py', 'n06_exchange.py'],
  },
  'S-2': {
    name: 'One JSON line in',
    desc: 'send_chat_turn() writes {"type":"user","message":{"role":"user","content":text},...} plus "\\n" to the session socket. On the first pass start_chat() opened the session first; afterwards the same process is reused.',
    chips: ['gateway.py', 'container.py'],
  },
  'S-3': {
    name: 'CLI calls the API with the full history',
    desc: 'The CLI appends the message to its in-process conversation and sends system prompt + tools + every previous turn. Because that prefix is identical to the previous request plus an append, the API serves it from the prompt cache.',
    chips: ['claude', 'API', 'cache'],
  },
  'S-4': {
    name: 'stream_event deltas → live tail',
    desc: 'content_block_start(text) stops the spinner and starts a transient rich Live; each content_block_delta appends to the tail (last N lines, plain text, blue). content_block_stop stops the Live and prints the whole block once as themed Markdown.',
    chips: ['render.py'],
  },
  'S-5': {
    name: 'assistant(tool_use) → cyan panel',
    desc: 'The finished assistant message lists its content blocks. A tool_use block is printed as a Panel titled "tool · <name>" with the JSON input (clipped at 1500 chars), then the spinner says "running tool…".',
    chips: ['render.py'],
  },
  'S-6': {
    name: 'Tool runs inside the container',
    desc: 'With --permission-prompts none nobody can answer a prompt, so --permission-mode auto lets a classifier approve safe calls and deny the rest. Either way the tool executes inside the gVisor sandbox, never on the host.',
    chips: ['auto', 'claude'],
  },
  'S-7': {
    name: 'user(tool_result) → grey / red panel',
    desc: 'Tool results arrive as a "user" event. Each tool_result block becomes a Panel (grey, or red with "· error" when is_error is set), clipped at 1200 chars; the spinner returns to "thinking…" while the CLI calls the API again.',
    chips: ['render.py'],
  },
  'S-8': {
    name: 'result → footer, answer returned',
    desc: 'The result event ends the turn: the renderer prints "<secs>s · <num_turns> turn(s)" and send_chat_turn() returns ev["result"]. exchange() appends it to state["transcript"] as an agent message and flips turn to "user".',
    chips: ['render.py', 'gateway.py', 'n06_exchange.py'],
  },
  'S-9': {
    name: 'interrupt() for the next reply',
    desc: 'The next pass of exchange() runs ask_user() first — an interrupt() — so the graph pauses with the CLI process still alive and its history intact. /quit closes stdin, waits for EOF and drops the session.',
    chips: ['n06_exchange.py', 'gateway.py'],
  },
};
