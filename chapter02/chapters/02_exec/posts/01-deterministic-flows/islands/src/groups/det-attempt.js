// One attempt of a cognitive step, drawn once by the generic SequenceDiagram
// (lays itself out):
//   `seq` / `seqText` → SequenceDiagram.jsx
// Geometry and structure first, every word the panels show second. Chip
// labels for the ids named in `links` come from REF_SHORT in ../data.js.
// Step ids DS-1..15 are distinct from the ids of groups/det-arch.js.

export const seq = {
  lanes: [
    { key: 'e2', label: 'E2 supervise', sub: 'engine' },
    { key: 'dm', label: 'DynamicMachine', sub: 'driver.py', variant: 'solid-safe' },
    { key: 'node', label: 'Cognitive node', sub: 'step_cognitive.py' },
    { key: 'cli', label: 'Claude CLI', sub: 'claude -p', variant: 'solid-danger' },
    { key: 'router', label: 'Router', sub: 'router.py' },
  ],
  phases: [
    { label: 'Start of the attempt', variant: 'safe', steps: ['DS-1', 'DS-2', 'DS-3', 'DS-4'] },
    { label: 'The model works', variant: 'danger', steps: ['DS-5', 'DS-6'] },
    {
      label: 'Checks',
      variant: 'safe',
      steps: [
        'DS-7',
        'DS-8',
        {
          alt: [
            { guard: 'all checks pass', tone: 'safe', steps: ['DS-9', 'DS-10', 'DS-11', 'DS-12', 'DS-13'] },
            { guard: 'else: StepFailure', tone: 'danger', steps: ['DS-14', 'DS-15'] },
          ],
        },
      ],
    },
  ],
  steps: [
    { id: 'DS-1', from: 'e2', to: 'dm', text: 'START or RETRY(node, reason)' },
    { id: 'DS-2', from: 'node', to: 'dm', text: 'gate.before_node_start' },
    { id: 'DS-3', from: 'dm', to: 'e2', text: 'NODE_STARTED', reply: true },
    { id: 'DS-4', from: 'node', to: 'node', text: 'compose context' },
    { id: 'DS-5', from: 'node', to: 'cli', text: 'run_turn --tools --allowedTools', code: 'ClaudeCli.run_turn' },
    { id: 'DS-6', from: 'cli', to: 'cli', text: 'model writes result.json' },
    { id: 'DS-7', from: 'node', to: 'node', text: 'validate result and handoff' },
    { id: 'DS-8', from: 'node', to: 'node', text: 'outcome and check_products' },
    { id: 'DS-9', from: 'node', to: 'dm', text: 'NODE_DONE' },
    { id: 'DS-10', from: 'dm', to: 'e2', text: 'NODE_DONE', reply: true },
    { id: 'DS-11', from: 'dm', to: 'router', text: 'select_edge(step)' },
    { id: 'DS-12', from: 'router', to: 'dm', text: 'exactly one edge', reply: true },
    { id: 'DS-13', from: 'dm', to: 'node', text: 'next node runs' },
    { id: 'DS-14', from: 'node', to: 'dm', text: 'StepFailure: NODE_FAILED' },
    { id: 'DS-15', from: 'dm', to: 'e2', text: 'E4: RETRY or escalate to S7', reply: true },
  ],
};

export const seqText = {
  label:
    'Sequence diagram of one attempt of a cognitive step between E2 supervise, the DynamicMachine, the cognitive node, the Claude CLI and the router: START or RETRY, the budget gate, NODE_STARTED, context composition, the model turn that writes result.json, the result checks, NODE_DONE and edge selection, with the StepFailure branch to E4',
  hint: 'Hover or click a step for what happens there and the code that does it. Green phases are checks the runtime performs; the red phase is the model at work. The lower branch is the failure path.',
  steps: {
    'DS-1': {
      name: 'E2 sends START or RETRY',
      desc: 'E2 supervise sends the next command to the DynamicMachine: START on the first node, RETRY(node, reason) after a failed attempt. The machine records the command before the status it causes.',
      code: 'engine/e2_supervise.py · dynamic/driver.py · contract.py',
    },
    'DS-2': {
      name: 'Budget gate before the node starts',
      desc: 'The node asks the run gate gate.before_node_start. A refusal carries a cause such as cancelled:budget:max_steps and ends the attempt as a run-budget failure.',
      code: 'ports.py (RunGate.before_node_start) · engine/budgets.py',
    },
    'DS-3': {
      name: 'NODE_STARTED',
      desc: 'The machine reports NODE_STARTED to the engine. E2 arms the per-attempt timer at this point.',
      code: 'engine/e2_supervise.py',
    },
    'DS-4': {
      name: 'Compose the context',
      desc: 'The runner composes the working folder active/ for this visit. It holds the step contract (with the step’s instruction under the heading “## Goal”), the files the step reads and the handoffs of upstream nodes. If the mandatory part does not fit its token budget the step fails at once and is not retried.',
      code: 'compose/step_contract.py · compose/context_pack.py · step_cognitive.py (do_compose)',
      links: ['DS-7'],
    },
    'DS-5': {
      name: 'One claude -p turn',
      desc: 'ClaudeCli.run_turn starts one claude -p child process in active/. The argument list carries the process-wide runtime tools as --tools and the step’s tools (or the runtime tools when the step lists none) as --allowedTools; a retry or an answer resumes the same session. The prompt goes in through standard input.',
      code: 'services/claude_cli.py (build_argv, run_turn) · ports.py (Launch.invariant_argv)',
    },
    'DS-6': {
      name: 'The model writes result.json',
      desc: 'The model works and ends the turn by writing result.json in its working folder. The runner deletes any earlier result.json before the turn, so a missing file means the model did not write one this turn.',
      code: 'step_cognitive.py (do_turn, _read_result)',
    },
    'DS-7': {
      name: 'Validate the result and the handoff note',
      desc: 'result.json is validated against node_result.schema.yml. The handoff note must carry a non-empty “done”. A result with the status “question” instead waits for the developer’s ANSWER and runs the next turn in the same session.',
      code: 'schemas/node_result.schema.yml · step_cognitive.py (_read_result)',
    },
    'DS-8': {
      name: 'Outcome and products',
      desc: 'When the step declares outcomes, the reported outcome must be one of step.outcomes. check_products then requires every declared product file in the work folder and, where a schema is declared, checks its content.',
      code: 'step_cognitive.py · dynamic/products.py (check_products)',
    },
    'DS-9': {
      name: 'Stage, move and finish',
      desc: 'The runner writes handoff.json, stages the output targets, moves active/ to nodes/<node>/ and reports NODE_DONE with the outcome and references to the products.',
      code: 'step_cognitive.py (do_handoff, do_move) · dynamic/journal.py',
    },
    'DS-10': {
      name: 'NODE_DONE reaches the engine',
      desc: 'The engine receives NODE_DONE. It does not choose the next node; the machine follows its own edges.',
      code: 'engine/e2_supervise.py',
    },
    'DS-11': {
      name: 'The router is asked for the edge',
      desc: 'After the node the machine calls select_edge for the step. A human step matches the edge by the review decision, a step with outcomes by the reported outcome, any other step by the done edge.',
      code: 'dynamic/router.py (select_edge, make_router)',
      links: ['DS-12'],
    },
    'DS-12': {
      name: 'Exactly one edge',
      desc: 'select_edge returns the destination of the single edge that holds. If zero edges or more than one match, it raises RoutingError; a validated process never causes this.',
      code: 'dynamic/router.py (RoutingError)',
    },
    'DS-13': {
      name: 'Next node',
      desc: 'The machine continues at the destination of the selected edge, or ends at END. Only edges the process defines are followed; the machine never selects a node itself.',
      code: 'dynamic/router.py · dynamic/builder.py',
    },
    'DS-14': {
      name: 'A check fails: StepFailure',
      desc: 'A failed schema, outcome or product check raises StepFailure. The machine reports NODE_FAILED with the kind and the reason, and waits for the engine’s next command.',
      code: 'dynamic/journal.py (StepFailure, run_attempts)',
      links: ['DS-15'],
    },
    'DS-15': {
      name: 'E4: retry or escalate',
      desc: 'E4 counts the failure. While the failures are fewer than the retry threshold and the failure is retryable it sends RETRY with the reason, and the node runs again in the same session. Otherwise it escalates through E7, which ends S6 and hands the run to S7.',
      code: 'engine/e4_failure_policy.py · engine/e7_suspend.py',
    },
  },
};
