// One ngspice run, drawn twice by the generic components:
//   `block` / `blockText` → BlockDiagram.jsx (hand laid out, viewBox units):
//                           who decides what around one run, top to bottom
//   `seq`   / `seqText`   → SequenceDiagram.jsx (lays itself out): one pass
//                           through the boost-design process
// Geometry and structure first, every word the panels show second. Chip
// labels for the `reqs` ids (HW-BST-01..08) come from REF_SHORT in ../data.js.
// Block diagram: the numbered circle sits on the connector that leaves each
// box and carries that box's text. 1 process graph, 2 model, 3 allow-list,
// 4 validate, 5 stage inputs, 6 create container, 7 container, 8 exec dir
// (dashed back to the model), plus the dashed outcome link model → graph.

export const block = {
  viewH: 800,
  outlines: [
    { x: 28, y: 16, w: 904, h: 116, label: 'Process graph · deterministic' },
    { x: 28, y: 150, w: 904, h: 130, label: 'Agent step · Claude CLI, model decides inside the step', variant: 'danger' },
    { x: 28, y: 314, w: 904, h: 162, label: 'Host · run_ephemeral_container tool', variant: 'info' },
    { x: 28, y: 510, w: 904, h: 130, label: 'Disposable container · uid 1001, network none', variant: 'safe' },
  ],
  nodes: {
    graph: {
      x: 200,
      y: 44,
      w: 560,
      h: 76,
      label: 'Process graph boost-design',
      sub: ['per step: claude -p, prompt and --allowedTools', 'routes on the outcome the model writes:', 'simulate ok → evaluate, tool_error → netlist, fail → calc'],
    },
    allow: {
      x: 48,
      y: 190,
      w: 400,
      h: 76,
      label: 'Bash allow-list (--allowedTools)',
      sub: ['simulate: run_ephemeral_container tool', 'ngspice:* and readraw.py:*'],
    },
    model: {
      x: 512,
      y: 190,
      w: 392,
      h: 76,
      label: 'Model (Claude CLI)',
      sub: ['sizing, netlist text, exec id,', 'runs the tool, reads results, outcome'],
      variant: 'solid-danger',
    },
    validate: {
      x: 48,
      y: 352,
      w: 250,
      h: 112,
      label: 'Validate',
      sub: ['profile.yml and input names', 'spice-safe: no .control shell,', 'source or codemodel', 'no unlisted .include/.lib', 'exec id is new'],
      variant: 'solid-dark',
    },
    stage: {
      x: 355,
      y: 352,
      w: 250,
      h: 112,
      label: 'Stage inputs',
      sub: ['src/ in place, others', 'copied to in/<name>', 'sha256 into meta.json'],
      variant: 'solid-dark',
    },
    create: {
      x: 662,
      y: 352,
      w: 250,
      h: 112,
      label: 'Create container',
      sub: ['fixed settings', 'verified by inspect', 'before start'],
      variant: 'solid-dark',
    },
    container: {
      x: 48,
      y: 548,
      w: 560,
      h: 80,
      label: 'ngspice-run wrapper',
      sub: [
        'run 1 without -r for .meas → sim.log; run 2 with -r → sim.raw',
        'mounts: src ro, in ro, out rw, tmpfs /work',
        'limits: timeout, out cap, memory',
      ],
    },
    exec: {
      x: 48,
      y: 690,
      w: 560,
      h: 80,
      label: 'Exec dir .output/exec/<id>',
      sub: ['stdout.log, stderr.log, meta.json, result.json (exit, cause)', 'out/sim.log, out/sim.raw'],
    },
  },
  links: [{ points: [[700, 190], [700, 120]], dir: 'end', dashed: true }],
  ips: [
    { id: 'IP-1', label: '1', points: [[600, 120], [600, 190]], dir: 'end', at: [600, 141] },
    { id: 'IP-2', label: '2', points: [[512, 228], [448, 228]], dir: 'end', at: [480, 228] },
    { id: 'IP-3', label: '3', points: [[173, 266], [173, 352]], dir: 'end', at: [173, 297] },
    { id: 'IP-4', label: '4', points: [[298, 408], [355, 408]], dir: 'end', at: [326, 408] },
    { id: 'IP-5', label: '5', points: [[605, 408], [662, 408]], dir: 'end', at: [633, 408] },
    { id: 'IP-6', label: '6', points: [[787, 464], [787, 493], [328, 493], [328, 548]], dir: 'end', at: [560, 493] },
    { id: 'IP-7', label: '7', points: [[328, 628], [328, 690]], dir: 'end', at: [328, 659] },
    { id: 'IP-8', label: '8', points: [[608, 730], [946, 730], [946, 228], [904, 228]], dir: 'end', dashed: true, at: [946, 480] },
  ],
  assets: [],
  threats: [],
};

export const blockText = {
  label:
    'Block diagram of one ngspice run, top to bottom: the deterministic process graph starts the Claude CLI for each step with an allowed tool list; the model calls the host tool run_ephemeral_container through its Bash tool; the host validates, stages inputs and creates a disposable container with no network; the container writes an exec directory that the model reads back, and the model hands its outcome back to the graph',
  hint: 'Each circle sits on the arrow leaving a box; hover or click one to see what that box does and who decides there. Red: the model, which is not trusted. Green: the container the simulator runs in. The dashed arrows are the model reading the results and the outcome it hands back to the graph.',
  items: {
    'IP-1': {
      name: 'Process graph boost-design',
      desc: 'The graph fixes the steps (calc, netlist, simulate, evaluate, report). For each step it starts one claude -p process with the step prompt and the step’s tool list as --allowedTools, then waits for it to exit. It reads the outcome the model wrote, checks that the step declares it, and follows the matching edge: simulate ok goes to evaluate, tool_error goes back to netlist, evaluate fail goes back to calc. The graph never runs the simulator and never reads the exec directory.',
      code: '.process/boost-design.yml; agent-graph: services/claude_cli.py, dynamic/step_cognitive.py, dynamic/router.py (select_edge)',
    },
    'IP-2': {
      name: 'Model (Claude CLI)',
      desc: 'Inside a step the model sizes L and C, writes the netlist text, picks the exec id, runs the tool through its Bash tool, reads result.json and sim.log, and writes the step outcome (the dashed arrow up). Nothing in the code checks those numbers: the graph only checks that the outcome is a declared one, and the host only checks what the model hands to the tool.',
      code: '.process/boost-design.yml (the step prompts)',
    },
    'IP-3': {
      name: 'Bash allow-list',
      desc: 'The CLI gets the step’s tool list as --allowedTools. In simulate the only Bash commands on it are run_ephemeral_container tool ngspice:* and python3 /workspace/src/ngspice/readraw.py:*; evaluate gets no Bash at all. How strictly the CLI enforces the list is not tested here; the list is read from the code.',
      code: '.process/boost-design.yml (tools); agent-graph: services/claude_cli.py (--allowedTools)',
    },
    'IP-4': {
      name: 'Validate',
      desc: 'The host reads the tool profile, checks that the exec id is new, checks the input names and expands argv from the profile, then applies the spice-safe policy: no .control shell, source or codemodel, and no .include or .lib that is not listed. A refusal happens before any exec directory exists.',
      code: 'internal/sequence/sequence.go (validate, validateInputs, buildArgv); internal/profile/tool.go (ExpandToolArgv; validateTool at profile load); internal/inputpolicy (spice-safe)',
    },
    'IP-5': {
      name: 'Stage inputs',
      desc: 'Inputs under src/ are mounted in place (read-only at /workspace/<rel>) and only hashed. Other inputs are copied to in/<name> in the exec directory (in/<name>/<base> when several are given). The sha256 of each input is written into meta.json.',
      code: 'internal/sequence/sequence.go (stage)',
    },
    'IP-6': {
      name: 'Create container',
      desc: 'The host builds the container from fixed settings the model cannot change and verifies them by inspect before the container starts. The user is uid 1001 and the network is none.',
      code: 'internal/kind/kind.go (NewPlan, Verify, Mounts)',
    },
    'IP-7': {
      name: 'ngspice-run wrapper',
      desc: 'The wrapper runs ngspice twice: once without -r so the .meas lines go to sim.log, once with -r to write sim.raw. The source is mounted read-only, in read-only, out read-write, and /work is a tmpfs. A timeout ends the run with exit 124 and cause timeout; exceeding the out cap gives exit 137 and cause out_cap; running out of memory also gives 137.',
      code: 'test/TC1_boost_converter/workspace/.tools/ngspice/profile.yml and Dockerfile; internal/sequence/phase.go (startAndWait)',
    },
    'IP-8': {
      name: 'Exec dir .output/exec/<id>',
      desc: 'The exec directory holds stdout.log, stderr.log, meta.json, result.json (exit code and cause), and out/sim.log and out/sim.raw. In the simulate step the model reads result.json, collects the .meas values from sim.log and inspects sim.raw with readraw.py, and writes them into sim.md. The evaluate step later judges from sim.md.',
      code: 'internal/sequence/sequence.go (Execute); .process/boost-design.yml (simulate)',
    },
  },
};

// `steps` of a phase hold step ids; a step from a lane to itself is drawn as a
// loop and `reply: true` dashes it.
export const seq = {
  lanes: [
    { key: 'graph', label: 'Process graph', sub: 'deterministic' },
    { key: 'model', label: 'Claude CLI', sub: 'model decides inside a step', variant: 'solid-danger' },
    { key: 'tool', label: 'run_ephemeral_container', sub: 'host tool', variant: 'solid-dark' },
    { key: 'container', label: 'Container', sub: 'no network', variant: 'solid-safe' },
  ],
  phases: [
    { label: 'Design', steps: ['1', '2'] },
    { label: 'Simulate', variant: 'safe', steps: ['3', '4', '5', '6', '7', '8', '9', '10', '11'] },
    { label: 'Judge', steps: ['12', '13', '14'] },
  ],
  steps: [
    { id: '1', from: 'graph', to: 'model', text: 'calc: size L and C by hand' },
    { id: '2', from: 'graph', to: 'model', text: 'netlist: six corner .cir files' },
    { id: '3', from: 'graph', to: 'model', text: 'simulate: claude -p with --allowedTools' },
    { id: '4', from: 'model', to: 'tool', text: 'Bash: tool ngspice --in netlist=<corner>.cir --exec-id bst<N>' },
    { id: '5', from: 'tool', to: 'tool', text: 'validate: profile, new id, argv, spice-safe' },
    { id: '6', from: 'tool', to: 'container', text: 'stage, create, verify, start, wait' },
    { id: '7', from: 'container', to: 'tool', text: 'exit 0, out/sim.log, out/sim.raw', reply: true },
    { id: '8', from: 'tool', to: 'model', text: 'exit 0, result.json cause ok', reply: true },
    { id: '9', from: 'model', to: 'model', text: 'read sim.log, readraw.py → sim.md' },
    { id: '10', from: 'model', to: 'graph', text: 'outcome ok', reply: true },
    { id: '11', from: 'graph', to: 'graph', text: 'simulate ok → evaluate' },
    { id: '12', from: 'graph', to: 'model', text: 'evaluate: sim.md against HW_REQ.md' },
    { id: '13', from: 'model', to: 'graph', text: 'outcome pass (fail → calc)', reply: true },
    { id: '14', from: 'graph', to: 'model', text: 'report: REPORT.md ends RESULT: PASS' },
  ],
};

export const seqText = {
  label:
    'Sequence diagram of one pass through the boost-design process: the graph starts the Claude CLI for each step; in simulate the model calls the host tool through its Bash tool, the tool validates and runs the container, the model reads the result and writes the outcome, and the graph routes to evaluate and report',
  hint: 'Hover or click a step for what happens there. Solid arrows are requests, dashed arrows are replies.',
  steps: {
    '1': {
      name: 'calc: size L and C by hand',
      desc: 'The graph enters the calc step and the model sizes the inductor and capacitor from the requirement. The numbers are the model’s own; the code does not check them. The model writes design.md and the step ends.',
      reqs: ['HW-BST-07', 'HW-BST-02'],
      code: '.process/boost-design.yml',
    },
    '2': {
      name: 'netlist: six corner netlists',
      desc: 'The model writes one netlist per corner to .input/boost_design/<corner>.cir, starting from bench.cir. The text of each netlist is the model’s work and is checked only by the tool’s input policy in step 5.',
      reqs: ['HW-BST-01', 'HW-BST-05', 'HW-BST-06'],
      code: '.process/boost-design.yml',
    },
    '3': {
      name: 'Start the simulate step',
      desc: 'The graph enters simulate and starts one claude -p process: the step prompt on stdin and the step’s tool list as --allowedTools (Read, Write, run_ephemeral_container tool ngspice:* and readraw.py:*). Then it waits for the process to exit. The graph executes nothing itself.',
      reqs: ['HW-BST-01', 'HW-BST-05', 'HW-BST-06'],
      code: 'agent-graph: services/claude_cli.py, dynamic/step_cognitive.py; .process/boost-design.yml (simulate)',
    },
    '4': {
      name: 'The model runs the tool',
      desc: 'The model runs the tool through its Bash tool, once per corner, six runs with exec ids bst<B> to bst<B+5>. The step prompt asks for a single command with no pipe, ; or redirect, and the allow-list permits only this command for the simulator.',
      code: '.process/boost-design.yml (simulate)',
    },
    '5': {
      name: 'Validate',
      desc: 'The host loads the profile, checks that the exec id is new, checks the input names and params, expands argv from the profile and applies the spice-safe policy. A refusal here exits 125 and leaves no exec directory behind.',
      code: 'internal/sequence/sequence.go (validate, validateInputs, buildArgv); internal/profile/tool.go (ExpandToolArgv); internal/inputpolicy',
      tests: 'tests/tool_test.go TestToolInputPolicyRefusesBeforeExecDir',
    },
    '6': {
      name: 'Stage, create, start, wait',
      desc: 'The host creates the exec directory, hashes inputs under src/ and copies the others into in/, creates the container with fixed settings, verifies them by inspect, writes meta.json, starts the container and waits. A timeout ends the run with exit 124 and cause timeout.',
      code: 'internal/sequence/sequence.go (prepare, stage); internal/kind/kind.go (NewPlan, Verify); internal/sequence/phase.go (startAndWait)',
      tests: 'tests/tool_test.go TestToolRunTimeoutPassedThrough',
    },
    '7': {
      name: 'Container result',
      desc: 'The wrapper has run ngspice twice and written sim.log and sim.raw under out/. The container exits 0 only when both ngspice runs succeed; otherwise it exits with the code of the first failing run.',
      code: 'test/TC1_boost_converter/workspace/.tools/ngspice/profile.yml',
      tests: 'tests/tool_test.go TestToolRunReference',
    },
    '8': {
      name: 'Tool result',
      desc: 'The tool writes result.json with exit 0 and cause ok beside stdout.log, stderr.log and meta.json, and exits with the container’s exit code. The model gets that exit code and the tool’s output, not the graph.',
      code: 'internal/sequence/sequence.go (Execute); internal/sequence/phase.go (writeResult)',
    },
    '9': {
      name: 'Read the results',
      desc: 'The model reads exit and cause from result.json, collects vout_avg, vout_avg_prev, vout_pp, il_avg and il_pp from sim.log and runs readraw.py on sim.raw. It writes one row per corner into sim.md, facts only.',
      reqs: ['HW-BST-01', 'HW-BST-05', 'HW-BST-06'],
      code: '.process/boost-design.yml (simulate)',
    },
    '10': {
      name: 'Outcome ok',
      desc: 'The model writes outcome ok when every corner has exit 0, cause ok and all five values, otherwise tool_error. Then the CLI process exits.',
      code: '.process/boost-design.yml (simulate); agent-graph: prompts/step_contract.py',
    },
    '11': {
      name: 'simulate ok → evaluate',
      desc: 'The graph reads the outcome, checks that the step declares it and follows the one matching edge to evaluate. A tool_error would send it back to netlist. The graph does not read the exec directory, so this route rests on the outcome the model wrote.',
      code: '.process/boost-design.yml (edges); agent-graph: dynamic/step_cognitive.py, dynamic/router.py (select_edge)',
    },
    '12': {
      name: 'evaluate',
      desc: 'The graph starts a new claude -p for evaluate with Read and Write only, no Bash. The model compares the numbers in sim.md and design.md with HW_REQ.md, one PASS, FAIL or N/A cell per requirement, and writes verify.md.',
      reqs: ['HW-BST-02', 'HW-BST-03', 'HW-BST-04', 'HW-BST-07', 'HW-BST-08'],
      code: '.process/boost-design.yml (evaluate)',
    },
    '13': {
      name: 'Outcome pass or fail',
      desc: 'The model sets outcome pass when no cell is FAIL, otherwise fail. The verdict is the model’s; on fail the graph goes back to calc.',
      code: '.process/boost-design.yml (evaluate)',
    },
    '14': {
      name: 'report',
      desc: 'The graph enters the report step and the model writes REPORT.md. The report step writes the line RESULT: PASS; the graph reaches it only through the pass edge from evaluate.',
      code: '.process/boost-design.yml',
    },
  },
};
