// IP-AGENT-NET adapter (part 3): the network-wide gate of agents-net, installed by create_agent_net.
//
// A container is attached to agents-net at `docker create`, so its port exists before `arm` runs. The gate keeps every
// port closed until it is armed. `table netdev agents_gate` is bound to the bridge device br-<id> (the nft bridge family
// is not available on the WSL2 kernel):
//
//	set released { type ether_addr; }
//	chain ingress  hook ingress device <br>  policy drop   ether saddr @released meta mark and <bit> == <bit> meta mark set meta mark and <~bit> accept
//	chain egress   hook egress  device <br>  policy drop   ether daddr @released accept
//
// `arm` adds the MAC of the container to `released`, with the container name as the element comment, in the same atomic
// transaction that opens its port table; `disarm` and remove take it out again (by MAC and by comment). A DOCKER-USER rule (comment agent-net:gate) drops bridged port-to-port traffic
// (bridge-nf-call-iptables must be 1 for it to see bridged frames).
package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"

	"agent-net/internal/config"
)

// Gate is the parsed `nft list table netdev agents_gate`.
type Gate struct {
	Table    *NftTable
	HasSet   bool     // set `released` exists
	SetType  string   // its type (ether_addr)
	Elements []string // MACs in the set, lower case
}

var (
	nftSetHead  = regexp.MustCompile(`^set (\S+) \{$`)
	nftSetType  = regexp.MustCompile(`^type (\S+)`)
	nftElements = regexp.MustCompile(`^elements\s*=\s*\{(.*)$`)
)

// parseGate reads the listing of the gate table: chains as for a port table, plus the set and its elements.
func parseGate(out string) *Gate {
	g := &Gate{Table: parseNftTable(config.GateTable, out)}
	inSet, inElems := false, false
	var elems strings.Builder
	for _, line := range strings.Split(out, "\n") {
		line = strings.Join(strings.Fields(line), " ")
		switch {
		case inElems:
			elems.WriteString(" " + line)
			if strings.Contains(line, "}") {
				inElems = false
			}
		case nftSetHead.MatchString(line):
			inSet = nftSetHead.FindStringSubmatch(line)[1] == config.GateSet
			g.HasSet = g.HasSet || inSet
		case inSet && line == "}":
			inSet = false
		case inSet && nftSetType.MatchString(line):
			g.SetType = nftSetType.FindStringSubmatch(line)[1]
		case inSet && nftElements.MatchString(line):
			elems.WriteString(nftElements.FindStringSubmatch(line)[1])
			inElems = !strings.Contains(line, "}")
		}
	}
	text := elems.String()
	if i := strings.Index(text, "}"); i >= 0 {
		text = text[:i]
	}
	for _, f := range strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == ' ' }) {
		if hw, err := net.ParseMAC(f); err == nil && len(hw) == 6 {
			g.Elements = append(g.Elements, hw.String())
		}
	}
	return g
}

// Has reports whether the MAC is in the set `released`.
func (g *Gate) Has(mac string) bool {
	hw, err := net.ParseMAC(mac)
	if err != nil {
		return false
	}
	for _, e := range g.Elements {
		if e == hw.String() {
			return true
		}
	}
	return false
}

// Problems lists the differences from the gate create_agent_net installs on the bridge device (empty when it holds):
// the set, both chains bound to the bridge with policy drop, and exactly the one rule in each chain.
func (g *Gate) Problems(bridge string) []string {
	var bad []string
	switch {
	case !g.HasSet:
		bad = append(bad, fmt.Sprintf("set %s is missing in %s", config.GateSet, config.GateTable))
	case g.SetType != "ether_addr":
		bad = append(bad, fmt.Sprintf("set %s has type %q, want ether_addr", config.GateSet, g.SetType))
	}
	if why := g.Table.Policies(bridge); why != "" {
		return append(bad, why)
	}
	for _, c := range []struct{ chain, key string }{{config.ChainIngress, "saddr"}, {config.ChainEgress, "daddr"}} {
		rules := g.Table.Chains[c.chain].Rules
		if len(rules) != 1 || !HasTokens(rules[0], gateRuleTokens(c.chain, c.key)...) {
			bad = append(bad, fmt.Sprintf("chain %s of %s must hold exactly the rule %s, has %v", c.chain, config.GateTable, GateRuleText(c.chain, c.key), rules))
		}
	}
	return bad
}

// GateRuleText is the one rule of a gate chain. Egress: `ether daddr @released accept`. Ingress additionally requires
// the port mark bit that the port table sets and clears it again (the other mark bits stay):
//
//	ether saddr @released meta mark and 0x10000000 == 0x10000000 meta mark set meta mark and 0xefffffff accept
func GateRuleText(chain, key string) string {
	text := fmt.Sprintf("ether %s @%s", key, config.GateSet)
	if chain == config.ChainIngress {
		text += fmt.Sprintf(" meta mark and %[1]s == %[1]s meta mark set meta mark and %[2]s", config.PortMark, config.PortMarkInv)
	}
	return text + " accept"
}

// gateRuleTokens are the tokens the listed rule of a gate chain must contain (the ingress rule also the mark condition
// and the mark clearing; an older gate without them is a mismatch).
func gateRuleTokens(chain, key string) []string {
	t := []string{"ether " + key, "@" + config.GateSet, "accept"}
	if chain == config.ChainIngress {
		t = append(t, "meta mark", config.PortMark, "meta mark set", config.PortMarkInv)
	}
	return t
}

// GateIptSpec is the DOCKER-USER rule that drops frames bridged from one agents-net port to another.
func GateIptSpec(bridge string) []string {
	return []string{"-i", bridge, "-o", bridge, "-m", "physdev", "--physdev-is-bridged", "-m", "comment", "--comment", config.GateComment, "-j", "DROP"}
}

// GateIptTokens are the tokens the listed gate rule must contain.
func GateIptTokens(bridge string) []string {
	return []string{"-i " + bridge, "-o " + bridge, "-m physdev", "--physdev-is-bridged", config.GateComment, "-j DROP"}
}

// IsGateComment matches the comment of the gate rule.
func IsGateComment(c string) bool { return c == config.GateComment }

func (n *Net) hasTable(ctx context.Context, name string) (bool, error) {
	out, err := n.run(ctx, config.NftExe, "list", "tables")
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) == 3 && f[0] == "table" && f[1] == config.NftFamily && f[2] == name {
			return true, nil
		}
	}
	return false, nil
}

// LoadGate loads the gate in ONE atomic `nft -f -`; an older table of that name is replaced.
func (n *Net) LoadGate(ctx context.Context, bridge string) error {
	if err := nftName("bridge", bridge); err != nil {
		return err
	}
	f, t, set := config.NftFamily, config.GateTable, config.GateSet
	script := fmt.Sprintf("add table %[1]s %[2]s\ndelete table %[1]s %[2]s\nadd table %[1]s %[2]s\n", f, t)
	script += fmt.Sprintf("add set %s %s %s { type ether_addr; }\n", f, t, set)
	for _, c := range []struct{ name, key string }{{config.ChainIngress, "saddr"}, {config.ChainEgress, "daddr"}} {
		script += fmt.Sprintf("add chain %s %s %s { type filter hook %s device \"%s\" priority 0; policy drop; }\n", f, t, c.name, c.name, bridge)
		script += fmt.Sprintf("add rule %s %s %s %s\n", f, t, c.name, GateRuleText(c.name, c.key))
	}
	return n.nftScript(ctx, script)
}

// ReadGate reads the gate table; found is false when it does not exist.
func (n *Net) ReadGate(ctx context.Context) (g *Gate, found bool, err error) {
	present, err := n.hasTable(ctx, config.GateTable)
	if err != nil || !present {
		return nil, false, err
	}
	out, err := n.run(ctx, config.NftExe, "list", "table", config.NftFamily, config.GateTable)
	if err != nil {
		return nil, true, err
	}
	return parseGate(string(out)), true, nil
}

// GateElementText is the nft element expression of one MAC in the set `released` (after `add`). The element carries
// the container name as its comment, so that disarm can find it by name when docker no longer reports the MAC. Element
// comments need nft >= 0.9.1 and a kernel with set element userdata (any 4.x or newer).
func GateElementText(mac, comment string) (string, error) {
	m, err := parseMAC(mac)
	if err != nil {
		return "", err
	}
	if err := nftName("container", comment); err != nil {
		return "", err
	}
	return fmt.Sprintf("element %s %s %s { %s comment \"%s\" }", config.NftFamily, config.GateTable, config.GateSet, m, comment), nil
}

// GateElem is one element of the set `released`: the MAC (lower case) and its comment ("" for an untagged element).
type GateElem struct {
	MAC     string
	Comment string
}

// GateElements lists the elements of `released` with their comments (`nft -j list set`). found is false when the gate
// table does not exist; a table without the set has no elements.
func (n *Net) GateElements(ctx context.Context) (elems []GateElem, found bool, err error) {
	present, err := n.hasTable(ctx, config.GateTable)
	if err != nil || !present {
		return nil, false, err
	}
	out, err := n.run(ctx, config.NftExe, "-j", "list", "set", config.NftFamily, config.GateTable, config.GateSet)
	if err != nil {
		var ce *CmdError
		if errors.As(err, &ce) && ce.Err == nil && strings.Contains(strings.ToLower(ce.Stderr), "no such file") {
			return nil, true, nil // the set is missing: the gate check reports that, there is no element to look at
		}
		return nil, true, err
	}
	elems, err = parseGateElements(out)
	return elems, true, err
}

// parseGateElements reads the JSON of `nft -j list set`: an element is a bare string, or an object
// {"elem": {"val": "<mac>", "comment": "<text>"}} when it carries a comment.
func parseGateElements(out []byte) ([]GateElem, error) {
	var doc struct {
		Nftables []struct {
			Set *struct {
				Elem []json.RawMessage `json:"elem"`
			} `json:"set"`
		} `json:"nftables"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(out), &doc); err != nil {
		return nil, fmt.Errorf("cannot parse nft set listing: %w", err)
	}
	var elems []GateElem
	for _, item := range doc.Nftables {
		if item.Set == nil {
			continue
		}
		for _, raw := range item.Set.Elem {
			var val, comment string
			var obj struct {
				Elem struct {
					Val     string `json:"val"`
					Comment string `json:"comment"`
				} `json:"elem"`
			}
			if json.Unmarshal(raw, &val) != nil {
				if err := json.Unmarshal(raw, &obj); err != nil {
					return nil, fmt.Errorf("cannot parse nft set element %s: %w", string(raw), err)
				}
				val, comment = obj.Elem.Val, obj.Elem.Comment
			}
			hw, err := net.ParseMAC(val)
			if err != nil || len(hw) != 6 {
				continue
			}
			elems = append(elems, GateElem{MAC: hw.String(), Comment: comment})
		}
	}
	return elems, nil
}

// RemoveGateElements takes out of `released` every element whose MAC is in macs or whose comment is comment (when
// comment is not empty) and returns how many it removed. An absent table or element is not an error.
func (n *Net) RemoveGateElements(ctx context.Context, macs []string, comment string) (int, error) {
	elems, found, err := n.GateElements(ctx)
	if err != nil || !found {
		return 0, err
	}
	want := map[string]bool{}
	for _, m := range macs {
		if hw, err := net.ParseMAC(m); err == nil {
			want[hw.String()] = true
		}
	}
	removed := 0
	for _, e := range elems {
		if !want[e.MAC] && !(comment != "" && e.Comment == comment) {
			continue
		}
		if _, err := n.run(ctx, config.NftExe, "delete", "element", config.NftFamily, config.GateTable, config.GateSet, "{", e.MAC, "}"); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

// RemoveGateMACs takes the MACs out of `released` and returns how many it removed. An absent table or element is
// not an error.
func (n *Net) RemoveGateMACs(ctx context.Context, macs []string) (int, error) {
	g, found, err := n.ReadGate(ctx)
	if err != nil || !found {
		return 0, err
	}
	removed := 0
	for _, mac := range macs {
		if !g.Has(mac) {
			continue
		}
		m, err := parseMAC(mac)
		if err != nil {
			return removed, err
		}
		if _, err := n.run(ctx, config.NftExe, "delete", "element", config.NftFamily, config.GateTable, config.GateSet, "{", m, "}"); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

// DeleteGate deletes the gate table when it exists and reports whether it did.
func (n *Net) DeleteGate(ctx context.Context) (bool, error) {
	present, err := n.hasTable(ctx, config.GateTable)
	if err != nil || !present {
		return false, err
	}
	if _, err := n.run(ctx, config.NftExe, "delete", "table", config.NftFamily, config.GateTable); err != nil {
		return false, err
	}
	return true, nil
}

// InstallGateRule puts the gate rule at the top of DOCKER-USER; a rule of an older network (another bridge) is
// deleted first.
func (n *Net) InstallGateRule(ctx context.Context, bridge string) error {
	if err := nftName("bridge", bridge); err != nil {
		return err
	}
	if _, err := n.IptDeleteTagged(ctx, config.ChainDockerUser, IsGateComment); err != nil {
		return err
	}
	return n.IptInsert(ctx, config.ChainDockerUser, GateIptSpec(bridge)...)
}
