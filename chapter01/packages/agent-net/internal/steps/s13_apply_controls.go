// SFR-NET-009 + SFR-NET-010 + SFR-NET-024 + SSR-NET-002 (USC-004 alt-1): apply the host-side controls on the container's veth / bridge, after
// ClosePort has loaded the closed port table. The first failing control stops the run and names itself; applied
// controls stay (SFR-NET-011). The source identity accept rules are NOT added here: they come with the release.
//
//	port isolation        bridge link set dev <veth> isolated on
//	mac learning          bridge link set dev <veth> learning off
//	port lock             bridge link set dev <veth> locked on                   (never unlocked while the container exists)
//	static FDB entry      bridge fdb replace <mac> dev <veth> master static      (<mac> = MAC docker assigned)
//	neighbour entry       ip neigh replace <ip> lladdr <mac> dev <bridge> nud permanent   (the host never ARPs for the agent, so no ARP request leaves towards it)
//	DNS egress rule       DOCKER-USER: allow the fixed resolvers 1.1.1.1 and 9.9.9.9 (53 udp+tcp)
//	endpoint egress rule  DOCKER-USER: allow api.anthropic.com / platform.claude.com (443 tcp)
//	egress default-deny   DOCKER-USER: drop the rest leaving agents-net
//	host-input deny       INPUT: drop from the container IP; replies to host-initiated flows allowed
//
// Every iptables rule carries the comment agent-net:<container>.
package steps

import (
	"context"
	"fmt"
	"net/netip"
	"strings"

	"agent-net/internal/config"
	"agent-net/internal/services"
)

// Control names, shared by apply, verify and the failure report.
const (
	ctlTable     = "port table"
	ctlIsolation = "port isolation"
	ctlLearning  = "mac learning off"
	ctlLock      = "port lock"
	ctlFdb       = "static FDB entry"
	ctlNeigh     = "neighbour entry"
	ctlEgress    = "egress default-deny"
	ctlInput     = "host-input default-deny"
	ctlDNS       = "DNS egress rule"      // SFR-NET-009 (a), SFR-NET-010, SFR-NET-021
	ctlEndpoint  = "endpoint egress rule" // SFR-NET-009 (b), SFR-NET-010, SFR-NET-021
	ctlRelease   = "port release"
	ctlGateSet   = "gate release set" // MAC in the set `released` of the agents-net gate
)

// dnsResolvers are the two fixed DNS resolvers of the DNS egress rule, primary first.
var dnsResolvers = []string{config.ResolverPrimary, config.ResolverSecondary}

// iptRule is one wanted rule: the arguments to insert and the tokens that must appear in `iptables -S`.
// Control is the control the rule belongs to (ctlDNS, ctlEndpoint, ctlEgress or ctlInput). Loose are the tokens of
// any rule of the same kind (check-gate: a rule with them but without Tokens is a mismatch, none at all is missing).
type iptRule struct {
	Chain   string
	Control string
	Spec    []string
	Tokens  []string
	Loose   []string
}

// acceptSpec is the one definition of an egress accept rule (src and dst are <ip>/32, tag the agent-net:<container>
// comment). Used by iptPlan (arm, check-gate) and by refresh_endpoints.
func acceptSpec(tag, src, dst, proto, port string) []string {
	spec := []string{"-s", src, "-d", dst + "/32", "-p", proto, "-m", proto, "--dport", port}
	return append(spec, "-m", "comment", "--comment", tag, "-j", "ACCEPT")
}

// iptPlan lists the wanted rules per chain, top to bottom. Rules carry the container name as comment.
// Used by arm (apply, verify) and check-gate, so both check the same tokens.
func iptPlan(env *Env) []iptRule {
	tag := config.CommentPrefix + env.Container
	src := env.Port.IP + "/32"
	comment := []string{"-m", "comment", "--comment", tag}
	var plan []iptRule

	accept := func(chain, control, dst, proto, port string) iptRule {
		spec := acceptSpec(tag, src, dst, proto, port)
		return iptRule{chain, control, spec,
			[]string{"-s " + src, "-d " + dst + "/32", "-p " + proto, "--dport " + port, tag, "-j ACCEPT"},
			[]string{tag, "--dport " + port, "-j ACCEPT"}}
	}

	// DOCKER-USER: leaving agents-net.
	for _, dst := range dnsResolvers {
		for _, proto := range []string{"udp", "tcp"} {
			plan = append(plan, accept(config.ChainDockerUser, ctlDNS, dst, proto, config.DNSPort))
		}
	}
	for _, ip := range env.Allowed.ModelIPs {
		plan = append(plan, accept(config.ChainDockerUser, ctlEndpoint, ip, "tcp", config.ModelAPIPort))
	}
	plan = append(plan, iptRule{
		Chain:   config.ChainDockerUser,
		Control: ctlEgress,
		Spec:    append(append([]string{"-s", src, "!", "-o", env.Bridge}, comment...), "-j", "DROP"),
		Tokens:  []string{"-s " + src, "! -o " + env.Bridge, tag, "-j DROP"},
		Loose:   []string{tag, "-j DROP"},
	})

	// INPUT: addressed to the host. Replies to host-initiated flows only; the resolvers are external, so no DNS accept.
	plan = append(plan, iptRule{
		Chain:   config.ChainInput,
		Control: ctlInput,
		Spec:    append(append([]string{"-s", src, "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED"}, comment...), "-j", "ACCEPT"),
		Tokens:  []string{"-s " + src, "--ctstate", "ESTABLISHED", "RELATED", tag, "-j ACCEPT"},
		Loose:   []string{tag, "ESTABLISHED", "-j ACCEPT"},
	})
	plan = append(plan, iptRule{
		Chain:   config.ChainInput,
		Control: ctlInput,
		Spec:    append(append([]string{"-s", src}, comment...), "-j", "DROP"),
		Tokens:  []string{"-s " + src, tag, "-j DROP"},
		Loose:   []string{tag, "-j DROP"},
	})
	return plan
}

func ApplyControls(ctx context.Context, env *Env) *Failure {
	veth := env.Port.Veth

	// A PERMANENT entry of the IP with another lladdr is a leftover of an earlier container, unless another live
	// endpoint owns that lladdr: then the IP is in conflict and nothing is applied.
	if e, found, err := env.Net.NeighEntry(ctx, env.Port.IP, env.Bridge); err != nil {
		return failControl(ctlNeigh, "neighbour entry of "+env.Port.IP+" unreadable", err)
	} else if found && e.Permanent() && !strings.EqualFold(e.LLAddr, env.Port.MAC) {
		live, err := liveMACs(ctx, env)()
		if err != nil {
			return failControl(ctlNeigh, "endpoints of "+config.NetworkName+" unreadable", err)
		}
		if contains(live, strings.ToLower(e.LLAddr)) {
			return failControl(ctlNeigh, fmt.Sprintf("conflict: %s on %s is pinned to %s, the MAC of another container on %s", env.Port.IP, env.Bridge, e.LLAddr, config.NetworkName), nil)
		}
		if err := env.Net.DelNeigh(ctx, env.Port.IP, env.Bridge); err != nil {
			return failControl(ctlNeigh, fmt.Sprintf("could not delete the stale entry of %s (%s) on %s", env.Port.IP, e.LLAddr, env.Bridge), err)
		}
	}

	if err := env.Net.IsolatePort(ctx, veth); err != nil {
		return failControl(ctlIsolation, "could not isolate "+veth, err)
	}
	if err := env.Net.DisableLearning(ctx, veth); err != nil {
		return failControl(ctlLearning, "could not disable learning on "+veth, err)
	}
	if err := env.Net.LockPort(ctx, veth); err != nil {
		return failControl(ctlLock, "could not lock "+veth, err)
	}
	if err := env.Net.PinFdb(ctx, env.Port.MAC, veth); err != nil {
		return failControl(ctlFdb, "could not pin "+env.Port.MAC+" on "+veth, err)
	}
	if err := env.Net.PinNeigh(ctx, env.Port.IP, env.Port.MAC, env.Bridge); err != nil {
		return failControl(ctlNeigh, fmt.Sprintf("could not pin %s as %s on %s", env.Port.IP, env.Port.MAC, env.Bridge), err)
	}

	if err := resolveModelAPIs(ctx, env); err != nil {
		return failControl(ctlEndpoint, "endpoint addresses could not be resolved", err)
	}
	for _, c := range []struct{ control, chain string }{
		{ctlEgress, config.ChainDockerUser},
		{ctlInput, config.ChainInput},
	} {
		if err := insertRules(ctx, env, c.chain); err != nil {
			return failControl(c.control, "rules could not be inserted into "+c.chain, err)
		}
	}
	return keepPaused(ctx, env, "after the controls were applied")
}

// insertRules inserts the missing rules of one chain, last wanted rule first, so each ends above the previous one.
func insertRules(ctx context.Context, env *Env, chain string) error {
	var rules []iptRule
	for _, r := range iptPlan(env) {
		if r.Chain == chain {
			rules = append(rules, r)
		}
	}
	for i := len(rules) - 1; i >= 0; i-- {
		lines, err := env.Net.IptRules(ctx, chain)
		if err != nil {
			return err
		}
		if services.HasIptRule(lines, rules[i].Tokens...) {
			continue
		}
		if err := env.Net.IptInsert(ctx, chain, rules[i].Spec...); err != nil {
			return err
		}
	}
	return nil
}

// resolveModelAPIs resolves the endpoint hosts host-side (the DNS resolvers are fixed, see dnsResolvers).
func resolveModelAPIs(ctx context.Context, env *Env) error {
	// api.anthropic.com and platform.claude.com (TCP 443), resolved now and fixed for the life of the rules.
	env.Allowed.ModelIPs = nil
	if env.Config.EndpointsErr != nil { // an unusable endpoints file is never replaced by the defaults
		return fmt.Errorf("endpoints file: %w", env.Config.EndpointsErr)
	}
	for _, host := range env.Config.ModelAPIs {
		if a, err := netip.ParseAddr(host); err == nil && a.Is4() {
			env.Allowed.ModelIPs = appendUnique(env.Allowed.ModelIPs, host)
			continue
		}
		ips, err := env.Host.LookupIPv4(ctx, host)
		if err != nil {
			return fmt.Errorf("model API %s: %w", host, err)
		}
		if len(ips) == 0 {
			return fmt.Errorf("model API %s has no IPv4 address", host)
		}
		for _, ip := range ips {
			env.Allowed.ModelIPs = appendUnique(env.Allowed.ModelIPs, ip)
		}
	}
	return nil
}
