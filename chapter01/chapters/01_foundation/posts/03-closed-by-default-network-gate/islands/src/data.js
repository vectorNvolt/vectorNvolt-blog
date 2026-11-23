// Short labels for the chips on the detail panel, keyed by id. Every id a
// panel shows as a chip (related hotspots) needs one here; the two diagrams
// of this post share this table, so their ids are distinct: IP-1..6 are the
// isolation architecture (groups/net-arch.js), IP-S1..13 the arm sequence
// (groups/net-arming.js).
export const REF_SHORT = {
  'IP-1': 'Host boundary',
  'IP-2': 'Per-port table',
  'IP-3': 'Port controls',
  'IP-4': 'agents_gate',
  'IP-5': 'iptables rules',
  'IP-6': 'Public internet',

  'IP-S1': 'Validate invocation',
  'IP-S2': 'Privilege and engine',
  'IP-S3': 'Record engine state',
  'IP-S4': 'Network and gate',
  'IP-S5': 'Require paused',
  'IP-S6': 'Clean slate',
  'IP-S7': 'Identify veth',
  'IP-S8': 'Close port',
  'IP-S9': 'Apply controls',
  'IP-S10': 'Verify controls',
  'IP-S11': 'Release port',
  'IP-S12': 'Verify engine state',
  'IP-S13': 'Report armed',
};
