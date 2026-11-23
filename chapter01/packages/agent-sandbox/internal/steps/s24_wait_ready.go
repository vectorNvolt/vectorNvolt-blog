// Readiness wait (create, after argument validation, before the agent lock and any docker call): <runtime dir>/ready must
// exist and hold the current dockerd MainPID, i.e. `agent_lifecycle restore` rebuilt the agents-net gate since this docker
// start. It runs before the agent lock is taken because the restore's step 4 (remove agent containers) needs that lock and
// writes the marker only afterwards: waiting while holding the lock would deadlock both until the timeout.
// (Residual window: docker may restart after the lock is taken and make the marker stale; it is not re-checked.)
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
