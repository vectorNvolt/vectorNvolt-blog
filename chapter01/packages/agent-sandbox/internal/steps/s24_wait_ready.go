// Readiness wait (create, after argument validation, before any docker call): <runtime dir>/ready must exist and hold
// the current dockerd MainPID, i.e. `agent_lifecycle restore` rebuilt the agents-net gate since this docker start.
package steps

import (
	"context"
	"fmt"

	"agent-sandbox/internal/services"
)

func WaitNetReady(ctx context.Context, env *Env) *Failure {
	if err := services.WaitReady(ctx, env.Runner, env.Config.RuntimeDir, env.Config.ReadyTimeout); err != nil {
		return &Failure{
			ExitCode:  ExitNetNotReady,
			ExitState: StateNetNotReady,
			Message: fmt.Sprintf("the agent network is not ready after %s: %v\n"+
				"check `systemctl status agent-restore.service` (it runs `agent_lifecycle restore` after each docker start)",
				env.Config.ReadyTimeout, err),
		}
	}
	return nil
}
