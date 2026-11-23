// SFR-NET-005 (USC-004 step 4, err-6): docker network create --driver bridge --subnet 172.30.0.0/24
// agents-net, WITHOUT --internal (the network keeps its NAT route out; the host-side egress rules of `arm` bound
// it). The gate (InstallGate, the next step) closes every port of the network until it is armed. No resolver file is written (create_agent_container writes /etc/resolv.conf inside the container). On
// failure the error is reported and nothing is rolled back.
package steps

import (
	"context"

	"agent-net/internal/config"
)

func CreateNetwork(ctx context.Context, env *Env) *Failure {
	if err := env.Docker.CreateNetwork(ctx); err != nil {
		return fail(ExitRuntime, StateCreateFailed, "network "+config.NetworkName+" could not be created", err)
	}
	return nil
}
