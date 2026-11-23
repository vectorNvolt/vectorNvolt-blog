// SFR-NET-017 (USC-007 step 8): `docker network rm agents-net`, when it exists (absent means already removed).
// A failure is recorded and reported by the verification (step 9): exit 2, REMOVE_INCOMPLETE, no rollback.
package steps

import (
	"context"
	"fmt"

	"agent-net/internal/config"
)

func RemoveNetwork(ctx context.Context, env *Env) *Failure {
	_, found, err := env.Docker.InspectNetwork(ctx, config.NetworkName)
	if err != nil {
		env.Left = append(env.Left, fmt.Sprintf("cannot inspect network %s: %v", config.NetworkName, err))
		return nil
	}
	if !found {
		return nil
	}
	if err := env.Docker.RemoveNetwork(ctx); err != nil {
		env.Left = append(env.Left, fmt.Sprintf("network %s could not be removed: %v", config.NetworkName, err))
	}
	return nil
}
