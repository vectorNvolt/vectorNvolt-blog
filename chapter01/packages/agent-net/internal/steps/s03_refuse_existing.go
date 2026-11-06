// SFR-NET-003 (USC-004 step 3, err-4): refuse when agents-net already exists; the existing network is not touched.
package steps

import (
	"context"
	"fmt"

	"agent-net/internal/config"
)

func RefuseExisting(ctx context.Context, env *Env) *Failure {
	_, found, err := env.Docker.InspectNetwork(ctx, config.NetworkName)
	if err != nil {
		return fail(ExitPrerequisite, StateNotCreated, "cannot inspect network "+config.NetworkName, err)
	}
	if found {
		return &Failure{
			ExitCode:  ExitPrerequisite,
			ExitState: StateAlreadyExists,
			Message:   fmt.Sprintf("network %s already exists; it is not modified", config.NetworkName),
		}
	}
	return nil
}
