// SFR-AGC-021, SSR-AGC-002 (USC-002 step 7, err-11): docker unpause only after the arming read-back passed.
// A failed unpause never removes the armed container: still paused -> CONTAINED, anything else -> contain as err-12.
package steps

import (
	"context"
	"fmt"
)

func ReleaseContainer(ctx context.Context, env *Env) *Failure {
	err := env.Docker.Unpause(ctx, env.Container)
	if err == nil {
		return nil
	}
	if info, ierr := env.Docker.Inspect(context.WithoutCancel(ctx), env.Container); ierr == nil && info.State.Paused {
		return &Failure{
			ExitCode:  ExitRuntime,
			ExitState: StateContained,
			Message: fmt.Sprintf("container unpause failed: %v\ncontainer %s is left paused, armed and unreleased (not removed); run remove_agent_container %s",
				err, env.Container, env.AgentID),
		}
	}
	state, note := containFailure(ctx, env)
	return &Failure{
		ExitCode:  ExitRuntime,
		ExitState: state,
		Message:   fmt.Sprintf("container unpause failed and its paused state is not confirmed: %v\n%s", err, note),
	}
}
