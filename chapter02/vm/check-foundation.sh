#!/bin/bash
# Asserts foundation contract items 1-7. Same output on every platform:
#   PASS|FAIL <n> <description>
# Run inside the VM as the login user:  bash -s < vm/check-foundation.sh
# Pinned Go version must match GO_VERSION in vm/foundation.sh.
# Checks the bare foundation: run it BEFORE create_agent_host. Item 7 (no docker)
# fails by design once create_agent_host has installed Docker.
set -uo pipefail

GO_VERSION=1.27.4
fail=0

run() {
  local n=$1 desc=$2
  shift 2
  if "$@" >/dev/null 2>&1 </dev/null; then
    echo "PASS $n $desc"
  else
    echo "FAIL $n $desc"
    fail=1
  fi
}

item1() {
  . /etc/os-release
  [ "$ID" = ubuntu ] && [ "$VERSION_ID" = 26.04 ] || return 1
  case "$(dpkg --print-architecture)" in amd64 | arm64) ;; *) return 1 ;; esac
}

item2() {
  local state
  state=$(systemctl is-system-running || true)
  [ -n "$state" ] && [ "$state" != offline ] || return 1
  cloud-init status --wait | grep -q 'status: done'
}

item3() {
  sudo -n true || return 1
  local cfg
  cfg=$(sudo -n sshd -T) || return 1
  echo "$cfg" | grep -qx 'passwordauthentication no' || return 1
  echo "$cfg" | grep -qx 'pubkeyauthentication yes'
}

item4() {
  local p
  for p in git ca-certificates curl build-essential; do
    dpkg -s "$p" || return 1
  done
}

item5() {
  /usr/local/go/bin/go version | grep -q "go${GO_VERSION} "
}

item6() {
  grep -qx 'FOUNDATION_VERSION=1' /etc/agent-foundation
}

item7() {
  local b
  for b in docker dockerd containerd containerd-shim-runc-v2; do
    command -v "$b" && return 1
  done
  systemctl list-unit-files 'docker*' 'containerd*' | grep -Eq '^(docker|containerd)' && return 1
  awk '$3 ~ /^(virtiofs|9p|sshfs|fuse\.sshfs|fuse\.reverse-sshfs)$/ { found = 1 } END { exit found }' /proc/mounts
}

run 1 "Ubuntu 26.04 (amd64 or arm64)" item1
run 2 "systemd running and cloud-init done" item2
run 3 "passwordless sudo; sshd key-only login" item3
run 4 "git ca-certificates curl build-essential installed" item4
run 5 "Go ${GO_VERSION} in /usr/local/go" item5
run 6 "marker /etc/agent-foundation FOUNDATION_VERSION=1" item6
run 7 "no docker/containerd, no host mounts" item7

exit "$fail"
