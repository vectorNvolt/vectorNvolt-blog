// SFR-AGC-018 (USC-006 step 6, err-6): read the engine again; agent-<id> must be gone.
package steps

import (
	"context"
	"fmt"
)

func VerifyRemoved(ctx context.Context, env *Env) *Failure {
	if env.Absent {
		return nil
	}
	exists, err := env.Docker.ContainerExists(ctx, env.Container)
	if err != nil {
		return fail(ExitRuntime, StateNotRemoved, "container removal could not be verified", err)
	}
	if exists {
		return &Failure{
			ExitCode:  ExitRuntime,
			ExitState: StateRemoveFailed,
			Message:   fmt.Sprintf("container %s still exists after removal", env.Container),
		}
	}
	return nil
}
