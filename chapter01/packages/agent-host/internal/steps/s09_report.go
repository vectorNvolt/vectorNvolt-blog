// SFR-HOST-010 (USC-001 step 9): report success on stdout (exit 0 is returned by the sequence).
package steps

import (
	"context"
	"fmt"
	"strings"

	"agent-host/internal/config"
)

func ReportSuccess(ctx context.Context, env *Env) *Failure {
	// The done marker makes a rerun refuse at preflight.
	if _, err := env.Host.Exec(ctx, []byte(config.DoneMarkerContent), WriteFileCmd(config.DoneMarkerPath, "644")...); err != nil {
		return fail(ExitRuntime, StatePartiallyConfigured, "writing "+config.DoneMarkerPath+" failed", err)
	}
	env.Log.Success(
		fmt.Sprintf("agent-host created and verified: host %s, default user %s", config.HostName, config.User),
		"installed on the host: docker, gVisor (runsc), python3, gnome-keyring, git, tar, nftables, iptables, iproute2, ca-certificates, curl, gnupg",
		fmt.Sprintf("tools installed in %s: %s", config.ToolInstallDir, strings.Join(config.ToolNames, ", ")),
	)
	return nil
}
