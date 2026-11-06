// SFR-AGC-002 (USC-002 step 2, err-3): refuse when agent-<id> exists in any state; leave it untouched.
package steps

import (
	"context"
	"fmt"
)

func RefuseDuplicate(ctx context.Context, env *Env) *Failure {
	exists, err := env.Docker.ContainerExists(ctx, env.Container)
	if err != nil {
		return fail(ExitRuntime, StateNotCreated, "cannot check for an existing container", err)
	}
	if exists {
		return &Failure{
			ExitCode:  ExitPrerequisite,
			ExitState: StateNotCreated,
			Message:   fmt.Sprintf("container %s already exists; it is not reused and not modified", env.Container),
		}
	}
	return nil
}
