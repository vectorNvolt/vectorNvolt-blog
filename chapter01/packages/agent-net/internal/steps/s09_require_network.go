// SFR-NET-008 precondition (USC-004 alt-1, alt-3): agents-net must exist; its host bridge device is read from the
// inspected network. Nothing is created here; the container is untouched (still paused, already attached to agents-net).
// For `arm` the gate of the network must be there too (nft table agents_gate, its DOCKER-USER rule,
// bridge-nf-call-iptables == 1): a network that predates the gate must be recreated (remove_agent_net, then
// create_agent_net). `check-gate` does not require it; it reports a missing gate as inconsistent.
// A missing network or gate is a precondition failure of the mode: GATED_UNARMED for `arm`, NOT_CHECKED for `check-gate`.
package steps

import (
	"context"
	"fmt"
	"strings"

	"agent-net/internal/config"
)

func RequireNetwork(ctx context.Context, env *Env) *Failure {
	state := notDone(env)
	info, found, err := env.Docker.InspectNetwork(ctx, config.NetworkName)
	if err != nil {
		return fail(ExitPrerequisite, state, "cannot inspect network "+config.NetworkName, err)
	}
	if !found {
		return &Failure{
			ExitCode:  ExitPrerequisite,
			ExitState: state,
			Message:   fmt.Sprintf("network %s does not exist; run create_agent_net first", config.NetworkName),
		}
	}
	env.Network = info
	env.Bridge = info.BridgeName()
	if env.Mode == ModeArm {
		if bad := gateProblems(ctx, env, env.Bridge); len(bad) > 0 {
			return &Failure{
				ExitCode:  ExitPrerequisite,
				ExitState: state,
				Message: fmt.Sprintf("network %s has no intact gate (%s); it predates the gate or was changed and must be recreated: run remove_agent_net, then create_agent_net",
					config.NetworkName, strings.Join(bad, "; ")),
			}
		}
	}
	return nil
}
