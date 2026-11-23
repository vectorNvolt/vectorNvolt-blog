---
id: boost-process
layer: process
tags: [rules, boost]
auto: false
---
Rules for the boost-converter design process.

- Before you start any step, read `.claude/skills/boost-converter-design/SKILL.md` in your working directory and follow it.
- Units: SI, always with an explicit suffix in text and tables (V, A, H, F, Hz, s, ohm). In netlists use ngspice suffixes: `u` micro, `m` milli, `n` nano, `Meg` mega. Never write a bare `M` (ngspice reads it as milli) and never mix units in one cell.
- Never edit `bench.cir` or `HW_REQ.md`. They are the fixed bench and the fixed requirements. A netlist is a copy of `bench.cir` with only its placeholders replaced.
- Write netlists only to `/workspace/.input/boost_design/<corner>.cir`, one file per corner.
- Run ngspice only through `run_ephemeral_container tool ngspice`. Never call ngspice directly, never use a shell wrapper.
- Never reuse an exec id. Exec ids are `bst<N>`: take the smallest base B >= 1 such that `bst<B>` to `bst<B+5>` are all absent from `/workspace/.output/exec/`, and use N = B + corner index (0 to 5, corner order `v09_l02 v09_l20 v12_l02 v12_l20 v15_l02 v15_l20`). Every run, including reruns after a failure, gets a new unused id.
- Read `sim.raw` only through `readraw.py`. Never read it with a text tool.
- Report every number together with the measure or calculation it came from (for example "vout_avg = 24.1 V from exec bst3, corner v12_l20").
- Never invent, estimate or round into a result. If a measure is missing, say so and treat the requirement as not verified.
- Judge a requirement only from the measured values of the simulation, not from the design calculation.
