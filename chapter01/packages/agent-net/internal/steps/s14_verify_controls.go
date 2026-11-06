// SFR-NET-010 + SFR-NET-024 (USC-004 alt-1): verify every control by reading its applied state at the host side
// (inspected, not requested): bridge link flags (isolated on, learning off, locked on), bridge fdb, the permanent
// neighbour entry (lladdr = agent MAC, state PERMANENT), the port table
// (both chains bound to the veth, policy drop, no accept rule yet), and the tagged
// iptables chains. Only when all hold does the sequence go on to release the port. A failure names the control
// (SFR-NET-011).
package steps

import (
	"context"
	"fmt"
	"strings"

	"agent-net/internal/config"
	"agent-net/internal/services"
)

func VerifyControls(ctx context.Context, env *Env) *Failure {
	veth, mac := env.Port.Veth, env.Port.MAC

	port, err := env.Net.PortState(ctx, veth)
	if err != nil {
		return failControl(ctlIsolation, "bridge port state unreadable", err)
	}
	if port.Master != env.Bridge {
		return failControl(ctlIsolation, fmt.Sprintf("%s has master %q, want %s", veth, port.Master, env.Bridge), nil)
	}
	if !port.Isolated {
		return failControl(ctlIsolation, veth+" is not isolated", nil)
	}
	if port.Learning {
		return failControl(ctlLearning, "learning is still on for "+veth, nil)
	}
	if !port.Locked {
		return failControl(ctlLock, veth+" is not locked", nil)
	}

	entries, err := env.Net.FdbEntries(ctx, veth)
	if err != nil {
		return failControl(ctlFdb, "bridge fdb unreadable", err)
	}
	pinned := false
	for _, e := range entries {
		if strings.EqualFold(e.MAC, mac) && e.Static() && e.Master == env.Bridge {
			pinned = true
		}
	}
	if !pinned {
		return failControl(ctlFdb, fmt.Sprintf("no static fdb entry for %s on %s", mac, veth), nil)
	}

	ne, nfound, err := env.Net.NeighEntry(ctx, env.Port.IP, env.Bridge)
	switch {
	case err != nil:
		return failControl(ctlNeigh, "neighbour entry unreadable", err)
	case !nfound:
		return failControl(ctlNeigh, fmt.Sprintf("no neighbour entry for %s on %s", env.Port.IP, env.Bridge), nil)
	case !strings.EqualFold(ne.LLAddr, mac) || !ne.Permanent():
		return failControl(ctlNeigh, fmt.Sprintf("neighbour entry for %s is %s %v, want %s PERMANENT", env.Port.IP, ne.LLAddr, ne.State, mac), nil)
	}

	table := env.PortTable()
	t, found, err := env.Net.ReadPortTable(ctx, table)
	switch {
	case err != nil:
		return failControl(ctlTable, "port table "+table+" unreadable", err)
	case !found:
		return failControl(ctlTable, "port table "+table+" is missing", nil)
	}
	if ok, why := t.Closed(veth); !ok {
		return failControl(ctlTable, why, nil)
	}

	// The gate still holds the port closed: the MAC is not in `released` yet (the release adds it).
	gate, gfound, err := env.Net.ReadGate(ctx)
	switch {
	case err != nil:
		return failControl(ctlGateSet, "table "+config.GateTable+" unreadable", err)
	case !gfound:
		return failControl(ctlGateSet, "table "+config.GateTable+" is missing", nil)
	case gate.Has(mac):
		return failControl(ctlGateSet, fmt.Sprintf("MAC %s is already in set %s of %s before the release", mac, config.GateSet, config.GateTable), nil)
	}

	// The DNS rule and the endpoint rule are distinct controls (SFR-NET-010); a rule names its own control.
	for _, c := range []struct{ control, chain string }{
		{ctlEgress, config.ChainDockerUser},
		{ctlInput, config.ChainInput},
	} {
		lines, err := env.Net.IptRules(ctx, c.chain)
		if err != nil {
			return failControl(c.control, c.chain+" unreadable", err)
		}
		for _, r := range iptPlan(env) {
			if r.Chain == c.chain && !services.HasIptRule(lines, r.Tokens...) {
				return failControl(r.Control, fmt.Sprintf("rule missing in %s: %s", c.chain, strings.Join(r.Spec, " ")), nil)
			}
		}
	}
	return nil
}
