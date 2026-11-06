package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"agent-net/internal/config"
	"agent-net/internal/sequence"
	"agent-net/internal/services"
)

// ---- fake process runner (docker, ip, bridge, nft, iptables, nsenter) ----

const (
	netID      = "abcdef0123456789abcdef0123456789"
	bridgeName = "br-abcdef012345"
	bridgeMAC  = "02:42:11:22:33:44"
	bridgeIP   = "172.30.0.1"
	container  = "web-1"
	tableName  = "as_port_web-1"
	vethName   = "vethabc"
	peerIndex  = 7
	agentIP    = "172.30.0.2"
	agentMAC   = "02:42:ac:1e:00:02"
	createCmd  = "docker network create --driver bridge --subnet 172.30.0.0/24 agents-net"
	inspectCmd = "docker network inspect --format {{json .}} agents-net"
	nftScript  = "nft -f -"
	brNfFile   = "/proc/sys/net/bridge/bridge-nf-call-iptables"
	gateRule   = "-i " + bridgeName + " -o " + bridgeName + " -m physdev --physdev-is-bridged -m comment --comment agent-net:gate -j DROP"
)

type call struct {
	Args  []string
	Stdin []byte
}

func (c call) joined() string { return strings.Join(c.Args, " ") }

type rule struct {
	contains string
	stdin    string // when set, the call's stdin must contain it too
	stderr   string
	code     int
	err      error
}

type netSpec struct{ ID, Name, Subnet string }

// fakeTable is one `table netdev as_port_<id>`: the veth its chains are bound to, per chain the policy and the rules.
type fakeTable struct {
	veth   string
	policy map[string]string
	rules  map[string][]string
}

func (t *fakeTable) clone() *fakeTable {
	c := &fakeTable{veth: t.veth, policy: map[string]string{}, rules: map[string][]string{}}
	for k, v := range t.policy {
		c.policy[k] = v
	}
	for k, v := range t.rules {
		c.rules[k] = append([]string(nil), v...)
	}
	return c
}

func (t *fakeTable) ruleCount() int {
	n := 0
	for _, r := range t.rules {
		n += len(r)
	}
	return n
}

// fakeGate is `table netdev agents_gate`: the bridge device its chains are bound to, the set `released` and its elements
// (lower case MACs), per chain the policy and the rules.
type fakeGate struct {
	device   string
	set      bool
	setType  string
	released []string
	comments map[string]string // mac -> element comment (container name); absent = untagged element
	policy   map[string]string
	rules    map[string][]string
}

// addElem puts a MAC (with its comment) into the set.
func (g *fakeGate) addElem(mac, comment string) {
	mac = strings.ToLower(mac)
	if g.has(mac) {
		return
	}
	g.released = append(g.released, mac)
	if comment != "" {
		if g.comments == nil {
			g.comments = map[string]string{}
		}
		g.comments[mac] = comment
	}
}

// delElem takes a MAC out of the set.
func (g *fakeGate) delElem(mac string) {
	mac = strings.ToLower(mac)
	var keep []string
	for _, m := range g.released {
		if m != mac {
			keep = append(keep, m)
		}
	}
	g.released = keep
	delete(g.comments, mac)
}

// setJSON prints the set like `nft -j list set netdev agents_gate released`.
func (g *fakeGate) setJSON() string {
	elems := []any{}
	for _, m := range g.released {
		if c := g.comments[m]; c != "" {
			elems = append(elems, map[string]any{"elem": map[string]any{"val": m, "comment": c}})
		} else {
			elems = append(elems, m)
		}
	}
	set := map[string]any{"family": "netdev", "name": config.GateSet, "table": config.GateTable, "type": "ether_addr", "handle": 2}
	if len(elems) > 0 {
		set["elem"] = elems
	}
	return mustJSON(map[string]any{"nftables": []any{
		map[string]any{"metainfo": map[string]any{"version": "1.0.9", "json_schema_version": 1}},
		map[string]any{"set": set},
	}})
}

// portMarkSet is the statement of the two port ingress accept rules that sets the port mark bit (other bits kept).
const portMarkSet = "meta mark set meta mark or 0x10000000"

// gateIngressRule is the exact ingress rule of the gate: it requires the port mark bit and clears it again.
const gateIngressRule = "ether saddr @released meta mark and 0x10000000 == 0x10000000 meta mark set meta mark and 0xefffffff accept"

// newGate is the gate create_agent_net installs on the bridge.
func newGate(device string) *fakeGate {
	return &fakeGate{device: device, set: true, setType: "ether_addr", policy: map[string]string{"ingress": "drop", "egress": "drop"},
		rules: map[string][]string{"ingress": {gateIngressRule}, "egress": {"ether daddr @released accept"}}}
}

func (g *fakeGate) clone() *fakeGate {
	if g == nil {
		return nil
	}
	c := &fakeGate{device: g.device, set: g.set, setType: g.setType, released: append([]string(nil), g.released...),
		comments: map[string]string{}, policy: map[string]string{}, rules: map[string][]string{}}
	for k, v := range g.comments {
		c.comments[k] = v
	}
	for k, v := range g.policy {
		c.policy[k] = v
	}
	for k, v := range g.rules {
		c.rules[k] = append([]string(nil), v...)
	}
	return c
}

func (g *fakeGate) has(mac string) bool {
	for _, m := range g.released {
		if m == strings.ToLower(mac) {
			return true
		}
	}
	return false
}

// listedRule turns the rule text that was sent to nft (`meta mark and`/`or`, ether saddr before ether daddr) into the
// text real nft 1.0.9 lists: `&` and `|`, and `ether daddr` before `ether saddr`.
func listedRule(r string) string {
	r = strings.ReplaceAll(r, " meta mark and ", " meta mark & ")
	r = strings.ReplaceAll(r, " meta mark or ", " meta mark | ")
	f := strings.Fields(r)
	if len(f) >= 6 && f[0] == "ether" && f[1] == "saddr" && f[3] == "ether" && f[4] == "daddr" {
		f = append([]string{f[3], f[4], f[5], f[0], f[1], f[2]}, f[6:]...)
	}
	return strings.Join(f, " ")
}

// listing prints the table like `nft list table netdev agents_gate`.
func (g *fakeGate) listing() string {
	out := fmt.Sprintf("table netdev %s {\n", config.GateTable)
	if g.set {
		out += fmt.Sprintf("\tset %s {\n\t\ttype %s\n", config.GateSet, g.setType)
		if len(g.released) > 0 {
			var els []string
			for _, m := range g.released {
				if c := g.comments[m]; c != "" {
					m += ` comment "` + c + `"`
				}
				els = append(els, m)
			}
			out += "\t\telements = { " + strings.Join(els, ", ") + " }\n"
		}
		out += "\t}\n\n"
	}
	for _, c := range []string{config.ChainIngress, config.ChainEgress} {
		out += fmt.Sprintf("\tchain %s {\n\t\ttype filter hook %s device \"%s\" priority filter; policy %s;\n", c, c, g.device, g.policy[c])
		for _, r := range g.rules[c] {
			out += "\t\t" + listedRule(r) + "\n"
		}
		out += "\t}\n"
	}
	return out + "}\n"
}

// gateIptRule is the DOCKER-USER rule of the gate as `iptables -S` lists it (without the leading -A DOCKER-USER).
func gateIptRule(bridge string) string {
	return strings.Join(services.GateIptSpec(bridge), " ")
}

type fakeRunner struct {
	mu    sync.Mutex
	calls []call
	rules []rule

	// docker and host state (create flow)
	netExists bool
	created   bool
	vanish    bool   // agents-net not inspectable after creation
	driver    string // overrides "bridge"
	subnet    string // overrides the requested subnet
	internal  bool   // the network reports Internal=true
	bridgeOpt string // com.docker.network.bridge.name
	others    []netSpec
	addrs     [][2]string // ifname, cidr
	routes    [][2]string // dst, dev
	onCreate  func()

	// container state (arm flow)
	pid         int
	noEndpoint  bool
	noPeer      bool
	wrongMaster bool
	extraNet    bool // docker inspect reports a second attached network (bridge) next to agents-net
	notPaused   bool // docker inspect reports State.Paused=false (legacy container and every ctrs entry)
	stopped     bool // docker inspect reports State.Running=false (legacy container)
	noPortFlags bool // bridge link output as without -d: no isolated/learning/locked keys

	// applied host state, changed only by the apply commands
	tamper   map[string]bool // command succeeds but the state is not applied
	nft      map[string]*fakeTable
	gate     *fakeGate // table netdev agents_gate; nil when it does not exist
	isolated bool
	learning bool
	locked   bool
	fdb      bool
	fdbGone  [2]bool           // fdb entry of the pinned MAC deleted: [0] the one without vlan, [1] the one of vlan 1
	neigh    map[string]string // ip -> lladdr of the permanent `ip neigh` entries on the bridge
	ipt      map[string][]string

	// container model (disarm, remove and some check-gate flows): when ctrs is nil the one legacy container is answered
	ctrs    map[string]*fakeCtr
	members []string // names listed by `docker ps -a --filter network=agents-net`
	deleted []string // port tables deleted, in order (the gate table is counted in gateDeleted)

	gateDeleted int // `nft delete table netdev agents_gate` calls that succeeded

	// Injected failures that are NOT docker's not-found answer (the engine is unreachable, permission denied, ...):
	// such a failure must never be read as "absent".
	netInspectErr string            // `docker network inspect` fails with this stderr
	ctrInspectErr map[string]string // `docker container inspect <name>` fails with this stderr; "*" is every container
	noChain       map[string]bool   // `iptables -S <chain>` answers that the chain does not exist

	// An agents-net that exists at the first call (netExists) comes with its gate, as create_agent_net leaves it: the nft
	// table with the bridge device and the DOCKER-USER rule. noGate / noGateRule leave either out (a network that
	// predates the gate).
	gateInit, noGate, noGateRule bool

	// HostConfig of the inspected container; "" is the required --network agents-net / --restart no ("default" restart is "").
	netMode, restart string
	runtime          string // "" is runsc

	// The container interface carries no address by default (runsc moves it into its netstack); nsWithIP puts the docker IP
	// on it (runc), nsOtherIP a different IPv4 address.
	nsWithIP  bool
	nsOtherIP string

	dupIface  bool // the container namespace has a second interface with the same MAC (and IP, if nsWithIP)
	hideRules bool // accept rules are written but `nft list table` does not show them

	// afterCall runs after every dispatched call (the call mutex is held: it must only change fake state).
	afterCall func(j string)

	// the docker CLI host option (`-H`) and the iptables lock option (`-w`) are removed from the recorded calls
	dockerHosts []string // per docker call: its -H value, "" when it had none
	iptNoWait   int      // iptables calls without -w
}

// fakeCtr is one container of the container model; pid 0 is a stopped container (no veth, and docker
// reports no IP/MAC for it unless keepIdentity is set; vethStays keeps the veth on the host).
type fakeCtr struct {
	ip, mac, veth string
	peer, pid     int
	fdb           bool
	fdbGone       [2]bool // as fakeRunner.fdbGone
	keepIdentity  bool
	vethStays     bool
	paused        bool // `docker rm -f` fails while set (runsc zombie); `docker unpause` clears it
}

func newRunner() *fakeRunner {
	return &fakeRunner{pid: 4242, learning: true, tamper: map[string]bool{}, ipt: map[string][]string{}, nft: map[string]*fakeTable{}}
}

// normalize removes `-H <host>` from docker calls and `-w` from iptables calls, noting whether they were there, so
// that the expected command strings of the tests stay readable.
func (f *fakeRunner) normalize(args []string) []string {
	switch {
	case len(args) >= 3 && args[0] == "docker" && args[1] == "-H":
		f.dockerHosts = append(f.dockerHosts, args[2])
		return append([]string{"docker"}, args[3:]...)
	case len(args) >= 1 && args[0] == "docker":
		f.dockerHosts = append(f.dockerHosts, "")
	case len(args) >= 2 && args[0] == "iptables" && args[1] == "-w":
		return append([]string{"iptables"}, args[2:]...)
	case len(args) >= 1 && args[0] == "iptables":
		f.iptNoWait++
	}
	return args
}

func (f *fakeRunner) Run(_ context.Context, stdin []byte, args ...string) ([]byte, []byte, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ensureGate()
	args = f.normalize(args)
	f.calls = append(f.calls, call{Args: append([]string(nil), args...), Stdin: stdin})
	j := strings.Join(args, " ")
	for _, r := range f.rules {
		if strings.Contains(j, r.contains) && (r.stdin == "" || strings.Contains(string(stdin), r.stdin)) {
			return nil, []byte(r.stderr), r.code, r.err
		}
	}
	out, errb, code, err := f.dispatch(j, args, stdin)
	if f.afterCall != nil {
		f.afterCall(j)
	}
	return out, errb, code, err
}

// ensureGate gives an agents-net that exists at the first call its gate (see gateInit). The state helpers call it too,
// so that a snapshot taken before the first run already contains the gate.
func (f *fakeRunner) ensureGate() {
	if f.gateInit {
		return
	}
	f.gateInit = true
	if !f.netExists {
		return
	}
	if !f.noGate {
		f.gate = newGate(f.bridge())
	}
	if !f.noGateRule {
		f.ipt["DOCKER-USER"] = append(f.ipt["DOCKER-USER"], gateIptRule(f.bridge()))
	}
}

// ctrInspectFailure is the injected stderr for `docker container inspect <name>` ("" when none).
func (f *fakeRunner) ctrInspectFailure(name string) string {
	if v, ok := f.ctrInspectErr[name]; ok {
		return v
	}
	return f.ctrInspectErr["*"]
}

// hostConfig is the HostConfig docker reports for the container.
func (f *fakeRunner) hostConfig() map[string]any {
	mode, restart := "agents-net", "no"
	if f.netMode != "" {
		mode = f.netMode
	}
	switch f.restart {
	case "":
	case "default":
		restart = ""
	default:
		restart = f.restart
	}
	runtime := "runsc"
	if f.runtime != "" {
		runtime = f.runtime
	}
	return map[string]any{"NetworkMode": mode, "Runtime": runtime, "RestartPolicy": map[string]any{"Name": restart}}
}

func ok(s string) ([]byte, []byte, int, error) { return []byte(s), nil, 0, nil }

func exit1(msg string) ([]byte, []byte, int, error) { return nil, []byte(msg), 1, nil }

// fdbHostEntries mirrors `bridge -j fdb show dev <veth>` of a real host after arm: one `fdb replace` leaves two static
// entries for the pinned MAC (one with "vlan":1, one without), next to the permanent entries of the veth's own MAC
// and the self multicast entries. gone marks the pinned entries already deleted.
func fdbHostEntries(mac, master string, pinned bool, gone [2]bool) []map[string]any {
	out := []map[string]any{}
	if pinned {
		if !gone[1] {
			out = append(out, map[string]any{"mac": mac, "vlan": 1, "master": master, "state": "static"})
		}
		if !gone[0] {
			out = append(out, map[string]any{"mac": mac, "master": master, "state": "static"})
		}
	}
	return append(out,
		map[string]any{"mac": "b2:74:b6:4b:2b:eb", "vlan": 1, "master": master, "state": "permanent"},
		map[string]any{"mac": "b2:74:b6:4b:2b:eb", "master": master, "state": "permanent"},
		map[string]any{"mac": "33:33:00:00:00:01", "flags": []string{"self"}, "state": "permanent"},
		map[string]any{"mac": "01:00:5e:00:00:01", "flags": []string{"self"}, "state": "permanent"},
		map[string]any{"mac": "33:33:ff:4b:2b:eb", "flags": []string{"self"}, "state": "permanent"})
}

func fdbNoEntry() ([]byte, []byte, int, error) {
	return nil, []byte("RTNETLINK answers: No such file or directory"), 255, nil
}

// fdbDel is `bridge fdb del <mac> dev <veth> [vlan <n>] master static`: it removes only the exactly matching
// (mac, vlan) entry and fails with ENOENT when there is none.
func fdbDel(pinned *bool, gone *[2]bool, args []string) ([]byte, []byte, int, error) {
	vlan := 0
	if len(args) > 7 && args[6] == "vlan" {
		vlan, _ = strconv.Atoi(args[7])
	}
	if !*pinned || vlan < 0 || vlan > 1 || gone[vlan] {
		return fdbNoEntry()
	}
	gone[vlan] = true
	if gone[0] && gone[1] {
		*pinned = false
	}
	return ok("")
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func (f *fakeRunner) agentsNetExists() bool { return f.netExists || (f.created && !f.vanish) }

func (f *fakeRunner) bridge() string {
	if f.bridgeOpt != "" {
		return f.bridgeOpt
	}
	return bridgeName
}

func netJSON(id, name, driver, subnet string, internal bool, opts map[string]string) string {
	return mustJSON(map[string]any{
		"Id": id, "Name": name, "Driver": driver, "Internal": internal,
		"IPAM":    map[string]any{"Config": []map[string]any{{"Subnet": subnet, "Gateway": bridgeIP}}},
		"Options": opts,
	})
}

func (f *fakeRunner) ctrByPid(pid int) *fakeCtr {
	for _, c := range f.ctrs {
		if c.pid == pid && pid > 0 {
			return c
		}
	}
	return nil
}

func (f *fakeRunner) ctrByVeth(veth string) *fakeCtr {
	for _, c := range f.ctrs {
		if c.veth == veth {
			return c
		}
	}
	return nil
}

// nsAddrInfo is the addr_info of the container interface in its netns: none (runsc), the docker IP or another IPv4.
func (f *fakeRunner) nsAddrInfo() []map[string]any {
	ip := f.nsOtherIP
	if f.nsWithIP {
		ip = agentIP
	}
	if ip == "" {
		return []map[string]any{}
	}
	return []map[string]any{{"family": "inet", "local": ip, "prefixlen": 24}}
}

func (f *fakeRunner) hostLinks() []map[string]any {
	return []map[string]any{{"ifindex": 2, "ifname": f.bridge(), "address": bridgeMAC}}
}

func (f *fakeRunner) dispatch(j string, args []string, stdin []byte) ([]byte, []byte, int, error) {
	switch {
	case f.netInspectErr != "" && strings.HasPrefix(j, "docker network inspect "):
		return exit1(f.netInspectErr)

	case strings.HasPrefix(j, "docker container inspect ") && f.ctrInspectFailure(args[len(args)-1]) != "":
		return exit1(f.ctrInspectFailure(args[len(args)-1]))

	case strings.HasPrefix(j, "iptables -S ") && f.noChain[args[2]]:
		return exit1("iptables: No chain/target/match by that name.")

	case f.ctrs != nil && strings.HasPrefix(j, "docker container inspect --format {{json .}} "):
		c := f.ctrs[args[len(args)-1]]
		if c == nil {
			return exit1("Error: No such container: " + args[len(args)-1])
		}
		ep := map[string]any{"IPAddress": "", "MacAddress": ""}
		if c.pid > 0 || c.keepIdentity {
			ep = map[string]any{"IPAddress": c.ip, "MacAddress": c.mac}
		}
		nets := map[string]any{"agents-net": ep}
		if f.extraNet {
			nets["bridge"] = map[string]any{"IPAddress": "172.17.0.2", "MacAddress": "02:42:ac:11:00:02"}
		}
		return ok(mustJSON(map[string]any{
			"State":           map[string]any{"Pid": c.pid, "Running": c.pid > 0, "Paused": !f.notPaused},
			"HostConfig":      f.hostConfig(),
			"NetworkSettings": map[string]any{"Networks": nets},
		}))

	case f.ctrs != nil && j == "ip -j link":
		out := append([]map[string]any{{"ifindex": 1, "ifname": "lo"}, {"ifindex": 3, "ifname": "eth0"}}, f.hostLinks()...)
		for _, c := range f.ctrs {
			if c.pid > 0 || c.vethStays {
				out = append(out, map[string]any{"ifindex": c.peer, "ifname": c.veth, "master": f.bridge()})
			}
		}
		return ok(mustJSON(out))

	case f.ctrs != nil && strings.HasPrefix(j, "nsenter -t "):
		pid, _ := strconv.Atoi(args[2])
		c := f.ctrByPid(pid)
		if c == nil {
			return exit1("nsenter: no such process")
		}
		return ok(mustJSON([]map[string]any{
			{"ifindex": 1, "ifname": "lo"},
			{"ifindex": 6, "ifname": "eth1", "address": c.mac, "link_index": c.peer,
				"addr_info": []map[string]any{}}, // runsc: the address is not in the netns
		}))

	case f.ctrs != nil && strings.HasPrefix(j, "bridge fdb replace "):
		if c := f.ctrByVeth(args[5]); c != nil {
			c.fdb, c.fdbGone = true, [2]bool{}
		}
		return ok("")

	case f.ctrs != nil && strings.HasPrefix(j, "bridge -j fdb show dev "):
		out := []map[string]any{}
		if c := f.ctrByVeth(args[len(args)-1]); c != nil {
			out = fdbHostEntries(c.mac, f.bridge(), c.fdb, c.fdbGone)
		}
		return ok(mustJSON(out))

	case strings.HasPrefix(j, "bridge fdb del "):
		if f.ctrs != nil {
			if c := f.ctrByVeth(args[5]); c != nil {
				return fdbDel(&c.fdb, &c.fdbGone, args)
			}
			return fdbNoEntry()
		}
		return fdbDel(&f.fdb, &f.fdbGone, args)

	case strings.HasPrefix(j, "ip neigh replace "):
		// ip neigh replace <ip> lladdr <mac> dev <bridge> nud permanent
		if f.neigh == nil {
			f.neigh = map[string]string{}
		}
		if !f.tamper["neigh"] {
			f.neigh[args[3]] = args[5]
		}
		return ok("")

	case strings.HasPrefix(j, "ip neigh del "):
		if _, there := f.neigh[args[3]]; !there {
			return exit1("RTNETLINK answers: No such file or directory")
		}
		delete(f.neigh, args[3])
		return ok("")

	case strings.HasPrefix(j, "ip -j -4 neigh show "):
		out := []map[string]any{}
		if mac, there := f.neigh[args[5]]; there {
			out = append(out, map[string]any{"dst": args[5], "dev": f.bridge(), "lladdr": mac, "state": []string{"PERMANENT"}})
		}
		return ok(mustJSON(out))

	case j == "docker ps -a --filter network=agents-net --format {{.Names}}":
		return ok(strings.Join(f.members, "\n") + "\n")

	case strings.HasPrefix(j, "docker inspect -f {{.State.Paused}} "):
		name := args[len(args)-1]
		if f.ctrs[name] == nil {
			return exit1("Error: No such container: " + name)
		}
		return ok(fmt.Sprintf("%t\n", f.ctrs[name].paused))

	case strings.HasPrefix(j, "docker unpause "):
		name := args[len(args)-1]
		if f.ctrs[name] == nil {
			return exit1("Error: No such container: " + name)
		}
		f.ctrs[name].paused = false
		return ok(name + "\n")

	case strings.HasPrefix(j, "docker rm -f "):
		name := args[len(args)-1]
		if f.ctrs[name] == nil {
			return exit1("Error: No such container: " + name)
		}
		if f.ctrs[name].paused { // like runsc on the real host: a paused sandbox cannot be killed
			return exit1("Error response from daemon: could not kill container: container " + name + " PID 1 is zombie and can not be killed")
		}
		delete(f.ctrs, name)
		for i, m := range f.members {
			if m == name {
				f.members = append(f.members[:i], f.members[i+1:]...)
				break
			}
		}
		return ok(name + "\n")

	case j == "docker network rm agents-net":
		if !f.agentsNetExists() {
			return exit1("Error: No such network: agents-net")
		}
		if len(f.members) > 0 {
			return exit1("Error: network agents-net has active endpoints")
		}
		f.netExists, f.created = false, false
		return ok("agents-net\n")

	case strings.HasPrefix(j, "iptables -D "):
		chain := args[2]
		n, err := strconv.Atoi(args[3])
		if err != nil && len(args) > 4 { // delete by exact spec
			spec := strings.Join(args[3:], " ")
			for i, l := range f.ipt[chain] {
				if l == spec {
					f.ipt[chain] = append(f.ipt[chain][:i], f.ipt[chain][i+1:]...)
					return ok("")
				}
			}
			return exit1("iptables: Bad rule (does a matching rule exist in that chain?).")
		}
		if err != nil || n < 1 || n > len(f.ipt[chain]) {
			return exit1("iptables: Index of deletion too big.")
		}
		f.ipt[chain] = append(f.ipt[chain][:n-1], f.ipt[chain][n:]...)
		return ok("")

	case strings.HasPrefix(j, "docker version"):
		return ok("27.0.1\n")

	case j == "docker network ls -q":
		var ids []string
		for _, o := range f.others {
			ids = append(ids, o.ID)
		}
		if f.agentsNetExists() {
			ids = append(ids, netID)
		}
		return ok(strings.Join(ids, "\n") + "\n")

	case strings.HasPrefix(j, "docker network inspect --format {{json .}} "):
		name := args[len(args)-1]
		for _, o := range f.others {
			if name == o.ID || name == o.Name {
				return ok(netJSON(o.ID, o.Name, "bridge", o.Subnet, false, nil))
			}
		}
		if (name == "agents-net" || name == netID) && f.agentsNetExists() {
			driver, subnet := "bridge", config.Subnet
			if f.driver != "" {
				driver = f.driver
			}
			if f.subnet != "" {
				subnet = f.subnet
			}
			opts := map[string]string{}
			if f.bridgeOpt != "" {
				opts["com.docker.network.bridge.name"] = f.bridgeOpt
			}
			var v map[string]any
			_ = json.Unmarshal([]byte(netJSON(netID, "agents-net", driver, subnet, f.internal, opts)), &v)
			ctrs := map[string]any{} // attached containers: the members that exist
			for _, m := range f.members {
				if c := f.ctrs[m]; c != nil {
					ctrs[m+"-id"] = map[string]any{"Name": m, "MacAddress": c.mac}
				}
			}
			v["Containers"] = ctrs
			return ok(mustJSON(v))
		}
		return exit1("Error: No such network: " + name)

	case strings.HasPrefix(j, "docker network create"):
		f.created = true
		if f.onCreate != nil {
			f.onCreate()
		}
		return ok(netID + "\n")

	case strings.HasPrefix(j, "docker network connect "), strings.HasPrefix(j, "docker network disconnect "):
		// Containers are attached at `docker create`; docker would reject a connect to a --network none container
		// and gVisor never sees the interface. No flow may issue either command (the tests assert that too).
		return exit1("Error: forbidden: attach is done at docker create")

	case strings.HasPrefix(j, "docker container inspect --format {{json .}} "):
		nets := map[string]any{"agents-net": map[string]any{"IPAddress": agentIP, "MacAddress": agentMAC}}
		if f.noEndpoint { // attached, but docker reports no address
			nets["agents-net"] = map[string]any{"IPAddress": "", "MacAddress": ""}
		}
		if f.extraNet {
			nets["bridge"] = map[string]any{"IPAddress": "172.17.0.2", "MacAddress": "02:42:ac:11:00:02"}
		}
		return ok(mustJSON(map[string]any{
			"State":           map[string]any{"Pid": f.pid, "Running": !f.stopped, "Paused": !f.notPaused},
			"HostConfig":      f.hostConfig(),
			"NetworkSettings": map[string]any{"Networks": nets},
		}))

	case j == "ip -j addr":
		var out []map[string]any
		for i, a := range f.addrs {
			ip, plen, _ := strings.Cut(a[1], "/")
			var n int
			fmt.Sscanf(plen, "%d", &n)
			out = append(out, map[string]any{
				"ifindex": i + 1, "ifname": a[0],
				"addr_info": []map[string]any{{"family": "inet", "local": ip, "prefixlen": n}},
			})
		}
		return ok(mustJSON(out))

	case j == "ip -j route show table all":
		out := []map[string]any{{"dst": "default", "dev": "eth0"}, {"dst": "0.0.0.0/0", "dev": "eth0"}}
		for _, r := range f.routes {
			out = append(out, map[string]any{"dst": r[0], "dev": r[1]})
		}
		return ok(mustJSON(out))

	case j == "ip -j link":
		master := f.bridge()
		if f.wrongMaster {
			master = "docker0"
		}
		return ok(mustJSON(append([]map[string]any{
			{"ifindex": 1, "ifname": "lo"},
			{"ifindex": 3, "ifname": "eth0"},
			{"ifindex": 5, "ifname": "vethother", "master": "docker0"},
			{"ifindex": peerIndex, "ifname": vethName, "master": master},
		}, f.hostLinks()...)))

	case strings.HasPrefix(j, fmt.Sprintf("nsenter -t %d -n ip -j addr", f.pid)):
		// A decoy on the default bridge network comes first; the agents-net interface is not eth0.
		out := []map[string]any{
			{"ifindex": 1, "ifname": "lo"},
			{"ifindex": 4, "ifname": "eth0", "address": "02:42:ac:11:00:02", "link_index": 5,
				"addr_info": []map[string]any{{"family": "inet", "local": "172.17.0.2", "prefixlen": 16}}},
		}
		if !f.noPeer {
			out = append(out, map[string]any{"ifindex": 6, "ifname": "eth1", "address": agentMAC, "link_index": peerIndex,
				"addr_info": f.nsAddrInfo()})
		}
		if f.dupIface {
			out = append(out, map[string]any{"ifindex": 8, "ifname": "eth2", "address": agentMAC, "link_index": peerIndex + 2,
				"addr_info": f.nsAddrInfo()})
		}
		return ok(mustJSON(out))

	case j == "nft list tables":
		out := "table inet filter\ntable ip nat\n"
		for _, n := range f.tableNames() {
			out += "table netdev " + n + "\n"
		}
		if f.gate != nil {
			out += "table netdev " + config.GateTable + "\n"
		}
		return ok(out)

	case j == "nft list table netdev "+config.GateTable:
		if f.gate == nil {
			return exit1("Error: No such file or directory")
		}
		return ok(f.gate.listing())

	case strings.HasPrefix(j, "nft delete table netdev "+config.GateTable):
		if f.gate == nil {
			return exit1("Error: No such file or directory")
		}
		f.gate = nil
		f.gateDeleted++
		return ok("")

	case j == "nft -j list set netdev "+config.GateTable+" "+config.GateSet:
		if f.gate == nil || !f.gate.set {
			return exit1("Error: No such file or directory")
		}
		return ok(f.gate.setJSON())

	case strings.HasPrefix(j, "nft delete element netdev "+config.GateTable+" "+config.GateSet+" { "):
		// nft delete element netdev agents_gate released { <mac> }
		mac := args[7]
		if f.gate == nil || !f.gate.has(mac) {
			return exit1("Error: No such file or directory")
		}
		if f.tamper["gatedelete"] { // the command succeeds but the element stays
			return ok("")
		}
		f.gate.delElem(mac)
		return ok("")

	case strings.HasPrefix(j, "nft list table netdev "):
		t := f.nft[args[4]]
		if t == nil {
			return exit1("Error: No such file or directory")
		}
		if f.hideRules { // the accept rules are in the kernel but the read-back does not see them
			t = t.clone()
			t.rules = map[string][]string{}
		}
		return ok(t.listing(args[4]))

	case strings.HasPrefix(j, "nft delete table netdev "):
		if f.nft[args[4]] == nil {
			return exit1("Error: No such file or directory")
		}
		delete(f.nft, args[4])
		f.deleted = append(f.deleted, args[4])
		return ok("")

	case j == nftScript:
		return f.nftApply(string(stdin))

	case strings.HasPrefix(j, "bridge -d -j link show dev "):
		port := map[string]any{"ifname": args[len(args)-1], "master": f.bridge(), "state": "forwarding", "priority": 32, "cost": 2,
			"hairpin": false, "guard": false, "root_block": false, "fastleave": false, "flood": true, "mcast_flood": true,
			"neigh_suppress": false, "vlan_tunnel": false}
		if !f.noPortFlags {
			port["isolated"], port["learning"], port["locked"] = f.isolated, f.learning, f.locked
		}
		return ok(mustJSON([]map[string]any{port}))

	case strings.HasPrefix(j, "bridge link set dev "):
		if strings.HasSuffix(j, "isolated on") && !f.tamper["isolated"] {
			f.isolated = true
		}
		if strings.HasSuffix(j, "learning off") && !f.tamper["learning"] {
			f.learning = false
		}
		if strings.HasSuffix(j, "locked on") && !f.tamper["locked"] {
			f.locked = true
		}
		return ok("")

	case strings.HasPrefix(j, "bridge fdb replace "):
		if !f.tamper["fdb"] {
			f.fdb, f.fdbGone = true, [2]bool{}
		}
		return ok("")

	case strings.HasPrefix(j, "bridge -j fdb show dev "):
		return ok(mustJSON(fdbHostEntries(agentMAC, f.bridge(), f.fdb, f.fdbGone)))

	case strings.HasPrefix(j, "iptables -S "):
		chain := args[2]
		lines := []string{"-N " + chain}
		for _, l := range f.ipt[chain] {
			lines = append(lines, "-A "+chain+" "+l)
		}
		return ok(strings.Join(lines, "\n") + "\n")

	case strings.HasPrefix(j, "iptables -I "):
		chain := args[2]
		key := map[string]string{"DOCKER-USER": "egress", "INPUT": "input"}[chain]
		if !f.tamper[key] {
			f.ipt[chain] = append([]string{strings.Join(args[4:], " ")}, f.ipt[chain]...)
		}
		return ok("")
	}
	return ok("")
}

func (f *fakeRunner) tableNames() []string {
	var names []string
	for n := range f.nft {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// listing prints the table like `nft list table netdev <name>`.
func (t *fakeTable) listing(name string) string {
	out := fmt.Sprintf("table netdev %s {\n", name)
	for _, c := range []string{config.ChainIngress, config.ChainEgress} {
		out += fmt.Sprintf("\tchain %s {\n\t\ttype filter hook %s device \"%s\" priority filter; policy %s;\n", c, c, t.veth, t.policy[c])
		for _, r := range t.rules[c] {
			out += "\t\t" + listedRule(r) + "\n"
		}
		out += "\t}\n"
	}
	return out + "}\n"
}

// nftApply runs an nft script atomically: all lines are applied to a copy that replaces the state only if every
// line is valid, as `nft -f` does.
func (f *fakeRunner) nftApply(script string) ([]byte, []byte, int, error) {
	work := map[string]*fakeTable{}
	for n, t := range f.nft {
		work[n] = t.clone()
	}
	wg := f.gate.clone()
	for _, line := range strings.Split(script, "\n") {
		w := strings.Fields(line)
		if len(w) == 0 {
			continue
		}
		bad := func(why string) ([]byte, []byte, int, error) { return exit1("nft: " + why + ": " + line) }
		if len(w) >= 4 && w[2] == "netdev" && w[3] == config.GateTable { // the agents-net gate
			switch {
			case len(w) == 4 && w[0] == "add" && w[1] == "table":
				if wg == nil {
					wg = &fakeGate{policy: map[string]string{}, rules: map[string][]string{}}
				}
			case len(w) == 4 && w[0] == "delete" && w[1] == "table":
				if wg == nil {
					return bad("no such table")
				}
				wg = nil
			case wg == nil:
				return bad("no such table")
			case len(w) == 9 && w[0] == "add" && w[1] == "set" && w[4] == config.GateSet: // add set netdev agents_gate released { type ether_addr; }
				wg.set, wg.setType = true, strings.TrimSuffix(w[7], ";")
			case len(w) == 17 && w[0] == "add" && w[1] == "chain":
				if w[4] != w[9] {
					return bad("unexpected hook")
				}
				wg.device = strings.Trim(w[11], `"`)
				wg.policy[w[4]] = strings.TrimSuffix(w[15], ";")
				if f.tamper["gatepolicy"] {
					wg.policy[w[4]] = "accept"
				}
			case len(w) > 5 && w[0] == "add" && w[1] == "rule":
				wg.rules[w[4]] = append(wg.rules[w[4]], strings.Join(w[5:], " "))
			case len(w) == 10 && w[0] == "add" && w[1] == "element" && w[4] == config.GateSet && w[7] == "comment": // add element netdev agents_gate released { <mac> comment "<name>" }
				if !f.tamper["gaterelease"] {
					wg.addElem(w[6], strings.Trim(w[8], `"`))
				}
			default:
				return bad("unsupported")
			}
			continue
		}
		switch {
		case len(w) == 4 && w[0] == "add" && w[1] == "table" && w[2] == "netdev":
			if work[w[3]] == nil {
				work[w[3]] = &fakeTable{policy: map[string]string{}, rules: map[string][]string{}}
			}
		case len(w) == 4 && w[0] == "delete" && w[1] == "table" && w[2] == "netdev":
			if work[w[3]] == nil {
				return bad("no such table")
			}
			delete(work, w[3])
		case len(w) == 17 && w[0] == "add" && w[1] == "chain" && w[2] == "netdev":
			t := work[w[3]]
			if t == nil {
				return bad("no such table")
			}
			if w[4] != w[9] { // the chain name is the hook name
				return bad("unexpected hook")
			}
			t.veth = strings.Trim(w[11], `"`)
			t.policy[w[4]] = strings.TrimSuffix(w[15], ";")
			if f.tamper["policy"] {
				t.policy[w[4]] = "accept"
			}
		case len(w) == 5 && w[0] == "flush" && w[1] == "chain" && w[2] == "netdev":
			t := work[w[3]]
			if t == nil {
				return bad("no such table")
			}
			t.rules[w[4]] = nil
		case len(w) > 5 && w[0] == "add" && w[1] == "rule" && w[2] == "netdev":
			t := work[w[3]]
			if t == nil {
				return bad("no such table")
			}
			if !f.tamper["release"] {
				t.rules[w[4]] = append(t.rules[w[4]], strings.Join(w[5:], " "))
			}
		default:
			return bad("unsupported")
		}
	}
	f.nft = work
	f.gate = wg
	return ok("")
}

// portRules returns the rules (all chains) of a table, nil if it does not exist.
func (f *fakeRunner) portRules(table string) []string {
	t := f.nft[table]
	if t == nil {
		return nil
	}
	return append(append([]string(nil), t.rules[config.ChainIngress]...), t.rules[config.ChainEgress]...)
}

// armedPorts returns the tables that carry accept rules (released ports).
func (f *fakeRunner) armedPorts() []string {
	var out []string
	for _, n := range f.tableNames() {
		if f.nft[n].ruleCount() > 0 {
			out = append(out, n)
		}
	}
	return out
}

func (f *fakeRunner) has(prefix string) bool { return f.count(prefix) > 0 }

func (f *fakeRunner) count(prefix string) int {
	n := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c.joined(), prefix) {
			n++
		}
	}
	return n
}

func (f *fakeRunner) index(prefix string) int {
	for i, c := range f.calls {
		if strings.HasPrefix(c.joined(), prefix) {
			return i
		}
	}
	return -1
}

// isRelease reports the atomic nft call that adds accept rules (it flushes the two chains of the port table and adds
// the MAC to the gate set); isLoad the one that loads a closed port table; isGateLoad the one that loads the gate.
func isRelease(c call) bool {
	return c.joined() == nftScript && strings.Contains(string(c.Stdin), "flush chain ") && strings.Contains(string(c.Stdin), "add rule ")
}

func isGateLoad(c call) bool {
	return c.joined() == nftScript && strings.Contains(string(c.Stdin), "add set netdev "+config.GateTable)
}

func isLoad(c call) bool {
	return c.joined() == nftScript && strings.Contains(string(c.Stdin), "add chain ") && !isGateLoad(c)
}

func (f *fakeRunner) releaseIndex() int {
	for i, c := range f.calls {
		if isRelease(c) {
			return i
		}
	}
	return -1
}

func (f *fakeRunner) released() bool { return f.releaseIndex() >= 0 }

// ---- fake host ----

type fakeHost struct {
	euid      int
	files     map[string]string
	links     map[string]string // symbolic links: the net namespaces of this process and of pid 1
	socketErr error
	lookup    map[string][]string
	lookupErr error
}

func statLine(pid int, start string) string {
	// Field 3 is the state; fields 4..21 are zeros; field 22 is the start time.
	return fmt.Sprintf("%d (dockerd) S %s%s 1000", pid, strings.Repeat("0 ", 18), start)
}

func newHost() *fakeHost {
	return &fakeHost{
		files: map[string]string{
			"/proc/self/status":       "Name:\ttest\nCapEff:\t000001ffffffffff\n",
			"/etc/docker/daemon.json": `{"runtimes":{"runsc":{"path":"/usr/local/bin/runsc"}}}`,
			"/var/run/docker.pid":     "4242\n",
			"/proc/4242/stat":         statLine(4242, "987654"),
			"/etc/resolv.conf":        "# generated\nnameserver 10.255.255.254\n",
			brNfFile:                  "1\n",
		},
		links:  map[string]string{"/proc/self/ns/net": "net:[4026531840]", "/proc/1/ns/net": "net:[4026531840]"},
		lookup: map[string][]string{"api.anthropic.com": {"160.79.104.10"}, "platform.claude.com": {"160.79.104.20"}},
	}
}

func (h *fakeHost) Geteuid() int { return h.euid }

func (h *fakeHost) ReadFile(path string) ([]byte, error) {
	if v, ok := h.files[path]; ok {
		return []byte(v), nil
	}
	return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
}

func (h *fakeHost) SocketAccessible(string) error { return h.socketErr }

func (h *fakeHost) Readlink(path string) (string, error) {
	if v, ok := h.links[path]; ok {
		return v, nil
	}
	return "", &fs.PathError{Op: "readlink", Path: path, Err: fs.ErrNotExist}
}

func (h *fakeHost) LookupIPv4(_ context.Context, host string) ([]string, error) {
	if h.lookupErr != nil {
		return nil, h.lookupErr
	}
	return h.lookup[host], nil
}

// ---- harness ----

type result struct {
	code        int
	stdout, err string
}

func run(t *testing.T, fr *fakeRunner, h *fakeHost, env map[string]string, args ...string) result {
	t.Helper()
	cfg := config.Load(func(k string) string { return env[k] })
	var out, errb bytes.Buffer
	code := sequence.Run(context.Background(), sequence.Deps{
		Config: cfg, Runner: fr, Host: h, Args: args, Stdout: &out, Stderr: &errb,
	})
	return result{code, out.String(), errb.String()}
}

func noSuccess(t *testing.T, r result) {
	t.Helper()
	if strings.Contains(r.stdout, "created and verified") || strings.Contains(r.stdout, "armed on") {
		t.Errorf("stdout must carry no success text on failure: %q", r.stdout)
	}
}

func reflectEq(a, b []string) bool { return strings.Join(a, ",") == strings.Join(b, ",") }

// ---- create: usage and preconditions ----

func TestUsageErrorsExit1WithNothingCalled(t *testing.T) {
	for _, args := range [][]string{
		{"x"}, {"arm"}, {"arm", "a", "b"}, {"create"}, {"arm", ""}, {"arm", "-bad"},
		{"arm", "bad name"}, {"arm", "a/b"}, {"ARM", "a"}, {"", ""}, {"arm", "agent-"},
		{"check-gate"}, {"check-gate", "a", "b"}, {"check-gate", "bad name"},
	} {
		fr := newRunner()
		r := run(t, fr, newHost(), nil, args...)
		if r.code != 1 || !strings.Contains(r.err, "usage: create_agent_net") {
			t.Errorf("args %q: exit %d, stderr %q", args, r.code, r.err)
		}
		if len(fr.calls) != 0 {
			t.Errorf("args %q: nothing may be called, got %v", args, fr.calls)
		}
		noSuccess(t, r)
	}
}

func TestUsageReportsTheNotDoneStateOfTheMode(t *testing.T) {
	for args, state := range map[string]string{
		"arm": "GATED_UNARMED", "disarm": "NOT_DISARMED", "check-gate": "NOT_CHECKED", "bogus": "NOT_CREATED",
	} {
		r := run(t, newRunner(), newHost(), nil, args)
		if r.code != 1 || !strings.Contains(r.err, "exit_state: "+state) {
			t.Errorf("%s: exit %d, stderr %q", args, r.code, r.err)
		}
	}
}

func TestPrivilegeFailuresExit1NothingCreated(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*fakeHost)
		want  string
	}{
		{"not root", func(h *fakeHost) { h.euid = 1000 }, "not running as root"},
		{"no CAP_NET_ADMIN", func(h *fakeHost) { h.files["/proc/self/status"] = "CapEff:\t0000000000000fff\n" }, "CAP_NET_ADMIN"},
		{"unreadable caps", func(h *fakeHost) { delete(h.files, "/proc/self/status") }, "CAP_NET_ADMIN"},
		{"socket", func(h *fakeHost) { h.socketErr = errors.New("permission denied") }, "docker socket /var/run/docker.sock"},
		{"other network namespace", func(h *fakeHost) { h.links["/proc/self/ns/net"] = "net:[4026532999]" }, "not in the agent host's own network scope"},
		{"network namespace unreadable", func(h *fakeHost) { delete(h.links, "/proc/1/ns/net") }, "own network scope could not be checked"},
	}
	for _, c := range cases {
		for _, args := range [][]string{nil, {"arm", container}, {"check-gate", container}, {"disarm", container}} {
			t.Run(c.name, func(t *testing.T) {
				fr, h := newRunner(), newHost()
				c.setup(h)
				r := run(t, fr, h, nil, args...)
				if r.code != 1 || !strings.Contains(r.err, c.want) {
					t.Fatalf("exit %d, stderr %q", r.code, r.err)
				}
				if len(fr.calls) != 0 {
					t.Errorf("nothing may be called: %v", fr.calls)
				}
				noSuccess(t, r)
			})
		}
	}
}

func TestSocketPathIsConfigurable(t *testing.T) {
	h := newHost()
	h.socketErr = errors.New("no such file")
	r := run(t, newRunner(), h, map[string]string{"AGENT_NET_DOCKER_SOCKET": "/run/other.sock"})
	if r.code != 1 || !strings.Contains(r.err, "/run/other.sock") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
}

func TestEngineUnreachableExit1(t *testing.T) {
	for _, args := range [][]string{nil, {"arm", container}} {
		fr := newRunner()
		fr.rules = []rule{{contains: "docker version", stderr: "Cannot connect to the Docker daemon", code: 1}}
		r := run(t, fr, newHost(), nil, args...)
		if r.code != 1 || !strings.Contains(r.err, "docker engine unreachable") || !strings.Contains(r.err, "Cannot connect") {
			t.Fatalf("exit %d, stderr %q", r.code, r.err)
		}
		if len(fr.calls) != 1 || fr.has("docker network") || fr.has("nft") {
			t.Errorf("only the engine check may run: %v", fr.calls)
		}
		noSuccess(t, r)
	}
}

func TestNetworkExistsExit1Untouched(t *testing.T) {
	fr := newRunner()
	fr.netExists = true
	r := run(t, fr, newHost(), nil)
	if r.code != 1 || !strings.Contains(r.err, "agents-net already exists") || !strings.Contains(r.err, "ALREADY_EXISTS") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	for _, p := range []string{"docker network create", "docker network rm", "docker network connect", "docker network ls", "ip ", "nft"} {
		if fr.has(p) {
			t.Errorf("%s must not run", p)
		}
	}
	noSuccess(t, r)
}

func TestRangeClashExit1(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*fakeRunner)
		want  string
	}{
		{"docker network", func(f *fakeRunner) {
			f.others = []netSpec{{"id1", "bridge", "172.17.0.0/16"}, {"id2", "other-net", "172.30.0.0/16"}}
		}, "docker network other-net subnet 172.30.0.0/16"},
		{"docker network inside", func(f *fakeRunner) {
			f.others = []netSpec{{"id2", "small", "172.30.0.128/25"}}
		}, "docker network small subnet 172.30.0.128/25"},
		{"host address", func(f *fakeRunner) {
			f.addrs = [][2]string{{"lo", "127.0.0.1/8"}, {"eth1", "172.30.0.5/24"}}
		}, "host address 172.30.0.5/24 on eth1"},
		{"host route", func(f *fakeRunner) {
			f.routes = [][2]string{{"172.30.0.0/20", "eth0"}}
		}, "host route 172.30.0.0/20 dev eth0"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fr := newRunner()
			c.setup(fr)
			r := run(t, fr, newHost(), nil)
			if r.code != 1 || !strings.Contains(r.err, c.want) || !strings.Contains(r.err, "NOT_CREATED") {
				t.Fatalf("exit %d, stderr %q", r.code, r.err)
			}
			if fr.has("docker network create") {
				t.Error("create must not be issued")
			}
			noSuccess(t, r)
		})
	}
}

func TestNoClashIgnoresDefaultRoutesAndDefaultPools(t *testing.T) {
	fr := newRunner()
	fr.others = []netSpec{{"id1", "bridge", "172.17.0.0/16"}, {"id2", "near", "172.31.0.0/24"}}
	fr.addrs = [][2]string{{"eth0", "172.29.255.1/24"}}
	fr.routes = [][2]string{{"10.0.0.0/8", "eth0"}}
	r := run(t, fr, newHost(), nil)
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
}

func TestEngineStateUnreadableExit1(t *testing.T) {
	fr, h := newRunner(), newHost()
	delete(h.files, "/var/run/docker.pid")
	r := run(t, fr, h, nil)
	if r.code != 1 || fr.has("docker network create") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
}

// ---- create: network creation ----

func TestCreateHappyPath(t *testing.T) {
	fr, h := newRunner(), newHost()
	r := run(t, fr, h, nil)
	if r.code != 0 || r.err != "" {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if !strings.Contains(r.stdout, "agent network agents-net created and verified") {
		t.Errorf("missing success report: %q", r.stdout)
	}
	for _, want := range []string{"arm <container>", "disarm <container>", "check-gate <container>"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("the success report must say that %q is available: %q", want, r.stdout)
		}
	}
	if !strings.Contains(r.stdout, "[1/9] validate invocation") {
		t.Errorf("missing progress line: %q", r.stdout)
	}
	if fr.count(createCmd) != 1 {
		t.Errorf("create command not issued exactly once: %v", fr.calls)
	}
	if fr.has("docker network create") && strings.Contains(fr.calls[fr.index("docker network create")].joined(), "--internal") {
		t.Error("the network must not be created with --internal")
	}
	first, create, last := fr.index(inspectCmd), fr.index(createCmd), -1
	for i, c := range fr.calls {
		if c.joined() == inspectCmd {
			last = i
		}
	}
	if !(first >= 0 && first < create && create < last) {
		t.Errorf("order inspect=%d create=%d re-inspect=%d", first, create, last)
	}
	for _, p := range []string{"docker network rm", "docker network connect", "docker network disconnect", "bridge", "systemctl", "docker restart", "sysctl", "nft delete", "iptables -D"} {
		if fr.has(p) {
			t.Errorf("%s must not run in the create flow", p)
		}
	}
	for _, c := range fr.calls {
		if strings.Contains(c.joined(), "daemon.json") || strings.Contains(c.joined(), "bridge-nf-call") {
			t.Errorf("daemon.json and the sysctl must not appear in any command: %s", c.joined())
		}
	}
}

// create installs the network-wide gate right after the network exists (before it is verified): one atomic nft
// script and the tagged DOCKER-USER rule; both are read back by the verification.
func TestCreateInstallsAndVerifiesTheGate(t *testing.T) {
	fr, h := newRunner(), newHost()
	r := run(t, fr, h, nil)
	if r.code != 0 || r.err != "" {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.count(nftScript) != 1 {
		t.Fatalf("nft -f calls = %d, want exactly the one gate load", fr.count(nftScript))
	}
	load := fr.calls[fr.index(nftScript)]
	wantScript := "add table netdev agents_gate\n" +
		"delete table netdev agents_gate\n" +
		"add table netdev agents_gate\n" +
		"add set netdev agents_gate released { type ether_addr; }\n" +
		"add chain netdev agents_gate ingress { type filter hook ingress device \"" + bridgeName + "\" priority 0; policy drop; }\n" +
		"add rule netdev agents_gate ingress " + gateIngressRule + "\n" +
		"add chain netdev agents_gate egress { type filter hook egress device \"" + bridgeName + "\" priority 0; policy drop; }\n" +
		"add rule netdev agents_gate egress ether daddr @released accept\n"
	if string(load.Stdin) != wantScript {
		t.Errorf("gate script =\n%s\nwant\n%s", load.Stdin, wantScript)
	}
	if g := fr.gate; g == nil || !g.set || g.device != bridgeName || len(g.released) != 0 || g.policy["ingress"] != "drop" || g.policy["egress"] != "drop" {
		t.Errorf("gate = %+v", g)
	}
	ins := "iptables -I DOCKER-USER 1 " + gateRule
	if fr.count("iptables -I") != 1 || !fr.has(ins) {
		t.Errorf("want exactly the one gate rule %q: %v", ins, fr.ipt)
	}
	if !reflectEq(fr.ipt["DOCKER-USER"], []string{gateRule}) {
		t.Errorf("DOCKER-USER = %v", fr.ipt["DOCKER-USER"])
	}
	// Order: network created, gate loaded and rule inserted, then the verification reads them back.
	create, gl, ir := fr.index(createCmd), fr.index(nftScript), fr.index("iptables -I")
	readGate, readIpt := fr.lastIndex("nft list table netdev "+config.GateTable), fr.lastIndex("iptables -S DOCKER-USER")
	if !(create >= 0 && create < gl && gl < ir && ir < readGate && ir < readIpt) {
		t.Errorf("order create=%d gate load=%d gate rule=%d read gate=%d read DOCKER-USER=%d", create, gl, ir, readGate, readIpt)
	}
	if !strings.Contains(r.stdout, "[7/9] install network gate") {
		t.Errorf("missing gate step line: %q", r.stdout)
	}
	// A port table or an armed port does not exist: only the gate was loaded.
	if len(fr.nft) != 0 {
		t.Errorf("port tables = %v", fr.tableNames())
	}
}

// bridge-nf-call-iptables is only read: a value other than 1 (or an unreadable file) fails the create with a failed
// control; the network stays and nothing of the gate is loaded.
func TestCreateFailsWhenBridgeNetfilterIsNotOne(t *testing.T) {
	for name, set := range map[string]func(*fakeHost){
		"zero":       func(h *fakeHost) { h.files[brNfFile] = "0\n" },
		"unreadable": func(h *fakeHost) { delete(h.files, brNfFile) },
	} {
		t.Run(name, func(t *testing.T) {
			fr, h := newRunner(), newHost()
			set(h)
			r := run(t, fr, h, nil)
			if r.code != 2 || !strings.HasPrefix(r.err, "failed control: network gate: ") || !strings.Contains(r.err, "bridge-nf-call-iptables") || !strings.Contains(r.err, "CREATED_UNVERIFIED") {
				t.Fatalf("exit %d, stderr %q", r.code, r.err)
			}
			if fr.has(nftScript) || fr.has("iptables -I") || fr.has("docker network rm") || fr.has("sysctl") {
				t.Errorf("no gate may be loaded, the network stays, no sysctl: %v", fr.calls)
			}
			if fr.gate != nil || !fr.agentsNetExists() {
				t.Errorf("gate=%+v network=%v", fr.gate, fr.agentsNetExists())
			}
			noSuccess(t, r)
		})
	}
}

func TestCreateGateFailuresLeaveTheNetwork(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*fakeRunner)
		want  string
	}{
		{"nft load fails", func(f *fakeRunner) {
			f.rules = []rule{{contains: nftScript, stdin: "add set", stderr: "Error: Operation not supported", code: 1}}
		}, "Operation not supported"},
		{"iptables insert fails", func(f *fakeRunner) {
			f.rules = []rule{{contains: "iptables -I DOCKER-USER", stderr: "iptables: Couldn't load match `physdev'", code: 1}}
		}, "physdev"},
		// the commands succeed but the read-back differs: the verification names the gate
		{"gate policy not drop", func(f *fakeRunner) { f.tamper["gatepolicy"] = true }, "gate: chain ingress of agents_gate has policy"},
		{"gate rule not applied", func(f *fakeRunner) { f.tamper["egress"] = true }, "gate: rule missing in DOCKER-USER"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fr := newRunner()
			c.setup(fr)
			r := run(t, fr, newHost(), nil)
			if r.code != 2 || !strings.Contains(r.err, c.want) || !strings.Contains(r.err, "CREATED_UNVERIFIED") {
				t.Fatalf("exit %d, stderr %q", r.code, r.err)
			}
			if fr.has("docker network rm") || !fr.agentsNetExists() {
				t.Error("the network must be left in place")
			}
			noSuccess(t, r)
		})
	}
}

func TestCreateWithoutDaemonJSONIsAState(t *testing.T) {
	h := newHost()
	delete(h.files, "/etc/docker/daemon.json")
	if r := run(t, newRunner(), h, nil); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
}

func TestCreateFailureExit2NoRollback(t *testing.T) {
	fr := newRunner()
	fr.rules = []rule{{contains: "docker network create", stderr: "Pool overlaps with other one on this address space", code: 1}}
	r := run(t, fr, newHost(), nil)
	if r.code != 2 || !strings.Contains(r.err, "Pool overlaps") || !strings.Contains(r.err, "CREATE_FAILED") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.has("docker network rm") || fr.has("docker rm") {
		t.Error("no rollback allowed")
	}
	noSuccess(t, r)
}

func TestVerifyMismatchExit2LeavesNetwork(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*fakeRunner, *fakeHost)
		want  string
	}{
		{"driver", func(f *fakeRunner, _ *fakeHost) { f.driver = "macvlan" }, `driver is "macvlan"`},
		{"internal", func(f *fakeRunner, _ *fakeHost) { f.internal = true }, "network is internal"},
		{"subnet", func(f *fakeRunner, _ *fakeHost) { f.subnet = "172.30.0.0/16" }, "subnet is [172.30.0.0/16]"},
		{"missing", func(f *fakeRunner, _ *fakeHost) { f.vanish = true }, "network agents-net is missing"},
		{"daemon.json changed", func(f *fakeRunner, h *fakeHost) {
			f.onCreate = func() { h.files["/etc/docker/daemon.json"] = `{"changed":true}` }
		}, "daemon.json changed"},
		{"daemon.json created", func(f *fakeRunner, h *fakeHost) {
			delete(h.files, "/etc/docker/daemon.json")
			f.onCreate = func() { h.files["/etc/docker/daemon.json"] = `{}` }
		}, "daemon.json changed"},
		{"engine restarted", func(f *fakeRunner, h *fakeHost) {
			f.onCreate = func() { h.files["/proc/4242/stat"] = statLine(4242, "999999") }
		}, "docker engine was restarted"},
		{"engine new pid", func(f *fakeRunner, h *fakeHost) {
			f.onCreate = func() {
				h.files["/var/run/docker.pid"] = "5000\n"
				h.files["/proc/5000/stat"] = statLine(5000, "1111111")
			}
		}, "docker engine was restarted"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fr, h := newRunner(), newHost()
			c.setup(fr, h)
			r := run(t, fr, h, nil)
			if r.code != 2 || !strings.Contains(r.err, c.want) || !strings.Contains(r.err, "CREATED_UNVERIFIED") {
				t.Fatalf("exit %d, stderr %q", r.code, r.err)
			}
			if fr.has("docker network rm") || fr.has("docker rm") {
				t.Error("the network must be left in place")
			}
			noSuccess(t, r)
		})
	}
}

// ---- arm ----

var armArgs = []string{"arm", container}

func armOK(t *testing.T, fr *fakeRunner, h *fakeHost, env map[string]string) result {
	t.Helper()
	fr.netExists = true
	r := run(t, fr, h, env, armArgs...)
	if r.code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", r.code, r.err)
	}
	return r
}

func TestArmSuccessReleasesPortLast(t *testing.T) {
	fr := newRunner()
	r := armOK(t, fr, newHost(), nil)
	if !strings.Contains(r.stdout, "container web-1 armed on agents-net") || r.err != "" {
		t.Errorf("stdout %q stderr %q", r.stdout, r.err)
	}
	if !reflectEq(fr.armedPorts(), []string{tableName}) {
		t.Errorf("armed ports = %v", fr.armedPorts())
	}

	// The container is already attached to agents-net (at docker create, gated closed): arm never connects or
	// disconnects it. Its closed table is loaded first and carries no accept rule.
	load, rel := -1, fr.releaseIndex()
	for i, c := range fr.calls {
		if isLoad(c) && load < 0 {
			load = i
		}
	}
	if !(load >= 0 && load < rel) {
		t.Errorf("load=%d release=%d", load, rel)
	}
	if strings.Contains(string(fr.calls[load].Stdin), "add rule") || strings.Contains(string(fr.calls[load].Stdin), "add element") {
		t.Errorf("the closing script must not contain accept rules or the gate element:\n%s", fr.calls[load].Stdin)
	}
	// Exactly one release call, atomic: all four accept rules in the one nft -f, and the MAC in the gate set.
	if n := fr.count(nftScript); n != 2 {
		t.Errorf("nft -f calls = %d, want 2 (load, release)", n)
	}
	if got := strings.Count(string(fr.calls[rel].Stdin), "add rule netdev "+tableName+" "); got != 4 {
		t.Errorf("accept rules in the release script = %d, want 4:\n%s", got, fr.calls[rel].Stdin)
	}
	if fr.gate == nil || !reflectEq(fr.gate.released, []string{agentMAC}) {
		t.Errorf("the MAC must be in the gate set: %+v", fr.gate)
	}
	// Release is the last mutation; only reads follow it.
	for i, c := range fr.calls {
		j := c.joined()
		mutating := strings.HasPrefix(j, "bridge link set") || strings.HasPrefix(j, "bridge fdb replace") ||
			strings.HasPrefix(j, "ip neigh replace") || strings.HasPrefix(j, "iptables -I") || strings.HasPrefix(j, "nft delete") || (j == nftScript && i != rel)
		if mutating && i > rel {
			t.Errorf("mutation after the release: %s", j)
		}
		if i > rel && !(j == "nft list tables" || strings.HasPrefix(j, "nft list table ")) {
			t.Errorf("only the read-back may follow the release: %s", j)
		}
	}
	// Every verification read happens before the release.
	for _, p := range []string{"bridge -d -j link show dev vethabc", "bridge -j fdb show dev vethabc", "ip -j -4 neigh show " + agentIP + " dev " + bridgeName, "iptables -S DOCKER-USER", "iptables -S INPUT"} {
		last := -1
		for i, c := range fr.calls {
			if strings.HasPrefix(c.joined(), p) {
				last = i
			}
		}
		if last < 0 || last > rel {
			t.Errorf("%q: last read at %d, release at %d", p, last, rel)
		}
	}
	// The container is neither started nor stopped nor removed nor unpaused (the caller does that), and never
	// connected to or disconnected from a network.
	for _, p := range []string{"docker start", "docker stop", "docker rm", "docker kill", "docker pause", "docker unpause", "docker network connect", "docker network disconnect"} {
		if fr.has(p) {
			t.Errorf("%s must not run", p)
		}
	}
}

// The release is ONE nft transaction: it flushes and fills the port table and adds the MAC to the gate set `released`.
func TestArmReleaseAddsTheMACToTheGateInTheSameTransaction(t *testing.T) {
	fr := newRunner()
	armOK(t, fr, newHost(), nil)
	rel := fr.calls[fr.releaseIndex()]
	script := string(rel.Stdin)
	for _, want := range []string{
		"flush chain netdev " + tableName + " ingress\n",
		"flush chain netdev " + tableName + " egress\n",
		"add rule netdev " + tableName + " ingress ",
		"add element netdev agents_gate released { " + agentMAC + " comment \"" + container + "\" }\n",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("release script lacks %q:\n%s", want, script)
		}
	}
	for i, c := range fr.calls {
		if strings.Contains(string(c.Stdin), "add element") && i != fr.releaseIndex() {
			t.Errorf("the gate element may only be added by the release transaction: %s", c.Stdin)
		}
	}
	// Before the release the gate is closed for the port: neither the closing script nor the verification reads it released.
	if fr.count(nftScript) != 2 || !fr.gate.has(agentMAC) {
		t.Errorf("nft -f calls %d, gate %+v", fr.count(nftScript), fr.gate)
	}
}

func TestArmAppliesTheControlsOnTheHostVeth(t *testing.T) {
	fr := newRunner()
	armOK(t, fr, newHost(), nil)
	if !fr.isolated || fr.learning || !fr.locked || !fr.fdb {
		t.Errorf("isolated=%v learning=%v locked=%v fdb=%v", fr.isolated, fr.learning, fr.locked, fr.fdb)
	}
	for _, want := range []string{
		"bridge link set dev vethabc isolated on",
		"bridge link set dev vethabc learning off",
		"bridge link set dev vethabc locked on",
		"bridge fdb replace " + agentMAC + " dev vethabc master static",
		"nsenter -t 4242 -n ip -j addr",
	} {
		if !fr.has(want) {
			t.Errorf("missing call %q", want)
		}
	}
	// The table is bound to the veth, both chains policy drop.
	tbl := fr.nft[tableName]
	if tbl == nil || tbl.veth != vethName || tbl.policy[config.ChainIngress] != "drop" || tbl.policy[config.ChainEgress] != "drop" {
		t.Fatalf("table = %+v", tbl)
	}
	// Source identity (layer 2) and destination limit (layer 3) accept rules.
	wantIn := []string{
		"ether saddr " + agentMAC + " ether daddr " + bridgeMAC + " ip saddr " + agentIP + " " + portMarkSet + " accept",
		"ether saddr " + agentMAC + " ether daddr ff:ff:ff:ff:ff:ff arp operation request arp saddr ether " + agentMAC + " arp saddr ip " + agentIP + " arp daddr ip " + bridgeIP + " " + portMarkSet + " accept",
	}
	wantOut := []string{
		"ether saddr " + bridgeMAC + " ether type ip accept",
		"ether saddr " + bridgeMAC + " ether daddr " + agentMAC + " arp operation reply accept",
	}
	if !reflectEq(tbl.rules[config.ChainIngress], wantIn) || !reflectEq(tbl.rules[config.ChainEgress], wantOut) {
		t.Errorf("ingress = %v\negress = %v", tbl.rules[config.ChainIngress], tbl.rules[config.ChainEgress])
	}
	// Nothing is executed inside the container, and no ebtables is involved.
	for _, c := range fr.calls {
		j := c.joined()
		if strings.HasPrefix(j, "docker exec") || strings.HasPrefix(j, "docker run") || strings.Contains(j, "ebtables") {
			t.Errorf("forbidden call: %s", j)
		}
	}
}

func TestArmPinsPermanentNeighbourEntryBeforeRelease(t *testing.T) {
	fr := newRunner()
	armOK(t, fr, newHost(), nil)
	if fr.neigh[agentIP] != agentMAC {
		t.Errorf("neighbour entries = %v, want %s -> %s", fr.neigh, agentIP, agentMAC)
	}
	apply := fr.index("ip neigh replace " + agentIP + " lladdr " + agentMAC + " dev " + bridgeName + " nud permanent")
	show := "ip -j -4 neigh show " + agentIP + " dev " + bridgeName
	pre, read := fr.index(show), -1 // pre: the stale-entry check before the apply; read: the read-back after it
	for i := apply + 1; apply >= 0 && i < len(fr.calls); i++ {
		if strings.HasPrefix(fr.calls[i].joined(), show) {
			read = i
			break
		}
	}
	if rel := fr.releaseIndex(); !(pre >= 0 && pre < apply && apply >= 0 && apply < read && read < rel) {
		t.Errorf("neigh show (pre-check)=%d, neigh replace=%d, neigh show (read-back)=%d, release=%d", pre, apply, read, rel)
	}
}

func TestArmNeighbourFailureFailsClosed(t *testing.T) {
	fr := newRunner()
	fr.netExists = true
	fr.rules = []rule{{contains: "ip neigh replace", stderr: "RTNETLINK answers: Operation not permitted", code: 2}}
	r := run(t, fr, newHost(), nil, armArgs...)
	if r.code != 2 || !strings.HasPrefix(r.err, "failed control: neighbour entry: ") || !strings.Contains(r.err, "GATED_UNARMED") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.released() || fr.has("iptables -I") || len(fr.armedPorts()) != 0 {
		t.Errorf("the port must stay closed and no later control may run: %v", fr.calls)
	}
}

func TestArmEgressAndInputRules(t *testing.T) {
	fr := newRunner()
	armOK(t, fr, newHost(), nil)
	if got := fr.count("iptables -I DOCKER-USER"); got != 7 {
		t.Errorf("DOCKER-USER inserts = %d, want 7 (dns udp+tcp for each of the two resolvers, two model apis, drop)", got)
	}
	if got := fr.count("iptables -I INPUT"); got != 2 {
		t.Errorf("INPUT inserts = %d, want 2 (replies, drop)", got)
	}
	first := fr.calls[fr.index("iptables -I DOCKER-USER")].joined()
	if !strings.Contains(first, "-j DROP") || !strings.Contains(first, "! -o "+bridgeName) {
		t.Errorf("the drop must be inserted first so the accepts end above it: %s", first)
	}
	var all []string
	for _, c := range fr.calls {
		if strings.HasPrefix(c.joined(), "iptables -I") {
			j := c.joined()
			all = append(all, j)
			if !strings.Contains(j, "--comment agent-net:web-1") || !strings.Contains(j, "-s 172.30.0.2/32") {
				t.Errorf("rule not tagged or not bound to the container ip: %s", j)
			}
		}
	}
	joined := strings.Join(all, "\n")
	for _, want := range []string{
		"-d 1.1.1.1/32 -p udp -m udp --dport 53",
		"-d 1.1.1.1/32 -p tcp -m tcp --dport 53",
		"-d 9.9.9.9/32 -p udp -m udp --dport 53",
		"-d 9.9.9.9/32 -p tcp -m tcp --dport 53",
		"-d 160.79.104.10/32 -p tcp -m tcp --dport 443",
		"-d 160.79.104.20/32 -p tcp -m tcp --dport 443",
		"iptables -I INPUT 1 -s 172.30.0.2/32 -m comment --comment agent-net:web-1 -j DROP",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in:\n%s", want, joined)
		}
	}
}

// SFR-NET-009: the endpoints are fixed (api.anthropic.com, platform.claude.com); AGENT_NET_MODEL_API is ignored.
func TestArmModelAPIOverrideIsIgnored(t *testing.T) {
	fr, h := newRunner(), newHost()
	armOK(t, fr, h, map[string]string{
		"AGENT_NET_MODEL_API": "203.0.113.5",
	})
	var joined string
	for _, c := range fr.calls {
		joined += c.joined() + "\n"
	}
	for _, want := range []string{"-d 1.1.1.1/32 -p udp", "-d 9.9.9.9/32 -p tcp", "-d 160.79.104.10/32 -p tcp -m tcp --dport 443", "-d 160.79.104.20/32 -p tcp -m tcp --dport 443"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(joined, "203.0.113.5") {
		t.Error("the environment variable must not change the endpoints")
	}
}

// The resolvers are the fixed external 1.1.1.1 and 9.9.9.9: INPUT never gets a DNS accept, even if the host owns one of them.
func TestArmInputHasNoDNSAccept(t *testing.T) {
	fr := newRunner()
	fr.addrs = [][2]string{{"eth0", "1.1.1.1/24"}}
	armOK(t, fr, newHost(), nil)
	if got := fr.count("iptables -I INPUT"); got != 2 {
		t.Errorf("INPUT inserts = %d, want 2 (replies, drop)", got)
	}
	for _, c := range fr.calls {
		if j := c.joined(); strings.HasPrefix(j, "iptables -I INPUT") && strings.Contains(j, "--dport") {
			t.Errorf("INPUT must carry no port accept: %s", j)
		}
	}
}

func TestArmReplacesAnOlderTableOfThatPortOnly(t *testing.T) {
	fr := newRunner()
	other := &fakeTable{veth: "vetholdd", policy: map[string]string{"ingress": "drop", "egress": "drop"},
		rules: map[string][]string{"ingress": {"ether saddr 02:42:ac:1e:00:09 accept"}}}
	fr.nft["as_port_old"] = other
	fr.nft[tableName] = &fakeTable{veth: "vethstale", policy: map[string]string{"ingress": "accept", "egress": "accept"},
		rules: map[string][]string{"ingress": {"accept"}}}
	armOK(t, fr, newHost(), nil)
	if fr.nft[tableName].veth != vethName || len(fr.nft[tableName].rules[config.ChainIngress]) != 2 {
		t.Errorf("the table of this port must be rebuilt: %+v", fr.nft[tableName])
	}
	if !reflectEq(fr.nft["as_port_old"].rules[config.ChainIngress], other.rules[config.ChainIngress]) || fr.nft["as_port_old"].veth != "vetholdd" {
		t.Error("the table of another port must not be touched")
	}
	for _, c := range fr.calls {
		// the stale table of this container is deleted by the clean slate; nothing else
		if j := c.joined(); (strings.HasPrefix(j, "nft delete") && j != "nft delete table netdev "+tableName) || strings.Contains(string(c.Stdin), "as_port_old") {
			t.Errorf("nothing of another port may be touched: %s %s", j, c.Stdin)
		}
	}
}

func TestArmIsIdempotentForRules(t *testing.T) {
	fr := newRunner()
	armOK(t, fr, newHost(), nil)
	ins, rules, tagged := fr.count("iptables -I"), len(fr.portRules(tableName)), fr.iptTagged(container)
	armOK(t, fr, newHost(), nil)
	// the clean slate removes the first run's rules and the run rebuilds them: the same state, no duplicates
	if fr.iptTagged(container) != tagged || len(fr.portRules(tableName)) != rules || fr.count("iptables -I") != 2*ins {
		t.Errorf("second run: tagged rules %d -> %d, nft rules %d -> %d, inserts %d -> %d", tagged, fr.iptTagged(container), rules, len(fr.portRules(tableName)), ins, fr.count("iptables -I"))
	}
	if !reflectEq(fr.gate.released, []string{agentMAC}) || fr.gate.comments[agentMAC] != container {
		t.Errorf("one tagged element: %+v", fr.gate)
	}
}

func TestArmUsesBridgeNameOption(t *testing.T) {
	fr := newRunner()
	fr.bridgeOpt = "br-agents"
	armOK(t, fr, newHost(), nil)
	if !fr.has("iptables -I DOCKER-USER 1 -s 172.30.0.2/32 ! -o br-agents") {
		t.Errorf("egress rule must name the bridge: %v", fr.ipt)
	}
}

func TestArmTableNameStripsAgentPrefix(t *testing.T) {
	fr := newRunner()
	fr.netExists = true
	fr.ctrs = map[string]*fakeCtr{"agent-a": {ip: agentIP, mac: agentMAC, veth: "vetha", peer: 7, pid: 4301}}
	fr.members = []string{"agent-a"}
	if r := run(t, fr, newHost(), nil, "arm", "agent-a"); r.code != 0 {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if !reflectEq(fr.tableNames(), []string{"as_port_a"}) {
		t.Errorf("tables = %v", fr.tableNames())
	}
}

func TestArmNetworkMissingExit1(t *testing.T) {
	fr := newRunner()
	r := run(t, fr, newHost(), nil, armArgs...)
	if r.code != 1 || !strings.Contains(r.err, "network agents-net does not exist") || !strings.Contains(r.err, "GATED_UNARMED") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.has("nft") || fr.has("docker network connect") || fr.has("bridge") {
		t.Errorf("nothing may be changed: %v", fr.calls)
	}
	noSuccess(t, r)
}

func TestArmRequiresAPausedContainerBeforeAnyControl(t *testing.T) {
	// Paused and running: arm proceeds (the paused state is read before any control is changed).
	fr := newRunner()
	armOK(t, fr, newHost(), nil)
	if inspect, change := fr.index("docker container inspect"), fr.index("bridge link set"); !(inspect >= 0 && inspect < change) {
		t.Errorf("the paused check must precede the first change: inspect=%d change=%d", inspect, change)
	}
	if first := fr.index(nftScript); fr.index("docker container inspect") > first {
		t.Errorf("the paused check must precede the first nft script (%d)", first)
	}

	for _, c := range []struct {
		name  string
		setup func(*fakeRunner)
		want  string
	}{
		{"not paused", func(f *fakeRunner) { f.notPaused = true }, "is not paused"},
		{"not running", func(f *fakeRunner) { f.stopped = true }, "is not paused"},
		{"unknown container", func(f *fakeRunner) {
			f.rules = []rule{{contains: "docker container inspect", stderr: "Error: No such container: web-1", code: 1}}
		}, "does not exist"},
	} {
		t.Run(c.name, func(t *testing.T) {
			fr := newRunner()
			c.setup(fr)
			fr.netExists = true
			r := run(t, fr, newHost(), nil, armArgs...)
			if r.code != 1 || !strings.Contains(r.err, c.want) || !strings.Contains(r.err, "GATED_UNARMED") {
				t.Fatalf("exit %d, stderr %q", r.code, r.err)
			}
			// The gate is only read (RequireNetwork); nothing is loaded, inserted or changed.
			for _, p := range []string{nftScript, "nft delete", "iptables -I", "iptables -D", "bridge", "nsenter", "ip ", "docker network connect"} {
				if fr.has(p) {
					t.Errorf("%s must not run for a container that is not paused: %v", p, fr.calls)
				}
			}
			if len(fr.nft) != 0 || fr.iptTagged(container) != 0 || len(fr.gate.released) != 0 {
				t.Errorf("no port state may be installed: nft=%v ipt=%v gate=%+v", fr.nft, fr.ipt, fr.gate)
			}
			noSuccess(t, r)
		})
	}
}

func TestArmFailuresKeepPortClosedAndNameTheControl(t *testing.T) {
	cases := []struct {
		name    string
		setup   func(*fakeRunner, *fakeHost)
		control string
		detail  string
	}{
		// apply failures
		{"load port table", func(f *fakeRunner, _ *fakeHost) {
			f.rules = []rule{{contains: nftScript, stdin: "add chain", stderr: "Error: Operation not supported", code: 1}}
		}, "port table", "Operation not supported"},
		{"apply isolation", func(f *fakeRunner, _ *fakeHost) {
			f.rules = []rule{{contains: "isolated on", stderr: "RTNETLINK answers: Operation not supported", code: 2}}
		}, "port isolation", "Operation not supported"},
		{"apply learning", func(f *fakeRunner, _ *fakeHost) {
			f.rules = []rule{{contains: "learning off", stderr: "boom", code: 2}}
		}, "mac learning off", "boom"},
		{"apply lock", func(f *fakeRunner, _ *fakeHost) {
			f.rules = []rule{{contains: "locked on", stderr: "boom", code: 2}}
		}, "port lock", "boom"},
		{"apply fdb", func(f *fakeRunner, _ *fakeHost) {
			f.rules = []rule{{contains: "bridge fdb replace", stderr: "boom", code: 2}}
		}, "static FDB entry", "boom"},
		{"apply neighbour", func(f *fakeRunner, _ *fakeHost) {
			f.rules = []rule{{contains: "ip neigh replace", stderr: "boom", code: 2}}
		}, "neighbour entry", "boom"},
		{"apply egress", func(f *fakeRunner, _ *fakeHost) {
			f.rules = []rule{{contains: "iptables -I DOCKER-USER", stderr: "iptables: no chain", code: 1}}
		}, "egress default-deny", "iptables: no chain"},
		{"apply input", func(f *fakeRunner, _ *fakeHost) {
			f.rules = []rule{{contains: "iptables -I INPUT", stderr: "iptables: boom", code: 1}}
		}, "host-input default-deny", "iptables: boom"},
		{"model api unresolvable", func(_ *fakeRunner, h *fakeHost) { h.lookupErr = errors.New("no such host") }, "endpoint egress rule", "no such host"},
		// verify failures: the command succeeded but the applied state is not there
		{"verify table policy", func(f *fakeRunner, _ *fakeHost) { f.tamper["policy"] = true }, "port table", "policy"},
		{"verify isolation", func(f *fakeRunner, _ *fakeHost) { f.tamper["isolated"] = true }, "port isolation", "not isolated"},
		{"verify learning", func(f *fakeRunner, _ *fakeHost) { f.tamper["learning"] = true }, "mac learning off", "learning is still on"},
		{"verify lock", func(f *fakeRunner, _ *fakeHost) { f.tamper["locked"] = true }, "port lock", "not locked"},
		{"port flags missing from bridge output", func(f *fakeRunner, _ *fakeHost) { f.noPortFlags = true }, "port isolation", "lacks the isolated flag"},
		{"verify fdb", func(f *fakeRunner, _ *fakeHost) { f.tamper["fdb"] = true }, "static FDB entry", "no static fdb entry"},
		{"verify neighbour", func(f *fakeRunner, _ *fakeHost) { f.tamper["neigh"] = true }, "neighbour entry", "no neighbour entry"},
		{"verify egress", func(f *fakeRunner, _ *fakeHost) { f.tamper["egress"] = true }, "DNS egress rule", "rule missing in DOCKER-USER"},
		{"verify input", func(f *fakeRunner, _ *fakeHost) { f.tamper["input"] = true }, "host-input default-deny", "rule missing in INPUT"},
		// release failures
		{"release command", func(f *fakeRunner, _ *fakeHost) {
			f.rules = []rule{{contains: nftScript, stdin: "add rule", stderr: "nft: boom", code: 1}}
		}, "port release", "nft: boom"},
		{"release not visible", func(f *fakeRunner, _ *fakeHost) { f.tamper["release"] = true }, "port release", "not released"},
		{"gate element not visible", func(f *fakeRunner, _ *fakeHost) { f.tamper["gaterelease"] = true }, "port release", "is not in set released"},
		{"gate table lost before the release", func(f *fakeRunner, _ *fakeHost) {
			f.rules = []rule{{contains: nftScript, stdin: "add element", stderr: "Error: No such file or directory", code: 1}}
		}, "port release", "No such file or directory"},
		// identification failures
		{"no pid", func(f *fakeRunner, _ *fakeHost) { f.pid = 0 }, "port identification", "no running process"},
		{"no endpoint", func(f *fakeRunner, _ *fakeHost) { f.noEndpoint = true }, "port identification", "no IP/MAC"},
		{"no peer interface", func(f *fakeRunner, _ *fakeHost) { f.noPeer = true }, "port identification", "no interface with MAC"},
		{"veth not on bridge", func(f *fakeRunner, _ *fakeHost) { f.wrongMaster = true }, "port identification", "has master"},
		{"MAC with a different IPv4", func(f *fakeRunner, _ *fakeHost) { f.nsOtherIP = "172.30.0.99" }, "port identification", "carries IPv4 172.30.0.99, want " + agentIP + " or none (runsc)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fr, h := newRunner(), newHost()
			fr.netExists = true
			c.setup(fr, h)
			r := run(t, fr, h, nil, armArgs...)
			if r.code != 2 {
				t.Fatalf("exit %d, want 2; stderr %q", r.code, r.err)
			}
			if !strings.HasPrefix(r.err, "failed control: "+c.control+": ") {
				t.Errorf("stderr must start with the failed control %q:\n%s", c.control, r.err)
			}
			if !strings.Contains(r.err, c.detail) || !strings.Contains(r.err, "GATED_UNARMED") {
				t.Errorf("stderr lacks %q or the exit state:\n%s", c.detail, r.err)
			}
			// The port is never released, so no accept rule exists; a table that was loaded stays, closed.
			if c.control != "port release" && fr.released() {
				t.Error("the port must not be released")
			}
			if len(fr.armedPorts()) != 0 {
				t.Errorf("no port may carry accept rules: %v", fr.armedPorts())
			}
			if fr.gate == nil || len(fr.gate.released) != 0 {
				t.Errorf("the MAC must not stay in the gate set: %+v", fr.gate)
			}
			for _, p := range []string{"docker rm", "docker stop", "docker start", "docker network connect", "docker network disconnect", "docker network rm"} {
				if fr.has(p) {
					t.Errorf("no rollback or lifecycle call allowed: %s", p)
				}
			}
			// The only deletion allowed is taking the MAC out of the gate set again after a failed release.
			for _, c := range fr.calls {
				if j := c.joined(); strings.HasPrefix(j, "nft delete") && !strings.HasPrefix(j, "nft delete element netdev agents_gate released") {
					t.Errorf("no rollback call allowed: %s", j)
				}
			}
			noSuccess(t, r)
		})
	}
}

// A network that predates the gate (or whose gate was changed) is a precondition failure of arm: exit 1, GATED_UNARMED,
// the operator is told to recreate the network; nothing is changed.
func TestArmRefusesWhenTheGateIsMissing(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*fakeRunner, *fakeHost)
	}{
		{"no gate table", func(f *fakeRunner, _ *fakeHost) { f.noGate = true }},
		{"no gate iptables rule", func(f *fakeRunner, _ *fakeHost) { f.noGateRule = true }},
		{"neither", func(f *fakeRunner, _ *fakeHost) { f.noGate, f.noGateRule = true, true }},
		{"bridge-nf-call-iptables is 0", func(_ *fakeRunner, h *fakeHost) { h.files[brNfFile] = "0\n" }},
		{"bridge-nf-call-iptables unreadable", func(_ *fakeRunner, h *fakeHost) { delete(h.files, brNfFile) }},
		{"gate chain policy accept", func(f *fakeRunner, _ *fakeHost) {
			f.ensureGate()
			f.gate.policy[config.ChainEgress] = "accept"
		}},
		{"gate set missing", func(f *fakeRunner, _ *fakeHost) {
			f.ensureGate()
			f.gate.set = false
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fr, h := newRunner(), newHost()
			fr.netExists = true
			c.setup(fr, h)
			r := run(t, fr, h, nil, armArgs...)
			if r.code != 1 || !strings.Contains(r.err, "GATED_UNARMED") || !strings.Contains(r.err, "predates the gate") ||
				!strings.Contains(r.err, "remove_agent_net, then create_agent_net") || strings.Contains(r.err, "failed control:") {
				t.Fatalf("exit %d, stderr %q", r.code, r.err)
			}
			for _, p := range []string{nftScript, "nft delete", "iptables -I", "iptables -D", "bridge", "nsenter", "docker container inspect", "docker network connect"} {
				if fr.has(p) {
					t.Errorf("%s must not run: %v", p, fr.calls)
				}
			}
			noSuccess(t, r)
		})
	}
}

func TestArmAppliedControlsStayAfterFailure(t *testing.T) {
	fr := newRunner()
	fr.netExists = true
	fr.rules = []rule{{contains: "iptables -I INPUT", stderr: "boom", code: 1}}
	r := run(t, fr, newHost(), nil, armArgs...)
	if r.code != 2 {
		t.Fatalf("exit %d", r.code)
	}
	tbl := fr.nft[tableName]
	if !fr.isolated || fr.learning || !fr.locked || !fr.fdb || len(fr.ipt["DOCKER-USER"]) != 8 || tbl == nil || tbl.ruleCount() != 0 {
		t.Error("already applied controls must stay in place and the port table must stay closed (no rollback)")
	}
	if fr.has("bridge link set dev vethabc isolated off") || fr.has("iptables -D") || fr.has("nft delete") || fr.gate == nil || len(fr.gate.released) != 0 {
		t.Error("no rollback command allowed")
	}
}

func TestArmNoHostConfigAccess(t *testing.T) {
	fr := newRunner()
	armOK(t, fr, newHost(), nil)
	for _, c := range fr.calls {
		j := c.joined()
		if strings.Contains(j, "daemon.json") || strings.Contains(j, "systemctl") || strings.Contains(j, "network create") {
			t.Errorf("forbidden call: %s", j)
		}
	}
}

// ---- check-gate ----

// checkLines returns all stdout lines of a check-gate run, unfiltered: stdout must be only the control lines and the
// summary (the `[n/m]` progress lines and the debug lines go to stderr).
func checkLines(out string) []string {
	if out == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(out, "\n"), "\n")
}

var checkArgs = []string{"check-gate", container}

func armedRunner(t *testing.T) *fakeRunner {
	t.Helper()
	fr := newRunner()
	armOK(t, fr, newHost(), nil)
	fr.calls = nil
	return fr
}

// gateLines is the expected check-gate stdout: the eleven controls of SFR-NET-021 (ok unless overridden by control
// label; the key "table" stands for the `table as_port_<id>` line) and the summary.
func gateLines(table string, over map[string]string, summary string) []string {
	var out []string
	for _, c := range []string{"table " + table, "chain policies", "accept rules", "agents-net gate", "gate release", "port flags", "static FDB entry", "neighbour entry",
		"DNS egress rule", "endpoint egress rule", "other tagged iptables rules", "container confinement"} {
		key := c
		if strings.HasPrefix(c, "table ") {
			key = "table"
		}
		state := "ok"
		if v, ok := over[key]; ok {
			state = v
		}
		out = append(out, c+": "+state)
	}
	return append(out, summary)
}

func allOKLines(summary string) []string { return gateLines(tableName, nil, summary) }

// without drops the iptables lines that contain sub.
func without(lines []string, sub string) []string {
	var out []string
	for _, l := range lines {
		if !strings.Contains(l, sub) {
			out = append(out, l)
		}
	}
	return out
}

// replaceIn replaces old by new in the iptables lines that contain match.
func replaceIn(lines []string, match, old, repl string) []string {
	var out []string
	for _, l := range lines {
		if strings.Contains(l, match) {
			l = strings.Replace(l, old, repl, 1)
		}
		out = append(out, l)
	}
	return out
}

// badLines is the expected check-gate stdout when the given controls are not ok.
func badLines(over map[string]string) []string { return gateLines(tableName, over, "inconsistent") }

func assertReadOnly(t *testing.T, fr *fakeRunner) {
	t.Helper()
	for _, c := range fr.calls {
		j := c.joined()
		for _, bad := range []string{"bridge link set", "bridge fdb replace", "bridge fdb del", "ip neigh replace", "ip neigh del", "iptables -I", "iptables -D", "iptables -A", "nft delete", "docker network connect", "docker rm"} {
			if strings.HasPrefix(j, bad) {
				t.Errorf("check-gate must only read: %s", j)
			}
		}
		if j == nftScript {
			t.Errorf("check-gate must not load an nft script: %s", c.Stdin)
		}
	}
}

func TestCheckGateReleased(t *testing.T) {
	fr := armedRunner(t)
	r := run(t, fr, newHost(), nil, checkArgs...)
	if r.code != 0 || !reflectEq(checkLines(r.stdout), allOKLines("released")) {
		t.Fatalf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.err)
	}
	assertReadOnly(t, fr)
}

// stdout is exactly the eleven control lines plus the summary, unfiltered; the `[n/m]` progress lines go to stderr.
func TestCheckGateStdoutIsOnlyControlsAndSummary(t *testing.T) {
	fr := armedRunner(t)
	r := run(t, fr, newHost(), nil, checkArgs...)
	want := strings.Join(allOKLines("released"), "\n") + "\n"
	if r.code != 0 || r.stdout != want {
		t.Fatalf("exit %d, stdout %q, want %q", r.code, r.stdout, want)
	}
	if strings.Contains(r.stdout, "[") || strings.Contains(r.stdout, "debug:") {
		t.Errorf("no progress or debug line on stdout: %q", r.stdout)
	}
	if !strings.Contains(r.err, "[1/7] validate invocation") || !strings.Contains(r.err, "[6/7] check port controls") {
		t.Errorf("progress lines must go to stderr: %q", r.err)
	}

	// a failing run keeps stdout clean too; DEBUG lines go to stderr as well
	fr = armedRunner(t)
	fr.isolated = false
	r = run(t, fr, newHost(), map[string]string{"AGENT_LOG_LEVEL": "DEBUG"}, checkArgs...)
	if want := strings.Join(badLines(map[string]string{"port flags": "mismatch"}), "\n") + "\n"; r.code != 2 || r.stdout != want {
		t.Fatalf("exit %d, stdout %q, want %q", r.code, r.stdout, want)
	}
	if !strings.Contains(r.err, "  debug: ") {
		t.Errorf("debug lines must go to stderr: %q", r.err)
	}
}

func TestCheckGateClosed(t *testing.T) {
	fr := armedRunner(t)
	for _, tb := range fr.nft {
		tb.rules = map[string][]string{} // as left by an arming run that stopped before the release
	}
	fr.gate.released = nil // the MAC is added only by the release
	r := run(t, fr, newHost(), nil, checkArgs...)
	if r.code != 0 || !reflectEq(checkLines(r.stdout), allOKLines("closed")) {
		t.Fatalf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.err)
	}
	assertReadOnly(t, fr)
}

// closed requires the MAC to be out of `released`, released requires it in; a mismatch is inconsistent. A missing
// gate table is inconsistent too.
func TestCheckGateStatesFollowTheGateSet(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*fakeRunner)
		want  []string
	}{
		{"closed port, MAC still in the set", func(f *fakeRunner) {
			for _, tb := range f.nft {
				tb.rules = map[string][]string{}
			}
		}, badLines(map[string]string{"gate release": "mismatch"})},
		{"released port, MAC not in the set", func(f *fakeRunner) { f.gate.released = nil },
			badLines(map[string]string{"gate release": "mismatch"})},
		{"gate table missing", func(f *fakeRunner) { f.gate = nil },
			badLines(map[string]string{"agents-net gate": "missing", "gate release": "missing"})},
		{"gate chain policy accept", func(f *fakeRunner) { f.gate.policy[config.ChainIngress] = "accept" },
			badLines(map[string]string{"agents-net gate": "mismatch"})},
		{"gate set missing (the MAC cannot be in it either)", func(f *fakeRunner) { f.gate.set = false },
			badLines(map[string]string{"agents-net gate": "mismatch", "gate release": "mismatch"})},
		{"gate iptables rule missing", func(f *fakeRunner) { f.ipt["DOCKER-USER"] = without(f.ipt["DOCKER-USER"], "agent-net:gate") },
			badLines(map[string]string{"agents-net gate": "mismatch"})},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fr := armedRunner(t)
			c.setup(fr)
			r := run(t, fr, newHost(), nil, checkArgs...)
			if r.code != 2 || !reflectEq(checkLines(r.stdout), c.want) || !strings.Contains(r.err, "GATE_INCONSISTENT") {
				t.Fatalf("exit %d, stdout lines %q, stderr %q", r.code, checkLines(r.stdout), r.err)
			}
			assertReadOnly(t, fr)
		})
	}
}

func TestCheckGateReportsEveryControlAfterAFailure(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*fakeRunner)
		want  []string
	}{
		{"no table", func(f *fakeRunner) { f.nft = map[string]*fakeTable{} },
			badLines(map[string]string{"table": "missing", "chain policies": "missing", "accept rules": "missing", "gate release": "mismatch"})},
		{"policy accept", func(f *fakeRunner) { f.nft[tableName].policy[config.ChainEgress] = "accept" },
			badLines(map[string]string{"chain policies": "mismatch"})},
		{"wrong veth", func(f *fakeRunner) { f.nft[tableName].veth = "vethother" },
			badLines(map[string]string{"chain policies": "mismatch"})},
		{"accept rule missing", func(f *fakeRunner) {
			f.nft[tableName].rules[config.ChainEgress] = f.nft[tableName].rules[config.ChainEgress][:1]
		}, badLines(map[string]string{"accept rules": "mismatch"})},
		{"extra accept rule", func(f *fakeRunner) {
			f.nft[tableName].rules[config.ChainIngress] = append(f.nft[tableName].rules[config.ChainIngress], "accept")
		}, badLines(map[string]string{"accept rules": "mismatch"})},
		{"not isolated", func(f *fakeRunner) { f.isolated = false },
			badLines(map[string]string{"port flags": "mismatch"})},
		{"learning on", func(f *fakeRunner) { f.learning = true },
			badLines(map[string]string{"port flags": "mismatch"})},
		{"not locked", func(f *fakeRunner) { f.locked = false },
			badLines(map[string]string{"port flags": "mismatch"})},
		{"no fdb entry", func(f *fakeRunner) { f.fdb = false },
			badLines(map[string]string{"static FDB entry": "missing"})},
		{"no neighbour entry", func(f *fakeRunner) { f.neigh = map[string]string{} },
			badLines(map[string]string{"neighbour entry": "missing"})},
		{"neighbour entry wrong lladdr", func(f *fakeRunner) { f.neigh[agentIP] = "02:00:00:00:00:99" },
			badLines(map[string]string{"neighbour entry": "mismatch"})},
		{"no tagged rules", func(f *fakeRunner) { f.ipt = map[string][]string{} },
			badLines(map[string]string{"agents-net gate": "mismatch", "DNS egress rule": "missing", "endpoint egress rule": "missing", "other tagged iptables rules": "missing"})},
		{"input replies accept missing", func(f *fakeRunner) { f.ipt["INPUT"] = f.ipt["INPUT"][1:] },
			badLines(map[string]string{"other tagged iptables rules": "mismatch"})},
		// DOCKER-USER top to bottom: dns 1.1.1.1 udp, tcp, 9.9.9.9 udp, tcp, two endpoint accepts, drop
		{"dns accept to a wrong destination", func(f *fakeRunner) {
			f.ipt["DOCKER-USER"] = without(f.ipt["DOCKER-USER"], "--dport 53")
			f.ipt["DOCKER-USER"] = append([]string{"-s " + agentIP + "/32 -d 8.8.8.8/32 -p udp -m udp --dport 53 -m comment --comment agent-net:" + container + " -j ACCEPT"}, f.ipt["DOCKER-USER"]...)
		}, badLines(map[string]string{"DNS egress rule": "mismatch"})},
		{"extra dns accept to another destination", func(f *fakeRunner) {
			f.ipt["DOCKER-USER"] = append([]string{"-s " + agentIP + "/32 -d 8.8.8.8/32 -p tcp -m tcp --dport 53 -m comment --comment agent-net:" + container + " -j ACCEPT"}, f.ipt["DOCKER-USER"]...)
		}, badLines(map[string]string{"DNS egress rule": "mismatch"})},
		{"dns rules missing", func(f *fakeRunner) { f.ipt["DOCKER-USER"] = without(f.ipt["DOCKER-USER"], "--dport 53") },
			badLines(map[string]string{"DNS egress rule": "missing"})},
		{"endpoint accept to a wrong destination", func(f *fakeRunner) {
			f.ipt["DOCKER-USER"] = without(f.ipt["DOCKER-USER"], "--dport 443")
			f.ipt["DOCKER-USER"] = append([]string{"-s " + agentIP + "/32 -d 203.0.113.9/32 -p tcp -m tcp --dport 443 -m comment --comment agent-net:" + container + " -j ACCEPT"}, f.ipt["DOCKER-USER"]...)
		}, badLines(map[string]string{"endpoint egress rule": "mismatch"})},
		{"endpoint rules missing", func(f *fakeRunner) { f.ipt["DOCKER-USER"] = without(f.ipt["DOCKER-USER"], "--dport 443") },
			badLines(map[string]string{"endpoint egress rule": "missing"})},
		{"wrong source on the egress default-deny", func(f *fakeRunner) {
			f.ipt["DOCKER-USER"] = replaceIn(f.ipt["DOCKER-USER"], "! -o "+bridgeName, "-s "+agentIP+"/32", "-s 172.30.0.99/32")
		}, badLines(map[string]string{"other tagged iptables rules": "mismatch"})},
		{"wrong source on the input default-deny", func(f *fakeRunner) {
			f.ipt["INPUT"] = replaceIn(f.ipt["INPUT"], "-j DROP", "-s "+agentIP+"/32", "-s 172.30.0.99/32")
		}, badLines(map[string]string{"other tagged iptables rules": "mismatch"})},
		{"wrong source on the replies accept", func(f *fakeRunner) {
			f.ipt["INPUT"] = replaceIn(f.ipt["INPUT"], "ESTABLISHED", "-s "+agentIP+"/32", "-s 172.30.0.99/32")
		}, badLines(map[string]string{"other tagged iptables rules": "mismatch"})},
		{"egress default-deny missing", func(f *fakeRunner) { f.ipt["DOCKER-USER"] = without(f.ipt["DOCKER-USER"], "! -o "+bridgeName) },
			badLines(map[string]string{"other tagged iptables rules": "mismatch"})},
		// SFR-NET-021: an unexpected tagged accept rule in INPUT counts as well as one in DOCKER-USER
		{"extra tagged accept in INPUT", func(f *fakeRunner) {
			f.ipt["INPUT"] = append([]string{"-s " + agentIP + "/32 -p tcp -m tcp --dport 22 -m comment --comment agent-net:" + container + " -j ACCEPT"}, f.ipt["INPUT"]...)
		}, badLines(map[string]string{"other tagged iptables rules": "mismatch"})},
		// whole-field matching: port 5353 is not port 53
		{"dns accept on port 5353 is not port 53", func(f *fakeRunner) {
			f.ipt["DOCKER-USER"] = replaceIn(f.ipt["DOCKER-USER"], "-d 1.1.1.1/32 -p udp", "--dport 53", "--dport 5353")
		}, badLines(map[string]string{"DNS egress rule": "mismatch", "other tagged iptables rules": "mismatch"})},
		// whole-field matching in the nft rules: 172.30.0.22 is not 172.30.0.2
		{"accept rule for an IP that starts like the agent IP", func(f *fakeRunner) {
			r := f.nft[tableName].rules[config.ChainIngress]
			// the rule is "... ip saddr <ip> meta mark set ... accept" since the port mark; the IP becomes 172.30.0.22
			if n := strings.Replace(r[0], "ip saddr "+agentIP+" meta mark", "ip saddr "+agentIP+"2 meta mark", 1); n != r[0] {
				r[0] = n
			} else {
				t.Fatalf("the test no longer changes the rule: %s", r[0])
			}
		}, badLines(map[string]string{"accept rules": "mismatch"})},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fr := armedRunner(t)
			c.setup(fr)
			r := run(t, fr, newHost(), nil, checkArgs...)
			if r.code != 2 || !reflectEq(checkLines(r.stdout), c.want) {
				t.Fatalf("exit %d, stdout lines %q, stderr %q", r.code, checkLines(r.stdout), r.err)
			}
			if !strings.Contains(r.err, "GATE_INCONSISTENT") || strings.Contains(r.err, "NOT_CHECKED") {
				t.Errorf("stderr must carry the exit state: %q", r.err)
			}
			// every control that is not ok is repeated on stderr with its label
			for _, l := range c.want {
				if strings.HasSuffix(l, ": missing") || strings.HasSuffix(l, ": mismatch") {
					if !strings.Contains(r.err, "FAILED CHECK: "+l+": ") {
						t.Errorf("stderr lacks the failed control %q:\n%s", l, r.err)
					}
				}
			}
			assertReadOnly(t, fr)
		})
	}
}

func TestCheckGateReadBackFailureExit2(t *testing.T) {
	fr := armedRunner(t)
	fr.rules = []rule{{contains: "nft list tables", stderr: "nft: boom", code: 1}}
	r := run(t, fr, newHost(), nil, checkArgs...)
	lines := checkLines(r.stdout)
	if r.code != 2 || lines[len(lines)-1] != "inconsistent" || !strings.Contains(r.err, "GATE_INCONSISTENT") {
		t.Fatalf("exit %d, stdout lines %q, stderr %q", r.code, lines, r.err)
	}
	if lines[0] != "table "+tableName+": mismatch" || len(lines) != 13 { // twelve controls (confinement included) and the summary
		t.Errorf("every control must still be reported: %q", lines)
	}
}

func TestCheckGateStoppedContainerHasNoVeth(t *testing.T) {
	fr := twoAgents()
	armAll(t, fr, agentA)
	fr.ctrs[agentA].pid = 0
	fr.calls = nil
	r := run(t, fr, newHost(), nil, "check-gate", agentA)
	want := gateLines("as_port_a", map[string]string{"accept rules": "mismatch", "port flags": "missing", "static FDB entry": "missing", "neighbour entry": "missing", "container confinement": "mismatch"}, "inconsistent")
	if r.code != 2 || !reflectEq(checkLines(r.stdout), want) {
		t.Fatalf("exit %d, stdout lines %q, stderr %q", r.code, checkLines(r.stdout), r.err)
	}
	assertReadOnly(t, fr)
}

// check-gate also requires restart policy "no", agents-net as the only network and a running container; a paused
// container is fine (create_agent_container runs check-gate before it unpauses).
func TestCheckGateConfinement(t *testing.T) {
	for name, c := range map[string]struct {
		setup func(*fakeRunner)
		ok    bool
	}{
		"paused is accepted":   {func(f *fakeRunner) {}, true},
		"restart always":       {func(f *fakeRunner) { f.restart = "always" }, false},
		"restart empty":        {func(f *fakeRunner) { f.restart = "default" }, false},
		"second network":       {func(f *fakeRunner) { f.extraNet = true }, false},
		"container not running": {func(f *fakeRunner) { f.stopped = true }, false},
	} {
		t.Run(name, func(t *testing.T) {
			fr := armedRunner(t)
			c.setup(fr)
			r := run(t, fr, newHost(), nil, checkArgs...)
			got := "container confinement: ok"
			if !c.ok {
				got = "container confinement: mismatch"
			}
			if !strings.Contains(r.stdout, got+"\n") || (c.ok && r.code != 0) || (!c.ok && r.code != 2) {
				t.Fatalf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.err)
			}
		})
	}
}

func TestCheckGateNotChecked(t *testing.T) {
	t.Run("network missing", func(t *testing.T) {
		r := run(t, newRunner(), newHost(), nil, checkArgs...)
		if r.code != 1 || !strings.Contains(r.err, "network agents-net does not exist") || !strings.Contains(r.err, "NOT_CHECKED") {
			t.Fatalf("exit %d, stderr %q", r.code, r.err)
		}
	})
	t.Run("unknown container", func(t *testing.T) {
		fr := twoAgents()
		r := run(t, fr, newHost(), nil, "check-gate", "agent-zzz")
		if r.code != 1 || !strings.Contains(r.err, "container agent-zzz does not exist") || !strings.Contains(r.err, "NOT_CHECKED") {
			t.Fatalf("exit %d, stderr %q", r.code, r.err)
		}
		if strings.Contains(r.stdout, "table:") || strings.Contains(r.stdout, "inconsistent") {
			t.Errorf("nothing was checked: %q", r.stdout)
		}
		assertReadOnly(t, fr)
	})
	t.Run("privilege", func(t *testing.T) {
		fr, h := twoAgents(), newHost()
		h.euid = 1000
		r := run(t, fr, h, nil, "check-gate", agentA)
		if r.code != 1 || !strings.Contains(r.err, "NOT_CHECKED") || len(fr.calls) != 0 {
			t.Fatalf("exit %d, stderr %q, calls %v", r.code, r.err, fr.calls)
		}
	})
}

// ---- absent versus failed: only docker's not-found answer is an absence ----

const engineDown = "Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?"

func TestUnreadableNetworkOrContainerIsNotAbsent(t *testing.T) {
	t.Run("create: network unreadable", func(t *testing.T) {
		fr := newRunner()
		fr.netInspectErr = engineDown
		r := run(t, fr, newHost(), nil)
		if r.code != 1 || !strings.Contains(r.err, "NOT_CREATED") || !strings.Contains(r.err, "Cannot connect") {
			t.Fatalf("exit %d, stderr %q", r.code, r.err)
		}
		if fr.has("docker network create") {
			t.Error("nothing may be created when the network cannot be read")
		}
		noSuccess(t, r)
	})
	for _, mode := range []struct {
		args  []string
		state string
	}{
		{armArgs, "GATED_UNARMED"}, {checkArgs, "NOT_CHECKED"}, {[]string{"disarm", container}, "NOT_DISARMED"},
	} {
		t.Run(mode.args[0]+": network unreadable", func(t *testing.T) {
			fr := newRunner()
			fr.netExists = true
			fr.netInspectErr = engineDown
			r := run(t, fr, newHost(), nil, mode.args...)
			if r.code != 1 || !strings.Contains(r.err, mode.state) || !strings.Contains(r.err, "Cannot connect") || strings.Contains(r.err, "does not exist") {
				t.Fatalf("exit %d, stderr %q", r.code, r.err)
			}
			for _, p := range []string{"docker network connect", "nft", "bridge", "iptables", "ip neigh"} {
				if fr.has(p) {
					t.Errorf("%s must not run: %v", p, fr.calls)
				}
			}
			noSuccess(t, r)
		})
		t.Run(mode.args[0]+": container unreadable", func(t *testing.T) {
			fr := newRunner()
			fr.netExists = true
			fr.ctrInspectErr = map[string]string{"*": "permission denied while trying to connect to the Docker daemon socket"}
			r := run(t, fr, newHost(), nil, mode.args...)
			if r.code != 1 || !strings.Contains(r.err, mode.state) || !strings.Contains(r.err, "permission denied") || strings.Contains(r.err, "does not exist") {
				t.Fatalf("exit %d, stderr %q", r.code, r.err)
			}
			// arm reads the gate before it inspects the container; nothing may be changed
			for _, p := range []string{"docker network connect", nftScript, "nft delete", "bridge", "iptables -I", "iptables -D", "ip neigh"} {
				if fr.has(p) {
					t.Errorf("%s must not run: %v", p, fr.calls)
				}
			}
			noSuccess(t, r)
		})
	}
}

func TestDockerNotFoundAnswersAreAbsent(t *testing.T) {
	for _, msg := range []string{
		"Error: No such network: agents-net", "Error response from daemon: network agents-net not found",
		"Error: No such object: agents-net", "ERROR: NO SUCH NETWORK",
	} {
		fr := newRunner()
		fr.netInspectErr = msg
		r := run(t, fr, newHost(), nil, armArgs...)
		if r.code != 1 || !strings.Contains(r.err, "network agents-net does not exist") {
			t.Errorf("%q: exit %d, stderr %q", msg, r.code, r.err)
		}
	}
	for _, msg := range []string{"Error: No such container: web-1", "Error: No such object: web-1", "Error response from daemon: container web-1 not found"} {
		fr := newRunner()
		fr.netExists = true
		fr.ctrInspectErr = map[string]string{"*": msg}
		r := run(t, fr, newHost(), nil, armArgs...)
		if r.code != 1 || !strings.Contains(r.err, "container web-1 does not exist") {
			t.Errorf("%q: exit %d, stderr %q", msg, r.code, r.err)
		}
	}
}

// ---- docker CLI socket and iptables lock ----

func TestDockerCliUsesTheCheckedSocket(t *testing.T) {
	for env, want := range map[string]string{"": "unix:///var/run/docker.sock", "/run/other.sock": "unix:///run/other.sock"} {
		fr := newRunner()
		run(t, fr, newHost(), map[string]string{"AGENT_NET_DOCKER_SOCKET": env})
		if len(fr.dockerHosts) == 0 {
			t.Fatal("no docker call")
		}
		for _, got := range fr.dockerHosts {
			if got != want {
				t.Errorf("socket %q: docker call with -H %q, want %q", env, got, want)
			}
		}
	}
}

func TestIptablesCallsWaitForTheLock(t *testing.T) {
	fr := armedRunner(t)
	run(t, fr, newHost(), nil, checkArgs...)
	if fr.count("iptables") == 0 || fr.iptNoWait != 0 {
		t.Errorf("iptables calls = %d, without -w = %d", fr.count("iptables"), fr.iptNoWait)
	}
}

func TestHasIptRuleMatchesWholeFields(t *testing.T) {
	line := "-A DOCKER-USER -s 172.30.0.2/32 -d 1.1.1.1/32 -p udp -m udp --dport 5353 -m comment --comment agent-net:web-10 -j ACCEPT"
	for tok, want := range map[string]bool{
		"--dport 53": false, "--dport 5353": true, "agent-net:web-1": false, "agent-net:web-10": true,
		"-s 172.30.0.2/32": true, "-s 172.30.0.2": false, "-j ACCEPT": true, "-d 1.1.1.1/32": true,
	} {
		if got := services.HasIptRule([]string{line}, tok); got != want {
			t.Errorf("token %q: %v, want %v", tok, got, want)
		}
	}
	state := "-A INPUT -s 172.30.0.2/32 -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT"
	if !services.HasIptRule([]string{state}, "--ctstate", "ESTABLISHED", "RELATED") {
		t.Error("the conntrack states must match as separate fields")
	}
}

func TestArmTagMatchIsExact(t *testing.T) {
	fr := newRunner()
	// a rule of web-10 that equals the first wanted rule of web-1 but for the tag
	other := "-s 172.30.0.2/32 -d 1.1.1.1/32 -p udp -m udp --dport 53 -m comment --comment agent-net:web-10 -j ACCEPT"
	fr.ipt["DOCKER-USER"] = []string{other}
	armOK(t, fr, newHost(), nil)
	if got := fr.count("iptables -I DOCKER-USER"); got != 7 {
		t.Errorf("DOCKER-USER inserts = %d, want 7: the rule of web-10 must not stand in for one of web-1", got)
	}
	if n := fr.iptTagged(container); n != 9 {
		t.Errorf("rules tagged for web-1 = %d, want 9", n)
	}
	if !strings.Contains(strings.Join(fr.ipt["DOCKER-USER"], "\n"), other) {
		t.Error("the rule of web-10 must stay")
	}
}

// ---- arm: preconditions on the container, pause kept during the run ----

func TestArmRequiresAgentsNetAndRestartNo(t *testing.T) {
	for name, c := range map[string]struct {
		mode, restart string
		extraNet      bool
	}{
		"bridge network": {"bridge", "", false}, "none network": {"none", "", false}, "restart always": {"", "always", false},
		"restart on-failure": {"", "on-failure", false}, "second network attached": {"", "", true},
		"restart policy empty": {"", "default", false}, "runtime runc": {"", "", false},
	} {
		t.Run(name, func(t *testing.T) {
			fr := newRunner()
			fr.netExists = true
			fr.netMode, fr.restart, fr.extraNet = c.mode, c.restart, c.extraNet
			if name == "runtime runc" {
				fr.runtime = "runc"
			}
			r := run(t, fr, newHost(), nil, armArgs...)
			if r.code != 1 || !strings.Contains(r.err, "was not created with --network agents-net --restart no") || !strings.Contains(r.err, "GATED_UNARMED") {
				t.Fatalf("exit %d, stderr %q", r.code, r.err)
			}
			for _, p := range []string{"docker network connect", "docker network disconnect", nftScript, "nft delete", "bridge", "iptables -I", "iptables -D", "nsenter"} {
				if fr.has(p) {
					t.Errorf("%s must not run: %v", p, fr.calls)
				}
			}
			noSuccess(t, r)
		})
	}
}

func TestArmKeepsTheContainerPausedThroughTheRun(t *testing.T) {
	cases := []struct {
		name   string
		hook   func(fr *fakeRunner) func(j string)
		detail string
		table  bool // the port table was already loaded: it must stay closed
	}{
		{"after the controls", func(fr *fakeRunner) func(string) {
			n := 0
			return func(j string) {
				if strings.HasPrefix(j, "iptables -I INPUT") {
					if n++; n == 2 {
						fr.notPaused = true
					}
				}
			}
		}, "after the controls were applied", true},
		{"before the release", func(fr *fakeRunner) func(string) {
			n := 0
			return func(j string) {
				if strings.HasPrefix(j, "nft list table netdev as_port_") { // read-backs of the port table: closing, then the verification
					if n++; n == 2 {
						fr.notPaused = true
					}
				}
			}
		}, "before the release", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fr := newRunner()
			fr.netExists = true
			fr.afterCall = c.hook(fr)
			r := run(t, fr, newHost(), nil, armArgs...)
			if r.code != 2 || !strings.HasPrefix(r.err, "failed control: container paused: ") || !strings.Contains(r.err, c.detail) || !strings.Contains(r.err, "GATED_UNARMED") {
				t.Fatalf("exit %d, stderr %q", r.code, r.err)
			}
			if fr.released() || len(fr.armedPorts()) != 0 {
				t.Errorf("the port must not be released: %v", fr.armedPorts())
			}
			if tbl := fr.nft[tableName]; c.table != (tbl != nil) || (tbl != nil && tbl.ruleCount() != 0) {
				t.Errorf("table = %+v, want loaded=%v and closed", tbl, c.table)
			}
			noSuccess(t, r)
		})
	}
}

// runsc deletes the address from the netns interface (the default fake); a runc-style interface still carrying MAC and IP matches too.
func TestArmIdentifiesThePortWithOrWithoutTheIPInTheNetns(t *testing.T) {
	for _, c := range []struct {
		name   string
		withIP bool
	}{{"runsc: MAC, no address", false}, {"runc: MAC and IP", true}} {
		t.Run(c.name, func(t *testing.T) {
			fr := newRunner()
			fr.netExists = true
			fr.nsWithIP = c.withIP
			r := run(t, fr, newHost(), nil, armArgs...)
			if r.code != 0 {
				t.Fatalf("exit %d, want 0; stderr %q", r.code, r.err)
			}
		})
	}
}

func TestArmFailsClosedOnAnAmbiguousInterface(t *testing.T) {
	fr := newRunner()
	fr.netExists = true
	fr.dupIface = true
	r := run(t, fr, newHost(), nil, armArgs...)
	if r.code != 2 || !strings.HasPrefix(r.err, "failed control: port identification: ") || !strings.Contains(r.err, "ambiguous") || !strings.Contains(r.err, "GATED_UNARMED") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.has(nftScript) || fr.has("bridge link set") || fr.has("bridge fdb") || fr.released() {
		t.Errorf("no control may be applied for an ambiguous port: %v", fr.calls)
	}
}

// as_port_<id> is derived from the name: agent-web and web both give as_port_web.
func TestArmRefusesATableOfAnotherPort(t *testing.T) {
	fr := newRunner()
	fr.netExists = true
	fr.ctrs = map[string]*fakeCtr{
		"agent-web": {ip: "172.30.0.2", mac: "02:42:ac:1e:00:02", veth: "vethw1", peer: 7, pid: 4301},
		"web":       {ip: "172.30.0.3", mac: "02:42:ac:1e:00:03", veth: "vethw2", peer: 8, pid: 4302},
	}
	fr.members = []string{"agent-web", "web"}
	armAll(t, fr, "agent-web")
	fr.calls = nil

	r := run(t, fr, newHost(), nil, "arm", "web")
	if r.code != 1 || !strings.Contains(r.err, "as_port_web") || !strings.Contains(r.err, "vethw1") || !strings.Contains(r.err, "GATED_UNARMED") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if tbl := fr.nft["as_port_web"]; tbl == nil || tbl.veth != "vethw1" || tbl.ruleCount() != 4 {
		t.Errorf("the table of agent-web must be untouched: %+v", tbl)
	}
	for _, p := range []string{nftScript, "bridge link set", "bridge fdb replace", "iptables -I", "ip neigh replace"} {
		if fr.has(p) {
			t.Errorf("%s must not run: %v", p, fr.calls)
		}
	}
	noSuccess(t, r)
}

// SFR-NET-011: a failure after the accept rules were written puts the port back to closed first.
func TestArmReleaseFailureClosesThePortAgain(t *testing.T) {
	cases := []struct {
		name     string
		setup    func(fr *fakeRunner)
		detail   string
		gateLeft bool // the gate cannot be read either, so the MAC cannot be taken out of the set again (the closed port table still blocks)
	}{
		{"read-back does not see the accept rules", func(fr *fakeRunner) { fr.hideRules = true }, "not released", false},
		{"read-back fails", func(fr *fakeRunner) {
			fr.afterCall = func(string) {
				if len(fr.armedPorts()) > 0 {
					fr.rules = []rule{{contains: "nft list table netdev", stderr: "nft: boom", code: 1}}
				}
			}
		}, "unreadable after release", true},
		{"the MAC is not in the gate set", func(fr *fakeRunner) { fr.tamper["gaterelease"] = true }, "is not in set released", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fr := newRunner()
			fr.netExists = true
			c.setup(fr)
			r := run(t, fr, newHost(), nil, armArgs...)
			if r.code != 2 || !strings.HasPrefix(r.err, "failed control: port release: ") || !strings.Contains(r.err, c.detail) || !strings.Contains(r.err, "GATED_UNARMED") {
				t.Fatalf("exit %d, stderr %q", r.code, r.err)
			}
			rel := fr.releaseIndex()
			if rel < 0 {
				t.Fatal("the accept rules must have been written")
			}
			closedAfter := false
			for i, call := range fr.calls {
				closedAfter = closedAfter || (i > rel && isLoad(call))
			}
			tbl := fr.nft[tableName]
			if !closedAfter || tbl == nil || tbl.ruleCount() != 0 || tbl.policy[config.ChainIngress] != "drop" || tbl.policy[config.ChainEgress] != "drop" {
				t.Errorf("the closed table must be loaded again after the release: closedAfter=%v table=%+v", closedAfter, tbl)
			}
			if len(fr.armedPorts()) != 0 {
				t.Errorf("no port may carry accept rules: %v", fr.armedPorts())
			}
			// closeAgain also takes the MAC out of the gate set again, before the closed table is loaded.
			if c.gateLeft {
				if fr.gate == nil || !reflectEq(fr.gate.released, []string{agentMAC}) || !strings.Contains(r.err, "could not be removed from agents_gate") {
					t.Errorf("gate = %+v, stderr %q", fr.gate, r.err)
				}
			} else {
				if fr.gate == nil || len(fr.gate.released) != 0 {
					t.Errorf("the MAC must be out of the gate set again: %+v", fr.gate)
				}
				if c.name != "the MAC is not in the gate set" { // there the element was never added: nothing to delete
					del, load := fr.index("nft delete element netdev agents_gate released { "+agentMAC+" }"), -1
					for i, call := range fr.calls {
						if i > rel && isLoad(call) && load < 0 {
							load = i
						}
					}
					if !(del > rel && del < load) {
						t.Errorf("release=%d delete element=%d closed table reloaded=%d", rel, del, load)
					}
				}
			}
			noSuccess(t, r)
		})
	}
}

// ---- engine state (SSR-NET-001) in every mode ----

func TestEngineStateIsRecordedAndComparedInArmAndCheckGate(t *testing.T) {
	changes := map[string]func(h *fakeHost){
		"daemon.json changed": func(h *fakeHost) { h.files["/etc/docker/daemon.json"] = `{"changed":true}` },
		"engine was restarted": func(h *fakeHost) {
			h.files["/proc/4242/stat"] = statLine(4242, "999999")
		},
	}
	for want, change := range changes {
		t.Run("arm: "+want, func(t *testing.T) {
			fr, h := newRunner(), newHost()
			fr.netExists = true
			fr.afterCall = func(j string) {
				if j == nftScript { // the closed port table is loaded
					change(h)
				}
			}
			r := run(t, fr, h, nil, armArgs...)
			if r.code != 2 || !strings.HasPrefix(r.err, "failed control: engine state: ") || !strings.Contains(r.err, want) || !strings.Contains(r.err, "GATED_UNARMED") {
				t.Fatalf("exit %d, stderr %q", r.code, r.err)
			}
			noSuccess(t, r)
		})
		t.Run("check-gate: "+want, func(t *testing.T) {
			fr, h := armedRunner(t), newHost()
			fr.afterCall = func(j string) {
				if j == "nft list tables" {
					change(h)
				}
			}
			r := run(t, fr, h, nil, checkArgs...)
			if r.code != 2 || !strings.Contains(r.err, "GATE_INCONSISTENT") || !strings.Contains(r.err, want) {
				t.Fatalf("exit %d, stderr %q", r.code, r.err)
			}
			assertReadOnly(t, fr)
		})
	}
	// the state is recorded before anything is changed: unreadable means nothing is done
	for _, c := range []struct {
		args  []string
		state string
	}{{armArgs, "GATED_UNARMED"}, {checkArgs, "NOT_CHECKED"}, {[]string{"disarm", container}, "NOT_DISARMED"}} {
		t.Run(c.args[0]+": engine state unreadable at the start", func(t *testing.T) {
			fr, h := newRunner(), newHost()
			fr.netExists = true
			delete(h.files, "/var/run/docker.pid")
			r := run(t, fr, h, nil, c.args...)
			if r.code != 1 || !strings.Contains(r.err, "engine state could not be recorded") || !strings.Contains(r.err, c.state) {
				t.Fatalf("exit %d, stderr %q", r.code, r.err)
			}
			for _, p := range []string{"docker network connect", "nft", "bridge", "iptables", "ip neigh"} {
				if fr.has(p) {
					t.Errorf("%s must not run: %v", p, fr.calls)
				}
			}
		})
	}
}
