// SFR-NET-012 (USC-004 alt-2, `disarm <container>`): without agents-net no container can be armed on it, so there
// is nothing to disarm and the run succeeds. Otherwise the host bridge device is read for the port identification.
package steps

import (
	"context"
	"fmt"

	"agent-net/internal/config"
)

func DisarmRequireNetwork(ctx context.Context, env *Env) *Failure {
	info, found, err := env.Docker.InspectNetwork(ctx, config.NetworkName)
	if err != nil {
		return fail(ExitPrerequisite, StateNotDisarmed, "cannot inspect network "+config.NetworkName, err)
	}
	if !found {
		env.Log.Success(fmt.Sprintf("network %s does not exist; nothing to disarm for %s", config.NetworkName, env.Container))
		env.Finished = true
		return nil
	}
	env.Network = info
	env.Bridge = info.BridgeName()
	return nil
}
