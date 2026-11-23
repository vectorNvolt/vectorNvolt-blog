// SFR-NET-013 (USC-007 step 4, err-3): list the containers attached to agents-net, stopped ones included. Every
// one must be named like an agent container (agent-<id>); if any is not, nothing is changed and the run is refused.
package steps

import (
	"context"
	"fmt"
	"strings"

	"agent-net/internal/config"
)

func RefuseForeign(ctx context.Context, env *Env) *Failure {
	env.Agents = nil
	if !env.NetworkPresent {
		return nil // the survey found no network (a read failure has stopped the run there), no attached containers
	}
	names, err := env.Docker.NetworkContainers(ctx)
	if err != nil {
		return fail(ExitPrerequisite, StateNotRemoved, "cannot list the containers on "+config.NetworkName, err)
	}
	var foreign []string
	for _, n := range names {
		if agentContainerRe.MatchString(n) {
			env.Agents = append(env.Agents, n)
		} else {
			foreign = append(foreign, n)
		}
	}
	if len(foreign) > 0 {
		return &Failure{
			ExitCode:  ExitPrerequisite,
			ExitState: StateNotRemoved,
			Message: fmt.Sprintf("containers on %s that are not agent containers (%s): %s; nothing was changed",
				config.NetworkName, config.AgentContainerPattern, strings.Join(foreign, ", ")),
		}
	}
	return nil
}
