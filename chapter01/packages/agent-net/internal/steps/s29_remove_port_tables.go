// SFR-NET-015 (USC-007 step 6): delete every leftover as_port_* netdev table, by name. Only reached after the agent
// containers are removed. Other nftables tables are not touched; an absent table is not an error. A failed deletion
// is recorded and the remaining removal steps still run: the verification (step 9) reports it, exit 2,
// REMOVE_INCOMPLETE, no rollback.
package steps

import (
	"context"
	"fmt"

	"agent-net/internal/config"
)

func RemovePortTables(ctx context.Context, env *Env) *Failure {
	names, err := env.Net.PortTables(ctx) // fresh listing: the disarm above already deleted most
	if err != nil {
		env.Left = append(env.Left, fmt.Sprintf("nftables tables unreadable: %v", err))
		return nil
	}
	for _, name := range names {
		if _, err := env.Net.DeleteTable(ctx, name); err != nil {
			env.Left = append(env.Left, fmt.Sprintf("table %s %s could not be deleted: %v", config.NftFamily, name, err))
		}
	}
	return nil
}
