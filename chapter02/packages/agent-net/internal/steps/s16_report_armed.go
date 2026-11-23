// SFR-NET-010 (USC-004 alt-1): report success to the calling tool on stdout once the port is released and verified;
// exit 0. The calling tool then unpauses the container.
package steps

import (
	"context"
	"fmt"

	"agent-net/internal/config"
)

func ReportArmed(_ context.Context, env *Env) *Failure {
	env.Log.Success(fmt.Sprintf("container %s armed on %s (veth %s, ip %s, table %s %s, port released)",
		env.Container, config.NetworkName, env.Port.Veth, env.Port.IP, config.NftFamily, env.PortTable()))
	return nil
}
