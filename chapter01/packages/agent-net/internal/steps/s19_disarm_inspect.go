// SFR-NET-012 (USC-004 alt-2): read the container's identity on agents-net from docker. Disarm is keyed by the
// container name, so a container that does not exist is not an error: Missing is set and the later steps still find
// what the name left behind (port table, tagged rules, tagged `released` elements). Inside remove_agent_net a missing
// container is simply already gone. A stopped container has no PID and docker reports no IP/MAC for it; its rules are
// then found by the later steps. Any other docker failure stops the run.
package steps

import (
	"context"
	"strings"

	"agent-net/internal/config"
)

func DisarmInspect(ctx context.Context, env *Env) *Failure {
	env.Disarm = Disarm{}
	info, found, err := env.Docker.LookupContainer(ctx, env.Container)
	if err != nil {
		return fail(ExitPrerequisite, notDone(env), "cannot inspect container "+env.Container, err)
	}
	if !found {
		env.Disarm.Missing = true
		return nil
	}
	env.Disarm.Pid = info.State.Pid
	if ep, ok := info.NetworkSettings.Networks[config.NetworkName]; ok {
		if ep.MacAddress != "" {
			env.Disarm.MACs = []string{strings.ToLower(ep.MacAddress)}
		}
		if ep.IPAddress != "" {
			env.Disarm.IPs = []string{ep.IPAddress}
		}
	}
	return nil
}
