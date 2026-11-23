// SFR-NET-012 (USC-004 alt-2): find everything the container name left on the host. The live veth is identified as in
// arming (IdentifyPort) when the container runs; a failure there is not fatal, the veth may be gone. The port is also
// found by the table name as_port_<id> (the veth its chains are bound to is read from it, and the container's MAC from
// its accept rules), by the iptables tag agent-net:<container> (the IPs of the tagged rules add to the identity:
// docker reports none for a stopped container) and by the comment <container> of the elements of the gate set
// `released` (their MACs add to the identity: this works for a container that is gone). Nothing is changed.
//
// A port table bound to the live veth of a running container that is not this one (agent-web and web both give
// as_port_web) is refused before anything is touched: exit 1, the not-done state of the mode.
package steps

import (
	"context"
	"fmt"

	"agent-net/internal/config"
)

// exactTag matches the comment of the rules arming added for the container.
func exactTag(env *Env) func(string) bool {
	tag := config.CommentPrefix + env.Container
	return func(c string) bool { return c == tag }
}

func DisarmFindPorts(ctx context.Context, env *Env) *Failure {
	d := &env.Disarm
	if d.Pid > 0 {
		env.Port = Port{}
		if IdentifyPort(ctx, env) == nil {
			d.Live = env.Port.Veth
			d.Veths = appendUnique(d.Veths, env.Port.Veth)
		}
	}
	for _, chain := range []string{config.ChainDockerUser, config.ChainInput} {
		refs, err := env.Net.IptTagged(ctx, chain, exactTag(env))
		if err != nil {
			return fail(ExitRuntime, incomplete(env), chain+" unreadable", err)
		}
		for _, r := range refs {
			if ip := r.Source(); ip != "" {
				d.IPs = appendUnique(d.IPs, ip)
			}
		}
	}
	t, found, err := env.Net.ReadPortTable(ctx, env.PortTable())
	if err != nil {
		return fail(ExitRuntime, incomplete(env), "port table "+env.PortTable()+" unreadable", err)
	}
	if found {
		if v := t.Device(); v != "" {
			d.Table = v
			d.Veths = appendUnique(d.Veths, v)
		}
		// A stopped container: docker reports no MAC, but the accept rules of its own table carry it. The table is
		// deleted by a later step, so the MAC is recovered now (fdb delete, neighbour entry check).
		for _, m := range t.AgentMACs() {
			d.MACs = appendUnique(d.MACs, m)
		}
	}
	// The elements of `released` that carry the container name as their comment (a gone container, no table).
	elems, _, err := env.Net.GateElements(ctx)
	if err != nil {
		return fail(ExitRuntime, incomplete(env), "set "+config.GateSet+" of "+config.GateTable+" unreadable", err)
	}
	for _, e := range elems {
		if e.Comment == env.Container {
			d.MACs = appendUnique(d.MACs, e.MAC)
		}
	}
	return refuseTableOfAnotherPort(ctx, env)
}

// refuseTableOfAnotherPort: the container runs on a live veth, and its port table name is bound to another live veth.
func refuseTableOfAnotherPort(ctx context.Context, env *Env) *Failure {
	d := &env.Disarm
	if d.Live == "" || d.Table == "" || d.Table == d.Live {
		return nil
	}
	links, err := env.Net.HostLinks(ctx)
	if err != nil {
		return fail(ExitRuntime, incomplete(env), "host links unreadable", err)
	}
	for _, l := range links {
		if l.IfName == d.Table {
			f := &Failure{
				ExitCode:  ExitPrerequisite,
				ExitState: notDone(env),
				Message: fmt.Sprintf("port table %s is bound to %s, the port of another container, not to %s of %s; refusing to touch it",
					env.PortTable(), d.Table, d.Live, env.Container),
			}
			if env.Mode == ModeArm {
				f.Control = ctlTable
			}
			return f
		}
	}
	return nil
}

func appendUnique(list []string, s string) []string {
	if contains(list, s) {
		return list
	}
	return append(list, s)
}
