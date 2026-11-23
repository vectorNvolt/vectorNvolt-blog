#!/bin/bash
# Regenerates vm/agent-host.lima.yaml and vm/agent-host.cloud-init.yaml from the
# *.in templates by embedding vm/foundation.sh at the @@FOUNDATION@@ line
# (indented like the placeholder).
#   vm/generate.sh          regenerate in place
#   vm/generate.sh --check  exit 1 if a committed file is stale
set -euo pipefail

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
targets=(agent-host.lima.yaml agent-host.cloud-init.yaml)

render() {
  awk -v f="$here/foundation.sh" '
    /^[ ]*@@FOUNDATION@@[ ]*$/ {
      ind = $0
      sub(/@@FOUNDATION@@.*/, "", ind)
      while ((getline line < f) > 0) {
        if (line == "") print ""; else print ind line
      }
      close(f)
      next
    }
    { print }
  ' "$1" > "$2"
}

if [ "${1:-}" = "--check" ]; then
  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT
  stale=0
  for t in "${targets[@]}"; do
    render "$here/$t.in" "$tmp/$t"
    if ! diff -u "$here/$t" "$tmp/$t"; then
      echo "STALE: vm/$t is out of date" >&2
      stale=1
    fi
  done
  if [ "$stale" -ne 0 ]; then
    echo "Run vm/generate.sh and commit the result." >&2
    exit 1
  fi
  echo "vm/ generated files are up to date"
else
  for t in "${targets[@]}"; do
    render "$here/$t.in" "$here/$t"
  done
fi
