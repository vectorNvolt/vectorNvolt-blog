package tests

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-sandbox/internal/sequence"
	"agent-sandbox/internal/services"
)

// exact counts the calls whose joined arguments equal s (prefix counting would include `create_agent_net disarm ...`).
func exact(fr *fakeRunner, s string) int {
	n := 0
	for _, c := range fr.calls {
		if c.joined() == s {
			n++
		}
	}
	return n
}

func exactIndex(fr *fakeRunner, s string) int {
	for i, c := range fr.calls {
		if c.joined() == s {
			return i
		}
	}
	return -1
}

const (
	createNetCmd = "/usr/local/bin/create_agent_net"
	removeNetCmd = "/usr/local/bin/remove_agent_net"
)

// ---- agent_lifecycle restore ----

type restoreResult struct {
	code        int
	stdout, err string
	dir         string // runtime dir (ready marker)
}

func (r restoreResult) marker() string { return filepath.Join(r.dir, "ready") }

func runRestore(t *testing.T, fr *fakeRunner, fs *fakeFS, dir string, isRoot bool, extra map[string]string) restoreResult {
	t.Helper()
	values := map[string]string{"AGENT_SANDBOX_RUNTIME_DIR": dir}
	for k, v := range extra {
		values[k] = v
	}
	cfg := testConfig("", values)
	var out, errb bytes.Buffer
	code := sequence.RunRestore(context.Background(), sequence.RestoreDeps{
		Config: cfg, Runner: fr, FS: fs, IsRoot: isRoot, Stdout: &out, Stderr: &errb,
	})
	if len(fr.unexpected) > 0 {
		t.Errorf("commands without a fake answer: %v", fr.unexpected)
	}
	return restoreResult{code, out.String(), errb.String(), dir}
}

func staleMarker(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ready"), []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRestoreRemovesAllAgentContainersThenRebuildsNetAndWritesMarker(t *testing.T) {
	dir := staleMarker(t)
	stateRoot := t.TempDir()
	sentinel := filepath.Join(stateRoot, "a", "claude", "login.json")
	if err := os.MkdirAll(filepath.Dir(sentinel), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sentinel, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	fs := &fakeFS{}
	fr := &fakeRunner{live: map[string]bool{"agent-a": true, "agent-b": true}}
	r := runRestore(t, fr, fs, dir, true, map[string]string{"AGENT_SANDBOX_STATE_ROOT": stateRoot})
	if r.code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", r.code, r.err)
	}
	if len(fr.live) != 0 || fr.count("docker rm -f agent-a") != 1 || fr.count("docker rm -f agent-b") != 1 {
		t.Errorf("both containers must be removed once: live %v calls %v", fr.live, fr.calls)
	}
	rmA, rmB := fr.index("docker rm -f agent-a"), fr.index("docker rm -f agent-b")
	disarmA := fr.index(createNetCmd + " disarm agent-a")
	removeNet, createNet := exactIndex(fr, removeNetCmd), exactIndex(fr, createNetCmd)
	netCheck := fr.index("docker network inspect agents-net")
	if !(disarmA >= 0 && disarmA < rmA && rmA < removeNet && rmB < removeNet && removeNet < createNet && createNet < netCheck) {
		t.Errorf("order disarm=%d rm=%d,%d remove_agent_net=%d create_agent_net=%d net-check=%d", disarmA, rmA, rmB, removeNet, createNet, netCheck)
	}
	data, err := os.ReadFile(r.marker())
	if err != nil || strings.TrimSpace(string(data)) != dockerPID {
		t.Errorf("marker = %q (%v), want dockerd main pid %s", data, err, dockerPID)
	}
	if fi, err := os.Stat(r.marker()); err != nil || fi.Mode().Perm() != 0o644 {
		t.Errorf("marker mode: %v %v", fi, err)
	}
	if fi, err := os.Stat(dir); err != nil || fi.Mode().Perm() != 0o755 {
		t.Errorf("runtime dir mode: %v %v", fi, err)
	}
	if !strings.Contains(r.stdout, "exit_state: RESTORED") {
		t.Errorf("stdout: %q", r.stdout)
	}
	if !exists(sentinel) || len(fs.removed) != 0 {
		t.Error("restore must never purge agent state")
	}
}

func TestRestoreFailureLeavesNoMarker(t *testing.T) {
	cases := []struct {
		name  string
		fr    *fakeRunner
		state string
		code  int
	}{
		{"create_agent_net fails", &fakeRunner{live: map[string]bool{}, rules: []rule{{contains: "create_agent_net", stderr: "gate build failed", code: 2}}}, "RESTORE_FAILED", 2},
		{"container removal fails", &fakeRunner{live: map[string]bool{"agent-a": true}, rules: []rule{{contains: "docker rm -f agent-a", stderr: "device busy", code: 1}}}, "RESTORE_FAILED", 2},
		{"docker never answers", &fakeRunner{rules: []rule{{contains: "docker info", stderr: "Cannot connect to the Docker daemon", code: 1}}}, "NOT_RESTORED", 2},
		{"network missing afterwards", &fakeRunner{live: map[string]bool{}, rules: []rule{{contains: "docker network inspect", stderr: "Error: No such network", code: 1}}}, "RESTORE_FAILED", 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := staleMarker(t)
			r := runRestore(t, c.fr, &fakeFS{}, dir, true, nil)
			if r.code != c.code || !strings.Contains(r.err, "exit_state: "+c.state) {
				t.Fatalf("exit %d, stderr %q", r.code, r.err)
			}
			if exists(r.marker()) {
				t.Error("no ready marker may exist after a failed restore (the stale one is removed first)")
			}
			if strings.Contains(r.stdout, "RESTORED") {
				t.Errorf("no success text: %q", r.stdout)
			}
		})
	}
}

func TestRestoreFailedRemovalSkipsNetRebuild(t *testing.T) {
	fr := &fakeRunner{live: map[string]bool{"agent-a": true}, rules: []rule{{contains: "docker rm -f agent-a", stderr: "device busy", code: 1}}}
	r := runRestore(t, fr, &fakeFS{}, staleMarker(t), true, nil)
	if r.code != 2 || !strings.Contains(r.err, "agent-a") || !strings.Contains(r.err, "device busy") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if exact(fr, removeNetCmd) != 0 || exact(fr, createNetCmd) != 0 {
		t.Errorf("the network must not be rebuilt after a failed removal: %v", fr.calls)
	}
}

func TestRestoreRequiresRoot(t *testing.T) {
	fr := &fakeRunner{}
	dir := staleMarker(t)
	r := runRestore(t, fr, &fakeFS{}, dir, false, nil)
	if r.code != 1 || !strings.Contains(r.err, "must run as root") || !strings.Contains(r.err, "NOT_RESTORED") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if len(fr.calls) != 0 {
		t.Errorf("nothing may run: %v", fr.calls)
	}
}

func TestRestoreToleratesDisarmAndMissingNetwork(t *testing.T) {
	fr := &fakeRunner{live: map[string]bool{"agent-a": true}, rules: []rule{
		{contains: createNetCmd + " disarm", stderr: "no such veth", code: 2},
		{contains: "remove_agent_net", stderr: "network agents-net not present", code: 1},
	}}
	r := runRestore(t, fr, &fakeFS{}, t.TempDir(), true, nil)
	if r.code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", r.code, r.err)
	}
	if fr.count("docker rm -f agent-a") != 1 || !exists(r.marker()) {
		t.Errorf("container removed and marker written expected: %v", fr.calls)
	}
}

// ---- agent_lifecycle guard ----

type fakeEvents struct{ events []services.GuardEvent }

func (e fakeEvents) Follow(_ context.Context, handle func(services.GuardEvent)) error {
	for _, ev := range e.events {
		handle(ev)
	}
	return errors.New("docker events ended")
}

// starts are container start events for the names.
func starts(names ...string) []services.GuardEvent {
	var evs []services.GuardEvent
	for _, n := range names {
		evs = append(evs, services.GuardEvent{Kind: services.EventStart, Name: n})
	}
	return evs
}

func runGuardEvents(t *testing.T, fr *fakeRunner, dir string, evs ...services.GuardEvent) (error, string) {
	t.Helper()
	cfg := testConfig("", map[string]string{"AGENT_SANDBOX_RUNTIME_DIR": dir})
	var errb bytes.Buffer
	err := sequence.RunGuard(context.Background(), sequence.GuardDeps{
		Config: cfg, Runner: fr, FS: &fakeFS{}, Events: fakeEvents{evs}, Stderr: &errb,
	})
	if len(fr.unexpected) > 0 {
		t.Errorf("commands without a fake answer: %v", fr.unexpected)
	}
	return err, errb.String()
}

// runGuard delivers start events; the reconcile pass at the guard start lists no container (hidePS), so that the event
// path is what is tested.
func runGuard(t *testing.T, fr *fakeRunner, dir string, names ...string) (error, string) {
	t.Helper()
	fr.rules = append([]rule{hidePS}, fr.rules...)
	return runGuardEvents(t, fr, dir, starts(names...)...)
}

var gateBroken = rule{contains: "check-gate", stdout: "control forward-drop: missing\n", code: 2}

// hidePS makes `docker ps -a` list nothing.
var hidePS = rule{contains: "docker ps -a", stdout: "\n"}

func TestGuardIgnoresStartWhileLockHeld(t *testing.T) {
	dir := t.TempDir()
	lock, err := services.TryLock(dir, agentID)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	fr := &fakeRunner{live: map[string]bool{"agent-dev-1": true}, rules: []rule{gateBroken}}
	gerr, log := runGuard(t, fr, dir, "agent-dev-1")
	if gerr == nil {
		t.Error("the guard returns the error of the ended event stream")
	}
	if fr.count("docker rm") != 0 || fr.has(createNetCmd+" check-gate") || fr.has("docker container inspect") || log != "" {
		t.Errorf("a tool-driven start must be ignored: calls %v log %q", fr.calls, log)
	}
}

// The reconcile pass at the guard start also skips an agent whose lock is held.
func TestGuardReconcileSkipsAgentWithLockHeld(t *testing.T) {
	dir := t.TempDir()
	lock, err := services.TryLock(dir, agentID)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	fr := &fakeRunner{live: map[string]bool{"agent-dev-1": true}, rules: []rule{gateBroken}}
	_, log := runGuardEvents(t, fr, dir)
	if fr.has("docker rm") || fr.has(createNetCmd+" check-gate") || log != "" {
		t.Errorf("calls %v log %q", fr.calls, log)
	}
}

func TestParseGuardEvent(t *testing.T) {
	for line, want := range map[string]services.GuardEvent{
		"container|start|agent-a|<no value>":  {Kind: "start", Name: "agent-a"},
		"container|die|agent-a|":              {Kind: "die", Name: "agent-a"},
		"container|update|agent-a|":           {Kind: "update", Name: "agent-a"},
		"container|unpause|agent-a|":          {Kind: "unpause", Name: "agent-a"},
		"network|connect|bridge|0123abcd":     {Kind: "connect", Container: "0123abcd", Network: "bridge"},
		"network|disconnect|agents-net|0123ab": {Kind: "disconnect", Container: "0123ab", Network: "agents-net"},
	} {
		if got, ok := services.ParseGuardEvent(line); !ok || got != want {
			t.Errorf("%q: got %+v ok=%v, want %+v", line, got, ok, want)
		}
	}
	for _, line := range []string{"", "container|pause|agent-a|", "network|create|x|y", "container|start||", "network|connect|bridge|", "junk"} {
		if _, ok := services.ParseGuardEvent(line); ok {
			t.Errorf("%q must be ignored", line)
		}
	}
}

// die of a container whose lock is free: the guard disarms it by name and leaves the container alone.
func TestGuardDisarmsOnDie(t *testing.T) {
	fr := &fakeRunner{live: map[string]bool{"agent-dev-1": true}, rules: []rule{hidePS, gateBroken}}
	_, log := runGuardEvents(t, fr, t.TempDir(), services.GuardEvent{Kind: services.EventDie, Name: "agent-dev-1"})
	if fr.count(createNetCmd+" disarm agent-dev-1") != 1 || fr.has("docker rm") || fr.has(createNetCmd+" check-gate") || log != "" {
		t.Errorf("calls %v log %q", fr.calls, log)
	}
	// a die event during a create/remove is ignored
	dir := t.TempDir()
	lock, _ := services.TryLock(dir, agentID)
	defer lock.Release()
	fr = &fakeRunner{rules: []rule{hidePS}}
	runGuardEvents(t, fr, dir, services.GuardEvent{Kind: services.EventDie, Name: "agent-dev-1"})
	if fr.has(createNetCmd + " disarm") {
		t.Errorf("lock held: nothing may run: %v", fr.calls)
	}
}

// update, network connect and network disconnect go through the same check as start; the network event is resolved
// from the container ID.
func TestGuardChecksUpdateAndNetworkEvents(t *testing.T) {
	for name, ev := range map[string]services.GuardEvent{
		"update":     {Kind: services.EventUpdate, Name: "agent-dev-1"},
		"connect":    {Kind: services.EventConnect, Container: "c0ffee", Network: "bridge"},
		"disconnect": {Kind: services.EventDisconnect, Container: "c0ffee", Network: "agents-net"},
	} {
		t.Run(name, func(t *testing.T) {
			fr := &fakeRunner{live: map[string]bool{"agent-dev-1": true}, ids: map[string]string{"c0ffee": "agent-dev-1"}, rules: []rule{hidePS, gateBroken}}
			_, log := runGuardEvents(t, fr, t.TempDir(), ev)
			if fr.count("docker rm -f agent-dev-1") != 1 || fr.index(createNetCmd+" disarm agent-dev-1") < 0 || !strings.Contains(log, "was removed") {
				t.Errorf("calls %v log %q", fr.calls, log)
			}
		})
	}
	// a network event of a container that is not an agent, or is gone, is ignored
	fr := &fakeRunner{live: map[string]bool{"web-1": true}, ids: map[string]string{"c1": "web-1"}, rules: []rule{hidePS, gateBroken}}
	_, log := runGuardEvents(t, fr, t.TempDir(),
		services.GuardEvent{Kind: services.EventConnect, Container: "c1", Network: "bridge"},
		services.GuardEvent{Kind: services.EventConnect, Container: "gone", Network: "bridge"})
	if fr.has("docker rm") || fr.has(createNetCmd+" disarm") || fr.has(createNetCmd+" check-gate") || log != "" {
		t.Errorf("calls %v log %q", fr.calls, log)
	}
}

// The guard holds the agent lock from its check to the end of the removal (the remove path takes none of its own).
func TestGuardHoldsTheLockThroughCheckAndRemove(t *testing.T) {
	dir := t.TempDir()
	var held []string
	fr := &fakeRunner{live: map[string]bool{"agent-dev-1": true}, rules: []rule{gateBroken}}
	fr.hook = func(j string) {
		for _, p := range []string{createNetCmd + " check-gate", createNetCmd + " disarm", "docker rm -f"} {
			if strings.HasPrefix(j, p) {
				l, err := services.TryLock(dir, agentID)
				if err == nil {
					l.Release()
					held = append(held, "FREE during "+j)
				}
			}
		}
	}
	runGuard(t, fr, dir, "agent-dev-1")
	if fr.count("docker rm -f agent-dev-1") != 1 || len(held) != 0 {
		t.Errorf("rm %d, lock was free during: %v", fr.count("docker rm -f agent-dev-1"), held)
	}
	if l, err := services.TryLock(dir, agentID); err != nil {
		t.Errorf("lock must be released at the end: %v", err)
	} else {
		l.Release()
	}
}

// Errors (not a clean gate failure): disarm first, remove only when the disarm exited 0, log both outcomes.
func TestGuardFailsClosedOnErrors(t *testing.T) {
	cases := map[string]rule{
		"check-gate cannot run (exit 1)": {contains: "check-gate", stderr: "docker unreachable", code: 1},
		"existence check fails":          {contains: "docker container inspect agent-dev-1", stderr: "Cannot connect to the Docker daemon", code: 1},
	}
	for name, errRule := range cases {
		t.Run(name, func(t *testing.T) {
			fr := &fakeRunner{live: map[string]bool{"agent-dev-1": true}, rules: []rule{errRule}}
			_, log := runGuard(t, fr, t.TempDir(), "agent-dev-1")
			disarm, rm := fr.index(createNetCmd+" disarm agent-dev-1"), fr.index("docker rm -f agent-dev-1")
			if !(disarm >= 0 && disarm < rm) || fr.live["agent-dev-1"] {
				t.Errorf("disarm=%d rm=%d calls %v", disarm, rm, fr.calls)
			}
			if !strings.Contains(log, "disarm of agent-dev-1 succeeded") || !strings.Contains(log, "was removed") {
				t.Errorf("both outcomes must be logged: %q", log)
			}
		})
	}
	t.Run("disarm fails: the container stays, paused", func(t *testing.T) {
		fr := &fakeRunner{live: map[string]bool{"agent-dev-1": true}, rules: []rule{
			{contains: "check-gate", stderr: "docker unreachable", code: 1},
			{contains: createNetCmd + " disarm", stderr: "nft: busy", code: 2},
		}}
		_, log := runGuard(t, fr, t.TempDir(), "agent-dev-1")
		if fr.has("docker rm") || !fr.live["agent-dev-1"] || fr.count("docker pause agent-dev-1") != 1 {
			t.Errorf("container must stay and be paused: %v", fr.calls)
		}
		if !strings.Contains(log, "disarm of agent-dev-1 failed") || !strings.Contains(log, "left in place") {
			t.Errorf("log %q", log)
		}
	})
}

// The reconcile pass at the guard start (and every period) gate-checks the free agents and cleans the gate set.
func TestGuardReconcileChecksContainersAndCleansTheGateSet(t *testing.T) {
	fr := &fakeRunner{
		live:        map[string]bool{"agent-dev-1": true, "agent-dev-2": true},
		releasedOut: "02:42:ac:1e:00:01 \n02:42:ac:1e:00:02 agent-gone\n02:42:ac:1e:00:03 agent-dev-1\n",
		rules:       []rule{{contains: "check-gate agent-dev-2", stdout: "control forward-drop: missing\n", code: 2}},
	}
	_, log := runGuardEvents(t, fr, t.TempDir())
	if fr.count("docker rm -f agent-dev-2") != 1 || fr.live["agent-dev-2"] || !fr.live["agent-dev-1"] {
		t.Errorf("only the container that fails the gate check goes: %v live %v", fr.calls, fr.live)
	}
	if fr.count(createNetCmd+" evict 02:42:ac:1e:00:01") != 1 {
		t.Errorf("the untagged element must be evicted: %v", fr.calls)
	}
	if fr.count(createNetCmd+" disarm agent-gone") != 1 || fr.count(createNetCmd+" evict") != 1 {
		t.Errorf("the element of a container that does not exist is disarmed by name: %v", fr.calls)
	}
	if fr.count(createNetCmd+" disarm agent-dev-1") != 0 || strings.Contains(log, "agent-dev-1:") {
		t.Errorf("the element of an existing container stays: %v", fr.calls)
	}
}

// create: the name-keyed disarm and the gate set reconcile run before docker create.
func TestCreateClearsLeftoversBeforeDockerCreate(t *testing.T) {
	fr := &fakeRunner{releasedOut: "02:42:ac:1e:00:09 \n"}
	r := run(t, fr, &fakeFS{}, agentID, nil)
	if r.code != 0 {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	disarm, evict, create := fr.index(createNetCmd+" disarm agent-dev-1"), fr.index(createNetCmd+" evict 02:42:ac:1e:00:09"), fr.index("docker create ")
	if !(disarm >= 0 && disarm < evict && evict < create) {
		t.Errorf("disarm=%d evict=%d create=%d", disarm, evict, create)
	}
	for name, rules := range map[string][]rule{
		"disarm fails": {{contains: createNetCmd + " disarm", stderr: "nft: busy", code: 2}},
		"evict fails":  {{contains: createNetCmd + " evict", stderr: "nft: busy", code: 2}},
	} {
		t.Run(name, func(t *testing.T) {
			fr := &fakeRunner{releasedOut: "02:42:ac:1e:00:09 \n", rules: rules}
			r := run(t, fr, &fakeFS{}, agentID, nil)
			if r.code != 2 || !strings.Contains(r.err, "NOT_CREATED") || fr.has("docker create") {
				t.Errorf("nothing may be created: exit %d, stderr %q, calls %v", r.code, r.err, fr.calls)
			}
		})
	}
}

func TestGuardRemovesOutsideStartWhenLockFree(t *testing.T) {
	dir := t.TempDir()
	fr := &fakeRunner{live: map[string]bool{"agent-dev-1": true}, rules: []rule{gateBroken}}
	_, log := runGuard(t, fr, dir, "agent-dev-1")
	if fr.count("docker rm -f agent-dev-1") != 1 || fr.live["agent-dev-1"] {
		t.Errorf("container must be removed: %v", fr.calls)
	}
	if fr.index(createNetCmd+" disarm agent-dev-1") < 0 {
		t.Errorf("the remove path disarms first: %v", fr.calls)
	}
	if !strings.Contains(log, "agent-dev-1") || !strings.Contains(log, "started outside the tools") || !strings.Contains(log, "was removed") {
		t.Errorf("journal line missing: %q", log)
	}
	if strings.Count(strings.TrimSpace(log), "\n") != 0 {
		t.Errorf("exactly one journal line expected: %q", log)
	}
	// The lock is free again afterwards.
	lock, err := services.TryLock(dir, agentID)
	if err != nil {
		t.Fatalf("lock must be released: %v", err)
	}
	lock.Release()
}

func TestGuardIgnoresArmedContainerWithFreeLock(t *testing.T) {
	// A late event of a create that already finished: the gate check passes, so the container is left alone.
	fr := &fakeRunner{live: map[string]bool{"agent-dev-1": true}}
	_, log := runGuard(t, fr, t.TempDir(), "agent-dev-1")
	if fr.has("docker rm") || log != "" {
		t.Errorf("an armed container must be left alone: calls %v log %q", fr.calls, log)
	}
}

func TestGuardIgnoresAbsentContainerAndForeignNames(t *testing.T) {
	fr := &fakeRunner{live: map[string]bool{}, rules: []rule{gateBroken}}
	_, log := runGuard(t, fr, t.TempDir(), "agent-dev-1", "other-1", "agent-BAD", "agent-", "agent-a/b")
	if fr.has("docker rm") || fr.has(createNetCmd+" check-gate") || fr.has(createNetCmd+" disarm") || log != "" {
		t.Errorf("nothing to do: calls %v log %q", fr.calls, log)
	}
}

func TestGuardNeverPurgesState(t *testing.T) {
	stateRoot := t.TempDir()
	state := filepath.Join(stateRoot, agentID, "claude", "login.json")
	if err := os.MkdirAll(filepath.Dir(state), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(state, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := testConfig("", map[string]string{"AGENT_SANDBOX_RUNTIME_DIR": t.TempDir(), "AGENT_SANDBOX_STATE_ROOT": stateRoot})
	fs := &fakeFS{}
	fr := &fakeRunner{live: map[string]bool{"agent-dev-1": true}, rules: []rule{gateBroken}}
	var errb bytes.Buffer
	_ = sequence.RunGuard(context.Background(), sequence.GuardDeps{
		Config: cfg, Runner: fr, FS: fs, Events: fakeEvents{starts("agent-dev-1")}, Stderr: &errb,
	})
	if fr.count("docker rm -f agent-dev-1") != 1 || !exists(state) || len(fs.removed) != 0 {
		t.Errorf("container removed, state kept: calls %v removed %v", fr.calls, fs.removed)
	}
}

// ---- remove: agent state is kept by default, --purge deletes it ----

func TestRemoveKeepsStateByDefault(t *testing.T) {
	stateRoot := t.TempDir()
	keep := filepath.Join(stateRoot, agentID, "claude", "login.json")
	if err := os.MkdirAll(filepath.Dir(keep), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keep, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	fs := &fakeFS{}
	r := runRemoveEnv(t, &fakeRunner{existing: true}, fs, "", map[string]string{"AGENT_SANDBOX_STATE_ROOT": stateRoot}, agentID)
	if r.code != 0 || !exists(keep) || len(fs.removed) != 0 {
		t.Fatalf("exit %d, stderr %q, removed %v", r.code, r.err, fs.removed)
	}
	if !strings.Contains(r.stdout, "agent state kept: "+filepath.Join(stateRoot, agentID)) {
		t.Errorf("stdout: %q", r.stdout)
	}
	// --purge-workspace alone does not touch the state either.
	r = runRemoveEnv(t, &fakeRunner{existing: true}, fs, "", map[string]string{"AGENT_SANDBOX_STATE_ROOT": stateRoot}, agentID, "--purge-workspace")
	if r.code != 0 || !exists(keep) {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
}

func TestPurgeDeletesOnlyOwnState(t *testing.T) {
	stateRoot := t.TempDir()
	own := filepath.Join(stateRoot, agentID)
	other := filepath.Join(stateRoot, "other-agent")
	for _, d := range []string{filepath.Join(own, "claude"), other} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	root := t.TempDir()
	ws := mkWorkspace(t, root)
	fs := &fakeFS{}
	r := runRemoveEnv(t, &fakeRunner{existing: true}, fs, root, map[string]string{"AGENT_SANDBOX_STATE_ROOT": stateRoot}, agentID, "--purge")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	if exists(own) || !exists(other) || !exists(stateRoot) || !exists(ws) {
		t.Errorf("only %s may be deleted (workspace is kept without --purge-workspace)", own)
	}
	if len(fs.removed) != 1 || fs.removed[0] != own {
		t.Errorf("removals = %v", fs.removed)
	}
	if !strings.Contains(r.stdout, "agent state purged: "+own) {
		t.Errorf("stdout: %q", r.stdout)
	}
}

func TestPurgeRejectsBadIDsBeforeAnyPathIsBuilt(t *testing.T) {
	base := t.TempDir()
	stateRoot := filepath.Join(base, "state")
	victim := filepath.Join(base, "victim")
	for _, d := range []string{stateRoot, victim} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"", ".", "..", "../victim", "a/../../victim", "/etc", "A", "dev_1", "dev-1/", "dev 1", strings.Repeat("a", 32)} {
		fr, fs := &fakeRunner{existing: true}, &fakeFS{}
		r := runRemoveEnv(t, fr, fs, "", map[string]string{"AGENT_SANDBOX_STATE_ROOT": stateRoot}, id, "--purge")
		if r.code != 1 || !strings.Contains(r.err, "usage: remove_agent_container") {
			t.Errorf("id %q: exit %d, stderr %q", id, r.code, r.err)
		}
		if len(fr.calls) != 0 || len(fs.removed) != 0 || !exists(victim) || !exists(stateRoot) {
			t.Errorf("id %q: nothing may be called or deleted: %v %v", id, fr.calls, fs.removed)
		}
	}
}

func TestPurgeRefusesSymlinkedStateFolder(t *testing.T) {
	stateRoot, outside := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "keep"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(stateRoot, agentID)); err != nil {
		t.Fatal(err)
	}
	fs := &fakeFS{}
	r := runRemoveEnv(t, &fakeRunner{existing: true}, fs, "", map[string]string{"AGENT_SANDBOX_STATE_ROOT": stateRoot}, agentID, "--purge")
	if r.code != 2 || !strings.Contains(r.err, "STATE_PURGE_FAILED") || !strings.Contains(r.err, "refusing to purge") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if len(fs.removed) != 0 || !exists(filepath.Join(outside, "keep")) {
		t.Error("nothing may be deleted")
	}
}

func TestPurgeAbsentStateIsNotAnError(t *testing.T) {
	stateRoot := t.TempDir()
	r := runRemoveEnv(t, &fakeRunner{existing: true}, &fakeFS{}, "", map[string]string{"AGENT_SANDBOX_STATE_ROOT": stateRoot}, agentID, "--purge")
	if r.code != 0 || !strings.Contains(r.stdout, "agent state not present, nothing to purge") {
		t.Fatalf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.err)
	}
}

func TestRemoveRefusedWhileAgentLockHeld(t *testing.T) {
	dir := t.TempDir()
	lock, err := services.TryLock(dir, agentID)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	fr := &fakeRunner{existing: true}
	r := runRemoveEnv(t, fr, &fakeFS{}, "", map[string]string{"AGENT_SANDBOX_RUNTIME_DIR": dir}, agentID)
	if r.code != 1 || !strings.Contains(r.err, "in progress") || !strings.Contains(r.err, "NOT_REMOVED") || len(fr.calls) != 0 {
		t.Fatalf("exit %d, stderr %q, calls %v", r.code, r.err, fr.calls)
	}
}
