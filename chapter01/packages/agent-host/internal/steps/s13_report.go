// SFR-HOST-010 (USC-001 step 13): report success on stdout (exit 0 is returned by the sequence).
package steps

import (
	"context"
	"fmt"
	"strings"

	"agent-host/internal/config"
)

func ReportSuccess(_ context.Context, env *Env) *Failure {
	env.Log.Success(
		fmt.Sprintf("agent-host created and verified: distro %s (WSL 2), default user %s", config.DistroName, config.User),
		fmt.Sprintf("install directory: %s", env.Config.InstallDir),
		"installed inside the distro: docker, gVisor (runsc), python3, gnome-keyring, git, tar, nftables, iptables, iproute2, ca-certificates, curl, gnupg",
		fmt.Sprintf("tools installed in %s: %s", config.ToolInstallDir, strings.Join(config.ToolNames, ", ")),
	)
	return nil
}
