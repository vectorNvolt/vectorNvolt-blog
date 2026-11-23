// SFR-NET-008 (USC-004 alt-1): close the port of the container attached at `docker create` (the agents-net gate
// already drops its frames; its MAC is not in `released` and is taken out again if an earlier run left it there). For its veth, load `table netdev
// as_port_<id>` with an ingress chain and an egress chain bound to the veth, both policy drop and no accept rule,
// in one atomic `nft -f -`, and read it back. Only this container's table is touched. The container is paused by
// the caller, so it cannot send a frame before this step has run.
//
// The table name is derived from the container name (agent-web and web both give as_port_web). Loading replaces a
// table of that name, so a table that is bound to the live veth of another port is refused first: exit 1,
// GATED_UNARMED, nothing loaded. A table bound to a veth that no longer exists is the stale table of an earlier
// container of that name and is replaced.
package steps

import (
	"context"
	"fmt"

	"agent-net/internal/config"
)

func ClosePort(ctx context.Context, env *Env) *Failure {
	const control = ctlTable
	table := env.PortTable()
	if f := refuseOtherPortTable(ctx, env, table); f != nil {
		return f
	}
	// A MAC left in `released` by an earlier arming run would keep the gate open for this port: take it out first.
	if _, err := env.Net.RemoveGateMACs(ctx, []string{env.Port.MAC}); err != nil {
		return failControl(control, "MAC "+env.Port.MAC+" could not be taken out of "+config.GateTable, err)
	}
	if err := env.Net.LoadClosedPort(ctx, table, env.Port.Veth); err != nil {
		return failControl(control, "port table "+table+" could not be loaded", err)
	}
	t, found, err := env.Net.ReadPortTable(ctx, table)
	switch {
	case err != nil:
		return failControl(control, "port table "+table+" unreadable after loading", err)
	case !found:
		return failControl(control, "port table "+table+" is missing after loading", nil)
	}
	if ok, why := t.Closed(env.Port.Veth); !ok {
		return failControl(control, "port is not closed: "+why, nil)
	}
	return nil
}

// refuseOtherPortTable refuses when table already exists and is bound to a live veth that is not this container's.
func refuseOtherPortTable(ctx context.Context, env *Env, table string) *Failure {
	t, found, err := env.Net.ReadPortTable(ctx, table)
	switch {
	case err != nil:
		return failControl(ctlTable, "port table "+table+" unreadable before loading", err)
	case !found:
		return nil
	}
	dev := t.Device()
	if dev == "" || dev == env.Port.Veth {
		return nil
	}
	links, err := env.Net.HostLinks(ctx)
	if err != nil {
		return failControl(ctlTable, "host links unreadable", err)
	}
	for _, l := range links {
		if l.IfName == dev {
			return &Failure{
				ExitCode:  ExitPrerequisite,
				ExitState: StateGatedUnarmed,
				Control:   ctlTable,
				Message: fmt.Sprintf("port table %s is bound to %s, the port of another container, not to %s of %s; refusing to replace it",
					table, dev, env.Port.Veth, env.Container),
			}
		}
	}
	return nil
}
