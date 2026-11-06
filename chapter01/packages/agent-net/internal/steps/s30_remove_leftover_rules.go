// SFR-NET-016 (USC-007 step 7): delete any iptables rule left with a comment starting agent-net: from DOCKER-USER
// and INPUT (rules of containers that were removed by other means), by rule number from a fresh listing, highest
// first. Other rules of those chains are not touched. A failed deletion is recorded and the remaining removal steps
// still run: the verification (step 9) reports it, exit 2, REMOVE_INCOMPLETE, no rollback.
package steps

import (
	"context"
	"fmt"

	"agent-net/internal/config"
)

func RemoveLeftoverRules(ctx context.Context, env *Env) *Failure {
	for _, chain := range []string{config.ChainDockerUser, config.ChainInput} {
		if _, err := env.Net.IptDeleteTagged(ctx, chain, anyTag); err != nil {
			env.Left = append(env.Left, fmt.Sprintf("rules could not be deleted from %s: %v", chain, err))
		}
	}
	return nil
}
