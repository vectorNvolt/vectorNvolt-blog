// SFR-NET-012 + SSR-NET-003 (USC-004 alt-2): first deletion of the disarm order, table, iptables rules, static fdb
// entry. Delete the container's `table netdev as_port_<id>`, which takes both chains and every accept rule with it.
// The port stays closed by `locked on` and port isolation, which are never changed here. Only this container's table
// is touched; an absent table is not an error.
package steps

import "context"

func DisarmDeleteTable(ctx context.Context, env *Env) *Failure {
	deleted, err := env.Net.DeleteTable(ctx, env.PortTable())
	if err != nil {
		return fail(ExitRuntime, incomplete(env), "port table "+env.PortTable()+" could not be deleted", err)
	}
	if deleted {
		env.Disarm.Deleted++
	}
	return nil
}
