// SFR-HOST-008 (USC-001 step 5): wait for systemd, then install Docker, gVisor, Python 3, gnome-keyring, git, tar, nftables, iptables, iproute2
// on the host. Runtime registration is a separate step (RegisterRuntime).
package steps

import (
	"context"
	_ "embed"
	"fmt"
	"regexp"
	"strings"
	"time"

	"agent-host/internal/config"
)

// NoDockerUpgradeCfg stops unattended-upgrades from upgrading (and restarting) Docker and its runtime.
const NoDockerUpgradeCfg = `Unattended-Upgrade::Package-Blacklist {
    "docker.io";
    "docker-buildx";
    "containerd";
    "runc";
    "runsc";
};
`

//go:embed scripts/install_deps.sh
var installDepsScript []byte

var fingerprintRe = regexp.MustCompile(`^[0-9A-F]{40}$`)

// WaitSystemdCmd waits (at most 120 s) for systemd to finish starting. It always exits 0: a healthy-enough system can
// report "degraded" and is-system-running exits non-zero then.
var WaitSystemdCmd = []string{"/bin/sh", "-c", `timeout 120 systemctl is-system-running --wait >/dev/null 2>&1; exit 0`}

func InstallDeps(ctx context.Context, env *Env) *Failure {
	start := time.Now()
	env.Log.Progress("waiting for systemd to finish starting (up to 120 s)")
	if _, err := env.Host.Exec(ctx, nil, WaitSystemdCmd...); err != nil {
		return fail(ExitRuntime, StatePartiallyConfigured, "waiting for systemd failed", err)
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
	onLine := func(stream, line string) {
		if rest, ok := strings.CutPrefix(line, "progress: "); ok {
			d := int(time.Since(start).Seconds())
			env.Log.Progress(fmt.Sprintf("%s (elapsed %02d:%02d since step start)", rest, d/60, d%60))
			return
		}
		env.Log.Debugf("%s: %s", stream, line)
	}
	if _, err := env.Host.ExecProgress(ctx, installDepsScript, onLine, "/bin/bash", "-s", "--", fpr); err != nil {
		return fail(ExitRuntime, StatePartiallyConfigured, "dependency installation failed", err)
	}
	if _, err := env.Host.Exec(ctx, []byte(NoDockerUpgradeCfg), WriteFileCmd(config.NoDockerUpgradeCfgPath, "644")...); err != nil {
		return fail(ExitRuntime, StatePartiallyConfigured, "writing "+config.NoDockerUpgradeCfgPath+" failed", err)
	}
	return nil
}
