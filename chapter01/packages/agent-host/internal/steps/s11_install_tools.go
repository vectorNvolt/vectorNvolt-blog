// SFR-HOST-011 / SSR-HOST-007 (USC-001 step 11): install the agent tools into /usr/local/bin as root:root 755
// and compare the SHA-256 computed inside agent-host with the reference hash from step 3; then install the
// agent-restore and agent-guard systemd units (lifecycle_units.go); seed /etc/agent-net/endpoints.json through
// refresh_endpoints (endpoints.go); finally the Windows keep-alive (keepalive.go).
package steps

import (
	"context"
	"fmt"
	"strings"

	"agent-host/internal/config"
)

func InstallTools(ctx context.Context, env *Env) *Failure {
	if _, err := env.WSL.Exec(ctx, "root", nil, SecureToolDirCmd...); err != nil {
		return fail(ExitRuntime, StatePartiallyConfigured, "securing "+config.ToolInstallDir+" (root:root 755) failed", err)
	}
	for _, name := range config.ToolNames {
		dst := config.ToolInstallDir + "/" + name
		if _, err := env.WSL.Exec(ctx, "root", env.ToolBuilds[name], WriteFileCmd(dst, "755")...); err != nil {
			return fail(ExitRuntime, StatePartiallyConfigured, "installing tool "+name+" failed", err)
		}
		out, err := env.WSL.Exec(ctx, "root", nil, "/usr/bin/sha256sum", dst)
		if err != nil {
			return fail(ExitRuntime, StatePartiallyConfigured, "computing SHA-256 of installed tool "+name+" failed", err)
		}
		var got string
		if f := strings.Fields(string(out)); len(f) > 0 {
			got = strings.ToLower(f[0])
		}
		if want := env.ToolHashes[name]; got != want {
			return &Failure{
				ExitCode:  ExitRuntime,
				ExitState: StatePartiallyConfigured,
				Message:   fmt.Sprintf("tool %s: SHA-256 mismatch: expected %s, actual %s", name, want, got),
			}
		}
	}
	if f := InstallLifecycleUnits(ctx, env); f != nil {
		return f
	}
	if f := SeedEndpoints(ctx, env); f != nil {
		return f
	}
	return InstallKeepAlive(ctx, env)
}
