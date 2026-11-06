// IP-AGENT-NET adapter (part 1): host network state and host-side controls through ip, bridge and iptables.
// Everything runs on the host side; nothing is executed inside a container.
package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"agent-net/internal/config"
)

// Net offers typed operations on the host network.
type Net struct {
	Runner Runner
}

func NewNet(r Runner) *Net { return &Net{Runner: r} }

func (n *Net) run(ctx context.Context, args ...string) ([]byte, error) {
	return runCmd(ctx, n.Runner, args...)
}

// AddrInfo and Iface mirror the parts of `ip -j addr` / `ip -j link` that are read.
type AddrInfo struct {
	Family    string `json:"family"`
	Local     string `json:"local"`
	PrefixLen int    `json:"prefixlen"`
}

type Iface struct {
	IfIndex   int        `json:"ifindex"`
	IfName    string     `json:"ifname"`
	Address   string     `json:"address"`
	LinkIndex int        `json:"link_index"`
	Master    string     `json:"master"`
	AddrInfo  []AddrInfo `json:"addr_info"`
}

func (n *Net) ifaces(ctx context.Context, args ...string) ([]Iface, error) {
	out, err := n.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	var v []Iface
	if err := json.Unmarshal(bytes.TrimSpace(out), &v); err != nil {
		return nil, fmt.Errorf("cannot parse output of %s: %w", strings.Join(args, " "), err)
	}
	return v, nil
}

// Range is an address range in use on the host, with a label for the failure report.
type Range struct {
	Label  string
	Prefix netip.Prefix
	Addr   netip.Addr // the interface address (addresses only)
}

// HostAddrs lists the IPv4 addresses of the host network namespace (`ip -j addr`).
func (n *Net) HostAddrs(ctx context.Context) ([]Range, error) {
	ifs, err := n.ifaces(ctx, config.IPExe, "-j", "addr")
	if err != nil {
		return nil, err
	}
	var out []Range
	for _, i := range ifs {
		for _, a := range i.AddrInfo {
			if a.Family != "inet" {
				continue
			}
			addr, err := netip.ParseAddr(a.Local)
			if err != nil {
				continue
			}
			out = append(out, Range{
				Label:  fmt.Sprintf("address %s/%d on %s", a.Local, a.PrefixLen, i.IfName),
				Prefix: netip.PrefixFrom(addr, a.PrefixLen).Masked(),
				Addr:   addr,
			})
		}
	}
	return out, nil
}

// HostRoutes lists the IPv4 routes of all tables (`ip -j route show table all`); default routes are skipped.
func (n *Net) HostRoutes(ctx context.Context) ([]Range, error) {
	data, err := n.run(ctx, config.IPExe, "-j", "route", "show", "table", "all")
	if err != nil {
		return nil, err
	}
	var routes []struct {
		Dst   string `json:"dst"`
		Dev   string `json:"dev"`
		Table string `json:"table"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(data), &routes); err != nil {
		return nil, fmt.Errorf("cannot parse ip route output: %w", err)
	}
	var out []Range
	for _, r := range routes {
		if r.Dst == "" || r.Dst == "default" || r.Dst == "0.0.0.0/0" {
			continue
		}
		p, err := netip.ParsePrefix(r.Dst)
		if err != nil {
			a, aerr := netip.ParseAddr(r.Dst)
			if aerr != nil {
				continue
			}
			p = netip.PrefixFrom(a, a.BitLen())
		}
		table := r.Table
		if table == "" {
			table = "main"
		}
		out = append(out, Range{Label: fmt.Sprintf("route %s dev %s table %s", r.Dst, r.Dev, table), Prefix: p.Masked()})
	}
	return out, nil
}

// HostLinks lists the host interfaces with their master (`ip -j link`).
func (n *Net) HostLinks(ctx context.Context) ([]Iface, error) {
	return n.ifaces(ctx, config.IPExe, "-j", "link")
}

// NetnsIfaces lists the interfaces in the network namespace of a container PID. The host's own
// `ip` is run in that namespace via nsenter; no code of the container is executed (SSR-NET-002).
func (n *Net) NetnsIfaces(ctx context.Context, pid int) ([]Iface, error) {
	return n.ifaces(ctx, config.NsenterExe, "-t", strconv.Itoa(pid), "-n", config.IPExe, "-j", "addr")
}

// BridgePort is the applied state of one bridge port (`bridge -d -j link show dev`).
type BridgePort struct {
	IfName   string `json:"ifname"`
	Master   string `json:"master"`
	Isolated bool   `json:"isolated"`
	Learning bool   `json:"learning"`
	Locked   bool   `json:"locked"`
}

// PortState needs `bridge -d`: without it iproute2 omits the port flags, which would decode as false.
// A missing flag is an error, never a default.
func (n *Net) PortState(ctx context.Context, dev string) (BridgePort, error) {
	out, err := n.run(ctx, config.BridgeExe, "-d", "-j", "link", "show", "dev", dev)
	if err != nil {
		return BridgePort{}, err
	}
	var ports []struct {
		IfName   string `json:"ifname"`
		Master   string `json:"master"`
		Isolated *bool  `json:"isolated"`
		Learning *bool  `json:"learning"`
		Locked   *bool  `json:"locked"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(out), &ports); err != nil {
		return BridgePort{}, fmt.Errorf("cannot parse bridge link output for %s: %w", dev, err)
	}
	if len(ports) == 0 {
		return BridgePort{}, fmt.Errorf("%s is not a bridge port", dev)
	}
	p := ports[0]
	for _, f := range []struct {
		name string
		v    *bool
	}{{"isolated", p.Isolated}, {"learning", p.Learning}, {"locked", p.Locked}} {
		if f.v == nil {
			return BridgePort{}, fmt.Errorf("bridge link output for %s lacks the %s flag", dev, f.name)
		}
	}
	return BridgePort{IfName: p.IfName, Master: p.Master, Isolated: *p.Isolated, Learning: *p.Learning, Locked: *p.Locked}, nil
}

func (n *Net) IsolatePort(ctx context.Context, dev string) error {
	_, err := n.run(ctx, config.BridgeExe, "link", "set", "dev", dev, "isolated", "on")
	return err
}

func (n *Net) DisableLearning(ctx context.Context, dev string) error {
	_, err := n.run(ctx, config.BridgeExe, "link", "set", "dev", dev, "learning", "off")
	return err
}

// LockPort sets the kernel MAC lock: frames from a MAC without a static fdb entry on the port are dropped.
// A port is never unlocked while its container exists (SSR-NET-003).
func (n *Net) LockPort(ctx context.Context, dev string) error {
	_, err := n.run(ctx, config.BridgeExe, "link", "set", "dev", dev, "locked", "on")
	return err
}

// PinFdb installs the static forwarding entry of the declared MAC on the port.
func (n *Net) PinFdb(ctx context.Context, mac, dev string) error {
	_, err := n.run(ctx, config.BridgeExe, "fdb", "replace", mac, "dev", dev, "master", "static")
	return err
}

// FdbEntry is one entry of `bridge -j fdb show dev`.
type FdbEntry struct {
	MAC    string   `json:"mac"`
	Master string   `json:"master"`
	State  string   `json:"state"`
	Flags  []string `json:"flags"`
	Vlan   int      `json:"vlan"` // 0 = the entry carries no vlan
}

// Static reports a non-learned entry. iproute2 prints a `static` fdb entry as "static" or
// "permanent" depending on its version, so both are accepted.
func (e FdbEntry) Static() bool {
	for _, s := range append([]string{e.State}, e.Flags...) {
		if s == "static" || s == "permanent" {
			return true
		}
	}
	return false
}

func (n *Net) FdbEntries(ctx context.Context, dev string) ([]FdbEntry, error) {
	out, err := n.run(ctx, config.BridgeExe, "-j", "fdb", "show", "dev", dev)
	if err != nil {
		return nil, err
	}
	var v []FdbEntry
	if err := json.Unmarshal(bytes.TrimSpace(out), &v); err != nil {
		return nil, fmt.Errorf("cannot parse bridge fdb output for %s: %w", dev, err)
	}
	return v, nil
}

// isNoChain reports iptables' answer that the chain does not exist.
func isNoChain(err error) bool {
	var ce *CmdError
	if !errors.As(err, &ce) || ce.Err != nil {
		return false
	}
	s := strings.ToLower(ce.Stderr)
	return strings.Contains(s, "no chain/target/match by that name") || strings.Contains(s, "chain does not exist")
}

// IptRules reads the rules of a chain (`iptables -w -S <chain>`). A chain that does not exist has no rules; any other
// failure is an error. Every iptables call waits for the xtables lock (-w), so that concurrent rule changes cannot shift
// the rule numbers between the listing and a deletion.
func (n *Net) IptRules(ctx context.Context, chain string) ([]string, error) {
	out, err := n.run(ctx, config.IptablesExe, "-w", "-S", chain)
	if err != nil {
		if isNoChain(err) {
			return nil, nil
		}
		return nil, err
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n"), nil
}

// IptInsert puts a rule at the top of a chain.
func (n *Net) IptInsert(ctx context.Context, chain string, spec ...string) error {
	_, err := n.run(ctx, append([]string{config.IptablesExe, "-w", "-I", chain, "1"}, spec...)...)
	return err
}

// IptDelete deletes the rule that matches the exact spec (`iptables -w -D <chain> <spec>`), not a rule number.
func (n *Net) IptDelete(ctx context.Context, chain string, spec ...string) error {
	_, err := n.run(ctx, append([]string{config.IptablesExe, "-w", "-D", chain}, spec...)...)
	return err
}

// IptRef is one rule of `iptables -S <chain>` with its 1-based rule number and its comment ("" if none).
type IptRef struct {
	Num     int
	Line    string
	Comment string
}

// Source is the address of the `-s` match of the rule ("" if none), without the /32.
func (r IptRef) Source() string {
	f := strings.Fields(r.Line)
	for i := 0; i+1 < len(f); i++ {
		if f[i] == "-s" {
			if p, err := netip.ParsePrefix(f[i+1]); err == nil {
				return p.Addr().String()
			}
			return f[i+1]
		}
	}
	return ""
}

// IptTagged lists the rules of a chain whose comment satisfies match.
func (n *Net) IptTagged(ctx context.Context, chain string, match func(comment string) bool) ([]IptRef, error) {
	lines, err := n.IptRules(ctx, chain)
	if err != nil {
		return nil, err
	}
	var out []IptRef
	num := 0
	for _, l := range lines {
		if !strings.HasPrefix(l, "-A "+chain+" ") {
			continue
		}
		num++
		f := strings.Fields(l)
		for i := 0; i+1 < len(f); i++ {
			if f[i] == "--comment" {
				if c := strings.Trim(f[i+1], `"`); match(c) {
					out = append(out, IptRef{Num: num, Line: l, Comment: c})
				}
				break
			}
		}
	}
	return out, nil
}

// IptDeleteTagged deletes the rules of a chain whose comment satisfies match, by rule number, highest first.
// Rules that are already gone are not an error. It returns the number deleted.
func (n *Net) IptDeleteTagged(ctx context.Context, chain string, match func(comment string) bool) (int, error) {
	refs, err := n.IptTagged(ctx, chain, match)
	if err != nil {
		return 0, err
	}
	deleted := 0
	for i := len(refs) - 1; i >= 0; i-- {
		if _, err := n.run(ctx, config.IptablesExe, "-w", "-D", chain, strconv.Itoa(refs[i].Num)); err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}

// DeleteFdb removes one static forwarding entry of the MAC from the port. The kernel keeps one entry per vlan
// (plus one without vlan) for a single `fdb replace`, and `fdb del` removes only the exactly matching one: vlan 0
// deletes the entry without vlan, vlan n > 0 the entry of that vlan.
func (n *Net) DeleteFdb(ctx context.Context, mac, dev string, vlan int) error {
	args := []string{config.BridgeExe, "fdb", "del", mac, "dev", dev}
	if vlan > 0 {
		args = append(args, "vlan", strconv.Itoa(vlan))
	}
	_, err := n.run(ctx, append(args, "master", "static")...)
	return err
}

// NeighEntry is one entry of `ip -j -4 neigh show`.
type NeighEntry struct {
	Dst    string   `json:"dst"`
	Dev    string   `json:"dev"`
	LLAddr string   `json:"lladdr"`
	State  []string `json:"state"`
}

// Permanent reports the PERMANENT neighbour state (set by `nud permanent`, never aged out or overwritten by ARP).
func (e NeighEntry) Permanent() bool {
	for _, s := range e.State {
		if strings.EqualFold(s, "permanent") {
			return true
		}
	}
	return false
}

// PinNeigh installs the permanent neighbour entry of the agent on the bridge: the host never sends an ARP request
// for the agent, the bridge only answers the agent's own request.
func (n *Net) PinNeigh(ctx context.Context, ip, mac, dev string) error {
	if _, err := parseIPv4(ip); err != nil {
		return err
	}
	if _, err := parseMAC(mac); err != nil {
		return err
	}
	_, err := n.run(ctx, config.IPExe, "neigh", "replace", ip, "lladdr", mac, "dev", dev, "nud", "permanent")
	return err
}

// NeighEntry reads the neighbour entry of the IP on the bridge; found is false when there is none.
func (n *Net) NeighEntry(ctx context.Context, ip, dev string) (e NeighEntry, found bool, err error) {
	if _, err := parseIPv4(ip); err != nil {
		return NeighEntry{}, false, err
	}
	out, err := n.run(ctx, config.IPExe, "-j", "-4", "neigh", "show", ip, "dev", dev)
	if err != nil {
		return NeighEntry{}, false, err
	}
	var v []NeighEntry
	if trimmed := bytes.TrimSpace(out); len(trimmed) > 0 {
		if err := json.Unmarshal(trimmed, &v); err != nil {
			return NeighEntry{}, false, fmt.Errorf("cannot parse ip neigh output for %s on %s: %w", ip, dev, err)
		}
	}
	if len(v) == 0 {
		return NeighEntry{}, false, nil
	}
	return v[0], true, nil
}

// DelNeigh deletes the neighbour entry of the IP on the bridge.
func (n *Net) DelNeigh(ctx context.Context, ip, dev string) error {
	_, err := n.run(ctx, config.IPExe, "neigh", "del", ip, "dev", dev)
	return err
}

// DeleteNeigh removes the neighbour entry of the IP from the bridge and reports whether it deleted one. An absent
// entry is not an error. An entry whose lladdr is one of macs is deleted. A PERMANENT entry with another lladdr is
// deleted too when that lladdr belongs to no live endpoint of agents-net (liveMACs; a leftover of an uncleanly
// disarmed earlier container); one that belongs to a live endpoint is left alone (SFR-NET-012, SSR-NET-004).
func (n *Net) DeleteNeigh(ctx context.Context, ip, dev string, macs []string, liveMACs func() ([]string, error)) (bool, error) {
	e, found, err := n.NeighEntry(ctx, ip, dev)
	if err != nil || !found {
		return false, err
	}
	owned := false
	for _, m := range macs {
		owned = owned || strings.EqualFold(m, e.LLAddr)
	}
	if !owned {
		if !e.Permanent() {
			return false, nil
		}
		live, err := liveMACs()
		if err != nil {
			return false, err
		}
		for _, m := range live {
			if strings.EqualFold(m, e.LLAddr) {
				return false, nil
			}
		}
	}
	if err := n.DelNeigh(ctx, ip, dev); err != nil {
		return false, err
	}
	return true, nil
}

// tokenFields splits a rule listing into whole fields: on blanks and commas (`ESTABLISHED,RELATED` is two states),
// with the quotes of a field dropped.
func tokenFields(s string) []string {
	f := strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == '\t' || r == ',' })
	for i := range f {
		f[i] = strings.Trim(f[i], `"`)
	}
	return f
}

// hasFields reports the token (one or more words) as whole consecutive fields of the line: `agent-net:web-1` is not
// in `agent-net:web-10`, `--dport 53` is not in `--dport 5353`.
func hasFields(fields []string, token string) bool {
	want := tokenFields(token)
	for i := 0; i+len(want) <= len(fields); i++ {
		match := true
		for j, w := range want {
			if fields[i+j] != w {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// HasTokens reports whether the line contains every token as whole fields.
func HasTokens(line string, tokens ...string) bool {
	fields := tokenFields(line)
	for _, t := range tokens {
		if !hasFields(fields, t) {
			return false
		}
	}
	return true
}

// HasIptRule reports whether one line of `iptables -S` contains every token as whole fields.
func HasIptRule(lines []string, tokens ...string) bool {
	for _, l := range lines {
		if HasTokens(l, tokens...) {
			return true
		}
	}
	return false
}
