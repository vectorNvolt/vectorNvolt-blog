// SFR-RTC-001/002 (USC-002 step 2, err-2): the engine must register runsc; report a mismatch, never repair it.
package steps

import (
	"context"
	"fmt"

	"agent-sandbox/internal/config"
)

func CheckRuntime(ctx context.Context, env *Env) *Failure {
	ok, err := env.Docker.RuntimeRegistered(ctx, config.Runtime)
	if err != nil {
		return fail(ExitPrerequisite, StateNotCreated, "docker engine unavailable or runtime list unreadable", err)
	}
	if !ok {
		return &Failure{
			ExitCode:  ExitPrerequisite,
			ExitState: StateNotCreated,
			Message: fmt.Sprintf("runtime mismatch: %q is not registered with Docker (daemon.json); "+
				"this tool does not read or modify daemon.json", config.Runtime),
		}
	}
	return nil
}
