// SFR-HOST-008 (USC-001 step 9): first wait for systemd and write /etc/resolv.conf (see InstallDeps), then install Docker, gVisor, Python 3, gnome-keyring, git, tar, nftables, iptables, iproute2
// inside agent-host only. Runtime registration is a separate step (RegisterRuntime).
package steps

import (
	"context"
	_ "embed"
	"fmt"
	"regexp"
	"strings"

	"agent-host/internal/config"
)

//go:embed scripts/install_deps.sh
var installDepsScript []byte

var fingerprintRe = regexp.MustCompile(`^[0-9A-F]{40}$`)

// RemoveLeftoverDriveDirsCmd: the first boot (before wsl.conf disables automount) creates empty /mnt/<drive> dirs; the spec requires /mnt/c absent.
var RemoveLeftoverDriveDirsCmd = []string{"/bin/sh", "-c",
	`rc=0; for d in /mnt/?; do [ -d "$d" ] || continue; /usr/bin/mountpoint -q "$d" && continue; /bin/rmdir "$d" || rc=1; done; exit $rc`}

// WaitSystemdCmd waits (at most 120 s) for systemd to finish starting. It always exits 0: a healthy-enough system can
// report "degraded" (user@0.service can fail here because of a shared cgroup) and is-system-running exits non-zero then.
var WaitSystemdCmd = []string{"/bin/sh", "-c", `timeout 120 systemctl is-system-running --wait >/dev/null 2>&1; exit 0`}

func InstallDeps(ctx context.Context, env *Env) *Failure {
	if _, err := env.WSL.Exec(ctx, "root", nil, WaitSystemdCmd...); err != nil {
		return fail(ExitRuntime, StatePartiallyConfigured, "waiting for systemd failed", err)
	}
	// Something removes a /etc/resolv.conf written before this boot (step 7); the remover is unidentified. A file written
	// after this boot survives restarts. Step 12 verifies it. mv -f in WriteFileCmd replaces a symlink at the destination
	// (/etc/resolv.conf may link to /mnt/wsl/resolv.conf) with this write.
	if _, err := env.WSL.Exec(ctx, "root", []byte(ResolvConf), WriteFileCmd(config.ResolvConfPath, "644")...); err != nil {
		return fail(ExitRuntime, StatePartiallyConfigured, "writing "+config.ResolvConfPath+" failed", err)
	}
	fpr := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(env.Config.GVisorKeyFingerprint), " ", ""))
	if fpr == "" {
		return &Failure{ExitCode: ExitRuntime, ExitState: StatePartiallyConfigured,
			Message: "gVisor apt signing key fingerprint is not pinned: set AGENT_HOST_GVISOR_KEY_FINGERPRINT (40 hex characters); refusing to trust the gVisor repository"}
	}
	if !fingerprintRe.MatchString(fpr) {
		return &Failure{ExitCode: ExitRuntime, ExitState: StatePartiallyConfigured,
			Message: fmt.Sprintf("gVisor apt signing key fingerprint %q is not 40 hex characters", env.Config.GVisorKeyFingerprint)}
	}
	if _, err := env.WSL.Exec(ctx, "root", nil, RemoveLeftoverDriveDirsCmd...); err != nil {
		return fail(ExitRuntime, StatePartiallyConfigured, "removing leftover drive mount directories failed (a /mnt/<drive> dir is mounted or not empty)", err)
	}
	if _, err := env.WSL.Exec(ctx, "root", installDepsScript, "/bin/bash", "-s", "--", fpr); err != nil {
		return fail(ExitRuntime, StatePartiallyConfigured, "dependency installation failed", err)
	}
	return nil
}
