// SFR-NET-019 (USC-007 step 2, err-1): remove_agent_net takes no arguments; anything else is a usage error.
package steps

import (
	"context"
	"regexp"

	"agent-net/internal/config"
)

// UsageRemove is printed for any invocation of remove_agent_net with arguments.
const UsageRemove = "usage: remove_agent_net\n" +
	"  takes no arguments; removes every agent container on " + config.NetworkName + ", the per-port tables, the tagged rules and the network"

var agentContainerRe = regexp.MustCompile(config.AgentContainerPattern)

func ValidateRemoveInvocation(_ context.Context, env *Env) *Failure {
	env.Mode = ModeRemove
	if len(env.Args) != 0 {
		return &Failure{ExitCode: ExitPrerequisite, ExitState: StateNotRemoved, Message: UsageRemove}
	}
	return nil
}
