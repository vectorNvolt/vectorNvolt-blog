// Short labels for the chips on the detail panel, keyed by id. Every id a
// panel shows as a chip (related hotspots, goals, requirements) needs one
// here. The two diagrams of this post (groups/ngspice.js) name the boost
// requirements HW-BST-01..08 (test/TC1_boost_converter HW_REQ.md) through
// `reqs`, so those eight ids are listed.
export const REF_SHORT = {
  'HW-BST-01': 'Vin 9-15 V',
  'HW-BST-02': 'Vout 24 V ±2 %',
  'HW-BST-03': 'Vout ripple ≤240 mV',
  'HW-BST-04': 'IL ripple ≤30 %',
  'HW-BST-05': 'Load 0.2-2 A',
  'HW-BST-06': 'fsw 200 kHz',
  'HW-BST-07': 'E6 parts ≥1.1x',
  'HW-BST-08': 'Steady state ≤120 mV',
};
