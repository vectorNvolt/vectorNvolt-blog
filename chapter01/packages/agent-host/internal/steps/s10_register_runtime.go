// SFR-HOST-012 / SSR-HOST-006 (USC-001 step 10): register runsc via `runsc install` in /etc/docker/daemon.json and restart docker.
package steps

import (
	"context"

	"agent-host/internal/config"
)

// SecureToolDirCmd makes /usr/local/bin root:root 755 (not writable by agent-operator).
var SecureToolDirCmd = []string{"/bin/sh", "-c", "chown root:root " + config.ToolInstallDir + " && chmod 755 " + config.ToolInstallDir}

// RunscInstallCmd is `runsc install --runtime=runsc -- <the eight runtimeArgs of SSR-HOST-006>`.
func RunscInstallCmd() []string {
	cmd := []string{config.RunscPath, "install", "--runtime=runsc", "--"}
	return append(cmd, config.RunscRuntimeArgs...)
}

func RegisterRuntime(ctx context.Context, env *Env) *Failure {
	if _, err := env.WSL.Exec(ctx, "root", nil, SecureToolDirCmd...); err != nil {
		return fail(ExitRuntime, StatePartiallyConfigured, "securing "+config.ToolInstallDir+" (root:root 755) failed", err)
	}
	// Copy (not symlink) the packaged binary to the root-owned path used in daemon.json.
	if _, err := env.WSL.Exec(ctx, "root", nil, "/usr/bin/install", "-o", "root", "-g", "root", "-m", "755", config.RunscPkgPath, config.RunscPath); err != nil {
		return fail(ExitRuntime, StatePartiallyConfigured, "installing "+config.RunscPath+" failed", err)
	}
	// Register through `runsc install`; the entry path is the executed binary (/usr/local/bin/runsc).
	if _, err := env.WSL.Exec(ctx, "root", nil, RunscInstallCmd()...); err != nil {
		return fail(ExitRuntime, StatePartiallyConfigured, "runsc install (runsc runtime registration in "+config.DaemonJSONPath+") failed", err)
	}
	if _, err := env.WSL.Exec(ctx, "root", nil, "/bin/sh", "-c", "chown root:root "+config.DaemonJSONPath+" && chmod 644 "+config.DaemonJSONPath); err != nil {
		return fail(ExitRuntime, StatePartiallyConfigured, "securing "+config.DaemonJSONPath+" (root:root 644) failed", err)
	}
	if _, err := env.WSL.Exec(ctx, "root", nil, "/usr/bin/systemctl", "restart", "docker"); err != nil {
		return fail(ExitRuntime, StatePartiallyConfigured, "restarting docker after runsc registration failed", err)
	}
	return nil
}
