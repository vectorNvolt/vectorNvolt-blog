package tests

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"agent-net/internal/config"
	"agent-net/internal/sequence"
)

// ---- harness for `create_agent_net disarm` (USC-004 alt-2) and remove_agent_net (USC-007) ----

const (
	agentA = "agent-a"
	agentB = "agent-b"
	tableA = "as_port_a"
	tableB = "as_port_b"
)

// twoAgents is a network with two running agent containers, not yet armed.
func twoAgents() *fakeRunner {
	fr := newRunner()
	fr.netExists = true
	fr.ctrs = map[string]*fakeCtr{
		agentA: {ip: "172.30.0.2", mac: "02:42:ac:1e:00:02", veth: "vetha", peer: 7, pid: 4301},
		agentB: {ip: "172.30.0.3", mac: "02:42:ac:1e:00:03", veth: "vethb", peer: 8, pid: 4302},
	}
	fr.members = []string{agentA, agentB}
	return fr
}

func armAll(t *testing.T, fr *fakeRunner, names ...string) {
	t.Helper()
	for _, n := range names {
		if r := run(t, fr, newHost(), nil, "arm", n); r.code != 0 {
			t.Fatalf("arm %s: exit %d, stderr:\n%s", n, r.code, r.err)
		}
	}
}

func runRemove(t *testing.T, fr *fakeRunner, args ...string) result {
	t.Helper()
	return runRemoveHost(t, fr, newHost(), args...)
}

func runRemoveHost(t *testing.T, fr *fakeRunner, h *fakeHost, args ...string) result {
	t.Helper()
	cfg := config.Load(func(string) string { return "" })
	var out, errb bytes.Buffer
	code := sequence.RunRemove(context.Background(), sequence.Deps{
		Config: cfg, Runner: fr, Host: h, Args: args, Stdout: &out, Stderr: &errb,
	})
	return result{code, out.String(), errb.String()}
}

// iptTagged counts the iptables rules (all chains) carrying the container's comment.
func (f *fakeRunner) iptTagged(name string) int {
	n := 0
	for _, rules := range f.ipt {
		for _, r := range rules {
			if strings.Contains(r+" ", "--comment agent-net:"+name+" ") {
				n++
			}
		}
	}
	return n
}

// state is a printable snapshot of everything the disarm and remove flows may change.
func (f *fakeRunner) state() string {
	f.ensureGate() // a network that exists has its gate before the first run, so a snapshot before it matches one after it
	var b strings.Builder
	if f.gate != nil {
		fmt.Fprintf(&b, "gate %+v\n", *f.gate)
	}
	for _, n := range f.tableNames() {
		t := f.nft[n]
		fmt.Fprintf(&b, "nft %s veth=%s policy=%v rules=%v\n", n, t.veth, t.policy, t.rules)
	}
	var ctrs []string
	for n := range f.ctrs {
		ctrs = append(ctrs, n)
	}
	sort.Strings(ctrs)
	fmt.Fprintf(&b, "neigh %v\n", f.neigh)
	fmt.Fprintf(&b, "ipt %v\nmembers %v\nctrs %v\nnet %v\n", f.ipt, f.members, ctrs, f.agentsNetExists())
	return b.String()
}

func (f *fakeRunner) lastIndex(prefix string) int {
	last := -1
	for i, c := range f.calls {
		if strings.HasPrefix(c.joined(), prefix) {
			last = i
		}
	}
	return last
}

// mutatedHost reports a call that changes nft, iptables or the bridge.
func (f *fakeRunner) mutatedHost() bool {
	for _, c := range f.calls {
		j := c.joined()
		if strings.HasPrefix(j, "iptables -D") || strings.HasPrefix(j, "iptables -I") || strings.HasPrefix(j, "bridge fdb") || strings.HasPrefix(j, "ip neigh") ||
			strings.HasPrefix(j, "bridge link set") || strings.HasPrefix(j, "nft delete") || j == nftScript {
			return true
		}
	}
	return false
}

func ghostTable() *fakeTable {
	return &fakeTable{veth: "vethgone", policy: map[string]string{"ingress": "drop", "egress": "drop"}, rules: map[string][]string{}}
}

// ---- disarm ----

func TestDisarmUsage(t *testing.T) {
	for _, args := range [][]string{{"disarm"}, {"disarm", "a", "b"}, {"disarm", ""}, {"disarm", "-bad"}, {"disarm", "a/b"}, {"disarm", "BAD!"}} {
		fr := newRunner()
		r := run(t, fr, newHost(), nil, args...)
		if r.code != 1 || !strings.Contains(r.err, "create_agent_net disarm <container>") || !strings.Contains(r.err, "NOT_DISARMED") ||
			strings.Contains(r.err, "NOT_CREATED") || len(fr.calls) != 0 || fr.mutatedHost() {
			t.Errorf("args %q: exit %d, stderr %q, calls %v", args, r.code, r.err, fr.calls)
		}
	}
}

func TestDisarmRemovesOnlyThatContainersTable(t *testing.T) {
	fr := twoAgents()
	armAll(t, fr, agentA, agentB)
	if len(fr.portRules(tableA)) != 4 || len(fr.portRules(tableB)) != 4 || fr.iptTagged(agentA) != 9 || fr.iptTagged(agentB) != 9 {
		t.Fatalf("both containers must be fully armed first:\n%s", fr.state())
	}
	fr.calls = nil

	r := run(t, fr, newHost(), nil, "disarm", agentA)
	if r.code != 0 || r.err != "" || !strings.Contains(r.stdout, "container agent-a disarmed on agents-net") {
		t.Fatalf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.err)
	}
	if fr.nft[tableA] != nil || fr.iptTagged(agentA) != 0 || fr.ctrs[agentA].fdb {
		t.Errorf("table, rules and fdb entry of agent-a must be gone:\n%s", fr.state())
	}
	// The other container is still armed, table, rules and fdb entry.
	if len(fr.portRules(tableB)) != 4 || fr.iptTagged(agentB) != 9 || !fr.ctrs[agentB].fdb {
		t.Errorf("agent-b must stay armed:\n%s", fr.state())
	}
	if !reflectEq(fr.deleted, []string{tableA}) {
		t.Errorf("deleted tables = %v", fr.deleted)
	}
	// The port stays locked and isolated: no flag is changed, no container is touched, nothing is loaded.
	for _, p := range []string{"bridge link set", "docker rm", "docker stop", "docker network rm", "docker network disconnect", nftScript} {
		if fr.has(p) {
			t.Errorf("%s must not run", p)
		}
	}
	if !fr.isolated || fr.learning || !fr.locked {
		t.Errorf("isolated=%v learning=%v locked=%v must be unchanged", fr.isolated, fr.learning, fr.locked)
	}
	// The MAC of agent-a left the gate set; the gate itself and the MAC of agent-b stay.
	if fr.gate == nil || fr.gate.has("02:42:ac:1e:00:02") || !fr.gate.has("02:42:ac:1e:00:03") || fr.gateDeleted != 0 {
		t.Errorf("gate set after disarming agent-a: %+v", fr.gate)
	}
	// Connecting or disconnecting a container is never part of disarm.
	if fr.has("docker network connect") || fr.has("docker network disconnect") {
		t.Error("disarm must not connect or disconnect")
	}
}

// disarm takes the MAC out of the gate set BEFORE it deletes the port table, and verifies the set afterwards.
func TestDisarmRemovesTheMACFromTheGateBeforeTheTable(t *testing.T) {
	fr := twoAgents()
	armAll(t, fr, agentA)
	if !fr.gate.has("02:42:ac:1e:00:02") {
		t.Fatalf("arm must have released the MAC: %+v", fr.gate)
	}
	fr.calls = nil
	r := run(t, fr, newHost(), nil, "disarm", agentA)
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	el, table := fr.index("nft delete element netdev agents_gate released { 02:42:ac:1e:00:02 }"), fr.index("nft delete table netdev "+tableA)
	if !(el >= 0 && el < table) {
		t.Errorf("order: delete element=%d delete table=%d", el, table)
	}
	if fr.gate.has("02:42:ac:1e:00:02") {
		t.Errorf("the MAC is still in the set: %+v", fr.gate)
	}
	// verified: the gate is read again after the deletions
	if fr.lastIndex("nft -j list set netdev agents_gate released") < fr.lastIndex("iptables -D") {
		t.Error("the gate set must be read back after the deletions")
	}
	// A second disarm has nothing to remove from the set: no delete element call.
	fr.calls = nil
	if r := run(t, fr, newHost(), nil, "disarm", agentA); r.code != 0 || fr.has("nft delete") {
		t.Errorf("second disarm: exit %d, calls %v", r.code, fr.calls)
	}
}

// The MAC must be gone from the set after disarm; a delete that does not take effect is DISARM_INCOMPLETE.
func TestDisarmVerifiesTheGateSet(t *testing.T) {
	fr := twoAgents()
	armAll(t, fr, agentA)
	fr.rules = []rule{{contains: "nft delete element", stderr: "nft: boom", code: 1}}
	r := run(t, fr, newHost(), nil, "disarm", agentA)
	if r.code != 2 || !strings.Contains(r.err, "DISARM_INCOMPLETE") || !strings.Contains(r.err, "nft: boom") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.nft[tableA] == nil {
		t.Error("the table is deleted after the gate; a failed gate step stops the disarm before it")
	}
	fr.rules = nil
	if r := run(t, fr, newHost(), nil, "disarm", agentA); r.code != 0 || fr.gate.has("02:42:ac:1e:00:02") || fr.nft[tableA] != nil {
		t.Fatalf("re-run: exit %d, stderr %q", r.code, r.err)
	}
}

func TestDisarmReportsAMACThatStaysInTheGateSet(t *testing.T) {
	fr := twoAgents()
	armAll(t, fr, agentA)
	fr.tamper["gatedelete"] = true
	r := run(t, fr, newHost(), nil, "disarm", agentA)
	if r.code != 2 || !strings.Contains(r.err, "DISARM_INCOMPLETE") || !strings.Contains(r.err, "MAC 02:42:ac:1e:00:02 is still in set released of agents_gate") || strings.Contains(r.stdout, "disarmed on") {
		t.Fatalf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.err)
	}
}

// Disarm tolerates a gate table that does not exist (a network that predates the gate) and a container that was
// never armed.
func TestDisarmToleratesAMissingGateOrElement(t *testing.T) {
	t.Run("no gate table", func(t *testing.T) {
		fr := twoAgents()
		armAll(t, fr, agentA)
		fr.gate = nil // the gate was deleted by hand
		fr.calls = nil
		r := run(t, fr, newHost(), nil, "disarm", agentA)
		if r.code != 0 || !strings.Contains(r.stdout, "disarmed") || fr.nft[tableA] != nil || fr.iptTagged(agentA) != 0 {
			t.Fatalf("exit %d, stdout %q, stderr %q\n%s", r.code, r.stdout, r.err, fr.state())
		}
		if fr.has("nft delete element") {
			t.Errorf("there is no element to delete: %v", fr.calls)
		}
	})
	t.Run("no element", func(t *testing.T) {
		fr := twoAgents()
		armAll(t, fr, agentA)
		fr.gate.released = nil // as if the MAC had been removed by hand
		fr.calls = nil
		r := run(t, fr, newHost(), nil, "disarm", agentA)
		if r.code != 0 || fr.nft[tableA] != nil || fr.has("nft delete element") {
			t.Fatalf("exit %d, stderr %q, calls %v", r.code, r.err, fr.calls)
		}
	})
}

func TestDisarmDeletesGateElementThenTableThenIptablesThenFdb(t *testing.T) {
	fr := twoAgents()
	armAll(t, fr, agentA)
	fr.calls = nil
	r := run(t, fr, newHost(), nil, "disarm", agentA)
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	gate, table, ipt, fdb := fr.index("nft delete element netdev agents_gate"), fr.index("nft delete table netdev "+tableA), fr.index("iptables -D"), fr.index("bridge fdb del")
	if !(gate >= 0 && gate < table && table < ipt && ipt < fdb) {
		t.Errorf("order: nft delete element=%d nft delete table=%d iptables -D=%d bridge fdb del=%d", gate, table, ipt, fdb)
	}
	if fr.lastIndex("iptables -D") > fdb {
		t.Error("every iptables rule must be deleted before the fdb entry")
	}
}

func TestDisarmDeletesNeighbourEntryAfterIptablesBeforeFdb(t *testing.T) {
	fr := twoAgents()
	armAll(t, fr, agentA, agentB)
	if fr.neigh["172.30.0.2"] != "02:42:ac:1e:00:02" || fr.neigh["172.30.0.3"] != "02:42:ac:1e:00:03" {
		t.Fatalf("both neighbour entries must exist after arm: %v", fr.neigh)
	}
	fr.calls = nil
	r := run(t, fr, newHost(), nil, "disarm", agentA)
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	if _, there := fr.neigh["172.30.0.2"]; there || fr.neigh["172.30.0.3"] == "" {
		t.Errorf("only the neighbour entry of agent-a may be gone: %v", fr.neigh)
	}
	ipt, neigh, fdb := fr.lastIndex("iptables -D"), fr.index("ip neigh del 172.30.0.2 dev "), fr.index("bridge fdb del")
	if !(ipt >= 0 && ipt < neigh && neigh < fdb) {
		t.Errorf("order: iptables -D=%d ip neigh del=%d bridge fdb del=%d", ipt, neigh, fdb)
	}

	// A second disarm finds no entry: it reads, but does not delete again.
	fr.calls = nil
	r = run(t, fr, newHost(), nil, "disarm", agentA)
	if r.code != 0 || fr.has("ip neigh del") {
		t.Errorf("second disarm: exit %d, stderr %q, calls %v", r.code, r.err, fr.calls)
	}
}

func TestDisarmStoppedContainerFindsTableByNameAndTag(t *testing.T) {
	for name, keep := range map[string]bool{"docker reports no identity": false, "docker reports the identity": true} {
		t.Run(name, func(t *testing.T) {
			fr := twoAgents()
			armAll(t, fr, agentA, agentB)
			fr.ctrs[agentA].pid = 0 // stopped: no veth, no namespace
			fr.ctrs[agentA].keepIdentity = keep
			fr.calls = nil

			r := run(t, fr, newHost(), nil, "disarm", agentA)
			if r.code != 0 || !strings.Contains(r.stdout, "disarmed") {
				t.Fatalf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.err)
			}
			if fr.nft[tableA] != nil || fr.iptTagged(agentA) != 0 {
				t.Errorf("table and rules of the stopped container are left:\n%s", fr.state())
			}
			if len(fr.portRules(tableB)) != 4 || fr.iptTagged(agentB) != 9 {
				t.Errorf("agent-b must stay armed:\n%s", fr.state())
			}
			if fr.has("nsenter") || fr.has("bridge fdb del") {
				t.Errorf("a stopped container has no namespace and no veth: %v", fr.calls)
			}
		})
	}
}

func TestDisarmNothingPresentIsSuccess(t *testing.T) {
	fr := twoAgents() // network and running container, but never armed
	before := fr.state()
	r := run(t, fr, newHost(), nil, "disarm", agentA)
	if r.code != 0 || !strings.Contains(r.stdout, "nothing to disarm") {
		t.Fatalf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.err)
	}
	if fr.mutatedHost() || fr.state() != before {
		t.Errorf("nothing may change:\n%s", fr.state())
	}
	// Running a second time after a real disarm is the same no-op.
	armAll(t, fr, agentA)
	if r := run(t, fr, newHost(), nil, "disarm", agentA); r.code != 0 || !strings.Contains(r.stdout, "disarmed") {
		t.Fatalf("exit %d, stdout %q", r.code, r.stdout)
	}
	if r := run(t, fr, newHost(), nil, "disarm", agentA); r.code != 0 || !strings.Contains(r.stdout, "nothing to disarm") {
		t.Fatalf("second disarm: exit %d, stdout %q, stderr %q", r.code, r.stdout, r.err)
	}
}

func TestDisarmWithoutNetworkIsSuccess(t *testing.T) {
	fr := newRunner() // no agents-net
	r := run(t, fr, newHost(), nil, "disarm", agentA)
	if r.code != 0 || !strings.Contains(r.stdout, "nothing to disarm") {
		t.Fatalf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.err)
	}
	if fr.has("docker container") || fr.has("nft") || fr.mutatedHost() {
		t.Errorf("nothing else may run: %v", fr.calls)
	}
}

// disarm is keyed by the name: a container that does not exist is not an error and nothing of others is touched.
func TestDisarmContainerAbsentNothingToDo(t *testing.T) {
	fr := twoAgents()
	armAll(t, fr, agentA)
	before := fr.state()
	fr.calls = nil
	r := run(t, fr, newHost(), nil, "disarm", "agent-zzz")
	if r.code != 0 || !strings.Contains(r.stdout, "nothing to disarm") {
		t.Fatalf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.err)
	}
	if fr.mutatedHost() || fr.state() != before {
		t.Error("nothing may change")
	}
}

// A container that is gone (no docker answer, no table) still leaves its tagged element in `released`: disarm finds it
// by the comment and removes only that element, never the gate table or the other agent's element.
func TestDisarmGoneContainerRemovesTaggedElementByName(t *testing.T) {
	fr := twoAgents()
	armAll(t, fr, agentA, agentB)
	delete(fr.ctrs, agentA)
	fr.dropMember(agentA)
	delete(fr.nft, tableA) // no table either
	fr.calls = nil
	r := run(t, fr, newHost(), nil, "disarm", agentA)
	if r.code != 0 || !strings.Contains(r.stdout, "disarmed") {
		t.Fatalf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.err)
	}
	if fr.gate == nil || fr.gate.has("02:42:ac:1e:00:02") || !fr.gate.has("02:42:ac:1e:00:03") || fr.gateDeleted != 0 || fr.has("nft delete table netdev agents_gate") {
		t.Errorf("only the element tagged agent-a may go: %+v", fr.gate)
	}
	if fr.iptTagged(agentA) != 0 || fr.iptTagged(agentB) != 9 || fr.nft[tableB] == nil {
		t.Errorf("agent-b must stay armed:\n%s", fr.state())
	}
	if again := run(t, fr, newHost(), nil, "disarm", agentA); again.code != 0 || !strings.Contains(again.stdout, "nothing to disarm") {
		t.Errorf("second run: exit %d, stdout %q", again.code, again.stdout)
	}
}

// A stopped container with no table: the MAC docker reports is removed from the set even without a comment.
func TestDisarmStoppedNoTableRemovesUntaggedElementOfItsMAC(t *testing.T) {
	fr := twoAgents()
	armAll(t, fr, agentA)
	fr.ctrs[agentA].pid = 0
	fr.ctrs[agentA].keepIdentity = true
	delete(fr.nft, tableA)
	fr.gate.comments = nil // as left by an older arm: untagged
	r := run(t, fr, newHost(), nil, "disarm", agentA)
	if r.code != 0 || fr.gate.has("02:42:ac:1e:00:02") {
		t.Fatalf("exit %d, stderr %q, gate %+v", r.code, r.err, fr.gate)
	}
}

func TestReleasedListsElementsWithComments(t *testing.T) {
	fr := twoAgents()
	armAll(t, fr, agentA)
	fr.gate.addElem("02:42:ac:1e:00:77", "") // untagged
	fr.calls = nil
	r := run(t, fr, newHost(), nil, "released")
	want := "02:42:ac:1e:00:02 agent-a\n02:42:ac:1e:00:77 \n"
	if r.code != 0 || r.stdout != want {
		t.Fatalf("exit %d, stdout %q, want %q, stderr %q", r.code, r.stdout, want, r.err)
	}
	if fr.mutatedHost() {
		t.Errorf("released must only read: %v", fr.calls)
	}
	// no gate table: nothing listed, exit 0
	fr.gate = nil
	if r := run(t, fr, newHost(), nil, "released"); r.code != 0 || r.stdout != "" {
		t.Errorf("no gate: exit %d, stdout %q", r.code, r.stdout)
	}
}

func TestEvictRemovesOneElementByMAC(t *testing.T) {
	fr := twoAgents()
	armAll(t, fr, agentA)
	fr.gate.addElem("02:42:ac:1e:00:77", "")
	r := run(t, fr, newHost(), nil, "evict", "02:42:AC:1E:00:77")
	if r.code != 0 || fr.gate.has("02:42:ac:1e:00:77") || !fr.gate.has("02:42:ac:1e:00:02") || fr.gateDeleted != 0 {
		t.Fatalf("exit %d, stderr %q, gate %+v", r.code, r.err, fr.gate)
	}
	// absent: exit 0
	if r := run(t, fr, newHost(), nil, "evict", "02:42:ac:1e:00:77"); r.code != 0 || !strings.Contains(r.stdout, "not in set") {
		t.Errorf("second evict: exit %d, stdout %q", r.code, r.stdout)
	}
	// a delete that does not take effect is EVICT_INCOMPLETE
	fr.gate.addElem("02:42:ac:1e:00:78", "")
	fr.tamper["gatedelete"] = true
	if r := run(t, fr, newHost(), nil, "evict", "02:42:ac:1e:00:78"); r.code != 2 || !strings.Contains(r.err, "EVICT_INCOMPLETE") {
		t.Errorf("tampered evict: exit %d, stderr %q", r.code, r.err)
	}
}

func TestReleasedAndEvictUsage(t *testing.T) {
	for _, args := range [][]string{{"released", "x"}, {"evict"}, {"evict", "not-a-mac"}, {"evict", "02:42:ac:1e:00:77", "x"}} {
		fr := newRunner()
		r := run(t, fr, newHost(), nil, args...)
		if r.code != 1 || !strings.Contains(r.err, "create_agent_net evict <mac>") || len(fr.calls) != 0 {
			t.Errorf("args %q: exit %d, stderr %q, calls %v", args, r.code, r.err, fr.calls)
		}
	}
	h := newHost()
	h.euid = 1000
	if r := run(t, twoAgents(), h, nil, "released"); r.code != 1 || !strings.Contains(r.err, "not running as root") {
		t.Errorf("released without privilege: exit %d, stderr %q", r.code, r.err)
	}
}

// arm starts from empty state: leftovers of an earlier run of that name (tagged element, table, rules) are gone first.
func TestArmCleansLeftoversOfTheNameFirst(t *testing.T) {
	fr := twoAgents()
	armAll(t, fr, agentA)
	fr.gate.addElem("02:42:ac:1e:00:99", agentA) // an orphan element tagged with the name
	fr.calls = nil
	if r := run(t, fr, newHost(), nil, "arm", agentA); r.code != 0 {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.gate.has("02:42:ac:1e:00:99") || !fr.gate.has("02:42:ac:1e:00:02") || fr.gate.comments["02:42:ac:1e:00:02"] != agentA {
		t.Errorf("gate after re-arm: %+v", fr.gate)
	}
	del, load := fr.index("nft delete element netdev agents_gate released { 02:42:ac:1e:00:99 }"), fr.index(nftScript)
	if !(del >= 0 && del < load) {
		t.Errorf("the clean slate must come before the port table is loaded: delete=%d load=%d", del, load)
	}
	if fr.iptTagged(agentA) != 9 || len(fr.portRules(tableA)) != 4 {
		t.Errorf("agent-a must be armed again:\n%s", fr.state())
	}
}

func TestDisarmPrivilegeFailureExit1(t *testing.T) {
	fr, h := twoAgents(), newHost()
	h.euid = 1000
	r := run(t, fr, h, nil, "disarm", agentA)
	if r.code != 1 || !strings.Contains(r.err, "not running as root") || !strings.Contains(r.err, "NOT_DISARMED") || len(fr.calls) != 0 {
		t.Fatalf("exit %d, stderr %q, calls %v", r.code, r.err, fr.calls)
	}
}

func TestDisarmIncompleteExit2(t *testing.T) {
	t.Run("iptables delete fails: the table is already gone", func(t *testing.T) {
		fr := twoAgents()
		armAll(t, fr, agentA)
		fr.rules = []rule{{contains: "iptables -D INPUT", stderr: "iptables: boom", code: 1}}
		r := run(t, fr, newHost(), nil, "disarm", agentA)
		if r.code != 2 || !strings.Contains(r.err, "iptables: boom") || !strings.Contains(r.err, "DISARM_INCOMPLETE") {
			t.Fatalf("exit %d, stderr %q", r.code, r.err)
		}
		if fr.nft[tableA] != nil {
			t.Errorf("the table is the first deletion and must be gone:\n%s", fr.state())
		}
		fr.rules = nil
		if r := run(t, fr, newHost(), nil, "disarm", agentA); r.code != 0 || fr.iptTagged(agentA) != 0 {
			t.Fatalf("re-run: exit %d, stderr %q", r.code, r.err)
		}
	})
	t.Run("table delete fails: rules and entry stay", func(t *testing.T) {
		fr := twoAgents()
		armAll(t, fr, agentA)
		fr.rules = []rule{{contains: "nft delete table", stderr: "nft: boom", code: 1}}
		r := run(t, fr, newHost(), nil, "disarm", agentA)
		if r.code != 2 || !strings.Contains(r.err, "nft: boom") || !strings.Contains(r.err, "DISARM_INCOMPLETE") {
			t.Fatalf("exit %d, stderr %q", r.code, r.err)
		}
		if fr.nft[tableA] == nil || fr.iptTagged(agentA) != 9 || !fr.ctrs[agentA].fdb {
			t.Errorf("nothing after the table may be deleted:\n%s", fr.state())
		}
		fr.rules = nil
		if r := run(t, fr, newHost(), nil, "disarm", agentA); r.code != 0 || fr.nft[tableA] != nil || fr.iptTagged(agentA) != 0 {
			t.Fatalf("re-run: exit %d, stderr %q", r.code, r.err)
		}
	})
}

// ---- remove_agent_net ----

func TestRemoveRejectsArguments(t *testing.T) {
	for _, args := range [][]string{{"x"}, {"--force"}, {"agent-a"}, {""}, {"a", "b"}} {
		fr := twoAgents()
		r := runRemove(t, fr, args...)
		if r.code != 1 || !strings.Contains(r.err, "usage: remove_agent_net") || !strings.Contains(r.err, "NOT_REMOVED") {
			t.Errorf("args %q: exit %d, stderr %q", args, r.code, r.err)
		}
		if len(fr.calls) != 0 {
			t.Errorf("args %q: nothing may be called, got %v", args, fr.calls)
		}
	}
}

func TestRemovePrivilegeFailureExit1(t *testing.T) {
	fr := twoAgents()
	h := newHost()
	h.euid = 1000
	cfg := config.Load(func(string) string { return "" })
	var out, errb bytes.Buffer
	code := sequence.RunRemove(context.Background(), sequence.Deps{Config: cfg, Runner: fr, Host: h, Stdout: &out, Stderr: &errb})
	if code != 1 || !strings.Contains(errb.String(), "remove_agent_net: step 2") || !strings.Contains(errb.String(), "NOT_REMOVED") || len(fr.calls) != 0 {
		t.Fatalf("exit %d, stderr %q, calls %v", code, errb.String(), fr.calls)
	}
}

func TestRemoveHappyPathContainersBeforeLeftoverTables(t *testing.T) {
	fr := twoAgents()
	fr.nft["keepme"] = ghostTable() // not an as_port_* table: must stay
	fr.ipt["INPUT"] = []string{"-s 10.9.9.9/32 -j ACCEPT"}
	fr.ipt["DOCKER-USER"] = []string{"-j RETURN"}
	armAll(t, fr, agentA, agentB)
	fr.nft["as_port_ghost"] = ghostTable() // left by a container removed by other means
	fr.calls = nil

	r := runRemove(t, fr)
	if r.code != 0 || r.err != "" {
		t.Fatalf("exit %d, stderr:\n%s", r.code, r.err)
	}
	for _, want := range []string{
		"container agent-a removed from agents-net (its workspace is kept)",
		"container agent-b removed from agents-net (its workspace is kept)",
		"2 agent container(s) removed (workspaces kept)",
		"[1/13] validate invocation",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, r.stdout)
		}
	}
	if len(fr.ctrs) != 0 || fr.agentsNetExists() {
		t.Errorf("containers and network must be gone:\n%s", fr.state())
	}
	if !reflectEq(fr.tableNames(), []string{"keepme"}) {
		t.Errorf("only the as_port_* tables may go: %v", fr.tableNames())
	}
	if !reflectEq(fr.ipt["INPUT"], []string{"-s 10.9.9.9/32 -j ACCEPT"}) || !reflectEq(fr.ipt["DOCKER-USER"], []string{"-j RETURN"}) {
		t.Errorf("rules of others must stay: %v", fr.ipt)
	}
	if !reflectEq(fr.deleted, []string{tableA, tableB, "as_port_ghost"}) {
		t.Errorf("deleted tables = %v", fr.deleted)
	}

	// Order: every container (disarmed, then removed) before any leftover table deletion, the network last.
	lastRm, ghost, rmNet := fr.lastIndex("docker rm -f"), fr.index("nft delete table netdev as_port_ghost"), fr.index("docker network rm agents-net")
	if !(lastRm >= 0 && lastRm < ghost && ghost < rmNet) {
		t.Errorf("last docker rm=%d leftover table deletion=%d network rm=%d", lastRm, ghost, rmNet)
	}
	// The gate table goes after the containers and before the network; its DOCKER-USER rule is deleted with the other
	// agent-net: rules; nothing of the gate is left.
	gate := fr.index("nft delete table netdev agents_gate")
	if !(lastRm < gate && gate < rmNet) || fr.gate != nil || fr.gateDeleted != 1 {
		t.Errorf("last docker rm=%d gate deletion=%d network rm=%d gate=%+v", lastRm, gate, rmNet, fr.gate)
	}
	for _, rules := range fr.ipt {
		for _, r := range rules {
			if strings.Contains(r, "agent-net:gate") {
				t.Errorf("the gate rule must be deleted: %s", r)
			}
		}
	}
	for _, c := range []struct{ name, table string }{{agentA, tableA}, {agentB, tableB}} {
		rm, del := fr.index("docker rm -f "+c.name), fr.index("nft delete table netdev "+c.table)
		if fr.count("docker rm -f "+c.name) != 1 || del < 0 || del > rm {
			t.Errorf("%s: the port table (%d) must be deleted before the container (%d)", c.name, del, rm)
		}
	}
	// Workspaces and the engine are not touched.
	for _, c := range fr.calls {
		j := c.joined()
		for _, bad := range []string{"docker stop", "docker kill", "docker volume", "docker restart", "daemon.json", "systemctl", " rm -rf", "bridge link set"} {
			if strings.Contains(j, bad) {
				t.Errorf("forbidden call: %s", j)
			}
		}
	}
	// Rerun: nothing is left, nothing to do.
	fr.calls = nil
	if r := runRemove(t, fr); r.code != 0 || !strings.Contains(r.stdout, "nothing to remove") {
		t.Fatalf("second run: exit %d, stdout %q, stderr %q", r.code, r.stdout, r.err)
	}
}

// Containers are attached to agents-net at `docker create`: a created but never armed container is listed by
// `docker ps --filter network=agents-net`, disarmed (nothing to remove from the gate) and removed.
func TestRemoveRemovesCreatedButUnarmedContainersAndTheGate(t *testing.T) {
	fr := twoAgents() // both attached and running, none armed
	r := runRemove(t, fr)
	if r.code != 0 || !strings.Contains(r.stdout, "2 agent container(s) removed (workspaces kept)") {
		t.Fatalf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.err)
	}
	if len(fr.ctrs) != 0 || fr.agentsNetExists() || fr.gate != nil || fr.gateDeleted != 1 {
		t.Errorf("containers, network and gate must be gone:\n%s", fr.state())
	}
	if fr.has("nft delete element") {
		t.Errorf("an unarmed container has no MAC in the set: %v", fr.calls)
	}
	if fr.index("docker rm -f") > fr.index("nft delete table netdev agents_gate") {
		t.Error("the gate table is deleted after the containers")
	}
}

func TestRemoveDeletesTheGateWhenOnlyTheGateIsLeft(t *testing.T) {
	fr := newRunner() // no network and no container, but the gate table and its rule are left
	fr.gate = newGate(bridgeName)
	fr.gateInit = true
	fr.ipt["DOCKER-USER"] = []string{gateIptRule(bridgeName)}
	r := runRemove(t, fr)
	if r.code != 0 || strings.Contains(r.stdout, "nothing to remove") || fr.gate != nil || len(fr.ipt["DOCKER-USER"]) != 0 {
		t.Fatalf("exit %d, stdout %q, stderr %q\n%s", r.code, r.stdout, r.err, fr.state())
	}
}

func TestRemoveReportsAGateThatCannotBeDeleted(t *testing.T) {
	fr := twoAgents()
	fr.rules = []rule{{contains: "nft delete table netdev agents_gate", stderr: "nft: boom", code: 1}}
	r := runRemove(t, fr)
	if r.code != 2 || !strings.Contains(r.err, "REMOVE_INCOMPLETE") || !strings.Contains(r.err, "table netdev agents_gate could not be deleted") || !strings.Contains(r.err, "nft: boom") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.agentsNetExists() || fr.gate == nil {
		t.Errorf("the network is removed anyway, the gate is left:\n%s", fr.state())
	}
	fr.rules = nil
	if r := runRemove(t, fr); r.code != 0 || fr.gate != nil {
		t.Fatalf("re-run: exit %d, stderr %q", r.code, r.err)
	}
}

func TestRemoveRefusesNonAgentContainerAndChangesNothing(t *testing.T) {
	for _, foreign := range []string{"web-1", "agent_x", "agent-", "Agent-a", "agent-1x"} {
		t.Run(foreign, func(t *testing.T) {
			fr := twoAgents()
			armAll(t, fr, agentA, agentB)
			fr.ctrs[foreign] = &fakeCtr{ip: "172.30.0.9", mac: "02:42:ac:1e:00:09", veth: "vethx", peer: 9, pid: 4399}
			fr.members = append(fr.members, foreign)
			before := fr.state()
			fr.calls = nil

			r := runRemove(t, fr)
			if r.code != 1 || !strings.Contains(r.err, foreign) || !strings.Contains(r.err, "NOT_REMOVED") || !strings.Contains(r.err, "nothing was changed") {
				t.Fatalf("exit %d, stderr %q", r.code, r.err)
			}
			if fr.state() != before || fr.mutatedHost() || fr.has("docker rm") || fr.has("docker network rm") {
				t.Errorf("nothing may change:\n%s", fr.state())
			}
		})
	}
}

func TestRemoveStoppedAgentContainerIsRemoved(t *testing.T) {
	fr := twoAgents()
	armAll(t, fr, agentA, agentB)
	fr.ctrs[agentB].pid = 0
	if r := runRemove(t, fr); r.code != 0 {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if len(fr.ctrs) != 0 || fr.agentsNetExists() || len(fr.tableNames()) != 0 {
		t.Errorf("everything must be gone:\n%s", fr.state())
	}
}

func TestRemoveNothingToRemoveExit0(t *testing.T) {
	fr := newRunner()
	r := runRemove(t, fr)
	if r.code != 0 || r.err != "" || !strings.Contains(r.stdout, "nothing to remove") {
		t.Fatalf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.err)
	}
	for _, p := range []string{"docker network rm", "docker rm", "docker ps"} {
		if fr.has(p) {
			t.Errorf("%s must not run", p)
		}
	}
	if fr.mutatedHost() {
		t.Errorf("nothing may be changed: %v", fr.calls)
	}
}

func TestRemoveStopsWhenStateUnreadable(t *testing.T) {
	for name, contains := range map[string]string{"nft": "nft list tables", "iptables": "iptables -S INPUT"} {
		t.Run(name, func(t *testing.T) {
			fr := twoAgents()
			armAll(t, fr, agentA)
			before := fr.state()
			fr.calls = nil
			fr.rules = []rule{{contains: contains, stderr: "boom", code: 1}}
			r := runRemove(t, fr)
			if r.code != 1 || !strings.Contains(r.err, "NOT_REMOVED") || !strings.Contains(r.err, "boom") {
				t.Fatalf("exit %d, stderr %q", r.code, r.err)
			}
			if fr.state() != before || fr.mutatedHost() || fr.has("docker rm") {
				t.Error("nothing may change when the state cannot be read")
			}
		})
	}
}

func TestRemoveFinishesLeftoversWithoutNetwork(t *testing.T) {
	fr := newRunner() // no network, but a port table and tagged rules are left
	fr.nft["as_port_ghost"] = ghostTable()
	fr.ipt["INPUT"] = []string{"-s 172.30.0.9/32 -m comment --comment agent-net:ghost -j DROP", "-s 1.1.1.1/32 -j ACCEPT"}
	fr.ipt["DOCKER-USER"] = []string{"-s 172.30.0.9/32 -m comment --comment agent-net:ghost -j DROP"}
	r := runRemove(t, fr)
	if r.code != 0 {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if len(fr.tableNames()) != 0 {
		t.Errorf("the leftover table must be gone:\n%s", fr.state())
	}
	if !reflectEq(fr.ipt["INPUT"], []string{"-s 1.1.1.1/32 -j ACCEPT"}) || len(fr.ipt["DOCKER-USER"]) != 0 {
		t.Errorf("only the tagged rules may go: %v", fr.ipt)
	}
	if fr.has("docker network rm") {
		t.Error("there is no network to remove")
	}
}

func TestRemovePartialFailureThenRerunFinishes(t *testing.T) {
	t.Run("container removal fails: the tables of the rest stay", func(t *testing.T) {
		fr := twoAgents()
		armAll(t, fr, agentA, agentB)
		fr.nft["as_port_ghost"] = ghostTable()
		fr.rules = []rule{{contains: "docker rm -f agent-b", stderr: "boom", code: 1}}
		r := runRemove(t, fr)
		if r.code != 2 || !strings.Contains(r.err, "REMOVE_INCOMPLETE") || !strings.Contains(r.err, "agent-b") || !strings.Contains(r.err, "boom") {
			t.Fatalf("exit %d, stderr %q", r.code, r.err)
		}
		if fr.ctrs[agentA] != nil || fr.ctrs[agentB] == nil {
			t.Errorf("agent-a must be removed, agent-b left:\n%s", fr.state())
		}
		if fr.nft["as_port_ghost"] == nil || fr.has("docker network rm") || fr.has("nft delete table netdev as_port_ghost") {
			t.Errorf("leftover tables and the network stay while a container is left: %v", fr.calls)
		}
		if strings.Contains(r.stdout, "agent network agents-net removed") {
			t.Errorf("no success text: %q", r.stdout)
		}
		fr.rules = nil
		if r := runRemove(t, fr); r.code != 0 {
			t.Fatalf("re-run: exit %d, stderr %q", r.code, r.err)
		}
		if len(fr.ctrs) != 0 || fr.agentsNetExists() || len(fr.tableNames()) != 0 {
			t.Errorf("the re-run must finish the job:\n%s", fr.state())
		}
	})

	t.Run("leftover table deletion fails: the remaining steps still run", func(t *testing.T) {
		fr := twoAgents()
		armAll(t, fr, agentA)
		fr.nft["as_port_ghost"] = ghostTable()
		fr.ipt["INPUT"] = append(fr.ipt["INPUT"], "-s 172.30.0.9/32 -m comment --comment agent-net:ghost -j DROP")
		fr.rules = []rule{{contains: "nft delete table netdev as_port_ghost", stderr: "nft: boom", code: 1}}
		r := runRemove(t, fr)
		if r.code != 2 || !strings.Contains(r.err, "REMOVE_INCOMPLETE") || !strings.Contains(r.err, "nft: boom") || !strings.Contains(r.err, "as_port_ghost") {
			t.Fatalf("exit %d, stderr %q", r.code, r.err)
		}
		if fr.agentsNetExists() || fr.iptTagged("ghost") != 0 || fr.nft["as_port_ghost"] == nil {
			t.Errorf("rules and network are removed anyway, the table is left:\n%s", fr.state())
		}
		fr.rules = nil
		if r := runRemove(t, fr); r.code != 0 || len(fr.tableNames()) != 0 {
			t.Fatalf("re-run: exit %d, stderr %q", r.code, r.err)
		}
	})

	t.Run("network removal fails", func(t *testing.T) {
		fr := twoAgents()
		armAll(t, fr, agentA, agentB)
		fr.rules = []rule{{contains: "docker network rm", stderr: "boom", code: 1}}
		r := runRemove(t, fr)
		if r.code != 2 || !strings.Contains(r.err, "REMOVE_INCOMPLETE") || !strings.Contains(r.err, "boom") {
			t.Fatalf("exit %d, stderr %q", r.code, r.err)
		}
		if len(fr.ctrs) != 0 || !fr.agentsNetExists() {
			t.Errorf("containers gone, network left:\n%s", fr.state())
		}
		fr.rules = nil
		if r := runRemove(t, fr); r.code != 0 || fr.agentsNetExists() {
			t.Fatalf("re-run: exit %d, stderr %q", r.code, r.err)
		}
	})

	t.Run("iptables delete fails: the network is still removed", func(t *testing.T) {
		fr := twoAgents()
		fr.members = nil // no container on the network
		fr.ctrs = map[string]*fakeCtr{}
		fr.ipt["INPUT"] = []string{"-s 172.30.0.9/32 -m comment --comment agent-net:ghost -j DROP"}
		fr.rules = []rule{{contains: "iptables -D INPUT", stderr: "boom", code: 1}}
		r := runRemove(t, fr)
		if r.code != 2 || !strings.Contains(r.err, "REMOVE_INCOMPLETE") || !strings.Contains(r.err, "boom") {
			t.Fatalf("exit %d, stderr %q", r.code, r.err)
		}
		if fr.agentsNetExists() {
			t.Error("the later steps must still run")
		}
		fr.rules = nil
		if r := runRemove(t, fr); r.code != 0 || len(fr.ipt["INPUT"]) != 0 {
			t.Fatalf("re-run: exit %d, stderr %q, ipt %v", r.code, r.err, fr.ipt)
		}
	})
}

// ---- absent versus failed in remove and disarm (SFR-NET-013, SFR-NET-018, SFR-NET-023, SSR-NET-003) ----

// dropMember takes a container off the list that `docker ps` answers.
func (f *fakeRunner) dropMember(name string) {
	var keep []string
	for _, m := range f.members {
		if m != name {
			keep = append(keep, m)
		}
	}
	f.members = keep
}

func TestRemoveStopsWhenDockerCannotBeRead(t *testing.T) {
	t.Run("network unreadable: exit 1, nothing changed", func(t *testing.T) {
		fr := twoAgents()
		armAll(t, fr, agentA)
		before := fr.state()
		fr.calls = nil
		fr.netInspectErr = engineDown
		r := runRemove(t, fr)
		if r.code != 1 || !strings.Contains(r.err, "NOT_REMOVED") || !strings.Contains(r.err, "Cannot connect") {
			t.Fatalf("exit %d, stderr %q", r.code, r.err)
		}
		if fr.state() != before || fr.mutatedHost() || fr.has("docker rm") || fr.has("docker network rm") {
			t.Errorf("nothing may change:\n%s", fr.state())
		}
	})

	t.Run("docker ps fails: exit 1, nothing changed", func(t *testing.T) {
		fr := twoAgents()
		armAll(t, fr, agentA, agentB)
		before := fr.state()
		fr.calls = nil
		fr.rules = []rule{{contains: "docker ps", stderr: "permission denied while trying to connect to the Docker daemon socket", code: 1}}
		r := runRemove(t, fr)
		if r.code != 1 || !strings.Contains(r.err, "NOT_REMOVED") || !strings.Contains(r.err, "permission denied") {
			t.Fatalf("exit %d, stderr %q", r.code, r.err)
		}
		if fr.state() != before || fr.mutatedHost() || fr.has("docker rm") || fr.has("docker network rm") {
			t.Errorf("nothing may change:\n%s", fr.state())
		}
	})

	t.Run("container inspect fails after earlier changes: exit 2, no leftover table deleted", func(t *testing.T) {
		fr := twoAgents()
		armAll(t, fr, agentA, agentB)
		fr.nft["as_port_ghost"] = ghostTable()
		fr.ctrInspectErr = map[string]string{agentB: "permission denied while trying to connect to the Docker daemon socket"}
		fr.calls = nil
		r := runRemove(t, fr)
		if r.code != 2 || !strings.Contains(r.err, "REMOVE_INCOMPLETE") || !strings.Contains(r.err, agentB) || !strings.Contains(r.err, "permission denied") {
			t.Fatalf("exit %d, stderr %q", r.code, r.err)
		}
		if fr.ctrs[agentA] != nil || fr.ctrs[agentB] == nil {
			t.Errorf("agent-a is removed, agent-b stays:\n%s", fr.state())
		}
		if !reflectEq(fr.deleted, []string{tableA}) || fr.nft[tableB] == nil || fr.nft["as_port_ghost"] == nil || fr.iptTagged(agentB) != 9 {
			t.Errorf("only the table of agent-a may be gone (deleted %v):\n%s", fr.deleted, fr.state())
		}
		if fr.has("docker network rm") || strings.Contains(r.stdout, "agent network agents-net removed") {
			t.Error("the network stays and there is no success text")
		}
		fr.ctrInspectErr = nil
		if r := runRemove(t, fr); r.code != 0 || len(fr.ctrs) != 0 || fr.agentsNetExists() || len(fr.tableNames()) != 0 {
			t.Fatalf("re-run: exit %d, stderr %q\n%s", r.code, r.err, fr.state())
		}
	})

	t.Run("network unreadable in the verification: exit 2", func(t *testing.T) {
		fr := twoAgents()
		armAll(t, fr, agentA, agentB)
		fr.afterCall = func(j string) {
			if j == "docker network rm agents-net" {
				fr.netInspectErr = engineDown
			}
		}
		r := runRemove(t, fr)
		if r.code != 2 || !strings.Contains(r.err, "REMOVE_INCOMPLETE") || !strings.Contains(r.err, "cannot inspect network agents-net") || !strings.Contains(r.err, "Cannot connect") {
			t.Fatalf("exit %d, stderr %q", r.code, r.err)
		}
		if strings.Contains(r.stdout, "agent network agents-net removed") {
			t.Errorf("no success text: %q", r.stdout)
		}
	})
}

func TestDisarmStopsWhenDockerCannotBeRead(t *testing.T) {
	for name, set := range map[string]func(*fakeRunner){
		"network":   func(f *fakeRunner) { f.netInspectErr = engineDown },
		"container": func(f *fakeRunner) { f.ctrInspectErr = map[string]string{"*": engineDown} },
	} {
		t.Run(name, func(t *testing.T) {
			fr := twoAgents()
			armAll(t, fr, agentA)
			before := fr.state()
			fr.calls = nil
			set(fr)
			r := run(t, fr, newHost(), nil, "disarm", agentA)
			if r.code != 1 || !strings.Contains(r.err, "NOT_DISARMED") || !strings.Contains(r.err, "Cannot connect") || strings.Contains(r.stdout, "nothing to disarm") {
				t.Fatalf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.err)
			}
			if fr.mutatedHost() || fr.state() != before {
				t.Errorf("nothing may change:\n%s", fr.state())
			}
		})
	}
}

func TestRemoveCountsOnlyContainersItRemoved(t *testing.T) {
	t.Run("gone before the inspect", func(t *testing.T) {
		fr := twoAgents()
		armAll(t, fr, agentA, agentB)
		fr.afterCall = func(j string) {
			if strings.HasPrefix(j, "docker ps -a") { // agent-b is removed by someone else right after the listing
				delete(fr.ctrs, agentB)
				fr.dropMember(agentB)
			}
		}
		r := runRemove(t, fr)
		if r.code != 0 || !strings.Contains(r.stdout, "1 agent container(s) removed (workspaces kept)") || strings.Contains(r.stdout, "2 agent container(s)") {
			t.Fatalf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.err)
		}
		if fr.agentsNetExists() || len(fr.tableNames()) != 0 || fr.iptTagged(agentB) != 0 {
			t.Errorf("everything must be gone:\n%s", fr.state())
		}
	})

	t.Run("docker rm answers not found", func(t *testing.T) {
		fr := twoAgents()
		armAll(t, fr, agentA, agentB)
		fr.afterCall = func(j string) {
			if strings.HasPrefix(j, "docker container inspect --format {{json .}} "+agentB) {
				delete(fr.ctrs, agentB) // gone between the inspect and `docker rm -f`
				fr.dropMember(agentB)
			}
		}
		r := runRemove(t, fr)
		if r.code != 0 || !strings.Contains(r.stdout, "container agent-b was already gone") || !strings.Contains(r.stdout, "1 agent container(s) removed (workspaces kept)") {
			t.Fatalf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.err)
		}
		if fr.count("docker rm -f agent-b") != 1 || fr.agentsNetExists() || len(fr.tableNames()) != 0 {
			t.Errorf("rm calls %d:\n%s", fr.count("docker rm -f agent-b"), fr.state())
		}
	})

	t.Run("all removed: both counted", func(t *testing.T) {
		fr := twoAgents()
		armAll(t, fr, agentA, agentB)
		if r := runRemove(t, fr); r.code != 0 || !strings.Contains(r.stdout, "2 agent container(s) removed (workspaces kept)") {
			t.Fatalf("exit %d, stdout %q", r.code, r.stdout)
		}
	})
}

func TestRemoveUnpausesPausedContainerBeforeRm(t *testing.T) {
	fr := twoAgents()
	armAll(t, fr, agentA, agentB)
	fr.ctrs[agentB].paused = true
	if r := runRemove(t, fr); r.code != 0 || !strings.Contains(r.stdout, "2 agent container(s) removed (workspaces kept)") {
		t.Fatalf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.err)
	}
	inspect, unpause, rm := fr.index("docker inspect -f {{.State.Paused}} "+agentB), fr.index("docker unpause "+agentB), fr.index("docker rm -f "+agentB)
	if !(inspect >= 0 && inspect < unpause && unpause < rm) {
		t.Errorf("order inspect=%d unpause=%d rm=%d: %v", inspect, unpause, rm, fr.calls)
	}
	if fr.count("docker unpause") != 1 {
		t.Errorf("only the paused container is unpaused: %v", fr.calls)
	}
	if fr.has("docker network connect") || fr.has("docker network disconnect") {
		t.Errorf("network connect/disconnect are forbidden: %v", fr.calls)
	}
}

func TestRemoveUnpauseFailureFailsTheRemoval(t *testing.T) {
	fr := twoAgents()
	armAll(t, fr, agentA, agentB)
	fr.ctrs[agentB].paused = true
	fr.rules = []rule{{contains: "docker unpause agent-b", stderr: "cannot unpause", code: 1}}
	r := runRemove(t, fr)
	if r.code != 2 || !strings.Contains(r.err, "unpause before remove") || !strings.Contains(r.err, "cannot unpause") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.count("docker rm -f agent-b") != 0 || fr.ctrs[agentB] == nil {
		t.Errorf("agent-b must be left, no rm after a failed unpause:\n%s", fr.state())
	}
}

func TestRemoveTreatsAMissingIptablesChainAsEmpty(t *testing.T) {
	t.Run("network left", func(t *testing.T) {
		fr := twoAgents()
		fr.members = nil
		fr.ctrs = map[string]*fakeCtr{}
		fr.noChain = map[string]bool{"DOCKER-USER": true}
		r := runRemove(t, fr)
		if r.code != 0 || !strings.Contains(r.stdout, "agent network agents-net removed") || fr.agentsNetExists() {
			t.Fatalf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.err)
		}
	})
	t.Run("nothing left", func(t *testing.T) {
		fr := newRunner()
		fr.noChain = map[string]bool{"DOCKER-USER": true}
		r := runRemove(t, fr)
		if r.code != 0 || !strings.Contains(r.stdout, "nothing to remove") {
			t.Fatalf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.err)
		}
	})
}

// ---- neighbour entry and stopped container (SFR-NET-012, SSR-NET-004) ----

func TestDisarmLeavesTheNeighbourEntryOfAnotherContainer(t *testing.T) {
	fr := twoAgents()
	armAll(t, fr, agentA)
	const foreign = "02:42:ac:1e:00:99"
	fr.ctrs["agent-c"] = &fakeCtr{ip: "172.30.0.9", mac: foreign, veth: "vethc", peer: 9, pid: 4303}
	fr.members = append(fr.members, "agent-c") // a live endpoint owns the MAC
	fr.neigh["172.30.0.2"] = foreign           // the entry for the IP of agent-a now belongs to another MAC
	fr.calls = nil
	r := run(t, fr, newHost(), nil, "disarm", agentA)
	if r.code != 0 {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.neigh["172.30.0.2"] != foreign || fr.has("ip neigh del") {
		t.Errorf("an entry with another lladdr must stay: %v, calls %v", fr.neigh, fr.calls)
	}
	if fr.nft[tableA] != nil || fr.iptTagged(agentA) != 0 {
		t.Errorf("the rest of the disarm must still happen:\n%s", fr.state())
	}
}

// A PERMANENT entry of the IP whose lladdr belongs to no live endpoint is a leftover of an earlier, uncleanly disarmed
// container: disarm deletes it.
func TestDisarmDeletesAForeignNeighbourEntryOfNoLiveEndpoint(t *testing.T) {
	fr := twoAgents()
	armAll(t, fr, agentA)
	fr.neigh["172.30.0.2"] = "82:11:0a:30:91:e5" // stale MAC, no container has it
	fr.calls = nil
	r := run(t, fr, newHost(), nil, "disarm", agentA)
	if r.code != 0 {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if _, there := fr.neigh["172.30.0.2"]; there || !fr.has("ip neigh del 172.30.0.2 dev ") {
		t.Errorf("the stale entry must be deleted: %v, calls %v", fr.neigh, fr.calls)
	}
}

func TestDisarmVerifyFailsOnALeftoverForeignNeighbourEntry(t *testing.T) {
	fr := twoAgents()
	armAll(t, fr, agentA)
	fr.neigh["172.30.0.2"] = "82:11:0a:30:91:e5"
	fr.afterCall = func(j string) { // the delete succeeds but the entry is back

		if strings.HasPrefix(j, "ip neigh del") {
			fr.neigh["172.30.0.2"] = "82:11:0a:30:91:e5"
		}
	}
	r := run(t, fr, newHost(), nil, "disarm", agentA)
	if r.code != 2 || !strings.Contains(r.err, "DISARM_INCOMPLETE") || !strings.Contains(r.err, "172.30.0.2 (lladdr 82:11:0a:30:91:e5)") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
}

// ---- arm and a stale neighbour entry ----

func TestArmDeletesAStaleNeighbourEntryAndProceeds(t *testing.T) {
	fr := twoAgents()
	fr.neigh = map[string]string{"172.30.0.2": "82:11:0a:30:91:e5"} // leftover, no container has this MAC
	fr.calls = nil
	r := run(t, fr, newHost(), nil, "arm", agentA)
	if r.code != 0 {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	del, replace := fr.index("ip neigh del 172.30.0.2 dev "), fr.index("ip neigh replace 172.30.0.2 lladdr 02:42:ac:1e:00:02")
	if !(del >= 0 && del < replace) || fr.neigh["172.30.0.2"] != "02:42:ac:1e:00:02" {
		t.Errorf("del=%d replace=%d neigh=%v", del, replace, fr.neigh)
	}
}

func TestArmFailsWhenTheNeighbourEntryBelongsToAnotherLiveEndpoint(t *testing.T) {
	fr := twoAgents()
	fr.neigh = map[string]string{"172.30.0.2": "02:42:ac:1e:00:03"} // the MAC of agent-b
	fr.calls = nil
	r := run(t, fr, newHost(), nil, "arm", agentA)
	if r.code != 2 || !strings.Contains(r.err, "failed control: neighbour entry: ") || !strings.Contains(r.err, "conflict") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.neigh["172.30.0.2"] != "02:42:ac:1e:00:03" || fr.has("ip neigh del") || fr.has("bridge link set") || fr.has("iptables -I") {
		t.Errorf("nothing may be changed: %v, calls %v", fr.neigh, fr.calls)
	}
}

func TestDisarmStoppedContainerRecoversMACAndVethFromItsTable(t *testing.T) {
	fr := twoAgents()
	armAll(t, fr, agentA, agentB)
	fr.ctrs[agentA].pid = 0 // stopped: docker reports no MAC, but the veth is still on the host
	fr.ctrs[agentA].vethStays = true
	fr.calls = nil
	r := run(t, fr, newHost(), nil, "disarm", agentA)
	if r.code != 0 {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if !fr.has("bridge fdb del 02:42:ac:1e:00:02 dev vetha master static") || fr.ctrs[agentA].fdb {
		t.Errorf("the static fdb entry on the veth must be deleted: %v", fr.calls)
	}
	if _, there := fr.neigh["172.30.0.2"]; there || fr.neigh["172.30.0.3"] == "" {
		t.Errorf("only the neighbour entry of agent-a may be gone: %v", fr.neigh)
	}
	if fr.nft[tableA] != nil || fr.iptTagged(agentA) != 0 || !fr.ctrs[agentB].fdb || len(fr.portRules(tableB)) != 4 {
		t.Errorf("agent-a is disarmed, agent-b stays armed:\n%s", fr.state())
	}
}

// The kernel creates two static entries (with "vlan":1 and without) from one `fdb replace`; arm must verify with that
// output and disarm must delete each by its own (mac, vlan).
func TestArmVerifiesAndDisarmDeletesBothVlanFdbEntries(t *testing.T) {
	fr := twoAgents()
	armAll(t, fr, agentA)
	if out, _, _, _ := fr.Run(context.Background(), nil, "bridge", "-j", "fdb", "show", "dev", "vetha"); strings.Count(string(out), `"mac":"02:42:ac:1e:00:02"`) != 2 ||
		!strings.Contains(string(out), `"vlan":1`) {
		t.Fatalf("the fake must show both entries of the pinned MAC: %s", out)
	}
	fr.calls = nil
	r := run(t, fr, newHost(), nil, "disarm", agentA)
	if r.code != 0 {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.count("bridge fdb del ") != 2 ||
		!fr.has("bridge fdb del 02:42:ac:1e:00:02 dev vetha vlan 1 master static") ||
		!fr.has("bridge fdb del 02:42:ac:1e:00:02 dev vetha master static") || fr.ctrs[agentA].fdb {
		t.Errorf("both static entries must be deleted, one per vlan: %v", fr.calls)
	}
}

// ---- port mark (gap 1b): the port table sets one mark bit, the gate ingress rule requires and clears it ----

func TestPortAcceptRulesSetTheMarkBitAndTheGateRequiresIt(t *testing.T) {
	fr := armedRunner(t)
	for _, r := range fr.nft[tableName].rules[config.ChainIngress] {
		if !strings.Contains(r, portMarkSet+" accept") {
			t.Errorf("ingress accept rule without the mark: %s", r)
		}
	}
	for _, r := range fr.nft[tableName].rules[config.ChainEgress] {
		if strings.Contains(r, "mark") {
			t.Errorf("the egress chain stays unchanged: %s", r)
		}
	}
	if got := fr.gate.rules[config.ChainIngress]; len(got) != 1 || got[0] != gateIngressRule {
		t.Errorf("gate ingress = %v", got)
	}
	if got := fr.gate.rules[config.ChainEgress]; len(got) != 1 || got[0] != "ether daddr @released accept" {
		t.Errorf("gate egress = %v", got)
	}
}

// nft receives `and`/`or` and lists `&`/`|`, with `ether daddr` before `ether saddr` (real nft 1.0.9 output): the
// read-back must accept the listed form. check-gate on an armed port is all ok only if it does.
func TestReadBackAcceptsTheListedFormOfTheMarkRules(t *testing.T) {
	fr := armedRunner(t)
	out, _, _, _ := fr.Run(context.Background(), nil, "nft", "list", "table", "netdev", tableName)
	gate, _, _, _ := fr.Run(context.Background(), nil, "nft", "list", "table", "netdev", "agents_gate")
	for _, want := range []string{"ether daddr " + bridgeMAC + " ether saddr " + agentMAC + " ip saddr " + agentIP + " meta mark set meta mark | 0x10000000 accept"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("port listing lacks %q:\n%s", want, out)
		}
	}
	if !strings.Contains(string(gate), "ether saddr @released meta mark & 0x10000000 == 0x10000000 meta mark set meta mark & 0xefffffff accept") {
		t.Errorf("gate listing:\n%s", gate)
	}
	for _, l := range []string{string(out), string(gate)} {
		if strings.Contains(l, " meta mark and ") || strings.Contains(l, " meta mark or ") {
			t.Errorf("nft lists & and |, not and/or:\n%s", l)
		}
	}
	if r := run(t, fr, newHost(), nil, checkArgs...); r.code != 0 || !reflectEq(checkLines(r.stdout), allOKLines("released")) {
		t.Fatalf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.err)
	}
}

// A gate without the mark condition, or without the clearing of the bit, is a gate mismatch: arm refuses (fail closed),
// check-gate reports it.
func TestGateWithoutTheMarkRuleIsAMismatch(t *testing.T) {
	for name, rule := range map[string]string{
		"old rule without mark":     "ether saddr @released accept",
		"mark required, not cleared": "ether saddr @released meta mark and 0x10000000 == 0x10000000 accept",
		"other bit":                 "ether saddr @released meta mark and 0x20000000 == 0x20000000 meta mark set meta mark and 0xdfffffff accept",
	} {
		t.Run(name, func(t *testing.T) {
			fr := armedRunner(t)
			fr.gate.rules[config.ChainIngress] = []string{rule}
			r := run(t, fr, newHost(), nil, checkArgs...)
			if r.code != 2 || !strings.Contains(r.stdout, "agents-net gate: mismatch\n") {
				t.Fatalf("check-gate: exit %d, stdout %q", r.code, r.stdout)
			}
			fr = newRunner()
			fr.netExists = true
			fr.ensureGate()
			fr.gate.rules[config.ChainIngress] = []string{rule}
			ar := run(t, fr, newHost(), nil, armArgs...)
			if ar.code != 1 || !strings.Contains(ar.err, "GATED_UNARMED") || !strings.Contains(ar.err, "no intact gate") || fr.has(nftScript) {
				t.Fatalf("arm: exit %d, stderr %q", ar.code, ar.err)
			}
		})
	}
}

// A port ingress accept rule without the mark statement is a mismatch of the accept rules.
func TestCheckGateReportsAPortRuleWithoutTheMark(t *testing.T) {
	fr := armedRunner(t)
	tbl := fr.nft[tableName]
	for i, r := range tbl.rules[config.ChainIngress] {
		tbl.rules[config.ChainIngress][i] = strings.Replace(r, portMarkSet+" ", "", 1)
	}
	r := run(t, fr, newHost(), nil, checkArgs...)
	if r.code != 2 || !reflectEq(checkLines(r.stdout), badLines(map[string]string{"accept rules": "mismatch"})) {
		t.Fatalf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.err)
	}
}

// ---- engine state, privilege scope and the iptables lock in disarm and remove ----

func TestDisarmAndRemoveCompareTheEngineState(t *testing.T) {
	t.Run("disarm: daemon.json changed", func(t *testing.T) {
		fr, h := twoAgents(), newHost()
		armAll(t, fr, agentA)
		fr.afterCall = func(j string) {
			if strings.HasPrefix(j, "nft delete table") {
				h.files["/etc/docker/daemon.json"] = `{"changed":true}`
			}
		}
		r := run(t, fr, h, nil, "disarm", agentA)
		if r.code != 2 || !strings.Contains(r.err, "DISARM_INCOMPLETE") || !strings.Contains(r.err, "daemon.json changed") || strings.Contains(r.stdout, "disarmed on") {
			t.Fatalf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.err)
		}
	})
	t.Run("remove: engine restarted", func(t *testing.T) {
		fr, h := twoAgents(), newHost()
		armAll(t, fr, agentA, agentB)
		fr.afterCall = func(j string) {
			if j == "docker network rm agents-net" {
				h.files["/proc/4242/stat"] = statLine(4242, "999999")
			}
		}
		r := runRemoveHost(t, fr, h)
		if r.code != 2 || !strings.Contains(r.err, "REMOVE_INCOMPLETE") || !strings.Contains(r.err, "docker engine was restarted") || strings.Contains(r.stdout, "agent network agents-net removed") {
			t.Fatalf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.err)
		}
	})
	t.Run("remove: engine state unreadable at the start", func(t *testing.T) {
		fr, h := twoAgents(), newHost()
		armAll(t, fr, agentA)
		before := fr.state()
		fr.calls = nil
		delete(h.files, "/var/run/docker.pid")
		r := runRemoveHost(t, fr, h)
		if r.code != 1 || !strings.Contains(r.err, "NOT_REMOVED") || !strings.Contains(r.err, "engine state could not be recorded") || fr.state() != before || fr.mutatedHost() {
			t.Fatalf("exit %d, stderr %q", r.code, r.err)
		}
	})
}

func TestRemoveRequiresTheHostNetworkScope(t *testing.T) {
	fr, h := twoAgents(), newHost()
	h.links["/proc/self/ns/net"] = "net:[4026532999]"
	r := runRemoveHost(t, fr, h)
	if r.code != 1 || !strings.Contains(r.err, "NOT_REMOVED") || !strings.Contains(r.err, "not in the agent host's own network scope") || len(fr.calls) != 0 {
		t.Fatalf("exit %d, stderr %q, calls %v", r.code, r.err, fr.calls)
	}
}

func TestDisarmAndRemoveIptablesCallsWaitForTheLock(t *testing.T) {
	fr := twoAgents()
	armAll(t, fr, agentA, agentB)
	if r := run(t, fr, newHost(), nil, "disarm", agentA); r.code != 0 {
		t.Fatalf("disarm: exit %d, stderr %q", r.code, r.err)
	}
	if r := runRemove(t, fr); r.code != 0 {
		t.Fatalf("remove: exit %d, stderr %q", r.code, r.err)
	}
	if fr.count("iptables -D") == 0 || fr.iptNoWait != 0 {
		t.Errorf("iptables -D calls = %d, iptables calls without -w = %d", fr.count("iptables -D"), fr.iptNoWait)
	}
}
