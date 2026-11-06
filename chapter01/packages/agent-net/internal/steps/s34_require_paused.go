// SFR-NET-008 precondition (USC-004 alt-1): the container must have been created with --network agents-net,
// --restart no and the runsc runtime (docker inspect .HostConfig.NetworkMode, .NetworkSettings.Networks,
// .HostConfig.RestartPolicy.Name exactly "no" and .HostConfig.Runtime),
// so it is attached to agents-net and nothing else, and must be running and paused (.State.Running and .State.Paused)
// before any port control is installed, because the paused container is what keeps it from sending a frame while its
// port is not yet closed (the gate of agents-net drops the frames of an unarmed port too). Reads only; nothing is
// changed. A container that is not as required is a precondition failure of `arm`: GATED_UNARMED, exit 1, same as a
// missing agents-net.
package steps

import (
	"context"
	"fmt"

	"agent-net/internal/config"
)

// ctlPaused names the control "the container stays paused during the whole arming run" (SFR-NET-008).
const ctlPaused = "container paused"

func RequirePaused(ctx context.Context, env *Env) *Failure {
	state := notDone(env)
	info, found, err := env.Docker.LookupContainer(ctx, env.Container)
	if err != nil {
		return fail(ExitPrerequisite, state, "cannot inspect container "+env.Container, err)
	}
	if !found {
		return &Failure{ExitCode: ExitPrerequisite, ExitState: state, Message: fmt.Sprintf("container %s does not exist", env.Container)}
	}
	// The sandbox passes `--restart no` explicitly, so docker reports exactly "no"; an empty policy is refused.
	mode, restart, runtime := info.HostConfig.NetworkMode, info.HostConfig.RestartPolicy.Name, info.HostConfig.Runtime
	_, onNet := info.NetworkSettings.Networks[config.NetworkName]
	if mode != config.NetworkName || !onNet || len(info.NetworkSettings.Networks) != 1 || restart != "no" || runtime != config.SandboxRuntime {
		return &Failure{
			ExitCode:  ExitPrerequisite,
			ExitState: state,
			Message: fmt.Sprintf("container %s was not created with --network %s --restart no --runtime %s (network mode %q, %d network(s) attached, restart policy %q, runtime %q)",
				env.Container, config.NetworkName, config.SandboxRuntime, mode, len(info.NetworkSettings.Networks), restart, runtime),
		}
	}
	if !info.State.Running || !info.State.Paused {
		return &Failure{
			ExitCode:  ExitPrerequisite,
			ExitState: state,
			Message: fmt.Sprintf("container %s is not paused (running=%v, paused=%v); start and pause it (docker pause) before arming",
				env.Container, info.State.Running, info.State.Paused),
		}
	}
	return nil
}

// keepPaused re-reads the container during the arming run and stops it (failed control, GATED_UNARMED) when the
// container is no longer running and paused. Once the port table exists it stays closed.
func keepPaused(ctx context.Context, env *Env, when string) *Failure {
	info, found, err := env.Docker.LookupContainer(ctx, env.Container)
	switch {
	case err != nil:
		return failControl(ctlPaused, "container "+env.Container+" unreadable "+when, err)
	case !found:
		return failControl(ctlPaused, fmt.Sprintf("container %s no longer exists %s", env.Container, when), nil)
	case !info.State.Running || !info.State.Paused:
		return failControl(ctlPaused, fmt.Sprintf("container %s is no longer running and paused %s (running=%v, paused=%v)",
			env.Container, when, info.State.Running, info.State.Paused), nil)
	}
	return nil
}
