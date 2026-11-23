# HW requirements: synchronous boost converter

Design one synchronous boost converter (single inductor L, single output capacitor C) that meets every requirement below at every corner. Open loop: the duty cycle is fixed per corner, there is no regulation loop.

| ID | Requirement | Limit | Verification (measure from `out/sim.log`) |
|---|---|---|---|
| HW-BST-01 | Input voltage range | 9 V to 15 V, nominal 12 V | Corner definition (Vin per corner, table below) |
| HW-BST-02 | Output voltage, average | 24 V +/- 2 % (23.52 V to 24.48 V) at every corner | `vout_avg` |
| HW-BST-03 | Output voltage ripple, peak to peak | <= 240 mV (1 % of 24 V) at every corner | `vout_pp` |
| HW-BST-04 | Inductor current ripple, peak to peak | dIL p-p <= 30 % of IL_avg at Iout = 2 A, for all Vin. At light load (0.2 A): record only, no limit | `il_pp`, `il_avg` (ratio `il_pp / il_avg`) |
| HW-BST-05 | Load range | 0.2 A to 2 A, resistive: Rload = 120 ohm (0.2 A) and 12 ohm (2 A) | Corner definition (Rload per corner) |
| HW-BST-06 | Switching frequency | fsw = 200 kHz (T = 5 us) | Fixed in `bench.cir`; duty width per corner set through `<D_TON>` |
| HW-BST-07 | Parts | One L and one C for all corners. Both from the E6 series (1.0 1.5 2.2 3.3 4.7 6.8, times 10^n) and >= 1.1 x the calculated minimum | Design calculation (L_min, C_min, 1.1 x margin, chosen E6 value) |
| HW-BST-08 | Steady state at the end of the run | abs(`vout_avg` - `vout_avg_prev`) <= 0.5 % of 24 V (120 mV) | `vout_avg`, `vout_avg_prev` |

Every "at every corner" requirement is judged on all six simulations. One failing corner fails the requirement.

## Corners

| Corner | Vin | Iout | Rload |
|---|---|---|---|
| `v09_l02` | 9 V | 0.2 A | 120 ohm |
| `v09_l20` | 9 V | 2 A | 12 ohm |
| `v12_l02` | 12 V | 0.2 A | 120 ohm |
| `v12_l20` | 12 V | 2 A | 12 ohm |
| `v15_l02` | 15 V | 0.2 A | 120 ohm |
| `v15_l20` | 15 V | 2 A | 12 ohm |

Netlist per corner: `/workspace/.input/boost_design/<corner>.cir`.

## Test bench

The bench (topology, switch model, `.tran`, `.meas` lines) is fixed in `bench.cir` in this folder. A netlist is that file with only the placeholders `<VIN> <D_TON> <L> <C> <RLOAD> <IL0> <VOUT0>` replaced.

Measures (all from `.meas` in the bench; run of 1 ms with `uic` and initial conditions on L and C):

| Name | Meaning | Window |
|---|---|---|
| `vout_avg` | average of v(out) | 0.9 ms to 1.0 ms |
| `vout_avg_prev` | average of v(out) | 0.8 ms to 0.9 ms |
| `vout_pp` | peak-to-peak of v(out) | 0.9 ms to 1.0 ms |
| `il_avg` | average inductor current, i(vsense) | 0.9 ms to 1.0 ms |
| `il_pp` | peak-to-peak inductor current, i(vsense) | 0.9 ms to 1.0 ms |

## Assumptions (ideal parts)

- L has no series resistance (no DCR). C has no ESR.
- Low-side and high-side FETs are ngspice `S` switches: Ron = 1 mohm, Roff = 1 Mohm.
- The high-side switch is driven inverted to the low-side switch, with no dead time: the converter stays in continuous conduction (CCM) at every corner, including 0.2 A.
- Gate drive edges: 1 ns rise and fall.
- No diode, no parasitics, no input filter. Ideal DC source at the input.

## E-series reference

- E6 multipliers: 1.0 1.5 2.2 3.3 4.7 6.8 (times 10^n).
