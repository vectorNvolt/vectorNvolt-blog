// SFR-NET-012 (USC-004 alt-2): second and third deletion of the disarm order (after the table): delete the iptables
// rules whose comment is exactly agent-net:<container> from DOCKER-USER and INPUT (whatever their source IP), then
// delete the permanent neighbour entry of each IP of this container (docker, tagged rules) on the bridge, never one
// owned by another live endpoint, then clear the static fdb entries of the container's MACs on every port of the bridge. `isolated on`, `locked on` and `learning off` stay on the veth. Absent rules and
// entries are not an error.
package steps

import (
	"context"
	"strings"

	"agent-net/internal/config"
	"agent-net/internal/services"
)

// liveMACs reads, when called, the MACs of the endpoints attached to agents-net now, without this container's.
func liveMACs(ctx context.Context, env *Env) func() ([]string, error) {
	return func() ([]string, error) { return env.Docker.LiveEndpointMACs(ctx, env.Container) }
}

// fdbPorts are the interfaces whose fdb is searched for the container's MACs: every port of the bridge (a MAC may
// sit on a port that is not the container's own any more) and the known veths, as far as they still exist.
func fdbPorts(links []services.Iface, bridge string, veths []string) []string {
	var out []string
	for _, l := range links {
		if (bridge != "" && l.Master == bridge) || contains(veths, l.IfName) {
			out = appendUnique(out, l.IfName)
		}
	}
	return out
}

func DisarmRemoveRules(ctx context.Context, env *Env) *Failure {
	d := &env.Disarm
	for _, chain := range []string{config.ChainDockerUser, config.ChainInput} {
		n, err := env.Net.IptDeleteTagged(ctx, chain, exactTag(env))
		d.Deleted += n
		if err != nil {
			return fail(ExitRuntime, incomplete(env), "rules could not be deleted from "+chain, err)
		}
	}

	// An entry whose lladdr is this container's MAC is deleted, and so is a PERMANENT one of no live endpoint (a leftover
	// of an earlier, uncleanly disarmed container); an entry owned by another live endpoint stays.
	for _, ip := range d.IPs {
		deleted, err := env.Net.DeleteNeigh(ctx, ip, env.Bridge, d.MACs, liveMACs(ctx, env))
		if err != nil {
			return fail(ExitRuntime, incomplete(env), "neighbour entry of "+ip+" on "+env.Bridge+" could not be deleted", err)
		}
		if deleted {
			d.Deleted++
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
			if !e.Static() || !contains(d.MACs, strings.ToLower(e.MAC)) {
				continue
			}
			if err := env.Net.DeleteFdb(ctx, e.MAC, veth, e.Vlan); err != nil {
				return fail(ExitRuntime, incomplete(env), "fdb entry of "+veth+" could not be deleted", err)
			}
			d.Deleted++
		}
	}
	return nil
}
