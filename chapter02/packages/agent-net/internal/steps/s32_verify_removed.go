// SFR-NET-018 (USC-007 step 9): read back and require that agents-net is gone, no as_port_* table and no agents_gate table is left and no
// iptables rule with an agent-net: comment is left. A leftover, and any failure recorded by steps 6-8, is reported by
// name (exit 2, REMOVE_INCOMPLETE).
package steps

import (
	"context"
	"fmt"

	"agent-net/internal/config"
)

func VerifyRemoved(ctx context.Context, env *Env) *Failure {
	bad := append([]string(nil), env.Left...)

	_, found, err := env.Docker.InspectNetwork(ctx, config.NetworkName)
	switch {
	case err != nil:
		return fail(ExitRuntime, StateRemoveIncomplete, "cannot inspect network "+config.NetworkName, err)
	case found:
		bad = append(bad, fmt.Sprintf("network %s still exists", config.NetworkName))
	}

	tables, err := env.Net.PortTables(ctx)
	if err != nil {
		return fail(ExitRuntime, StateRemoveIncomplete, "nftables tables unreadable", err)
	}
	for _, name := range tables {
		bad = append(bad, fmt.Sprintf("table %s %s still exists", config.NftFamily, name))
	}

	if _, gfound, err := env.Net.ReadGate(ctx); err != nil {
		return fail(ExitRuntime, StateRemoveIncomplete, "table "+config.GateTable+" unreadable", err)
	} else if gfound {
		bad = append(bad, fmt.Sprintf("table %s %s still exists", config.NftFamily, config.GateTable))
	}

	for _, chain := range []string{config.ChainDockerUser, config.ChainInput} {
		refs, err := env.Net.IptTagged(ctx, chain, anyTag)
		if err != nil {
			return fail(ExitRuntime, StateRemoveIncomplete, chain+" unreadable", err)
		}
		for _, r := range refs {
			bad = append(bad, fmt.Sprintf("rule %d of %s is still there: %s", r.Num, chain, r.Line))
		}
	}

	if len(bad) > 0 {
		return &Failure{
			ExitCode:  ExitRuntime,
			ExitState: StateRemoveIncomplete,
			Message:   "the setup is not fully removed; run remove_agent_net again",
			Checks:    bad,
		}
	}
	return nil
}
