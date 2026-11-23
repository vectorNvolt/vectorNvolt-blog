// SFR-NET-023 (USC-007 step 3): read what exists of the setup before any change: the network, the as_port_* netdev
// tables, the agents_gate table and the tagged iptables rules (the gate rule included). Containers attached to agents-net
// (see RefuseForeign and RemoveAgents) include created containers that were never armed. A part that is absent is readable (docker's "no such network" answer; an
// iptables chain that does not exist has no rules); one that cannot be read stops the run with
// nothing changed (exit 1, NOT_REMOVED). When none of it exists there is nothing to remove and the run succeeds.
package steps

import (
	"context"
	"fmt"
	"strings"

	"agent-net/internal/config"
)

// anyTag matches the comment of every rule the arming unit added.
func anyTag(c string) bool { return strings.HasPrefix(c, config.CommentPrefix) }

func SurveyRemoval(ctx context.Context, env *Env) *Failure {
	info, found, err := env.Docker.InspectNetwork(ctx, config.NetworkName)
	if err != nil {
		return fail(ExitPrerequisite, StateNotRemoved, "cannot inspect network "+config.NetworkName, err)
	}
	env.NetworkPresent = found
	if found {
		env.Network = info
		env.Bridge = info.BridgeName()
	}
	env.Tables, err = env.Net.PortTables(ctx)
	if err != nil {
		return fail(ExitPrerequisite, StateNotRemoved, "nftables tables unreadable", err)
	}
	_, gatePresent, err := env.Net.ReadGate(ctx)
	if err != nil {
		return fail(ExitPrerequisite, StateNotRemoved, "nftables table "+config.GateTable+" unreadable", err)
	}
	tagged := 0
	for _, chain := range []string{config.ChainDockerUser, config.ChainInput} {
		refs, err := env.Net.IptTagged(ctx, chain, anyTag)
		if err != nil {
			return fail(ExitPrerequisite, StateNotRemoved, chain+" unreadable", err)
		}
		tagged += len(refs)
	}
	if !found && len(env.Tables) == 0 && !gatePresent && tagged == 0 {
		env.Log.Success(fmt.Sprintf("nothing to remove: no network %s, no %s* or %s table, no %s rules", config.NetworkName, config.PortTablePfx, config.GateTable, config.CommentPrefix))
		env.Finished = true
	}
	return nil
}
