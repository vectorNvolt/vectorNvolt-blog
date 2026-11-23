// SFR-NET-014 + SSR-NET-003 (USC-007 step 5): for each agent container, disarm its port (the code of
// `disarm`, in-process: table, iptables rules, fdb entry) and then `docker rm -f` it. The port stays locked and
// isolated throughout. The first failure stops the run with the tables and rules of the containers still to
// remove in place; that includes a container that cannot be inspected (any failure but docker's not-found answer),
// which stops the run before any table of the later steps is touched. A container that is gone by then (not found
// by the inspect or by `docker rm -f`) counts as already removed and is not counted as removed by this run.
// Workspaces are never touched.
package steps

import (
	"context"
	"fmt"
	"strings"

	"agent-net/internal/config"
)

func RemoveAgents(ctx context.Context, env *Env) *Failure {
	env.Removed = nil
	for i, name := range env.Agents {
		left := strings.Join(env.Agents[i:], ", ")
		env.Container = name
		env.Disarm = Disarm{}
		for _, step := range []func(context.Context, *Env) *Failure{DisarmInspect, DisarmFindPorts, DisarmCloseGate, DisarmDeleteTable, DisarmRemoveRules, DisarmVerify} {
			if f := step(ctx, env); f != nil {
				f.ExitCode, f.ExitState = ExitRuntime, StateRemoveIncomplete
				f.Message = fmt.Sprintf("container %s could not be disarmed (still to remove: %s): %s", name, left, f.Message)
				return f
			}
			if env.Disarm.Missing {
				break // gone since it was listed: nothing to disarm or remove
			}
		}
		if env.Disarm.Missing {
			continue
		}
		removed, err := env.Docker.RemoveContainer(ctx, name)
		if err != nil {
			return fail(ExitRuntime, StateRemoveIncomplete, fmt.Sprintf("container %s could not be removed (still to remove: %s)", name, left), err)
		}
		if !removed {
			env.Log.Success(fmt.Sprintf("container %s was already gone", name))
			continue
		}
		env.Removed = append(env.Removed, name)
		env.Log.Success(fmt.Sprintf("container %s removed from %s (its workspace is kept)", name, config.NetworkName))
	}
	env.Container = ""
	return nil
}
