// SFR-NET-012 (USC-004 alt-2): report the outcome to the calling tool on stdout; exit 0.
package steps

import (
	"context"
	"fmt"

	"agent-net/internal/config"
)

func ReportDisarmed(_ context.Context, env *Env) *Failure {
	if env.Disarm.Deleted == 0 {
		env.Log.Success(fmt.Sprintf("container %s: nothing to disarm on %s", env.Container, config.NetworkName))
		return nil
	}
	env.Log.Success(fmt.Sprintf("container %s disarmed on %s (%d tables, rules and entries removed)",
		env.Container, config.NetworkName, env.Disarm.Deleted))
	return nil
}
