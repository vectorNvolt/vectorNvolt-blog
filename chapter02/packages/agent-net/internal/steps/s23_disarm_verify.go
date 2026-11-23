// SFR-NET-012 (USC-004 alt-2): read back and require that no element of the gate set `released` carries the container
// name as its comment or one of the collected MACs, and that its port table, its tagged iptables rules and its static
// fdb entry are gone. The state is inspected, not assumed from the delete commands.
package steps

import (
	"context"
	"fmt"
	"strings"

	"agent-net/internal/config"
)

func DisarmVerify(ctx context.Context, env *Env) *Failure {
	d := &env.Disarm
	var bad []string

	_, found, err := env.Net.ReadPortTable(ctx, env.PortTable())
	if err != nil {
		return fail(ExitRuntime, incomplete(env), "port table "+env.PortTable()+" unreadable", err)
	}
	if found {
		bad = append(bad, fmt.Sprintf("table %s %s is still there", config.NftFamily, env.PortTable()))
	}

	elems, _, err := env.Net.GateElements(ctx)
	if err != nil {
		return fail(ExitRuntime, incomplete(env), "set "+config.GateSet+" of "+config.GateTable+" unreadable", err)
	}
	for _, e := range elems {
		switch {
		case contains(d.MACs, e.MAC):
			bad = append(bad, fmt.Sprintf("MAC %s is still in set %s of %s", e.MAC, config.GateSet, config.GateTable))
		case e.Comment == env.Container:
			bad = append(bad, fmt.Sprintf("element %s with comment %s is still in set %s of %s", e.MAC, e.Comment, config.GateSet, config.GateTable))
		}
	}

	for _, chain := range []string{config.ChainDockerUser, config.ChainInput} {
		refs, err := env.Net.IptTagged(ctx, chain, exactTag(env))
		if err != nil {
			return fail(ExitRuntime, incomplete(env), chain+" unreadable", err)
		}
		for _, r := range refs {
			bad = append(bad, fmt.Sprintf("rule %d of %s is still there: %s", r.Num, chain, r.Line))
		}
	}

	for _, ip := range d.IPs {
		e, found, err := env.Net.NeighEntry(ctx, ip, env.Bridge)
		if err != nil {
			return fail(ExitRuntime, incomplete(env), "neighbour entry of "+ip+" on "+env.Bridge+" unreadable", err)
		}
		if !found {
			continue
		}
		// An entry with the lladdr of another live endpoint is not ours and is no leftover; a PERMANENT one of no live
		// endpoint is.
		lladdr := strings.ToLower(e.LLAddr)
		stale := contains(d.MACs, lladdr)
		if !stale && e.Permanent() {
			live, err := liveMACs(ctx, env)()
			if err != nil {
				return fail(ExitRuntime, incomplete(env), "endpoints of "+config.NetworkName+" unreadable", err)
			}
			stale = !contains(live, lladdr)
		}
		if stale {
			bad = append(bad, fmt.Sprintf("neighbour entry of %s (lladdr %s) on %s is still there", ip, e.LLAddr, env.Bridge))
		}
	}

	links, err := env.Net.HostLinks(ctx)
	if err != nil {
		return fail(ExitRuntime, incomplete(env), "host links unreadable", err)
	}
	for _, veth := range fdbPorts(links, env.Bridge, d.Veths) {
		entries, err := env.Net.FdbEntries(ctx, veth)
		if err != nil {
			return fail(ExitRuntime, incomplete(env), "bridge fdb of "+veth+" unreadable", err)
		}
		for _, e := range entries {
			if e.Static() && contains(d.MACs, strings.ToLower(e.MAC)) {
				bad = append(bad, fmt.Sprintf("static fdb entry %s on %s is still there", e.MAC, veth))
			}
		}
	}

	if len(bad) > 0 {
		return &Failure{
			ExitCode:  ExitRuntime,
			ExitState: incomplete(env),
			Message:   fmt.Sprintf("port table, rules or entries of container %s are left", env.Container),
			Checks:    bad,
		}
	}
	return nil
}
