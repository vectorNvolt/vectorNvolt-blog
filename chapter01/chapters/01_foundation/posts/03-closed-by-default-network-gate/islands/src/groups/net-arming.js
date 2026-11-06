// The arm sequence of create_agent_net (armPipeline, 13 steps in five phases),
// drawn once by the generic BlockDiagram as three rows read left to right:
//   `block` / `blockText` → BlockDiagram.jsx (hand laid out, viewBox units)
// Geometry and structure first, every word the panels show second. Chip
// labels for the ids named in `links` come from REF_SHORT in ../data.js.
// Each step's numbered circle sits on a short arrow that enters its box from
// above; the plain arrows between the boxes show the order. A row ends with an
// elbow arrow (the circle of the next row's first step) that runs down and back
// to the left. The caller (create_agent_container) brackets the run: it
// creates, starts and pauses before arm, and checks, unpauses and starts the
// agent after it. Those boxes carry no number: they are not steps of arm.
// Box positions: five slots per row at x = 36, 220, 404, 588, 772 (w 156).

export const block = {
  viewH: 816,
  outlines: [
    { x: 28, y: 16, w: 908, h: 100, label: 'Caller · create_agent_container, before arm' },
    { x: 28, y: 144, w: 908, h: 200, label: 'Preconditions' },
    { x: 28, y: 372, w: 172, h: 200, label: 'Clean slate', variant: 'safe' },
    { x: 212, y: 372, w: 356, h: 200, label: 'Identify and close', variant: 'safe' },
    { x: 580, y: 372, w: 356, h: 200, label: 'Controls and read-back', variant: 'safe' },
    { x: 28, y: 600, w: 540, h: 200, label: 'Release', variant: 'danger' },
    { x: 580, y: 600, w: 356, h: 200, label: 'Caller · after arm' },
  ],
  nodes: {
    caller: {
      x: 36,
      y: 44,
      w: 300,
      h: 60,
      label: 'Create, start, pause',
      sub: ['docker create on agents-net, docker start', 'docker pause, then arm <container>'],
    },
    validate: { x: 36, y: 228, w: 156, h: 96, label: 'Validate invocation', sub: ['arm + valid container name', 'else usage text, exit 1'] },
    privilege: { x: 220, y: 228, w: 156, h: 96, label: 'Privilege and engine', sub: ['root, CAP_NET_ADMIN,', 'docker socket, engine'] },
    record: { x: 404, y: 228, w: 156, h: 96, label: 'Record engine state', sub: ['daemon.json SHA-256', 'engine start identity'] },
    network: { x: 588, y: 228, w: 156, h: 96, label: 'Network and gate', sub: ['agents-net exists', 'gate has the mark rule'] },
    paused: { x: 772, y: 228, w: 156, h: 96, label: 'Require paused', sub: ['runsc, restart "no"', 'agents-net only', 'running and paused'] },
    clean: { x: 36, y: 456, w: 156, h: 96, label: 'Clean slate', sub: ['name-keyed disarm', 'leftovers removed first'] },
    identify: {
      x: 220,
      y: 456,
      w: 156,
      h: 96,
      label: 'Identify host veth',
      sub: ['PID, IP, MAC from inspect', 'veth read via nsenter', 'cross-checked'],
    },
    close: {
      x: 404,
      y: 456,
      w: 156,
      h: 96,
      label: 'Close port',
      sub: ['evict MAC from released', 'load as_port_<id>', 'policy drop, one nft -f -'],
    },
    apply: { x: 588, y: 456, w: 156, h: 96, label: 'Apply controls', sub: ['port flags, static FDB', 'neighbour entry', 'iptables rules'] },
    verify: {
      x: 772,
      y: 456,
      w: 156,
      h: 96,
      label: 'Verify controls',
      sub: ['read back: bridge, nft,', 'iptables; first mismatch', 'stops the run'],
    },
    release: {
      x: 36,
      y: 684,
      w: 156,
      h: 96,
      label: 'Release port',
      sub: ['one nft transaction:', 'marked accept rules +', 'MAC and name in released'],
      variant: 'solid-dark',
    },
    engine: { x: 220, y: 684, w: 156, h: 96, label: 'Verify engine state', sub: ['daemon.json hash and', 'engine identity unchanged'] },
    report: { x: 404, y: 684, w: 156, h: 96, label: 'Report armed', sub: ['exit 0; the caller', 'may now unpause'] },
    check: { x: 588, y: 684, w: 156, h: 96, label: 'Check, then unpause', sub: ['check-gate (read only)', 'docker unpause'] },
    agent: { x: 772, y: 684, w: 156, h: 96, label: 'Start the agent', sub: ['docker exec -u 1001', 'only now'] },
  },
  links: [
    { points: [[192, 276], [220, 276]], dir: 'end' },
    { points: [[376, 276], [404, 276]], dir: 'end' },
    { points: [[560, 276], [588, 276]], dir: 'end' },
    { points: [[744, 276], [772, 276]], dir: 'end' },
    { points: [[192, 504], [220, 504]], dir: 'end' },
    { points: [[376, 504], [404, 504]], dir: 'end' },
    { points: [[560, 504], [588, 504]], dir: 'end' },
    { points: [[744, 504], [772, 504]], dir: 'end' },
    { points: [[192, 732], [220, 732]], dir: 'end' },
    { points: [[376, 732], [404, 732]], dir: 'end' },
    { points: [[560, 732], [588, 732]], dir: 'end' },
    { points: [[744, 732], [772, 732]], dir: 'end' },
  ],
  ips: [
    { id: 'IP-S1', label: '1', points: [[176, 104], [176, 228]], dir: 'end', at: [176, 196] },
    { id: 'IP-S2', label: '2', points: [[298, 180], [298, 228]], dir: 'end', at: [298, 204] },
    { id: 'IP-S3', label: '3', points: [[482, 180], [482, 228]], dir: 'end', at: [482, 204] },
    { id: 'IP-S4', label: '4', points: [[666, 180], [666, 228]], dir: 'end', at: [666, 204] },
    { id: 'IP-S5', label: '5', points: [[850, 180], [850, 228]], dir: 'end', at: [850, 204] },
    { id: 'IP-S6', label: '6', points: [[850, 324], [850, 358], [176, 358], [176, 456]], dir: 'end', at: [176, 428] },
    { id: 'IP-S7', label: '7', points: [[298, 408], [298, 456]], dir: 'end', at: [298, 432] },
    { id: 'IP-S8', label: '8', points: [[482, 408], [482, 456]], dir: 'end', at: [482, 432] },
    { id: 'IP-S9', label: '9', points: [[666, 408], [666, 456]], dir: 'end', at: [666, 432] },
    { id: 'IP-S10', label: '10', points: [[850, 408], [850, 456]], dir: 'end', at: [850, 432] },
    { id: 'IP-S11', label: '11', points: [[850, 552], [850, 586], [176, 586], [176, 684]], dir: 'end', at: [176, 656] },
    { id: 'IP-S12', label: '12', points: [[298, 636], [298, 684]], dir: 'end', at: [298, 660] },
    { id: 'IP-S13', label: '13', points: [[482, 636], [482, 684]], dir: 'end', at: [482, 660] },
  ],
  assets: [],
  threats: [],
};

export const blockText = {
  label:
    'Block diagram of the arm sequence in three rows read left to right: the caller creates, starts and pauses the container; arm runs 13 steps in five phases (preconditions, clean slate, identify and close, controls and read-back, release); the caller then runs check-gate, unpauses and starts the agent with docker exec',
  hint: 'Numbered circles mark the 13 steps of arm, row by row; hover or click one to see what the step does. Phases are the dashed regions. Green: the container is paused and its port is closed. Red: the release, the last operation, which opens the port. The two boxes at the top and the two at the bottom right belong to the caller, not to arm.',
  items: {
    'IP-S1': {
      name: 'Step 1: validate invocation',
      desc: [
        '• Accepted: exactly arm followed by one container name.',
        '• Name: must match ^[a-zA-Z0-9][a-zA-Z0-9_.-]*$ (a docker container name) and must give a non-empty port table id for as_port_<id>.',
        '• Refused name: gate, because its rule tag agent-net:gate is the tag of the network gate rule.',
        '• Anything else: prints the usage text, reports the not-done state GATED_UNARMED and exits with code 1, before anything is touched.',
      ].join('\n'),
      code: 'agent-net: s01_validate_invocation.go (ValidateInvocation)',
      links: [],
    },
    'IP-S2': {
      name: 'Step 2: check privilege and engine',
      desc: [
        '• Checks: runs as root (euid 0).',
        '• Checks: CAP_NET_ADMIN is in its effective capability set.',
        '• Checks: it is in the agent host’s own network namespace (the same as pid 1, where the bridge, nftables and iptables state lives).',
        '• Checks: it can reach the Docker socket. All missing items are named together.',
        '• Then: asks the Docker engine for its version.',
        '• Failure: a missing privilege or an unreachable engine exits with code 1.',
        '• Effect: it changes nothing.',
      ].join('\n'),
      code: 'agent-net: s02_check_privilege_engine.go (CheckPrivilege)',
      links: [],
    },
    'IP-S3': {
      name: 'Step 3: record engine state',
      desc: [
        '• Records: the SHA-256 of /etc/docker/daemon.json and the Docker engine start identity.',
        '• Use: step 12 compares both, so a change of the engine during the run is detected.',
      ].join('\n'),
      code: 'agent-net: s05_record_engine_state.go (RecordEngineState)',
      links: [],
    },
    'IP-S4': {
      name: 'Step 4: require agents-net and its gate',
      desc: [
        '• Required: the network agents-net and its agents_gate table must exist, and the gate must have the mark rule.',
        '• Mismatch: a gate or port rule without the mark counts as a mismatch.',
        '• Consequence: a network created before the mark was added must be recreated.',
      ].join('\n'),
      code: 'agent-net: s09_require_network.go (RequireNetwork)',
      links: [],
    },
    'IP-S5': {
      name: 'Step 5: require a paused container',
      desc: [
        '• Runtime: runsc.',
        '• Restart policy: exactly no (an empty policy is refused).',
        '• Network: attached to agents-net only.',
        '• State: running and paused.',
        '• Caller: arm does not create, start, pause or unpause the container; the caller does.',
      ].join('\n'),
      code: 'agent-net: s34_require_paused.go (RequirePaused)',
      links: [],
    },
    'IP-S6': {
      name: 'Step 6: clean slate (name-keyed disarm)',
      desc: [
        '• Cleanup: the same name-keyed cleanup as disarm runs first, so arm always builds from an empty state.',
        '• Removed: whatever an earlier container or run of that name left (gate elements, port table, tagged iptables rules, neighbour and FDB entries).',
        '• Failure: a leftover that cannot be removed fails the arm.',
      ].join('\n'),
      code: 'agent-net: s40_clean_slate.go (CleanSlate)',
      links: [],
    },
    'IP-S7': {
      name: 'Step 7: identify the host veth',
      desc: [
        '• Inputs: the container PID, IP and MAC, taken from the Docker inspect data.',
        '• Veth: found from host state, the host kernel’s view of the container’s network namespace, read as root through nsenter.',
        '• Trust: nothing reported by code inside the container is used.',
        '• Cross-check: the veth against the agent MAC, the container IP and the agents-net bridge master.',
        '• Failure: any mismatch fails the arm.',
      ].join('\n'),
      code: 'agent-net: s12_identify_port.go (IdentifyPort)',
      links: [],
    },
    'IP-S8': {
      name: 'Step 8: close the port (load the port table)',
      desc: [
        '• Evict: the MAC is removed from the set released.',
        '• Load: the initial port table as_port_<id>, with policy drop and no accept rule, in one atomic nft -f - transaction.',
        '• Result: from here the port is closed at the veth as well as at the gate.',
      ].join('\n'),
      code: 'agent-net: s10_close_port.go (ClosePort)',
      links: [],
    },
    'IP-S9': {
      name: 'Step 9: apply the host-side controls',
      desc: [
        '• Port flags on the veth: isolated on, learning off, locked on.',
        '• Static FDB entry.',
        '• Permanent neighbour entry on the bridge.',
        '• iptables rules in DOCKER-USER and INPUT.',
        '• Failure: the first failure stops the run with exit code 2.',
      ].join('\n'),
      code: 'agent-net: s13_apply_controls.go (ApplyControls)',
      links: [],
    },
    'IP-S10': {
      name: 'Step 10: verify the controls (read back)',
      desc: [
        '• Read-back: each control (bridge -j link show, nft list table, iptables -S), compared with the specification.',
        '• Failure: a mismatch stops the run with exit code 2, and the port stays closed.',
      ].join('\n'),
      code: 'agent-net: s14_verify_controls.go (VerifyControls)',
      links: [],
    },
    'IP-S11': {
      name: 'Step 11: release the port (one nft transaction)',
      desc: [
        '• Re-check: the container is still running and paused.',
        '• Transaction: one nft -f - transaction appends the accept rules to the container’s port table (the ingress ones set the port mark).',
        '• Transaction: the same transaction adds its MAC to the set released with the container name as the element comment.',
        '• Failure: if the release or its read-back fails, closeAgain removes the MAC, loads the port closed and the tool exits with code 2.',
      ].join('\n'),
      code: 'agent-net: s15_release_port.go (ReleasePort, closeAgain)',
      links: [],
    },
    'IP-S12': {
      name: 'Step 12: verify the engine state',
      desc: [
        '• Compared: the daemon.json hash and the Docker engine start identity, against the values recorded in step 3.',
        '• Failure: a change is reported as a failure.',
      ].join('\n'),
      code: 'agent-net: s35_verify_engine_unchanged.go (VerifyEngineUnchanged)',
      links: [],
    },
    'IP-S13': {
      name: 'Step 13: report armed',
      desc: [
        '• Arm: reports success and exits with code 0. The container is still paused.',
        '• Caller, in order: runs check-gate, unpauses the container, and only after that starts the agent with docker exec -u 1001.',
        '• Effect: in that order the agent never runs while the port is closed or unchecked.',
      ].join('\n'),
      code: 'agent-net: s16_report_armed.go (ReportArmed); agent-sandbox: s22_release_container.go',
      links: [],
    },
  },
};
