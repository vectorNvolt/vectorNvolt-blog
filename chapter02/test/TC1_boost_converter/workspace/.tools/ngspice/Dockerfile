# Manual-test image for the ngspice tool profile (tools/ngspice/profile.yml). Tag: exec-ngspice:1
# Like exec-python:1 it has no USER and no ENTRYPOINT/CMD: the engine sets the user and the
# profile's argv runs `ngspice-run`, which runs ngspice twice (ngspice skips every .meas in
# batch mode when -r is set, so measures and waveforms come from separate runs).
FROM ubuntu:26.04
RUN apt-get update \
 && apt-get install -y --no-install-recommends ngspice \
 && rm -rf /var/lib/apt/lists/*
RUN printf '%s\n' \
  '#!/bin/sh' \
  '# ngspice-run <outdir> <netlist>' \
  '# Run 1 writes .meas results to sim.log (ngspice skips .meas when -r is set).' \
  '# Run 2 writes the waveforms to sim.raw; its own log goes to sim_raw.log.' \
  '# Exit: first non-zero ngspice exit code, else 0.' \
  '[ $# -eq 2 ] || { echo "usage: ngspice-run <outdir> <netlist>" >&2; exit 2; }' \
  'out=$1; net=$2' \
  'ngspice -b -n -o "$out/sim.log" "$net"; rc1=$?' \
  'ngspice -b -n -r "$out/sim.raw" -o "$out/sim_raw.log" "$net"; rc2=$?' \
  '[ "$rc1" -ne 0 ] && exit "$rc1"' \
  'exit "$rc2"' \
  > /usr/local/bin/ngspice-run \
 && chmod 755 /usr/local/bin/ngspice-run
