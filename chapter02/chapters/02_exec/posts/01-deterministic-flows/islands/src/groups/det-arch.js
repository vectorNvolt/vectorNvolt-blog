// The layered architecture of agent_graph, drawn once by the generic
// BlockDiagram (top to bottom):
//   `block` / `blockText` → BlockDiagram.jsx (hand laid out, viewBox units)
// Geometry and structure first, every word the panels show second. Chip
// labels for the ids named in `links` come from REF_SHORT in ../data.js.
// Box variants: safe = the static master graph (same for every process),
// danger = the step type that lets a model act inside the step, info = code of
// another package. A numbered circle sits on the connector that leads into
// each layer and carries that layer's text.

export const block = {
  viewH: 1140,
  outlines: [
    { x: 180, y: 110, w: 760, h: 230, label: 'Static master graph · graph.py', variant: 'safe' },
    { x: 180, y: 390, w: 760, h: 110, label: 'S6 execution engine · engine/graph.py' },
    { x: 20, y: 710, w: 920, h: 120, label: 'Step runners · one per step type' },
    { x: 20, y: 870, w: 920, h: 120, label: 'Backends · services/' },
    { x: 20, y: 1020, w: 320, h: 96, label: 'Other package · agent-exec', variant: 'info' },
  ],
  nodes: {
    cli: { x: 400, y: 28, w: 300, h: 52, label: 'CLI', sub: 'agent_graph run' },
    yaml: { x: 20, y: 230, w: 140, h: 64, label: 'Process YAML', sub: '.process/*.yml' },
    master: { x: 400, y: 140, w: 300, h: 56, label: 'Master graph S1 to S8', sub: 'TRANSITIONS table', variant: 'solid-safe' },
    s3: { x: 200, y: 230, w: 250, h: 64, label: 'S3 parse_process', sub: 'validate the process' },
    s5: { x: 670, y: 230, w: 250, h: 64, label: 'S5 build_graph', sub: 'compile_dynamic · builder.py' },
    engine: {
      x: 200,
      y: 420,
      w: 720,
      h: 64,
      label: 'Engine E1 to E7',
      sub: ['E1 load · E2 supervise · E3 hil · E4 failure policy', 'E5 output check · E6 cleanup · E7 suspend'],
    },
    machine: {
      x: 250,
      y: 550,
      w: 600,
      h: 110,
      label: 'DynamicMachine · driver.py',
      sub: [
        'commands in: START RESTART HIL_OK HIL_NOK',
        'ANSWER RETRY CANCEL SUSPEND',
        'statuses out: NODE_STARTED NODE_DONE WAITING',
        'NODE_FAILED COMPLETED SUSPENDED',
      ],
    },
    procedure: { x: 40, y: 740, w: 280, h: 70, label: 'Procedure step', sub: ['no LLM', 'step_procedure.py'] },
    human: { x: 340, y: 740, w: 280, h: 70, label: 'Human step', sub: ['no LLM', 'step_human.py'] },
    cognitive: { x: 640, y: 740, w: 280, h: 70, label: 'Cognitive step', sub: ['LLM inside the step', 'step_cognitive.py'], variant: 'danger' },
    sandbox: { x: 40, y: 900, w: 130, h: 64, label: 'SandboxExec', small: true },
    container: { x: 190, y: 900, w: 130, h: 64, label: 'ContainerExec', small: true },
    claude: { x: 640, y: 900, w: 280, h: 64, label: 'ClaudeCli', sub: 'claude -p' },
    ephemeral: { x: 40, y: 1050, w: 260, h: 52, label: 'run_ephemeral_container', variant: 'info', small: true },
  },
  links: [
    { points: [[425, 196], [425, 230]], dir: 'end' },
    { points: [[685, 196], [685, 230]], dir: 'end' },
  ],
  ips: [
    { id: 'DA-1', label: '1', points: [[550, 80], [550, 140]], dir: 'end', at: [550, 112] },
    { id: 'DA-2', label: '2', points: [[160, 262], [200, 262]], dir: 'end', at: [180, 262] },
    { id: 'DA-3', label: '3', points: [[795, 294], [795, 420]], dir: 'end', at: [795, 365] },
    { id: 'DA-4', label: '4', points: [[550, 484], [550, 550]], dir: 'both', at: [550, 517] },
    { id: 'DA-5', label: '5', points: [[280, 660], [280, 740]], dir: 'end', at: [280, 700] },
    { id: 'DA-6', label: '6', points: [[480, 660], [480, 740]], dir: 'end', at: [480, 700] },
    { id: 'DA-7', label: '7', points: [[800, 660], [800, 740]], dir: 'end', at: [800, 700] },
    { id: 'DA-8', label: '8', points: [[105, 810], [105, 900]], dir: 'end', at: [105, 852] },
    { id: 'DA-9', label: '9', points: [[255, 810], [255, 900]], dir: 'end', at: [255, 852] },
    { id: 'DA-10', label: '10', points: [[255, 964], [255, 1050]], dir: 'end', at: [255, 1005] },
    { id: 'DA-11', label: '11', points: [[780, 810], [780, 900]], dir: 'end', at: [780, 852] },
  ],
  assets: [],
  threats: [],
};

export const blockText = {
  label:
    'Block diagram of the agent_graph architecture, top to bottom: the CLI, the static master graph S1 to S8 with the process YAML feeding S3, S5 compiling the dynamic machine, the engine E1 to E7, the DynamicMachine, the procedure, human and cognitive step runners, and the backends SandboxExec, ContainerExec (to run_ephemeral_container in agent-exec) and ClaudeCli',
  hint: 'Numbered circles mark the connectors between layers; hover or click one to see what the layer it leads into does. Green: the static master graph, the same for every process. Red: the step type that runs a model inside the step. Blue: code of another package.',
  items: {
    'DA-1': {
      name: 'CLI into the static master graph',
      desc: [
        '• Entry: agent_graph run, optionally with --resume <runID>, parsed in main.py.',
        '• Master graph: graph.py builds one LangGraph graph over the states S1 start, S2 clarify, S3 parse_process, S4 bind_input, S5 build_graph, S6 execution, S7 resolution and S8 report.',
        '• Routing: a state may only move to the targets listed for it in the TRANSITIONS table; the graph is the same for every process.',
        '• S7 resolution: every exception flow of S1 to S6 ends there instead of ending the run.',
      ].join('\n'),
      code: 'main.py · graph.py (build_master, run_master, TRANSITIONS)',
      links: [],
    },
    'DA-2': {
      name: 'Process YAML into S3 validate',
      desc: [
        '• Source: the developer writes the process as a YAML file under .process/.',
        '• S3: parse_process loads the selected process and validates its structure, transitions, data flow and the skill and snippet declarations against the pools.',
        '• Gate: any finding leaves the run in PROCESS_INVALID and goes to S7; nothing runs before S3 passes.',
      ].join('\n'),
      code: 'nodes/n03_parse_process.py · process/validate.py',
      links: [],
    },
    'DA-3': {
      name: 'S5 compiles the dynamic machine, S6 runs the engine',
      desc: [
        '• S5: compile_dynamic in builder.py compiles the process into a dynamic LangGraph machine, one node per step plus one conditional router per step with one branch per distinct destination, and the run writes graph.json.',
        '• Failure: a compile error is BUILD_FAILED and goes to S7; otherwise S5 waits for the developer’s confirmation to start.',
        '• S6: execution builds the engine context and calls the engine, which owns E1 to E7 and the dynamic machine.',
        '• E1 load builds the machine and chooses the first command; E2 supervise sends it and watches the machine until it pauses.',
      ].join('\n'),
      code: 'nodes/n05_build_graph.py · dynamic/builder.py (compile_dynamic) · nodes/n06_execution.py · engine/graph.py',
      links: ['DA-4'],
    },
    'DA-4': {
      name: 'Engine and DynamicMachine: commands in, statuses out',
      desc: [
        '• Surface: DynamicMachine is the only surface of the dynamic machine that the engine sees.',
        '• In: the machine changes only on the commands START, RESTART, HIL_OK, HIL_NOK, ANSWER, RETRY, CANCEL and SUSPEND.',
        '• Out: every change is reported as NODE_STARTED, NODE_DONE, WAITING, NODE_FAILED, COMPLETED or SUSPENDED.',
        '• Routing in the engine: after E2 the pending status picks the next engine node (WAITING to E3, NODE_FAILED to E4, COMPLETED to E5, SUSPENDED to E7).',
      ].join('\n'),
      code: 'dynamic/driver.py (DynamicMachine) · contract.py · engine/graph.py (route_pending)',
      links: [],
    },
    'DA-5': {
      name: 'Machine to the procedure step runner',
      desc: [
        '• Runner: builder.py maps each step type to one runner function; a procedure step runs run_procedure.',
        '• No model: the step runs an argument list and checks the products it declares.',
        '• Backend: see the next two connectors.',
      ].join('\n'),
      code: 'dynamic/builder.py (_RUNNERS) · dynamic/step_procedure.py (run_procedure)',
      links: [],
    },
    'DA-6': {
      name: 'Machine to the human step runner',
      desc: [
        '• Runner: run_human pauses the machine with a WAITING status that carries the brief and the items to review.',
        '• No model: the conversation with the developer is E3’s job; HIL_OK follows the hil_ok edge and HIL_NOK(feedback) the hil_nok edge.',
      ].join('\n'),
      code: 'dynamic/step_human.py (run_human) · engine/e3_hil.py',
      links: [],
    },
    'DA-7': {
      name: 'Machine to the cognitive step runner',
      desc: [
        '• Runner: run_cognitive starts one Claude session per node visit and composes the context into the active/ folder.',
        '• Model: the step is the only place a model runs; it must write result.json with a handoff note, and the runner checks it before the step is done.',
        '• Next: the second diagram follows one attempt of this runner.',
      ].join('\n'),
      code: 'dynamic/step_cognitive.py (run_cognitive)',
      links: [],
    },
    'DA-8': {
      name: 'Procedure step to SandboxExec',
      desc: [
        '• SandboxExec runs the argument list as a child of the runtime, with the agent sandbox as the surrounding container.',
        '• The command runs in its own process group, so a cancel can end it.',
      ].join('\n'),
      code: 'services/container.py (SandboxExec.run)',
      links: [],
    },
    'DA-9': {
      name: 'Procedure step with isolate: true to ContainerExec',
      desc: [
        '• Selection: a step with isolate: true and a profile calls ContainerExec.run_isolated.',
        '• Tool: ContainerExec starts one ephemeral execution container per call through the Go tool run_ephemeral_container.',
        '• Result: the tool’s result.json is authoritative for the exit code and the cause; a cancel or timeout sends SIGTERM to the tool.',
      ].join('\n'),
      code: 'dynamic/step_procedure.py (run_procedure) · services/container.py (ContainerExec.run_isolated)',
      links: ['DA-10'],
    },
    'DA-10': {
      name: 'ContainerExec to run_ephemeral_container (agent-exec)',
      desc: [
        '• Boundary: run_ephemeral_container belongs to the agent-exec package, not to agent-graph.',
        '• Lookup: the tool is found through an argument, the AGENT_GRAPH_EXEC_TOOL variable, the PATH or a default location.',
      ].join('\n'),
      code: 'services/container.py (resolve_tool) · packages/agent-exec',
      links: [],
    },
    'DA-11': {
      name: 'Cognitive step to ClaudeCli',
      desc: [
        '• Backend: ClaudeCli starts one claude -p child process per turn, with an argument list and in its own process group.',
        '• Prompt: written to the child’s standard input.',
        '• Environment: ANTHROPIC_* and CLAUDE_CODE_USE_* variables are removed from the child’s environment.',
        '• Failures: an unavailable model maps to LlmUnavailable; a cancel kills the process group.',
      ].join('\n'),
      code: 'services/claude_cli.py (ClaudeCli.build_argv, run_turn)',
      links: [],
    },
  },
};
