// IP-AGENT-NET adapter (part 2): the per-port nftables table of one agent container.
//
// The WSL2 kernel (6.6.87.2-microsoft-standard-WSL2) has no nftables bridge family, but the netdev family with both
// the ingress and the egress hook is available. Each agent port (the host-side veth of the container on agents-net)
// gets `table netdev as_port_<id>` with two base chains bound to that veth (USS-004):
//
//	chain ingress  hook ingress device <veth>  policy drop   every frame the agent sends
//	chain egress   hook egress  device <veth>  policy drop   every frame sent to the agent
//
// A table is loaded closed (both chains policy drop, no rule) when the port is armed. The accept rules (source
// identity, layer 2, and destination limit, layer 3) are added at release by ONE atomic `nft -f -`. Disarm and
// remove delete the whole table. Every call is `nft ...` with argv only (no shell); scripts go to nft on stdin.
package services

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"strings"

	"agent-net/internal/config"
)

// NftChain is one listed base chain of a port table.
type NftChain struct {
	Hook   string   // ingress or egress
	Device string   // the veth the chain is bound to
	Policy string   // drop or accept
	Rules  []string // listed rules, one per line
}

// NftTable is the parsed `nft list table netdev as_port_<id>`.
type NftTable struct {
	Name   string
	Chains map[string]*NftChain
}

var (
	nftChainHead = regexp.MustCompile(`^chain (\S+) \{$`)
	nftHook      = regexp.MustCompile(`\bhook (\w+)`)
	nftDevice    = regexp.MustCompile(`\bdevice "?([^"\s;]+)"?`)
	nftPolicy    = regexp.MustCompile(`\bpolicy (\w+)`)
)

// parseNftTable reads the text listing of one table. Rule lines are kept as listed (whitespace collapsed).
func parseNftTable(name, out string) *NftTable {
	t := &NftTable{Name: name, Chains: map[string]*NftChain{}}
	var cur *NftChain
	for _, line := range strings.Split(out, "\n") {
		line = strings.Join(strings.Fields(line), " ")
		switch {
		case line == "":
		case nftChainHead.MatchString(line):
			cur = &NftChain{}
			t.Chains[nftChainHead.FindStringSubmatch(line)[1]] = cur
		case line == "}":
			cur = nil
		case cur == nil:
		case strings.HasPrefix(line, "type "):
			if m := nftHook.FindStringSubmatch(line); m != nil {
				cur.Hook = m[1]
			}
			if m := nftDevice.FindStringSubmatch(line); m != nil {
				cur.Device = m[1]
			}
			if m := nftPolicy.FindStringSubmatch(line); m != nil {
				cur.Policy = m[1]
			}
		default:
			cur.Rules = append(cur.Rules, line)
		}
	}
	return t
}

// Closed reports both chains bound to the veth at their hook, policy drop, and no rule: the state of an armed,
// not yet released port. Why is empty when it holds, otherwise it names the first difference.
func (t *NftTable) Closed(veth string) (ok bool, why string) {
	if why = t.Policies(veth); why != "" {
		return false, why
	}
	for _, name := range []string{config.ChainIngress, config.ChainEgress} {
		if n := len(t.Chains[name].Rules); n > 0 {
			return false, fmt.Sprintf("chain %s of %s has %d rule(s), want none before the release", name, t.Name, n)
		}
	}
	return true, ""
}

// Policies is "" when both chains exist, are bound to the veth at their hook and have policy drop (veth "" skips the
// device check), else a description of the first difference.
func (t *NftTable) Policies(veth string) string {
	for _, c := range []struct{ name, hook string }{{config.ChainIngress, "ingress"}, {config.ChainEgress, "egress"}} {
		ch := t.Chains[c.name]
		switch {
		case ch == nil:
			return fmt.Sprintf("chain %s is missing in %s", c.name, t.Name)
		case !strings.EqualFold(ch.Policy, "drop"):
			return fmt.Sprintf("chain %s of %s has policy %q, want drop", c.name, t.Name, ch.Policy)
		case ch.Hook != c.hook:
			return fmt.Sprintf("chain %s of %s is on hook %q, want %s", c.name, t.Name, ch.Hook, c.hook)
		case veth != "" && ch.Device != veth:
			return fmt.Sprintf("chain %s of %s is bound to %q, want %s", c.name, t.Name, ch.Device, veth)
		}
	}
	return ""
}

// Device is the veth the table is bound to ("" if unreadable).
func (t *NftTable) Device() string {
	for _, name := range []string{config.ChainIngress, config.ChainEgress} {
		if c := t.Chains[name]; c != nil && c.Device != "" {
			return c.Device
		}
	}
	return ""
}

// RuleCount is the number of rules in both chains.
func (t *NftTable) RuleCount() int {
	n := 0
	for _, c := range t.Chains {
		n += len(c.Rules)
	}
	return n
}

// has reports a listed rule of the chain that contains every token as whole fields (`ip saddr 172.30.0.2` is not in
// `ip saddr 172.30.0.22`).
func (t *NftTable) has(chain string, tokens []string) bool {
	c := t.Chains[chain]
	if c == nil {
		return false
	}
	for _, r := range c.Rules {
		if HasTokens(r, tokens...) {
			return true
		}
	}
	return false
}

// AgentMACs are the agent MACs that the accept rules of a released port carry: the `ether saddr` of the ingress rules
// and the `ether daddr` of the egress rules (lower case, without duplicates). The bridge MAC and the broadcast MAC are
// on the other side of those rules and are not returned. A closed table carries none.
func (t *NftTable) AgentMACs() []string {
	var out []string
	collect := func(chain, key string) {
		c := t.Chains[chain]
		if c == nil {
			return
		}
		for _, r := range c.Rules {
			f := tokenFields(r)
			for i := 0; i+2 < len(f); i++ {
				if f[i] != "ether" || f[i+1] != key {
					continue
				}
				hw, err := net.ParseMAC(f[i+2])
				if err != nil || len(hw) != 6 {
					continue
				}
				mac, dup := hw.String(), false
				for _, v := range out {
					dup = dup || v == mac
				}
				if !dup {
					out = append(out, mac)
				}
			}
		}
	}
	collect(config.ChainIngress, "saddr")
	collect(config.ChainEgress, "daddr")
	return out
}

// PortAddrs are the four addresses the accept rules of a released port are built from.
type PortAddrs struct {
	MAC, IP             string // the agent (as docker assigned them)
	BridgeMAC, BridgeIP string // the agents-net bridge interface on the host: the only allowed peer
}

// NftRule is one accept rule of a released port: the text sent to nft and the tokens its listing must contain.
type NftRule struct {
	Chain  string
	Text   string
	Tokens []string
}

// SetMarkText is the statement of the port ingress accept rules that sets the port mark bit and keeps the other bits.
func SetMarkText() string { return "meta mark set meta mark or " + config.PortMark }

// AcceptRules are the layer 2 (source identity) and layer 3 (destination limit) accept rules of a released port.
// Everything not accepted here is dropped by the policy of the two chains (IPv6 and any other frame type too).
func AcceptRules(p PortAddrs) ([]NftRule, error) {
	mac, err := parseMAC(p.MAC)
	if err != nil {
		return nil, err
	}
	bmac, err := parseMAC(p.BridgeMAC)
	if err != nil {
		return nil, err
	}
	ip, err := parseIPv4(p.IP)
	if err != nil {
		return nil, err
	}
	bip, err := parseIPv4(p.BridgeIP)
	if err != nil {
		return nil, err
	}
	in, eg, bc := config.ChainIngress, config.ChainEgress, config.BroadcastMAC
	setMark := SetMarkText()
	return []NftRule{
		// agent -> bridge: IPv4 from the declared MAC and IP, addressed to the bridge MAC only
		// Both ingress accept rules also set PortMark (other mark bits kept): the gate on the bridge requires it.
		{in, fmt.Sprintf("ether saddr %s ether daddr %s ip saddr %s %s accept", mac, bmac, ip, setMark),
			[]string{"ether saddr " + mac, "ether daddr " + bmac, "ip saddr " + ip, "meta mark set", config.PortMark, "accept"}},
		// agent -> broadcast: ARP request for the bridge IP, sender pinned to the declared MAC and IP
		{in, fmt.Sprintf("ether saddr %s ether daddr %s arp operation request arp saddr ether %s arp saddr ip %s arp daddr ip %s %s accept", mac, bc, mac, ip, bip, setMark),
			[]string{"ether saddr " + mac, "ether daddr " + bc, "arp operation request", "arp saddr ether " + mac, "arp saddr ip " + ip, "arp daddr ip " + bip, "meta mark set", config.PortMark, "accept"}},
		// There is no agent -> bridge ARP reply rule: the host's neighbour entry for the agent is permanent, so the
		// host never sends an ARP request to the agent.
		// bridge -> agent: IPv4 sent from the bridge MAC only
		{eg, fmt.Sprintf("ether saddr %s ether type ip accept", bmac), []string{"ether saddr " + bmac, "ether type ip", "accept"}},
		// bridge -> agent: ARP reply to the agent only (the bridge answers the agent's request, it never asks)
		{eg, fmt.Sprintf("ether saddr %s ether daddr %s arp operation reply accept", bmac, mac),
			[]string{"ether saddr " + bmac, "ether daddr " + mac, "arp operation reply", "accept"}},
	}, nil
}

// Released reports every accept rule of the port in its chain and no other rule in either chain (the chain
// policies are a separate check, Policies).
func (t *NftTable) Released(rules []NftRule) (ok bool, why string) {
	for _, r := range rules {
		if !t.has(r.Chain, r.Tokens) {
			return false, fmt.Sprintf("accept rule missing in chain %s of %s: %s", r.Chain, t.Name, r.Text)
		}
	}
	if n := t.RuleCount(); n != len(rules) {
		return false, fmt.Sprintf("%s has %d rule(s), want exactly the %d accept rules", t.Name, n, len(rules))
	}
	return true, ""
}

func parseMAC(s string) (string, error) {
	hw, err := net.ParseMAC(s)
	if err != nil || len(hw) != 6 {
		return "", fmt.Errorf("refusing to put MAC %q into an nftables rule", s)
	}
	return hw.String(), nil
}

func parseIPv4(s string) (string, error) {
	a, err := netip.ParseAddr(s)
	if err != nil || !a.Is4() {
		return "", fmt.Errorf("refusing to put IP %q into an nftables rule", s)
	}
	return a.String(), nil
}

var nftIdent = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

// nftName refuses anything that could break the nft script or argv.
func nftName(kind, s string) error {
	if !nftIdent.MatchString(s) || len(s) > 60 {
		return fmt.Errorf("refusing to put %s %q into an nftables command", kind, s)
	}
	return nil
}

// PortTables lists the names of the as_port_* netdev tables (`nft list tables`); other tables are ignored.
func (n *Net) PortTables(ctx context.Context) ([]string, error) {
	out, err := n.run(ctx, config.NftExe, "list", "tables")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) == 3 && f[0] == "table" && f[1] == config.NftFamily && strings.HasPrefix(f[2], config.PortTablePfx) {
			names = append(names, f[2])
		}
	}
	return names, nil
}

func (n *Net) hasPortTable(ctx context.Context, name string) (bool, error) {
	names, err := n.PortTables(ctx)
	if err != nil {
		return false, err
	}
	for _, v := range names {
		if v == name {
			return true, nil
		}
	}
	return false, nil
}

// ReadPortTable reads one port table; found is false when it does not exist.
func (n *Net) ReadPortTable(ctx context.Context, name string) (t *NftTable, found bool, err error) {
	if err := nftName("table", name); err != nil {
		return nil, false, err
	}
	present, err := n.hasPortTable(ctx, name)
	if err != nil || !present {
		return nil, false, err
	}
	out, err := n.run(ctx, config.NftExe, "list", "table", config.NftFamily, name)
	if err != nil {
		return nil, true, err
	}
	return parseNftTable(name, string(out)), true, nil
}

func (n *Net) nftScript(ctx context.Context, script string) error {
	_, err := runCmdIn(ctx, n.Runner, []byte(script), config.NftExe, "-f", "-")
	return err
}

// LoadClosedPort loads the port table closed in ONE atomic `nft -f -`: any older table of that name is replaced,
// and both chains are bound to the veth with policy drop and no rule.
func (n *Net) LoadClosedPort(ctx context.Context, table, veth string) error {
	if err := nftName("table", table); err != nil {
		return err
	}
	if err := nftName("port", veth); err != nil {
		return err
	}
	f := config.NftFamily
	script := fmt.Sprintf("add table %[1]s %[2]s\ndelete table %[1]s %[2]s\nadd table %[1]s %[2]s\n", f, table)
	for _, c := range []struct{ name, hook string }{{config.ChainIngress, "ingress"}, {config.ChainEgress, "egress"}} {
		script += fmt.Sprintf("add chain %s %s %s { type filter hook %s device \"%s\" priority 0; policy drop; }\n", f, table, c.name, c.hook, veth)
	}
	return n.nftScript(ctx, script)
}

// ReleasePort adds the accept rules of the port in ONE atomic `nft -f -`: the only operation that opens a port.
// The chains are flushed first in the same transaction, so a repeated release leaves exactly these rules. The same
// transaction adds the MAC of the agent to `released` of the agents-net gate: both open together or not at all.
func (n *Net) ReleasePort(ctx context.Context, table, mac, container string, rules []NftRule) error {
	if err := nftName("table", table); err != nil {
		return err
	}
	el, err := GateElementText(mac, container)
	if err != nil {
		return err
	}
	f := config.NftFamily
	var b strings.Builder
	for _, c := range []string{config.ChainIngress, config.ChainEgress} {
		fmt.Fprintf(&b, "flush chain %s %s %s\n", f, table, c)
	}
	for _, r := range rules {
		fmt.Fprintf(&b, "add rule %s %s %s %s\n", f, table, r.Chain, r.Text)
	}
	fmt.Fprintf(&b, "add %s\n", el)
	return n.nftScript(ctx, b.String())
}

// DeleteTable deletes the port table when it exists and reports whether it did. An absent table is not an error.
func (n *Net) DeleteTable(ctx context.Context, name string) (bool, error) {
	if err := nftName("table", name); err != nil {
		return false, err
	}
	found, err := n.hasPortTable(ctx, name)
	if err != nil || !found {
		return false, err
	}
	if _, err := n.run(ctx, config.NftExe, "delete", "table", config.NftFamily, name); err != nil {
		return false, err
	}
	return true, nil
}
