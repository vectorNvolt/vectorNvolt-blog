// SFR-NET-004 (USC-004 step 3, err-5): 172.30.0.0/24 must overlap no docker network subnet, host address or
// host route (all tables, including the WSL NAT interface). Default routes are ignored; docker's default
// address pools are not a criterion. Reads only; nothing is repaired.
package steps

import (
	"context"
	"fmt"
	"net/netip"

	"agent-net/internal/config"
)

func CheckRangeClash(ctx context.Context, env *Env) *Failure {
	want := netip.MustParsePrefix(config.Subnet)
	var clashes []string

	nets, err := env.Docker.ListNetworks(ctx)
	if err != nil {
		return fail(ExitPrerequisite, StateNotCreated, "cannot list docker networks", err)
	}
	for _, n := range nets {
		for _, s := range n.Subnets() {
			if p, err := netip.ParsePrefix(s); err == nil && p.Overlaps(want) {
				clashes = append(clashes, fmt.Sprintf("docker network %s subnet %s", n.Name, s))
			}
		}
	}

	addrs, err := env.Net.HostAddrs(ctx)
	if err != nil {
		return fail(ExitPrerequisite, StateNotCreated, "cannot read host addresses", err)
	}
	for _, a := range addrs {
		if a.Prefix.Overlaps(want) {
			clashes = append(clashes, "host "+a.Label)
		}
	}

	routes, err := env.Net.HostRoutes(ctx)
	if err != nil {
		return fail(ExitPrerequisite, StateNotCreated, "cannot read host routes", err)
	}
	for _, r := range routes {
		if r.Prefix.Overlaps(want) {
			clashes = append(clashes, "host "+r.Label)
		}
	}

	if len(clashes) > 0 {
		return &Failure{
			ExitCode:  ExitPrerequisite,
			ExitState: StateNotCreated,
			Message:   fmt.Sprintf("%s overlaps a range in use; nothing created, nothing changed", config.Subnet),
			Checks:    clashes,
		}
	}
	return nil
}
