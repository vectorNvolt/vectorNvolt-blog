#!/usr/bin/env bash
# Piped to `bash -s -- <gvisor-key-fingerprint>` as root inside agent-host (SFR-HOST-008). Touches nothing outside the distro.
# Runtime registration (daemon.json, runsc path, docker restart) is a separate step (SFR-HOST-012).
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive

die() { echo "install_deps: $*" >&2; exit 1; }

# Everything runs inside a function so that bash has parsed it completely before
# stdin (the script itself) is redirected away from the commands below.
main() {
  local want="${1:-}" tmp pubs
  [ -n "$want" ] || die "gVisor key fingerprint argument missing"

  # The distro was just restarted by `wsl.exe --terminate`; wait for systemd.
  systemctl is-system-running --wait || true

  apt-get update || die "apt-get update (base) failed"
  apt-get install -y docker.io docker-buildx python3 gnome-keyring git tar nftables iptables iproute2 ca-certificates curl gnupg \
    || die "apt-get install of base dependencies failed"

  # gVisor apt repository, see https://gvisor.dev/docs/user_guide/install/
  install -d -m 0755 /usr/share/keyrings || die "creating /usr/share/keyrings failed"
  tmp="$(mktemp)" || die "creating temp file for the gVisor key failed"
  trap "rm -f '$tmp'" EXIT
  curl -fsSL https://gvisor.dev/archive.key -o "$tmp" || die "downloading the gVisor signing key failed"

  # Verify the pinned fingerprint BEFORE the key is trusted; fail closed on any mismatch.
  pubs="$(gpg --batch --show-keys --with-colons --fingerprint "$tmp" 2>/dev/null \
    | awk -F: '$1=="pub"{p=1;next} $1=="fpr"&&p{print toupper($10);p=0}')" \
    || die "reading the gVisor signing key fingerprint failed"
  [ "$pubs" = "$want" ] || die "gVisor signing key fingerprint mismatch: expected $want, got ${pubs:-<none>}"

  gpg --batch --dearmor --yes -o /usr/share/keyrings/gvisor-archive-keyring.gpg "$tmp" \
    || die "dearmoring the gVisor signing key failed"
  echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/gvisor-archive-keyring.gpg] https://storage.googleapis.com/gvisor/releases release main" \
    > /etc/apt/sources.list.d/gvisor.list || die "writing the gVisor apt source failed"
  apt-get update || die "apt-get update (gVisor repository) failed"
  apt-get install -y runsc || die "apt-get install of runsc failed"

  systemctl enable --now docker || die "enabling docker failed"
}

main "$@" </dev/null
