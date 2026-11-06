// SFR-NET-010 + SFR-NET-009 (source identity) + SFR-NET-024 (layer 3 destination limit) + SFR-NET-011 (USC-004 alt-1): release the
// port last, only after every verification passed. The accept rules are added to the container's own table, and the
// MAC of the agent to the set `released` of the agents-net gate, in ONE atomic `nft -f -` (the same transaction
// flushes the two chains), then read back and verified again:
//
//	ingress  agent MAC + IPv4 source -> bridge MAC; ARP request for the bridge IP (sender pinned)
//	egress   IPv4 from the bridge MAC; ARP reply from the bridge MAC to the agent MAC
//
// There is no ingress ARP reply rule: the host's neighbour entry for the agent is permanent (applied and verified
// before this step), so the host never sends an ARP request. Everything else (IPv6, any other frame type, any other peer) falls to the policy drop of the two chains. This is
// the only step that opens a port; on any earlier failure it never runs.
//
// The container is read again immediately before the release: it must still be running and paused. If anything fails
// after the accept rules were added (read-back error, missing table, wrong policy, wrong rules), the port is put back
// to closed first (the closed table is loaded again: policy drop, no accept rule), then the step fails.
package steps

import (
	"context"
	"fmt"

	"agent-net/internal/config"
	"agent-net/internal/services"
)

func ReleasePort(ctx context.Context, env *Env) *Failure {
	const control = ctlRelease

	links, err := env.Net.HostLinks(ctx)
	if err != nil {
		return failControl(control, "host links unreadable", err)
	}
	bridgeMAC := ""
	for _, l := range links {
		if l.IfName == env.Bridge {
			bridgeMAC = l.Address
		}
	}
	bridgeIP := env.Network.Gateway()
	if bridgeMAC == "" || bridgeIP == "" {
		return failControl(control, fmt.Sprintf("MAC (%q) or IP (%q) of the bridge %s unknown", bridgeMAC, bridgeIP, env.Bridge), nil)
	}
	rules, err := services.AcceptRules(services.PortAddrs{MAC: env.Port.MAC, IP: env.Port.IP, BridgeMAC: bridgeMAC, BridgeIP: bridgeIP})
	if err != nil {
		return failControl(control, "accept rules could not be built", err)
	}

	if f := keepPaused(ctx, env, "before the release"); f != nil {
		return f
	}
	table := env.PortTable()
	if err := env.Net.ReleasePort(ctx, table, env.Port.MAC, env.Container, rules); err != nil {
		return closeAgain(ctx, env, failControl(control, "accept rules could not be added to "+table+" and the MAC to "+config.GateTable, err))
	}
	t, found, err := env.Net.ReadPortTable(ctx, table)
	switch {
	case err != nil:
		return closeAgain(ctx, env, failControl(control, "port table "+table+" unreadable after release", err))
	case !found:
		return closeAgain(ctx, env, failControl(control, "port table "+table+" is missing after release", nil))
	}
	if why := t.Policies(env.Port.Veth); why != "" {
		return closeAgain(ctx, env, failControl(control, "port is not released: "+why, nil))
	}
	if ok, why := t.Released(rules); !ok {
		return closeAgain(ctx, env, failControl(control, "port is not released: "+why, nil))
	}
	g, gfound, err := env.Net.ReadGate(ctx)
	switch {
	case err != nil:
		return closeAgain(ctx, env, failControl(control, "table "+config.GateTable+" unreadable after release", err))
	case !gfound:
		return closeAgain(ctx, env, failControl(control, "table "+config.GateTable+" is missing after release", nil))
	case !g.Has(env.Port.MAC):
		return closeAgain(ctx, env, failControl(control, fmt.Sprintf("MAC %s is not in set %s of %s after release", env.Port.MAC, config.GateSet, config.GateTable), nil))
	}
	return nil
}

// closeAgain puts the port back to closed (SFR-NET-011) after a failure past the release command and returns the
// failure: the MAC leaves the set `released` of the gate first, then the closed port table is loaded again.
func closeAgain(ctx context.Context, env *Env, f *Failure) *Failure {
	if _, err := env.Net.RemoveGateMACs(ctx, []string{env.Port.MAC}); err != nil {
		f.Message += fmt.Sprintf("; the MAC could not be removed from %s again: %v", config.GateTable, err)
	}
	if err := env.Net.LoadClosedPort(ctx, env.PortTable(), env.Port.Veth); err != nil {
		f.Message += fmt.Sprintf("; the port could not be closed again: %v", err)
	}
	return f
}
