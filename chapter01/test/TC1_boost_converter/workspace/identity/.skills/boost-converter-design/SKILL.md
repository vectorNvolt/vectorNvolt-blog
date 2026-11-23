---
name: boost-converter-design
description: Size L and C of an open-loop synchronous boost converter by hand, fill the fixed ngspice bench per corner, run it, read the .meas results and judge HW-BST requirements
tags: [power, boost, ngspice]
---
# Boost converter design procedure

Applies to the requirements in `HW_REQ.md` (IDs `HW-BST-01` to `HW-BST-08`) and the template `bench.cir`. Ideal parts, continuous conduction (CCM), open loop, fixed duty per corner.

## 1. Sizing formulas

Symbols: Vin input, Vout = 24 V target, Iout load current, T = 1/fsw = 5 us, fsw = 200 kHz.

| Quantity | Formula |
|---|---|
| Duty | D = 1 - Vin / Vout |
| Average inductor current | IL_avg = Iout * Vout / Vin |
| Inductor ripple p-p | dIL = Vin * D / (L * fsw) |
| Output ripple p-p | dVout = Iout * D / (C * fsw) |

Procedure:

1. Compute D and IL_avg for every Vin (9 V, 12 V, 15 V) at Iout = 2 A.
2. Inductor: `L_min = Vin * D / (0.3 * IL_avg * fsw)` (HW-BST-04, ripple <= 30 % of IL_avg). Evaluate it at every Vin at 2 A and take the maximum.
3. Capacitor: `C_min = Iout * D / (dVout_max * fsw)` with dVout_max = 240 mV (HW-BST-03). D and Iout both grow toward the worst case: find the Vin and Iout that give the largest C_min and use it.
4. Margin and E6 (HW-BST-07): multiply each minimum by 1.1, then pick the smallest E6 value (1.0 1.5 2.2 3.3 4.7 6.8, times 10^n) that is >= that product. One L and one C for all six corners.
5. Recompute dIL and dVout with the chosen E6 values at every corner and check them against the limits before writing netlists.

Per corner, also compute the values that go into the netlist:

| Value | Formula |
|---|---|
| `IL0` | IL_avg of that corner |
| `VOUT0` | 24 V (initial output voltage) |
| `D_TON` | D * T - 1 ns |

`D_TON` is the pulse width of the gate drive. The 1 ns edges make the switch conduct about 1 ns longer than the flat width, so subtract 1 ns. Keep at least 4 significant digits (for example write `2.499u`, not `2.5u`, if D * T = 2.5 us).

## 2. Fill the bench template

Copy `bench.cir` and replace only these placeholders, in every occurrence:

| Placeholder | Meaning | Unit and ngspice suffix |
|---|---|---|
| `<VIN>` | input voltage of the corner | V, plain number (`12`) |
| `<D_TON>` | gate pulse width | s, with suffix (`u` for us, `n` for ns), for example `3.124u` |
| `<L>` | inductance | H, `u` for uH, for example `33u` |
| `<C>` | capacitance | F, `u` for uF, for example `33u` |
| `<RLOAD>` | load resistance of the corner | ohm, plain number (`12`, `120`) |
| `<IL0>` | initial inductor current | A, plain number |
| `<VOUT0>` | initial output voltage | V, plain number |

Rules:

- Write one netlist per corner to `/workspace/.input/boost_design/<corner>.cir` (corners: `v09_l02 v09_l20 v12_l02 v12_l20 v15_l02 v15_l20`).
- No placeholder (`<...>`) may remain in the file. Do not change any other line.
- Suffixes: `u` = 1e-6, `m` = 1e-3, `n` = 1e-9, `Meg` = 1e6. `M` alone means milli in ngspice: never use it for mega.
- The `spice-safe` policy refuses a `.control` block, `.include` and `.lib`. The bench has none; do not add any.

## 3. Run ngspice

One command per corner, one new exec id each:

```
run_ephemeral_container tool ngspice --exec-id <id> --in netlist=/workspace/.input/boost_design/<corner>.cir
```

- Take an exec id that does not exist in `/workspace/.output/exec/` (list it first). A used id is refused with `exec dir for --exec-id <id> already exists` (exit 125, cause `usage`). Any run that created its exec dir has used its id, whether it passed or failed.
- Exec ids are `bst<N>`. Pick the smallest base B >= 1 such that `bst<B>` to `bst<B+5>` are all absent from `/workspace/.output/exec/`, then use N = B + corner index (corner order `v09_l02 v09_l20 v12_l02 v12_l20 v15_l02 v15_l20`, index 0 to 5). If a run is refused with "exec dir already exists", choose a new B > N with `bst<B>` to `bst<B+5-index>` absent and rerun that corner once.
- Single command only: no pipe, no `;`, no `&&`, no redirect.
- Results: `/workspace/.output/exec/<id>/result.json` (`exit`, `cause`), `out/sim.log`, `out/sim.raw`, `out/sim_raw.log` (waveform run log), `stderr.log`. Success is `"exit": 0` and `"cause": "ok"`.

## 4. Read the results

`.meas` results are lines in `out/sim.log`, one per measure. Illustrative sample line (the numbers differ per run):

```
vout_avg             =  2.40012e+01 from=  9.00000e-04 to=  1.00000e-03
```

Find the five measures with:

```
grep -E "^(vout_avg|vout_avg_prev|vout_pp|il_avg|il_pp)" /workspace/.output/exec/<id>/out/sim.log
```

`vout_avg` is a prefix of `vout_avg_prev`, so the grep returns both lines. A parser must split each line on `=`, strip the name on the left, match it exactly (`vout_avg` is not `vout_avg_prev`), and tolerate variable whitespace around the name and the `=`. A measure that ngspice could not evaluate prints `failed` instead of a value: treat it as missing.

Waveform values at chosen times (never read `sim.raw` directly):

```
python3 -I /workspace/src/ngspice/readraw.py /workspace/.output/exec/<id>/out/sim.raw --at 0.0009,0.00095,0.001
```

Other options: `--all` (every row), `--svg <out.svg> --vars <name>[,<name>...]` (plot; for example `--vars 'v(out)'`). The inductor current is `i(vsense)`.

## 5. Judge the requirements

From the measured values only. Per corner:

| Requirement | Check |
|---|---|
| HW-BST-02 | 23.52 V <= `vout_avg` <= 24.48 V |
| HW-BST-03 | `vout_pp` <= 0.240 V |
| HW-BST-04 | At Iout = 2 A corners: `il_pp / il_avg` <= 0.30. At 0.2 A corners: record the ratio only |
| HW-BST-08 | abs(`vout_avg` - `vout_avg_prev`) <= 0.120 V |

Design-level checks (no simulation needed): HW-BST-01 and -05 hold by construction when the six corners are simulated as listed; HW-BST-06 is fixed in the bench; HW-BST-07 is judged from the calculation (E6 value, >= 1.1 x minimum, same L and C in all six netlists).

A requirement "at every corner" passes only if all six corners pass. If any fails, redo the sizing (section 1) and say which corner and which measure failed, with the numbers.

## 6. Common failures

| Symptom | Likely cause and fix |
|---|---|
| `vout_avg` far from 24 V and `vout_avg` differs from `vout_avg_prev` (run not settled) | Initial conditions wrong. Check `IL0` = IL_avg of the corner and `VOUT0` = 24 V; check they replaced the right placeholders and the IC lines are intact. A wrong duty width also shifts the settled value: recheck `D_TON`. |
| `vout_avg` off by a few percent but settled | Duty width rule: `D_TON` must be D * T - 1 ns, with enough digits. Recompute; do not round to 2 digits. |
| Exit 125 with `policy: spice-safe` (cause `policy`) | Netlist contains `.control`, `.include` or `.lib`, or a forbidden construct. Restore the netlist from `bench.cir` and fill only the placeholders. |
| Exit 125, cause `usage`, "exec dir ... already exists" | Reused exec id. Pick a new id. |
| ngspice error in `stderr.log` (for example "unknown unit", "syntax") | A suffix or value was written wrongly (`M` for mega, missing `u`, leftover `<...>`). Fix the value and rerun with a new exec id. |
| A `.meas` line missing or `failed` in `sim.log` | The simulation did not run to 1 ms or a node name was changed. Check `stderr.log` and the `.tran` line; do not edit the bench lines, restore from `bench.cir`. |
| `il_pp / il_avg` > 30 % at 2 A | L too small, or wrong E6 choice. Recompute section 1 step 2 at every Vin. |
| `vout_pp` > 240 mV | C too small. Recompute section 1 step 3 and the 1.1 x margin. |
