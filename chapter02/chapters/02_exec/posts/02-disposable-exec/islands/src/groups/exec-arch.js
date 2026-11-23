// The architecture of one disposable execution, drawn twice by the generic
// components:
//   `block` / `blockText` → BlockDiagram.jsx (hand laid out, viewBox units)
//   `seq`   / `seqText`   → SequenceDiagram.jsx (lays itself out)
// Geometry and structure first, every word the panels show second. Chip
// labels for the ids named in `links` come from REF_SHORT in ../data.js.
// Box variants: danger = untrusted caller, solid-dark = shared infrastructure
// (authorization plugin, dockerd), safe = the disposable container.
// A numbered circle sits on each connector and carries that connector's text.

export const block = {
  viewH: 704,
  outlines: [
    { x: 20, y: 16, w: 920, h: 434, label: 'Agent container · trusted harness inside gVisor' },
    { x: 20, y: 490, w: 920, h: 190, label: 'Disposable container · one per execution', variant: 'safe' },
  ],
  nodes: {
    caller: { x: 60, y: 46, w: 420, h: 54, label: 'Caller (agent or graph)', sub: 'names a profile; passes --in and --param', variant: 'danger' },
    cli: { x: 60, y: 160, w: 420, h: 54, label: 'run_ephemeral_container', sub: 'validate · stage · create · verify · start · clean up' },
    authz: { x: 130, y: 274, w: 280, h: 54, label: 'ephemeral-authz plugin', sub: 'default-deny Docker authorization', variant: 'solid-dark' },
    dockerd: { x: 130, y: 372, w: 280, h: 54, label: 'dockerd', sub: 'Engine API on a Unix socket', variant: 'solid-dark' },
    workspace: {
      x: 540,
      y: 160,
      w: 380,
      h: 88,
      label: 'Workspace /workspace',
      sub: ['src/ and in/ read-only · out/ read-write', 'results: result.json · meta.json', 'stdout.log · stderr.log'],
    },
    exec: {
      x: 60,
      y: 530,
      w: 840,
      h: 124,
      label: 'Execution container',
      sub: [
        'user 1001 · all capabilities dropped · no-new-privileges',
        'read-only root · tmpfs /work /tmp /home/run',
        'memory · CPU · pids limits · wall-clock timeout',
        'network none (install fetch: host)',
      ],
      variant: 'safe',
    },
  },
  links: [{ points: [[270, 328], [270, 372]], dir: 'end' }],
  ips: [
    { id: 'IP-1', label: '1', points: [[270, 100], [270, 160]], dir: 'end', at: [270, 130] },
    { id: 'IP-2', label: '2', points: [[270, 214], [270, 274]], dir: 'end', at: [270, 244] },
    { id: 'IP-3', label: '3', points: [[380, 426], [380, 530]], dir: 'end', at: [380, 470] },
    { id: 'IP-4', label: '4', points: [[730, 248], [730, 530]], dir: 'end', at: [730, 390] },
    { id: 'IP-5', label: '5', points: [[450, 214], [450, 530]], dir: 'both', at: [450, 360] },
    { id: 'IP-6', label: '6', points: [[480, 187], [540, 187]], dir: 'end', at: [510, 187] },
  ],
  assets: [],
  threats: [],
};

export const blockText = {
  label:
    'Block diagram of one disposable execution, top to bottom: the caller, the run_ephemeral_container harness, the ephemeral-authz plugin and dockerd inside the agent container, and below them the disposable execution container, with the workspace it mounts and the results the harness writes there',
  hint: 'Numbered circles mark each interface of one execution; hover or click one to see what it is configured to do. Red box: untrusted caller. Dark boxes: shared infrastructure. Green box: the disposable container.',
  items: {
    'IP-1': {
      name: 'Caller to run_ephemeral_container: data, not argv',
      desc: [
        '• Input: the caller names a profile and supplies data, not argv, image or mounts.',
        '• Kind tool: the profile fixes argv; only --in name=path and typed --param values (checked by Param.Check) are accepted.',
        '• Placeholders: {out}, {in.x} and {param.x} are expanded by ExpandToolArgv.',
        '• Usage errors: exit 125 and nothing is created.',
      ].join('\n'),
      code: 'agent-exec: internal/cli/cli.go (Parse), internal/profile/tool.go (ExpandToolArgv)',
      links: [],
    },
    'IP-2': {
      name: 'Engine API through the authorization plugin',
      desc: [
        '• Transport: the Engine API over the Unix socket with net/http; no docker CLI is used.',
        '• Endpoints: the plugin is default-deny on routes.',
        '• Create bodies: no Binds, no Devices, no Privileged; Runtime empty or runc.',
        '• Ceilings: memory, cpus and pids within MaxMemoryBytes 2 GiB, MaxNanoCPUs 2 and MaxPids 512.',
        '• Identity: name and label carry the same exec id.',
        '• Bind sources: checked component by component against symlinks.',
      ].join('\n'),
      code: 'agent-exec: internal/authz/policy.go (checkCreate)',
      links: [],
    },
    'IP-3': {
      name: 'Create, verify, then start',
      desc: [
        '• Create: the container is created, not started, then inspected.',
        '• Verify: kind.Verify compares the inspected container to the plan.',
        '• Mismatch: cause verify, and the container is never started.',
        '• Image: loaded from image.tar if absent; never pulled or built.',
      ].join('\n'),
      code: 'agent-exec: internal/sequence/sequence.go (ensureImage)',
      links: [],
    },
    'IP-4': {
      name: 'Mounts: exact allowlist',
      desc: [
        '• Allowlist: bind source equals target; src/ read-only, the execution’s own in/ read-only, its own out/ read-write.',
        '• Kind build: adds a read-write build/.',
        '• Install fetch: gets .packages read-only and dl/ read-write.',
        '• Anything else: refused by the plugin.',
        '• Harness side: access to the workspace by the tool itself goes through wsfs, using openat2 with RESOLVE_BENEATH, RESOLVE_NO_SYMLINKS and RESOLVE_NO_MAGICLINKS, and an O_NOFOLLOW component walk as fallback.',
      ].join('\n'),
      code: 'agent-exec: internal/authz/policy.go (bindRules, checkMounts), internal/kind/kind.go (Mounts)',
      links: [],
    },
    'IP-5': {
      name: 'Network and limits',
      desc: [
        '• Network: none for every phase except the install fetch phase (host).',
        '• Limits: memory, CPU and pids come from the profile within the ceilings.',
        '• Deadline: one wall-clock deadline (exit 124).',
        '• Output cap: out_max is checked by polling the size of the kind’s watch dirs: out/, plus build/ for build and dl/ and the target env dir for install (exit 137, cause out_cap); the default poll is 1 s, so output can exceed the cap between ticks.',
        '• OOM: detected from engine events (137).',
        '• Cancel: SIGTERM or SIGINT cancels the run (130).',
      ].join('\n'),
      code: 'agent-exec: internal/sequence/phase.go (startAndWait), internal/kind/kind.go (Network)',
      links: [],
    },
    'IP-6': {
      name: 'Results',
      desc: [
        '• result.json: written once and authoritative; holds exit, cause, exec_dir, image_id, start and end.',
        '• Cleanup: the container is removed (AutoRemove plus explicit cleanup).',
        '• Exec dir: never reused.',
      ].join('\n'),
      code: 'agent-exec: internal/sequence/sequence.go (Execute)',
      links: [],
    },
  },
};

// `steps` of a phase hold step ids and fragments: { alt: [operand, …] },
// operand = { guard, tone: 'safe' | 'danger', steps: [...] } (may nest).
// A step from a lane to itself is drawn as a loop; `reply: true` dashes it.
export const seq = {
  lanes: [
    { key: 'caller', label: 'Caller', sub: 'agent or graph' },
    { key: 'harness', label: 'run_ephemeral_container', sub: 'harness' },
    { key: 'engine', label: 'Engine', sub: 'dockerd + authz', variant: 'solid-dark' },
    { key: 'container', label: 'Container', sub: 'disposable', variant: 'solid-safe' },
  ],
  phases: [
    { label: 'Before anything exists', variant: 'safe', steps: ['S1', 'S2'] },
    { label: 'Prepare', steps: ['S3'] },
    {
      label: 'Create and verify',
      steps: [
        'S4',
        'S5',
        'S6',
        {
          alt: [
            { guard: 'matches plan', tone: 'safe', steps: ['S7'] },
            { guard: 'mismatch', tone: 'danger', steps: ['S8'] },
          ],
        },
      ],
    },
    { label: 'Run', steps: ['S9', 'S10'] },
    { label: 'Finish', steps: ['S11', 'S12'] },
  ],
  steps: [
    { id: 'S1', from: 'caller', to: 'harness', text: 'run <profile> / tool <profile>' },
    { id: 'S2', from: 'harness', to: 'harness', text: 'validate args, profile, inputs, params, argv, input policy' },
    { id: 'S3', from: 'harness', to: 'engine', text: 'ping · remove stale · locks · create exec dir' },
    { id: 'S4', from: 'harness', to: 'harness', text: 'stage inputs · load image.tar if absent' },
    { id: 'S5', from: 'harness', to: 'engine', text: 'create (not started)' },
    { id: 'S6', from: 'engine', to: 'harness', text: 'inspect → Verify', reply: true },
    { id: 'S7', from: 'harness', to: 'container', text: 'start' },
    { id: 'S8', from: 'harness', to: 'harness', text: 'cause verify, never started' },
    { id: 'S9', from: 'harness', to: 'harness', text: 'watch deadline, out_max, cancel; SIGKILL on breach' },
    { id: 'S10', from: 'container', to: 'harness', text: 'exit · timeout · OOM · out cap', reply: true },
    { id: 'S11', from: 'harness', to: 'engine', text: 'remove containers' },
    { id: 'S12', from: 'harness', to: 'caller', text: 'result.json (exit, cause)', reply: true },
  ],
};

const FAKE = ' These are unit tests against a fake engine, not a run on a real engine.';

export const seqText = {
  label:
    'Sequence diagram of one execution: the caller asks the harness, which validates, prepares, creates and verifies the container through the engine, starts it only if it matches the plan, watches it run, removes it and returns result.json',
  hint: 'Hover or click a step for what is intended to happen there and the unit tests that cover it. Nothing here comes from a run on a real engine.',
  steps: {
    S1: {
      name: 'Caller asks for an execution',
      desc: '• The caller names a profile and passes --in and --param; the entry is run or tool.',
      code: 'agent-exec: internal/cli/cli.go (Parse)',
    },
    S2: {
      name: 'Validation before anything exists',
      desc: [
        '• Checks: arguments, profile, inputs, params, argv and the input policy.',
        '• Failure: a usage or policy error exits 125 and nothing is created.',
      ].join('\n'),
      code: 'agent-exec: internal/sequence/sequence.go (validate)',
      tests: 'TestUsageErrorsCreateNothing.' + FAKE,
    },
    S3: {
      name: 'Prepare',
      desc: [
        '• Engine: ping, then remove stale containers.',
        '• Host: take the locks and create the exec directory, which is never reused.',
      ].join('\n'),
      code: 'agent-exec: internal/sequence/sequence.go (Execute)',
    },
    S4: {
      name: 'Stage inputs, load image',
      desc: [
        '• Inputs: staged into the execution’s own in/.',
        '• Image: loaded from image.tar if absent; never pulled or built.',
      ].join('\n'),
      code: 'agent-exec: internal/sequence/sequence.go (Execute)',
    },
    S5: {
      name: 'Create, not started',
      desc: '• The create body goes through the authorization plugin, which is default-deny.',
      code: 'agent-exec: internal/authz/policy.go (checkCreate)',
    },
    S6: {
      name: 'Inspect and verify',
      desc: '• The container is inspected and kind.Verify compares the result to the plan.',
      code: 'agent-exec: internal/kind/kind.go (Verify)',
    },
    S7: {
      name: 'Matches the plan: start',
      desc: '• Only a container that matched the plan is started.',
      code: 'agent-exec: internal/sequence/phase.go (startAndWait)',
      tests: 'TestRunOK.' + FAKE,
    },
    S8: {
      name: 'Mismatch: never started',
      desc: '• The cause is verify and the container is never started.',
      code: 'agent-exec: internal/kind/kind.go (Verify)',
      tests: 'TestVerifyMismatch.' + FAKE,
    },
    S9: {
      name: 'Watch the run',
      desc: [
        '• One wall-clock deadline (exit 124).',
        '• out_max is checked by polling the size of the kind’s watch dirs (out/, plus build/ or dl/ and the target env dir; exit 137, cause out_cap); output can exceed the cap between ticks.',
        '• SIGTERM or SIGINT cancels (130).',
        '• On a breach the container is sent SIGKILL.',
      ].join('\n'),
      code: 'agent-exec: internal/sequence/phase.go (startAndWait)',
      tests: 'TestRunTimeout, TestRunOutCap, TestRunCancel.' + FAKE,
    },
    S10: {
      name: 'Container ends',
      desc: '• The harness learns the exit status, or an OOM from engine events (137), or the timeout or cap that ended the run.',
      code: 'agent-exec: internal/sequence/phase.go (startAndWait)',
      tests: 'TestRunOOM, TestRunOK.' + FAKE,
    },
    S11: {
      name: 'Remove containers',
      desc: '• Removal through AutoRemove plus explicit cleanup.',
      code: 'agent-exec: internal/sequence/sequence.go (Execute)',
    },
    S12: {
      name: 'Result returned',
      desc: '• result.json is written once and is authoritative: exit, cause, exec_dir, image_id, start, end.',
      code: 'agent-exec: internal/sequence/sequence.go (Execute)',
    },
  },
};
