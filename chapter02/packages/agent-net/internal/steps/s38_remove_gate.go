// USC-007: delete `table netdev agents_gate` (the agents-net gate, with its set `released`), when it exists, after the
// agent containers are removed and before the network is. The DOCKER-USER gate rule (comment agent-net:gate) is
// deleted by the previous step together with the other agent-net: rules. An absent table is not an error. A failed
// deletion is recorded and the remaining removal steps still run: the verification reports it, exit 2,
// REMOVE_INCOMPLETE, no rollback.
package steps

import (
	"context"
	"fmt"

	"agent-net/internal/config"
)

func RemoveGate(ctx context.Context, env *Env) *Failure {
	if _, err := env.Net.DeleteGate(ctx); err != nil {
		env.Left = append(env.Left, fmt.Sprintf("table %s %s could not be deleted: %v", config.NftFamily, config.GateTable, err))
	}
	return nil
}
