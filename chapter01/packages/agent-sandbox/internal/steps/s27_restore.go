// agent_lifecycle restore steps: after a docker start (WSL VM restart, dockerd restart) the nft/iptables gate is gone and
// every agent container is dead. Remove all agent-* containers through the remove path, rebuild agents-net and its gate,
// verify, and only then write the ready marker (dockerd MainPID) that create_agent_container waits for.
package steps

import (
	"context"
	"fmt"
	"strings"
	"time"

	"agent-sandbox/internal/config"
	"agent-sandbox/internal/services"
)

func restoreFail(code int, state, msg string, checks []string) *Failure {
	return &Failure{ExitCode: code, ExitState: state, Message: msg, Checks: checks}
}

// RestoreCheckRoot: must run as root.
func RestoreCheckRoot(_ context.Context, env *Env) *Failure {
	if !env.IsRoot {
		return restoreFail(ExitPrerequisite, StateNotRestored, "agent_lifecycle restore must run as root", nil)
	}
	return nil
}

// RestoreRemoveMarker deletes the ready marker first, so create fails with NET_NOT_READY while the rebuild runs or fails.
func RestoreRemoveMarker(_ context.Context, env *Env) *Failure {
	if err := services.RemoveReadyMarker(env.Config.RuntimeDir); err != nil {
		return fail(ExitRuntime, StateNotRestored, "cannot remove the ready marker", err)
	}
	return nil
}

// RestoreWaitDocker waits (bounded) until the docker daemon answers and records its MainPID.
func RestoreWaitDocker(ctx context.Context, env *Env) *Failure {
	deadline := time.Now().Add(env.Config.DockerWait)
	step := services.PollInterval(env.Config.DockerWait)
	for {
		err := env.Docker.Reachable(ctx)
		if err == nil {
			pid, perr := services.DockerMainPID(ctx, env.Runner)
			if perr == nil {
				env.DockerPID = pid
				return nil
			}
			err = perr
		}
		if time.Now().After(deadline) {
			return fail(ExitRuntime, StateNotRestored, fmt.Sprintf("docker daemon not ready after %s", env.Config.DockerWait), err)
		}
		if serr := services.Sleep(ctx, step); serr != nil {
			return fail(ExitRuntime, StateNotRestored, "interrupted while waiting for the docker daemon", serr)
		}
	}
}

// RestoreRemoveAgents removes every agent-* container (any state). State directories are never touched.
func RestoreRemoveAgents(ctx context.Context, env *Env) *Failure {
	names, err := env.Docker.ListAgentContainers(ctx)
	if err != nil {
		return fail(ExitRuntime, StateRestoreFailed, "cannot list agent containers", err)
	}
	var failed []string
	for _, name := range names {
		id := strings.TrimPrefix(name, config.ContainerPrefix)
		if !agentIDRe.MatchString(id) {
			// Not a tool-created name: no per-port controls can exist for it; just remove it.
			if err := env.Docker.Remove(context.WithoutCancel(ctx), name); err != nil {
				failed = append(failed, fmt.Sprintf("%s: %v", name, err))
			}
			continue
		}
		if code, detail := env.RemoveAgent(ctx, id); code != 0 {
			failed = append(failed, fmt.Sprintf("%s: remove path exited %d: %s", name, code, strings.TrimSpace(detail)))
		}
	}
	if len(failed) > 0 {
		return restoreFail(ExitRuntime, StateRestoreFailed, "not every agent container could be removed", failed)
	}
	return nil
}

// RestoreRebuildNet runs remove_agent_net (an absent network is not an error) then create_agent_net.
func RestoreRebuildNet(ctx context.Context, env *Env) *Failure {
	if err := env.Arm.RemoveNet(ctx); err != nil {
		fmt.Fprintf(env.Log.Err, "warning: remove_agent_net did not succeed (network may be absent): %v\n", err)
	}
	if err := env.Arm.CreateNet(ctx); err != nil {
		return fail(ExitRuntime, StateRestoreFailed, fmt.Sprintf("%s failed", env.Config.ArmBin), err)
	}
	return nil
}

// RestoreVerifyGate: create_agent_net exited 0 (it verifies its own gate); agents-net must exist.
func RestoreVerifyGate(ctx context.Context, env *Env) *Failure {
	ok, err := env.Docker.NetworkExists(ctx, config.NetworkName)
	if err != nil {
		return fail(ExitRuntime, StateRestoreFailed, "cannot verify network "+config.NetworkName, err)
	}
	if !ok {
		return restoreFail(ExitRuntime, StateRestoreFailed, fmt.Sprintf("network %s does not exist after create_agent_net", config.NetworkName), nil)
	}
	return nil
}

// RestoreWriteMarker writes the ready marker, but only if dockerd was not restarted during the rebuild.
func RestoreWriteMarker(ctx context.Context, env *Env) *Failure {
	pid, err := services.DockerMainPID(ctx, env.Runner)
	if err != nil {
		return fail(ExitRuntime, StateRestoreFailed, "cannot read the dockerd main pid", err)
	}
	if pid != env.DockerPID {
		return restoreFail(ExitRuntime, StateRestoreFailed,
			fmt.Sprintf("dockerd restarted during the restore (main pid %s, was %s); run it again", pid, env.DockerPID), nil)
	}
	if err := services.WriteReadyMarker(env.Config.RuntimeDir, pid); err != nil {
		return fail(ExitRuntime, StateRestoreFailed, "cannot write the ready marker", err)
	}
	env.Log.Success(
		fmt.Sprintf("agent network restored; ready marker %s holds dockerd main pid %s", services.ReadyMarkerPath(env.Config.RuntimeDir), pid),
		"exit_state: "+StateRestored,
	)
	return nil
}
