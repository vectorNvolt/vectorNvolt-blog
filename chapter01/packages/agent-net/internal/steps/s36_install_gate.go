// USC-004 step 5 (create): install the network-wide gate of agents-net right after the network is created, before it is
// verified or reported. Containers are attached to agents-net at `docker create` (docker cannot connect a container
// that was created with --network none, and gVisor never sees interfaces added after the start), so their ports exist
// before `arm`; the gate keeps every port closed until its MAC is added to the set `released`.
//
//	table netdev agents_gate, loaded by ONE atomic `nft -f -`:
//	  set released { type ether_addr; }
//	  chain ingress  hook ingress device <br>  priority 0  policy drop   ether saddr @released meta mark and 0x10000000 == 0x10000000 meta mark set meta mark and 0xefffffff accept
//	  chain egress   hook egress  device <br>  priority 0  policy drop   ether daddr @released accept
//	DOCKER-USER (top): -i <br> -o <br> -m physdev --physdev-is-bridged -m comment --comment agent-net:gate -j DROP
//	  (port-to-port traffic between not yet armed ports; armed ports isolate themselves)
//
// /proc/sys/net/bridge/bridge-nf-call-iptables must be 1, or bridged frames do not traverse iptables and the rule above
// would not see them. It is only read, never written. A failure leaves the network in place: CREATED_UNVERIFIED.
// The same read-back (gateProblems) runs in the verification of create and in the precondition of `arm`.
package steps

import (
	"context"
	"fmt"
	"strings"

	"agent-net/internal/config"
	"agent-net/internal/services"
)

// ctlGate names the control "the agents-net gate" in a failure report.
const ctlGate = "network gate"

// gateFailure is a create failure after the network exists.
func gateFailure(what string, err error) *Failure {
	f := fail(ExitRuntime, StateCreatedUnverified, what, err)
	if err == nil {
		f.Message = what
	}
	f.Control = ctlGate
	return f
}

// brNfProblem is "" when bridge-nf-call-iptables reads 1, else the reason.
func brNfProblem(env *Env) string {
	raw, err := env.Host.ReadFile(config.BrNfCallIptables)
	if err != nil {
		return fmt.Sprintf("%s unreadable (is br_netfilter loaded?): %v", config.BrNfCallIptables, err)
	}
	if v := strings.TrimSpace(string(raw)); v != "1" {
		return fmt.Sprintf("%s is %q, want 1: bridged frames would bypass iptables (this tool never writes sysctls)", config.BrNfCallIptables, v)
	}
	return ""
}

// gateProblems reads back the gate of the bridge: bridge-nf-call-iptables, the nft table (set, chains, policies,
// rules) and the DOCKER-USER rule. Empty when everything holds.
func gateProblems(ctx context.Context, env *Env, bridge string) []string {
	var bad []string
	if why := brNfProblem(env); why != "" {
		bad = append(bad, why)
	}
	g, found, err := env.Net.ReadGate(ctx)
	switch {
	case err != nil:
		bad = append(bad, fmt.Sprintf("table %s %s unreadable: %v", config.NftFamily, config.GateTable, err))
	case !found:
		bad = append(bad, fmt.Sprintf("table %s %s does not exist", config.NftFamily, config.GateTable))
	default:
		bad = append(bad, g.Problems(bridge)...)
	}
	lines, err := env.Net.IptRules(ctx, config.ChainDockerUser)
	switch {
	case err != nil:
		bad = append(bad, config.ChainDockerUser+" unreadable: "+err.Error())
	case !services.HasIptRule(lines, services.GateIptTokens(bridge)...):
		bad = append(bad, fmt.Sprintf("rule missing in %s: %s", config.ChainDockerUser, strings.Join(services.GateIptSpec(bridge), " ")))
	}
	return bad
}

func InstallGate(ctx context.Context, env *Env) *Failure {
	info, found, err := env.Docker.InspectNetwork(ctx, config.NetworkName)
	switch {
	case err != nil:
		return gateFailure("cannot inspect network "+config.NetworkName+" to install the gate", err)
	case !found:
		return gateFailure("network "+config.NetworkName+" is missing, the gate cannot be installed", nil)
	}
	bridge := info.BridgeName()
	if why := brNfProblem(env); why != "" {
		return gateFailure("gate not installed; network "+config.NetworkName+" left in place: "+why, nil)
	}
	if err := env.Net.LoadGate(ctx, bridge); err != nil {
		return gateFailure("table "+config.NftFamily+" "+config.GateTable+" could not be loaded", err)
	}
	if err := env.Net.InstallGateRule(ctx, bridge); err != nil {
		return gateFailure("gate rule could not be inserted into "+config.ChainDockerUser, err)
	}
	return nil
}
