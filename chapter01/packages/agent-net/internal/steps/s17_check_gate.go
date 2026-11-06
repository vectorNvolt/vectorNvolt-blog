// SFR-NET-021 (USC-004 alt-3), `check-gate <container>`: read-only report of the host-side controls of one
// container's port. Nothing is created, changed or deleted.
//
// stdout: one line per control, `<control>: ok|missing|mismatch`, for table as_port_<id>, chain policies, accept
// rules, agents-net gate (table agents_gate, its DOCKER-USER rule, bridge-nf-call-iptables), gate release (the MAC in
// the set `released`), port flags, static FDB entry, neighbour entry, DNS egress rule, endpoint egress rule and other
// tagged iptables rules (every control is checked even after a failure), then the summary line
// `closed` (table, no accept rules, MAC not in `released`), `released` (verified accept rules, MAC in `released`) or
// `inconsistent` (also when the gate table is missing). The `[n/m]` step lines and
// debug lines go to stderr, so stdout is only the control lines and the summary.
//
// Exit 0: every control ok. Exit 1, state NOT_CHECKED: nothing was checked (invalid invocation, unknown container,
// missing privilege or prerequisite). Exit 2, state GATE_INCONSISTENT: at least one control is missing or mismatched,
// or a read-back failed.
package steps

import (
	"context"
	"fmt"
	"strings"

	"agent-net/internal/config"
	"agent-net/internal/services"
)

// Control states printed by check-gate.
const (
	ctlOK       = "ok"
	ctlMissing  = "missing"
	ctlMismatch = "mismatch"
)

// Control labels of check-gate (SFR-NET-021). The other labels are shared with arm: ctlFdb, ctlNeigh, ctlDNS, ctlEndpoint.
// The table control is `table as_port_<id>` with the container's table name.
const (
	ctlGatePolicies = "chain policies"
	ctlGateAccept   = "accept rules"
	ctlGateFlags    = "port flags"
	ctlOtherRules   = "other tagged iptables rules"
	ctlGateTable    = "agents-net gate"
	ctlGateRelease  = "gate release"
	ctlConfinement  = "container confinement"
)

// CheckInspect: the container must exist (a missing or unreadable container is "not checked").
func CheckInspect(ctx context.Context, env *Env) *Failure {
	_, found, err := env.Docker.LookupContainer(ctx, env.Container)
	if err != nil {
		return fail(ExitPrerequisite, StateNotChecked, "cannot inspect container "+env.Container, err)
	}
	if !found {
		return &Failure{ExitCode: ExitPrerequisite, ExitState: StateNotChecked, Message: fmt.Sprintf("container %s does not exist", env.Container)}
	}
	return nil
}

// portCheck collects the result line of each control and the reason of every one that is not ok.
type portCheck struct {
	lines []string
	bad   []string
}

func (p *portCheck) add(control, state, why string) {
	p.lines = append(p.lines, control+": "+state)
	if state != ctlOK {
		p.bad = append(p.bad, fmt.Sprintf("%s: %s: %s", control, state, why))
	}
}

func (p *portCheck) allOK() bool { return len(p.bad) == 0 }

func CheckPortControls(ctx context.Context, env *Env) *Failure {
	var p portCheck

	// The live veth, found from the host as in arming. A container without one (stopped, not attached) leaves the
	// port flags and the fdb entry unreadable; the table and the iptables rules are found by name and tag.
	env.Port = Port{}
	veth, noVeth := "", ""
	if f := IdentifyPort(ctx, env); f != nil {
		noVeth = f.Message
	} else {
		veth = env.Port.Veth
	}

	table := env.PortTable()
	t, found, err := env.Net.ReadPortTable(ctx, table)
	released, hasAccept := false, false // released: accept rules verified; hasAccept: the table holds any accept rule
	var tableMACs []string
	tableControl := "table " + table
	switch {
	case err != nil:
		for _, c := range []string{tableControl, ctlGatePolicies, ctlGateAccept} {
			p.add(c, ctlMismatch, fmt.Sprintf("table %s unreadable: %v", table, err))
		}
	case !found:
		for _, c := range []string{tableControl, ctlGatePolicies, ctlGateAccept} {
			p.add(c, ctlMissing, fmt.Sprintf("table %s %s does not exist", config.NftFamily, table))
		}
	default:
		p.add(tableControl, ctlOK, "")
		switch why := t.Policies(veth); {
		case why == "":
			p.add(ctlGatePolicies, ctlOK, "")
		case len(t.Chains) == 0:
			p.add(ctlGatePolicies, ctlMissing, why)
		default:
			p.add(ctlGatePolicies, ctlMismatch, why)
		}
		released = checkAcceptRules(ctx, env, &p, t)
		tableMACs = t.AgentMACs()
		hasAccept = t.RuleCount() > 0
	}

	checkGate(ctx, env, &p, hasAccept, tableMACs)
	checkPortFlags(ctx, env, &p, veth, noVeth)
	checkFdb(ctx, env, &p, veth, noVeth)
	checkNeigh(ctx, env, &p, veth, noVeth)
	checkTaggedRules(ctx, env, &p)
	checkConfinement(ctx, env, &p)

	switch {
	case !p.allOK():
		p.lines = append(p.lines, "inconsistent")
	case released:
		p.lines = append(p.lines, "released")
	default:
		p.lines = append(p.lines, "closed")
	}
	env.Log.Success(p.lines...)

	if !p.allOK() {
		return &Failure{
			ExitCode:  ExitRuntime,
			ExitState: StateGateInconsistent,
			Message:   fmt.Sprintf("port controls of container %s are missing or differ", env.Container),
			Checks:    p.bad,
		}
	}
	return nil
}

// checkAcceptRules: no accept rule is "closed" (ok); all accept rules of the port and no other is "released" (ok);
// anything else is a mismatch. It reports whether the port is released.
func checkAcceptRules(ctx context.Context, env *Env, p *portCheck, t *services.NftTable) bool {
	const control = ctlGateAccept
	if t.RuleCount() == 0 {
		p.add(control, ctlOK, "")
		return false
	}
	if env.Port.MAC == "" || env.Port.IP == "" {
		p.add(control, ctlMismatch, "accept rules are present but the MAC and IP of the container are unknown, so they cannot be verified")
		return false
	}
	links, err := env.Net.HostLinks(ctx)
	if err != nil {
		p.add(control, ctlMismatch, fmt.Sprintf("host links unreadable: %v", err))
		return false
	}
	bridgeMAC := ""
	for _, l := range links {
		if l.IfName == env.Bridge {
			bridgeMAC = l.Address
		}
	}
	rules, err := services.AcceptRules(services.PortAddrs{MAC: env.Port.MAC, IP: env.Port.IP, BridgeMAC: bridgeMAC, BridgeIP: env.Network.Gateway()})
	if err != nil {
		p.add(control, ctlMismatch, fmt.Sprintf("expected accept rules unknown: %v", err))
		return false
	}
	if ok, why := t.Released(rules); !ok {
		p.add(control, ctlMismatch, why)
		return false
	}
	p.add(control, ctlOK, "")
	return true
}

// checkGate reports the agents-net gate (table agents_gate with its set and chains, the DOCKER-USER rule,
// bridge-nf-call-iptables) and the state of the container's MAC in the set `released`: a closed port (its table holds
// no accept rule) must not be in it, a port with accept rules must be (anything else is a mismatch). A missing gate
// table is missing, hence inconsistent. tableMACs are the MACs the accept rules of the port table carry.
func checkGate(ctx context.Context, env *Env, p *portCheck, hasAccept bool, tableMACs []string) {
	g, found, err := env.Net.ReadGate(ctx)
	problems := gateProblems(ctx, env, env.Bridge)
	switch {
	case err != nil:
		p.add(ctlGateTable, ctlMismatch, fmt.Sprintf("table %s unreadable: %v", config.GateTable, err))
	case !found:
		p.add(ctlGateTable, ctlMissing, strings.Join(problems, "; "))
	case len(problems) > 0:
		p.add(ctlGateTable, ctlMismatch, strings.Join(problems, "; "))
	default:
		p.add(ctlGateTable, ctlOK, "")
	}

	mac := env.Port.MAC
	if mac == "" && len(tableMACs) > 0 {
		mac = tableMACs[0]
	}
	switch {
	case err != nil || !found:
		p.add(ctlGateRelease, ctlMissing, fmt.Sprintf("table %s is not available, so the MAC cannot be looked up in set %s", config.GateTable, config.GateSet))
	case mac == "" && hasAccept:
		p.add(ctlGateRelease, ctlMismatch, "the MAC of the container is unknown, so it cannot be looked up in set "+config.GateSet)
	case mac == "":
		p.add(ctlGateRelease, ctlOK, "")
	case hasAccept && !g.Has(mac):
		p.add(ctlGateRelease, ctlMismatch, fmt.Sprintf("port has accept rules but %s is not in set %s", mac, config.GateSet))
	case !hasAccept && g.Has(mac):
		p.add(ctlGateRelease, ctlMismatch, fmt.Sprintf("port has no accept rule but %s is in set %s", mac, config.GateSet))
	default:
		p.add(ctlGateRelease, ctlOK, "")
	}
}

func checkPortFlags(ctx context.Context, env *Env, p *portCheck, veth, noVeth string) {
	const control = ctlGateFlags
	if veth == "" {
		p.add(control, ctlMissing, "no host-side veth found: "+noVeth)
		return
	}
	port, err := env.Net.PortState(ctx, veth)
	if err != nil {
		p.add(control, ctlMismatch, fmt.Sprintf("bridge port state of %s unreadable: %v", veth, err))
		return
	}
	var diff []string
	if port.Master != env.Bridge {
		diff = append(diff, fmt.Sprintf("master is %q, want %s", port.Master, env.Bridge))
	}
	if !port.Isolated {
		diff = append(diff, "isolated is off")
	}
	if port.Learning {
		diff = append(diff, "learning is on")
	}
	if !port.Locked {
		diff = append(diff, "locked is off")
	}
	if len(diff) > 0 {
		p.add(control, ctlMismatch, veth+": "+strings.Join(diff, ", "))
		return
	}
	p.add(control, ctlOK, "")
}

func checkFdb(ctx context.Context, env *Env, p *portCheck, veth, noVeth string) {
	const control = ctlFdb
	if veth == "" {
		p.add(control, ctlMissing, "no host-side veth found: "+noVeth)
		return
	}
	entries, err := env.Net.FdbEntries(ctx, veth)
	if err != nil {
		p.add(control, ctlMismatch, fmt.Sprintf("bridge fdb of %s unreadable: %v", veth, err))
		return
	}
	for _, e := range entries {
		if strings.EqualFold(e.MAC, env.Port.MAC) && e.Static() {
			p.add(control, ctlOK, "")
			return
		}
	}
	p.add(control, ctlMissing, fmt.Sprintf("no static fdb entry for %s on %s", env.Port.MAC, veth))
}

// checkNeigh: the permanent neighbour entry of the agent IP on the bridge, lladdr = the agent MAC. A container
// without a live veth has no known IP, so the entry is reported missing like the fdb entry.
func checkNeigh(ctx context.Context, env *Env, p *portCheck, veth, noVeth string) {
	const control = ctlNeigh
	if veth == "" {
		p.add(control, ctlMissing, "no host-side veth found: "+noVeth)
		return
	}
	e, found, err := env.Net.NeighEntry(ctx, env.Port.IP, env.Bridge)
	switch {
	case err != nil:
		p.add(control, ctlMismatch, fmt.Sprintf("neighbour entry of %s on %s unreadable: %v", env.Port.IP, env.Bridge, err))
	case !found:
		p.add(control, ctlMissing, fmt.Sprintf("no neighbour entry for %s on %s", env.Port.IP, env.Bridge))
	case !strings.EqualFold(e.LLAddr, env.Port.MAC) || !e.Permanent():
		p.add(control, ctlMismatch, fmt.Sprintf("neighbour entry for %s is %s %v, want %s PERMANENT", env.Port.IP, e.LLAddr, e.State, env.Port.MAC))
	default:
		p.add(control, ctlOK, "")
	}
}

// checkConfinement: the container must have restart policy exactly "no", be attached to agents-net and to no other
// network, and be running. It is not required to be unpaused: create_agent_container runs check-gate while the
// container is still paused.
func checkConfinement(ctx context.Context, env *Env, p *portCheck) {
	const control = ctlConfinement
	info, found, err := env.Docker.LookupContainer(ctx, env.Container)
	switch {
	case err != nil:
		p.add(control, ctlMismatch, fmt.Sprintf("container %s unreadable: %v", env.Container, err))
		return
	case !found:
		p.add(control, ctlMissing, fmt.Sprintf("container %s does not exist", env.Container))
		return
	}
	var diff []string
	if r := info.HostConfig.RestartPolicy.Name; r != "no" {
		diff = append(diff, fmt.Sprintf("restart policy is %q, want no", r))
	}
	if _, on := info.NetworkSettings.Networks[config.NetworkName]; !on || len(info.NetworkSettings.Networks) != 1 {
		diff = append(diff, fmt.Sprintf("attached to %d network(s), want only %s", len(info.NetworkSettings.Networks), config.NetworkName))
	}
	if !info.State.Running {
		diff = append(diff, "container is not running")
	}
	if len(diff) > 0 {
		p.add(control, ctlMismatch, strings.Join(diff, ", "))
		return
	}
	p.add(control, ctlOK, "")
}

// dportOf is the value of the `--dport` match of a rule line ("" if none): whole value, so 5353 is not 53.
func dportOf(line string) string {
	f := strings.Fields(line)
	for i := 0; i+1 < len(f); i++ {
		if f[i] == "--dport" {
			return f[i+1]
		}
	}
	return ""
}

// checkTaggedRules reads the rules tagged agent-net:<container> and reports the DNS egress rule, the endpoint egress
// rule and the other tagged rules (default-deny of both chains, replies accept) as three distinct controls. The wanted
// rules come from iptPlan, the plan arm applies and verifies, so the tokens are the same: destination, protocol,
// port, `-s <ip>`, ESTABLISHED,RELATED with the source, default-deny with the source. The endpoint addresses are
// resolved on the host again, as arm does (a CDN address that changed since arm shows as a mismatch).
// A control is ok when every wanted rule is there and no other accept rule of its kind is; missing when nothing of
// its kind is there; mismatch otherwise (a rule of the kind with another destination, source or port).
func checkTaggedRules(ctx context.Context, env *Env, p *portCheck) {
	groups := []string{ctlDNS, ctlEndpoint, ctlOtherRules}
	chains := []string{config.ChainDockerUser, config.ChainInput}
	refs := map[string][]services.IptRef{}
	lines := map[string][]string{}
	total := 0
	for _, chain := range chains {
		r, err := env.Net.IptTagged(ctx, chain, exactTag(env))
		if err != nil {
			for _, g := range groups {
				p.add(g, ctlMismatch, chain+" unreadable: "+err.Error())
			}
			return
		}
		refs[chain] = r
		for _, ref := range r {
			lines[chain] = append(lines[chain], ref.Line)
		}
		total += len(r)
	}
	if total == 0 {
		for _, g := range groups {
			p.add(g, ctlMissing, "no rule tagged "+config.CommentPrefix+env.Container)
		}
		return
	}
	if env.Port.IP == "" { // no live veth: the source is the one the tagged rules carry
		for _, chain := range chains {
			for _, ref := range refs[chain] {
				if env.Port.IP == "" {
					env.Port.IP = ref.Source()
				}
			}
		}
	}
	endpointErr := resolveModelAPIs(ctx, env)
	plan := iptPlan(env)

	type finding struct {
		present  bool // a rule of this kind exists
		problems []string
	}
	found := map[string]*finding{}
	for _, g := range groups {
		found[g] = &finding{}
	}
	groupOf := func(control string) string {
		if control == ctlDNS || control == ctlEndpoint {
			return control
		}
		return ctlOtherRules
	}
	if endpointErr != nil {
		f := found[ctlEndpoint]
		f.present = true
		f.problems = append(f.problems, "endpoint addresses could not be resolved: "+endpointErr.Error())
	}
	for _, r := range plan {
		f := found[groupOf(r.Control)]
		switch {
		case services.HasIptRule(lines[r.Chain], r.Tokens...):
			f.present = true
		default:
			if services.HasIptRule(lines[r.Chain], r.Loose...) {
				f.present = true // a rule of this kind, but not the wanted one
			}
			f.problems = append(f.problems, "missing in "+r.Chain+": "+strings.Join(r.Spec, " "))
		}
	}
	// "only": a tagged accept rule in DOCKER-USER or INPUT that no wanted rule explains is a mismatch of its kind.
	// An unexpected accept rule of INPUT is always an "other" rule: it is addressed to the host.
	for _, chain := range chains {
		for _, ref := range refs[chain] {
			if !services.HasTokens(ref.Line, "-j ACCEPT") {
				continue
			}
			wanted := false
			for _, r := range plan {
				wanted = wanted || (r.Chain == chain && services.HasIptRule([]string{ref.Line}, r.Tokens...))
			}
			if wanted {
				continue
			}
			g := ctlOtherRules
			if chain == config.ChainDockerUser {
				switch dportOf(ref.Line) {
				case config.DNSPort:
					g = ctlDNS
				case config.ModelAPIPort:
					g = ctlEndpoint
				}
			}
			if g == ctlEndpoint && endpointErr != nil {
				continue
			}
			found[g].present = true
			found[g].problems = append(found[g].problems, "unexpected rule in "+chain+": "+ref.Line)
		}
	}

	for _, g := range groups {
		f := found[g]
		switch {
		case len(f.problems) == 0:
			p.add(g, ctlOK, "")
		case !f.present:
			p.add(g, ctlMissing, strings.Join(f.problems, "; "))
		default:
			p.add(g, ctlMismatch, strings.Join(f.problems, "; "))
		}
	}
}
