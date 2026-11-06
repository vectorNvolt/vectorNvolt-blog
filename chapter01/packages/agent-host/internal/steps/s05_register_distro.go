// SFR-HOST-004 (USC-001 step 5): register agent-host as a WSL2 distro from the verified rootfs.
package steps

import (
	"context"
	"os"
)

func RegisterDistro(ctx context.Context, env *Env) *Failure {
	if err := os.MkdirAll(env.Config.InstallDir, 0o755); err != nil {
		return fail(ExitRuntime, StateNotCreated, "install directory not writable", err)
	}
	if err := env.WSL.Import(ctx, env.Config.InstallDir, env.RootfsPath); err != nil {
		return fail(ExitRuntime, StateNotCreated, "distro import failed", err)
	}
	return nil
}
