package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"agent-sandbox/internal/config"
	"agent-sandbox/internal/sequence"
	"agent-sandbox/internal/services"
)

// ---- fake process runner (docker and create_agent_net) ----

type call struct {
	Args  []string
	Stdin []byte
}

func (c call) joined() string { return strings.Join(c.Args, " ") }

// rule overrides the default answer for every command whose joined arguments contain `contains`.
// Rules are tried in order; the first one that applies wins.
type rule struct {
	contains string
	stdout   string
	stderr   string
	code     int
	err      error

	skip        int   // matching calls that pass through to the default before the rule applies
	limit       int   // the rule applies at most this many times (0 = always)
	pausedAfter *bool // when set, the fake container's paused flag takes this value as the rule applies

	seen, hits int
}

func boolPtr(b bool) *bool { return &b }

type fakeRunner struct {
	mu         sync.Mutex
	calls      []call
	rules      []rule
	unexpected []string // commands no default and no rule covers (answered with exit 99)
	existing   bool     // container agent-<id> exists (answers the plain `container inspect`)
	ws         string   // workspace path, set by the harness
	claude     string   // per-agent Claude state path, set by the harness
	resolv     string   // host resolv.conf path, set by the harness
	paused     bool     // set by `docker pause`, cleared by `docker unpause`

	// live, when non-nil, replaces `existing`: the containers that exist; `docker rm` deletes them, `docker ps -a` lists them.
	live map[string]bool

	hook        func(joined string) // runs at the start of every call (the call mutex is held)
	releasedOut string            // what `create_agent_net released` prints
	ids         map[string]string // container ID -> name (network events)

	mutate    func(s *stateSpec) // adjusts the default full inspect output
	rawVerify string             // replaces the inspect output while the container is unpaused (final verification)
}

const resolvGood = "nameserver 1.1.1.1\nnameserver 9.9.9.9\n"

// dockerPID is what the fake `systemctl show -p MainPID` answers and what the shared ready marker holds.
const dockerPID = "4242"

const idOut = "uid=1001(agent) gid=1001(agent) groups=1001(agent),999(docker)\n"

func (f *fakeRunner) Run(_ context.Context, stdin []byte, args ...string) ([]byte, []byte, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call{Args: append([]string(nil), args...), Stdin: stdin})
	joined := strings.Join(args, " ")
	if f.hook != nil {
		f.hook(joined)
	}
	for i := range f.rules {
		r := &f.rules[i]
		if !strings.Contains(joined, r.contains) {
			continue
		}
		if r.seen < r.skip {
			r.seen++
			continue
		}
		if r.limit > 0 && r.hits >= r.limit {
			continue
		}
		r.hits++
		if r.pausedAfter != nil {
			f.paused = *r.pausedAfter
		}
		return []byte(r.stdout), []byte(r.stderr), r.code, r.err
	}
	// Unknown commands fail (exit 99) so a new call cannot pass silently.
	switch {
	case strings.HasPrefix(joined, "docker info"):
		return []byte(`{"runc":{"path":"runc"},"runsc":{"path":"/usr/local/bin/runsc"}}`), nil, 0, nil
	case strings.HasPrefix(joined, "docker pause "):
		f.paused = true
		return nil, nil, 0, nil
	case strings.HasPrefix(joined, "docker unpause "):
		f.paused = false
		return nil, nil, 0, nil
	case strings.HasPrefix(joined, "docker inspect -f {{.State.Paused}} "):
		return []byte(fmt.Sprintf("%t\n", f.paused)), nil, 0, nil
	case strings.HasPrefix(joined, "docker rm "):
		if f.paused { // like runsc on the real host: a paused sandbox cannot be killed
			return nil, []byte("Error response from daemon: cannot remove container: could not kill container: container " +
				args[len(args)-1] + " PID 1 is zombie and can not be killed"), 1, nil
		}
		delete(f.live, args[len(args)-1])
		return nil, nil, 0, nil
	case joined == "systemctl show -p MainPID --value docker.service":
		return []byte(dockerPID + "\n"), nil, 0, nil
	case strings.HasPrefix(joined, "docker ps -a "):
		var names []string
		for n := range f.live {
			names = append(names, n)
		}
		sort.Strings(names)
		return []byte(strings.Join(names, "\n") + "\n"), nil, 0, nil
	case joined == "/usr/local/bin/create_agent_net", joined == "/usr/local/bin/remove_agent_net":
		return nil, nil, 0, nil
	case strings.HasPrefix(joined, "docker exec -u 0:0 ") && strings.HasSuffix(joined, " cat /etc/resolv.conf"):
		return []byte(resolvGood), nil, 0, nil
	case strings.HasPrefix(joined, "docker create "),
		strings.HasPrefix(joined, "docker start "),
		strings.HasPrefix(joined, "docker exec -u 0:0 "):
		return nil, nil, 0, nil
	case strings.HasPrefix(joined, "docker network inspect "), strings.HasPrefix(joined, "docker image inspect "):
		return []byte("[]"), nil, 0, nil
	// Read-back probes match any container name (agent-<id>), not only agent-dev-1.
	case strings.HasPrefix(joined, "docker exec -u 1001 ") && strings.Contains(joined, " cat /etc/resolv.conf"):
		return []byte(resolvGood), nil, 0, nil
	case strings.HasPrefix(joined, "docker exec -u 1001 ") && strings.Contains(joined, " stat -c"):
		return []byte("root:root 644\n"), nil, 0, nil
	case strings.HasPrefix(joined, "docker exec -u 1001 ") && strings.Contains(joined, " sh -c : >> /etc/resolv.conf"):
		return nil, []byte("sh: Permission denied"), 1, nil // the agent user cannot write
	// An explicit group (-u 1001:1001) makes Docker skip the supplementary groups (mirrors the host).
	case strings.HasPrefix(joined, "docker exec -u 1001:1001 ") && strings.HasSuffix(joined, " id -nG"):
		return []byte("agent\n"), nil, 0, nil
	case strings.HasPrefix(joined, "docker exec -u 1001 ") && strings.HasSuffix(joined, " id -nG"):
		return []byte("agent docker\n"), nil, 0, nil
	case strings.HasPrefix(joined, "docker exec -u 1001 ") && strings.HasSuffix(joined, " id"):
		return []byte(idOut), nil, 0, nil
	case strings.HasPrefix(joined, "docker container inspect --format {{.Name}} "):
		id := args[len(args)-1]
		name := f.ids[id]
		if name == "" {
			name = id
		}
		if f.live != nil && f.live[name] || f.live == nil && f.existing {
			return []byte("/" + name + "\n"), nil, 0, nil
		}
		return nil, []byte("Error: No such container: " + id), 1, nil
	case len(args) == 2 && args[0] != "docker" && args[1] == "released":
		return []byte(f.releasedOut), nil, 0, nil
	case len(args) == 3 && args[0] != "docker" && args[1] == "evict":
		return nil, nil, 0, nil
	case strings.Contains(joined, "container inspect --format"):
		if f.rawVerify != "" && !f.paused {
			return []byte(f.rawVerify), nil, 0, nil
		}
		s := okState(f.ws, f.claude, f.resolv)
		s.Paused = f.paused
		if f.mutate != nil {
			f.mutate(&s)
		}
		return []byte(s.json()), nil, 0, nil
	case strings.HasPrefix(joined, "docker container inspect "):
		if f.live != nil {
			if f.live[args[len(args)-1]] {
				return []byte("{}"), nil, 0, nil
			}
			return nil, []byte("Error: No such container: " + args[len(args)-1]), 1, nil
		}
		if f.existing {
			return []byte("{}"), nil, 0, nil
		}
		return nil, []byte("Error: No such container: " + args[len(args)-1]), 1, nil
	case len(args) >= 3 && args[0] != "docker" && (args[1] == "arm" || args[1] == "check-gate" || args[1] == "disarm"):
		return nil, nil, 0, nil
	}
	f.unexpected = append(f.unexpected, joined)
	return nil, []byte("fakeRunner: unexpected command: " + joined), 99, nil
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

func (f *fakeRunner) lastIndex(prefix string) int {
	last := -1
	for i, c := range f.calls {
		if strings.HasPrefix(c.joined(), prefix) {
			last = i
		}
	}
	return last
}

// ---- inspect output built in code (no fixture files) ----

type stateSpec struct {
	Running     bool
	Paused      bool
	Runtime     string
	Restart     string
	Source      string
	Dest        string
	MountType   string
	RW          bool
	ExtraMounts []string // destinations of additional bind mounts
	User        string
	CapDrop     []string
	CapAdd      []string
	SecurityOpt []string
	Memory      int64
	NanoCpus    int64
	PidsLimit   int64
	Tmpfs       map[string]string
	Networks    []string
	Std         []stdMount // the Claude state and resolv.conf binds
	Env         []string
}

type stdMount struct {
	Dest, Source string
	RW           bool
}

func okState(ws, claude, resolv string) stateSpec {
	return stateSpec{
		Running: true, Runtime: "runsc", Restart: "no",
		Source: ws, Dest: "/workspace", MountType: "bind", RW: true,
		Std: []stdMount{
			{"/home/agent/.claude", claude, true},
			{"/etc/resolv.conf", resolv, false},
		},
		Env:         []string{"PATH=/usr/bin", "CLAUDE_CONFIG_DIR=/home/agent/.claude"},
		User:        "",
		CapDrop:     []string{"ALL"},
		CapAdd:      append([]string(nil), config.Capabilities...),
		SecurityOpt: []string{"no-new-privileges"},
		Memory:      4294967296,
		NanoCpus:    2000000000,
		PidsLimit:   512,
		Tmpfs: map[string]string{
			"/tmp":            "rw,noexec,nosuid,size=512m",
			"/var/lib/docker": "rw,nosuid,size=2g",
		},
		Networks: []string{"agents-net"},
	}
}

func (s stateSpec) json() string {
	mounts := []map[string]any{{"Type": s.MountType, "Source": s.Source, "Destination": s.Dest, "RW": s.RW}}
	for _, m := range s.Std {
		mounts = append(mounts, map[string]any{"Type": "bind", "Source": m.Source, "Destination": m.Dest, "RW": m.RW})
	}
	for _, d := range s.ExtraMounts {
		mounts = append(mounts, map[string]any{"Type": "bind", "Source": "/host/extra", "Destination": d, "RW": true})
	}
	nets := map[string]any{}
	for _, n := range s.Networks {
		nets[n] = map[string]any{}
	}
	b, _ := json.Marshal(map[string]any{
		"State":  map[string]any{"Running": s.Running, "Paused": s.Paused},
		"Config": map[string]any{"User": s.User, "Env": s.Env},
		"HostConfig": map[string]any{
			"Runtime":       s.Runtime,
			"RestartPolicy": map[string]any{"Name": s.Restart},
			"CapDrop":       s.CapDrop,
			"CapAdd":        s.CapAdd,
			"SecurityOpt":   s.SecurityOpt,
			"Memory":        s.Memory,
			"NanoCpus":      s.NanoCpus,
			"PidsLimit":     s.PidsLimit,
			"Tmpfs":         s.Tmpfs,
		},
		"Mounts":          mounts,
		"NetworkSettings": map[string]any{"Networks": nets},
	})
	return string(b)
}

// ---- fake filesystem over a temp dir ----

type fakeFS struct {
	mkdirs   []string
	chowns   []string
	removed  []string // RemoveAll calls (remove_agent_container)
	mkdirErr error
	chownErr error
	// owner reports the uid owning a path; nil means every existing path is owned by uid 1001 (agent-operator).
	owner func(path string) int
}

func (f *fakeFS) IsDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// inShared reports a path under the shared state/etc/run dirs; those are not part of the workspace call records.
func inShared(path string) bool {
	return strings.HasPrefix(path, sharedBase+string(filepath.Separator))
}

func (f *fakeFS) MkdirAll(path string, perm os.FileMode) error {
	if !inShared(path) {
		f.mkdirs = append(f.mkdirs, path)
	}
	if f.mkdirErr != nil {
		return f.mkdirErr
	}
	return os.MkdirAll(path, perm)
}

// Mkdir creates one directory and fails with an exist error when present, like os.Mkdir.
func (f *fakeFS) Mkdir(path string, perm os.FileMode) error {
	if !inShared(path) {
		f.mkdirs = append(f.mkdirs, path)
	}
	if f.mkdirErr != nil {
		return f.mkdirErr
	}
	return os.Mkdir(path, perm)
}

func (f *fakeFS) Chown(path string, uid, gid int) error {
	if !inShared(path) {
		f.chowns = append(f.chowns, path)
	}
	return f.chownErr
}

func (f *fakeFS) Chmod(path string, perm os.FileMode) error { return os.Chmod(path, perm) }

func (f *fakeFS) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

func (f *fakeFS) WriteFile(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}

// Owner fails for a missing path (without following a final symlink) and reports uid 1001 by default.
func (f *fakeFS) Owner(path string) (int, int, error) {
	if _, err := os.Lstat(path); err != nil {
		return 0, 0, err
	}
	if f.owner != nil {
		return f.owner(path), 1001, nil
	}
	return 1001, 1001, nil
}

// ---- harness ----

const agentID = "dev-1"

// sharedBase holds the default runtime dir (with a valid ready marker), state root and host resolv.conf of the tests.
var sharedBase string

func TestMain(m *testing.M) {
	base, err := os.MkdirTemp("", "agent-sandbox-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	sharedBase = base
	if err := os.MkdirAll(filepath.Join(base, "run"), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(filepath.Join(base, "run", "ready"), []byte(dockerPID+"\n"), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(base)
	os.Exit(code)
}

// testConfig is the configuration of every test: temp runtime/state/resolv paths, short waits, `extra` overrides.
func testConfig(root string, extra map[string]string) config.Config {
	values := map[string]string{
		"AGENT_SANDBOX_RUNTIME_DIR":   filepath.Join(sharedBase, "run"),
		"AGENT_SANDBOX_STATE_ROOT":    filepath.Join(sharedBase, "state"),
		"AGENT_SANDBOX_HOST_RESOLV":   filepath.Join(sharedBase, "etc", "resolv.conf"),
		"AGENT_SANDBOX_READY_TIMEOUT": "200ms",
		"AGENT_SANDBOX_DOCKER_WAIT":   "200ms",
	}
	if root != "" {
		values["AGENT_SANDBOX_WORKSPACE_ROOT"] = root
	}
	for k, v := range extra {
		values[k] = v
	}
	return config.Load(func(k string) string { return values[k] })
}

type result struct {
	code        int
	stdout, err string
	ws          string
	fs          *fakeFS
}

// run uses a fresh temp dir as the Workspace-Agents root.
func run(t *testing.T, fr *fakeRunner, fs *fakeFS, id string, env map[string]string) result {
	t.Helper()
	return runAt(t, fr, fs, id, t.TempDir(), env)
}

// runAt uses `root` as the Workspace-Agents root unless env overrides it.
func runAt(t *testing.T, fr *fakeRunner, fs *fakeFS, id, root string, env map[string]string) result {
	t.Helper()
	cfg := testConfig(root, env)
	ws := filepath.Join(cfg.WorkspaceRoot, id)
	fr.ws = ws
	fr.claude = filepath.Join(cfg.StateRoot, id, "claude")
	fr.resolv = cfg.HostResolv
	var out, errb bytes.Buffer
	code := sequence.Run(context.Background(), sequence.Deps{
		Config:  cfg,
		Runner:  fr,
		FS:      fs,
		AgentID: id,
		Stdout:  &out,
		Stderr:  &errb,
	})
	if len(fr.unexpected) > 0 {
		t.Errorf("commands without a fake answer: %v", fr.unexpected)
	}
	return result{code, out.String(), errb.String(), ws, fs}
}

const successText = "agent container agent-dev-1 created"

func noSuccess(t *testing.T, r result) {
	t.Helper()
	if strings.Contains(r.stdout, "created") || strings.Contains(r.stdout, "workspace:") {
		t.Errorf("stdout must carry no success text on failure: %q", r.stdout)
	}
}

const inspectVerify = "docker container inspect --format {{json .}} agent-dev-1"

// ---- tests ----

func TestHappyPath(t *testing.T) {
	fr, fs := &fakeRunner{}, &fakeFS{}
	r := run(t, fr, fs, agentID, nil)
	if r.code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", r.code, r.err)
	}
	if !strings.Contains(r.stdout, successText) || !strings.Contains(r.stdout, r.ws) {
		t.Errorf("missing success report: %q", r.stdout)
	}
	if r.err != "" {
		t.Errorf("stderr should be empty: %q", r.err)
	}
	for _, want := range []string{"[1/14] validate agentID", "[2/14] acquire agent lock", "[3/14] wait for agent network ready",
		"[4/14] check preconditions", "[5/14] prepare workspace folder", "[6/14] prepare agent state and resolv.conf",
		"[7/14] clear leftover network state", "[8/14] create and start container", "[9/14] pause container", "[10/14] arm agent network",
		"[11/14] release container", "[12/14] check resolv.conf", "[13/14] verify final state", "[14/14] report success"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("missing progress line %q: %q", want, r.stdout)
		}
	}
	if last, text := strings.Index(r.stdout, "[14/14]"), strings.Index(r.stdout, successText); last < 0 || text < last {
		t.Errorf("success text must come only at the last step: %q", r.stdout)
	}
	for _, d := range []string{r.ws, filepath.Join(r.ws, ".output")} {
		if fi, err := os.Stat(d); err != nil || !fi.IsDir() {
			t.Errorf("folder %s not created: %v", d, err)
		}
	}
	wantDirs := []string{r.ws, filepath.Join(r.ws, ".output")}
	if strings.Join(fs.mkdirs, "|") != strings.Join(wantDirs, "|") {
		t.Errorf("mkdir calls = %v, want %v", fs.mkdirs, wantDirs)
	}
	if strings.Join(fs.chowns, "|") != strings.Join(wantDirs, "|") {
		t.Errorf("chown calls = %v, want %v", fs.chowns, wantDirs)
	}
	// Before the container exists: its name-keyed disarm, then the reconcile of the gate set.
	cleanDisarm, released, create0 := fr.index("/usr/local/bin/create_agent_net disarm agent-dev-1"), fr.index("/usr/local/bin/create_agent_net released"), fr.index("docker create ")
	if !(cleanDisarm >= 0 && cleanDisarm < released && released < create0) {
		t.Errorf("order disarm=%d released=%d create=%d", cleanDisarm, released, create0)
	}
	create := fr.index("docker create ")
	start := fr.index("docker start agent-dev-1")
	pause := fr.index("docker pause agent-dev-1")
	pauseCheck := fr.index("docker container inspect --format")
	arm := fr.index("/usr/local/bin/create_agent_net arm agent-dev-1")
	gate := fr.index("/usr/local/bin/create_agent_net check-gate agent-dev-1")
	unpause := fr.index("docker unpause agent-dev-1")
	resolvCheck := fr.index("docker exec -u 0:0 agent-dev-1 cat /etc/resolv.conf")
	verify := fr.lastIndex(inspectVerify)
	order := []int{create, start, pause, pauseCheck, arm, gate, unpause, resolvCheck, verify}
	for i := 1; i < len(order); i++ {
		if order[i-1] < 0 || order[i-1] >= order[i] {
			t.Fatalf("order create,start,pause,pause-check,arm,check-gate,unpause,resolv-check,verify = %v", order)
		}
	}
	if fr.has("docker rm") {
		t.Error("nothing may be removed on success")
	}
	if fr.count("docker pause") != 1 || fr.count("docker unpause") != 1 {
		t.Errorf("exactly one pause and one unpause expected: %v", fr.calls)
	}
}

func TestCreateArgumentShape(t *testing.T) {
	fr := &fakeRunner{}
	r := run(t, fr, &fakeFS{}, agentID, nil)
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	var create string
	for _, c := range fr.calls {
		if strings.HasPrefix(c.joined(), "docker create ") {
			create = c.joined()
		}
	}
	for _, want := range []string{
		"docker create --name agent-dev-1 --runtime=runsc --restart no --network agents-net --cap-drop ALL",
		"--security-opt no-new-privileges", "--memory 4g", "--cpus 2",
		"--pids-limit 512 --tmpfs /tmp:rw,noexec,nosuid,size=512m --tmpfs /var/lib/docker:rw,nosuid,size=2g",
		"-v " + r.ws + ":/workspace:rw", " agent-image:latest",
	} {
		if !strings.Contains(create, want) {
			t.Errorf("create lacks %q: %s", want, create)
		}
	}
	if got := strings.Count(create, "--cap-add"); got != 15 {
		t.Errorf("cap-add count = %d, want 15", got)
	}
	if strings.Count(create, "--restart") != 1 || !strings.Contains(create, "--restart no ") {
		t.Errorf("create must set the restart policy to no, exactly once: %s", create)
	}
	if strings.Contains(create, "--init") {
		t.Error("create must not contain --init")
	}
}

func TestInvalidIDExits1WithoutDockerCalls(t *testing.T) {
	for _, id := range []string{"", "Agent", "1abc", "a_b", "a b", "a/b", strings.Repeat("a", 32)} {
		fr, fs := &fakeRunner{}, &fakeFS{}
		r := run(t, fr, fs, id, nil)
		if r.code != 1 {
			t.Errorf("id %q: exit %d, want 1", id, r.code)
		}
		if !strings.Contains(r.err, "usage: create_agent_container") {
			t.Errorf("id %q: stderr %q", id, r.err)
		}
		if len(fr.calls) != 0 || len(fs.mkdirs) != 0 {
			t.Errorf("id %q: no engine or fs call allowed, got %v", id, fr.calls)
		}
		noSuccess(t, r)
	}
}

func TestMaxLengthIDAccepted(t *testing.T) {
	fr := &fakeRunner{}
	r := run(t, fr, &fakeFS{}, "a"+strings.Repeat("b", 30), nil)
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
}

func TestRuntimeMissingExits1(t *testing.T) {
	fr := &fakeRunner{rules: []rule{{contains: "docker info"}}} // empty output: no runtime list
	r := run(t, fr, &fakeFS{}, agentID, nil)
	if r.code != 1 {
		t.Fatalf("exit %d, want 1", r.code)
	}
	noSuccess(t, r)
	if fr.has("docker create") {
		t.Error("create must not run")
	}
}

func TestRunscAbsentFromRuntimesExits1(t *testing.T) {
	fr := &fakeRunner{}
	r := runWithRuntimes(t, fr, `{"runc":{"path":"runc"}}`)
	if r.code != 1 || !strings.Contains(r.err, "runtime mismatch") || !strings.Contains(r.err, "daemon.json") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.has("docker create") || fr.has("docker container inspect") {
		t.Errorf("nothing may follow the mismatch: %v", fr.calls)
	}
	noSuccess(t, r)
}

// runtimeRunner overrides the docker info answer and delegates the rest.
type runtimeRunner struct {
	inner    *fakeRunner
	runtimes string
}

func (r runtimeRunner) Run(ctx context.Context, stdin []byte, args ...string) ([]byte, []byte, int, error) {
	if len(args) > 1 && args[0] == "docker" && args[1] == "info" {
		r.inner.mu.Lock()
		r.inner.calls = append(r.inner.calls, call{Args: append([]string(nil), args...)})
		r.inner.mu.Unlock()
		return []byte(r.runtimes), nil, 0, nil
	}
	return r.inner.Run(ctx, stdin, args...)
}

func runWithRuntimes(t *testing.T, fr *fakeRunner, runtimes string) result {
	t.Helper()
	cfg := testConfig(t.TempDir(), nil)
	fr.ws = filepath.Join(cfg.WorkspaceRoot, agentID)
	fr.claude = filepath.Join(cfg.StateRoot, agentID, "claude")
	fr.resolv = cfg.HostResolv
	var out, errb bytes.Buffer
	code := sequence.Run(context.Background(), sequence.Deps{
		Config: cfg, Runner: runtimeRunner{inner: fr, runtimes: runtimes}, FS: &fakeFS{},
		AgentID: agentID, Stdout: &out, Stderr: &errb,
	})
	return result{code: code, stdout: out.String(), err: errb.String()}
}

func TestEngineDownExits1(t *testing.T) {
	fr := &fakeRunner{rules: []rule{{contains: "docker info", stderr: "Cannot connect to the Docker daemon", code: 1}}}
	r := run(t, fr, &fakeFS{}, agentID, nil)
	if r.code != 1 || !strings.Contains(r.err, "Cannot connect to the Docker daemon") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	noSuccess(t, r)
	if fr.count("docker") != 1 {
		t.Errorf("only the runtime check may run: %v", fr.calls)
	}
}

func TestDockerBinaryMissingExits1(t *testing.T) {
	fr := &fakeRunner{rules: []rule{{contains: "docker info", code: -1, err: errors.New("executable file not found")}}}
	r := run(t, fr, &fakeFS{}, agentID, nil)
	if r.code != 1 || !strings.Contains(r.err, "executable file not found") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	noSuccess(t, r)
}

func TestDuplicateContainerExits1(t *testing.T) {
	fr, fs := &fakeRunner{existing: true}, &fakeFS{}
	r := run(t, fr, fs, agentID, nil)
	if r.code != 1 || !strings.Contains(r.err, "agent-dev-1 already exists") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	for _, p := range []string{"docker create", "docker start", "docker rm", "docker pause", "docker network", "docker image"} {
		if fr.has(p) {
			t.Errorf("%s must not run", p)
		}
	}
	if len(fs.mkdirs) != 0 {
		t.Error("workspace must not be touched")
	}
	noSuccess(t, r)
}

func TestDuplicateCheckEngineErrorIsNotAbsence(t *testing.T) {
	fr := &fakeRunner{rules: []rule{{contains: "docker container inspect agent-dev-1", stderr: "Cannot connect to the Docker daemon", code: 1}}}
	fs := &fakeFS{}
	r := run(t, fr, fs, agentID, nil)
	if r.code != 2 || !strings.Contains(r.err, "cannot check for an existing container") || !strings.Contains(r.err, "Cannot connect to the Docker daemon") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.has("docker create") || fr.has("docker network") || len(fs.mkdirs) != 0 {
		t.Errorf("nothing may follow an unreadable duplicate check: %v", fr.calls)
	}
	noSuccess(t, r)
}

func TestMissingPrerequisitesExit1(t *testing.T) {
	cases := []struct {
		name  string
		rules []rule
		env   map[string]string
		want  string
	}{
		{"network", []rule{{contains: "docker network inspect agents-net", stderr: "Error: No such network: agents-net", code: 1}}, nil, "network agents-net"},
		{"image", []rule{{contains: "docker image inspect agent-image:latest", stderr: "Error: No such image: agent-image:latest", code: 1}}, nil, "image agent-image:latest"},
		{"workspace root", nil, map[string]string{"AGENT_SANDBOX_WORKSPACE_ROOT": "/nonexistent/Workspace-Agents"}, "workspace folder /nonexistent/Workspace-Agents"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fr, fs := &fakeRunner{rules: c.rules}, &fakeFS{}
			r := run(t, fr, fs, agentID, c.env)
			if r.code != 1 || !strings.Contains(r.err, c.want) {
				t.Fatalf("exit %d, stderr %q", r.code, r.err)
			}
			if fr.has("docker create") || fr.has("docker pull") || fr.has("docker network create") || len(fs.mkdirs) != 0 {
				t.Errorf("nothing may be created or pulled: %v", fr.calls)
			}
			noSuccess(t, r)
		})
	}
}

func TestAllMissingPrerequisitesAreNamed(t *testing.T) {
	fr := &fakeRunner{rules: []rule{
		{contains: "docker network inspect", stderr: "Error: No such network", code: 1},
		{contains: "docker image inspect", stderr: "Error: No such image", code: 1},
	}}
	r := run(t, fr, &fakeFS{}, agentID, map[string]string{"AGENT_SANDBOX_WORKSPACE_ROOT": "/nonexistent/ws"})
	if r.code != 1 {
		t.Fatalf("exit %d", r.code)
	}
	for _, want := range []string{"network agents-net", "image agent-image:latest", "workspace folder /nonexistent/ws"} {
		if !strings.Contains(r.err, want) {
			t.Errorf("stderr lacks %q:\n%s", want, r.err)
		}
	}
}

// ---- Workspace-Agents root preconditions (SFR-AGC-023) ----

func TestWorkspaceRootPreconditionsExit1WithNoChange(t *testing.T) {
	notDir := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(notDir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "root-link")
	if err := os.Symlink(t.TempDir(), link); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		root string
		fs   *fakeFS
		want string
	}{
		{"missing", filepath.Join(t.TempDir(), "absent"), &fakeFS{}, "does not exist"},
		{"symlink", link, &fakeFS{}, "is a symbolic link"},
		{"not a directory", notDir, &fakeFS{}, "is not a directory"},
		{"wrong owner", t.TempDir(), &fakeFS{owner: func(string) int { return 0 }}, "is owned by uid 0, want 1001"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fr := &fakeRunner{}
			r := runAt(t, fr, c.fs, agentID, c.root, nil)
			if r.code != 1 || !strings.Contains(r.err, "workspace folder "+c.root) || !strings.Contains(r.err, c.want) {
				t.Fatalf("exit %d, stderr %q", r.code, r.err)
			}
			if len(c.fs.mkdirs) != 0 || len(c.fs.chowns) != 0 {
				t.Errorf("no folder may be created or changed: mkdir %v chown %v", c.fs.mkdirs, c.fs.chowns)
			}
			if fr.has("docker create") || fr.has("docker pause") {
				t.Errorf("nothing may be created: %v", fr.calls)
			}
			if !strings.Contains(r.err, "NOT_CREATED") {
				t.Errorf("exit state NOT_CREATED expected: %q", r.err)
			}
			noSuccess(t, r)
		})
	}
}

// ---- workspace and .output folder (SFR-AGC-004/005) ----

func TestWorkspaceFailureExits2WithoutCreate(t *testing.T) {
	for name, fs := range map[string]*fakeFS{
		"mkdir": {mkdirErr: errors.New("permission denied")},
		"chown": {chownErr: errors.New("operation not permitted")},
	} {
		t.Run(name, func(t *testing.T) {
			fr := &fakeRunner{}
			r := run(t, fr, fs, agentID, nil)
			if r.code != 2 {
				t.Fatalf("exit %d, want 2; stderr %q", r.code, r.err)
			}
			if fr.has("docker create") || fr.has("docker start") {
				t.Error("nothing may be created or started")
			}
			noSuccess(t, r)
		})
	}
}

func TestExistingWorkspaceIsReused(t *testing.T) {
	root := t.TempDir()
	ws := filepath.Join(root, agentID)
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(ws, "keep.txt")
	if err := os.WriteFile(keep, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := &fakeFS{}
	r := runAt(t, &fakeRunner{}, fs, agentID, root, nil)
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("existing contents must be kept: %v", err)
	}
	output := filepath.Join(ws, ".output")
	if fi, err := os.Stat(output); err != nil || !fi.IsDir() {
		t.Errorf(".output must be created inside an existing workspace: %v", err)
	}
	if len(fs.chowns) != 2 || fs.chowns[0] != ws || fs.chowns[1] != output {
		t.Errorf("chown calls = %v", fs.chowns)
	}
}

func TestExistingOutputFolderIsReused(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, agentID, ".output")
	if err := os.MkdirAll(output, 0o755); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(output, "result.txt")
	if err := os.WriteFile(keep, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := &fakeFS{}
	r := runAt(t, &fakeRunner{}, fs, agentID, root, nil)
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("existing .output contents must be kept: %v", err)
	}
	if len(fs.chowns) != 2 {
		t.Errorf("both folders are handed to agent-operator, chown calls = %v", fs.chowns)
	}
}

func TestWorkspaceOrOutputThatIsASymlinkExits2(t *testing.T) {
	t.Run("workspace", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Symlink(t.TempDir(), filepath.Join(root, agentID)); err != nil {
			t.Fatal(err)
		}
		fr, fs := &fakeRunner{}, &fakeFS{}
		r := runAt(t, fr, fs, agentID, root, nil)
		if r.code != 2 || !strings.Contains(r.err, "not a real directory") {
			t.Fatalf("exit %d, stderr %q", r.code, r.err)
		}
		if len(fs.chowns) != 0 || fr.has("docker create") {
			t.Errorf("nothing may be changed or created: chown %v calls %v", fs.chowns, fr.calls)
		}
		noSuccess(t, r)
	})
	t.Run("output", func(t *testing.T) {
		root := t.TempDir()
		ws := filepath.Join(root, agentID)
		if err := os.MkdirAll(ws, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(t.TempDir(), filepath.Join(ws, ".output")); err != nil {
			t.Fatal(err)
		}
		fr, fs := &fakeRunner{}, &fakeFS{}
		r := runAt(t, fr, fs, agentID, root, nil)
		if r.code != 2 || !strings.Contains(r.err, "not a real directory") {
			t.Fatalf("exit %d, stderr %q", r.code, r.err)
		}
		if len(fs.chowns) != 0 || fr.has("docker create") {
			t.Errorf("nothing may be changed or created: chown %v calls %v", fs.chowns, fr.calls)
		}
		noSuccess(t, r)
	})
}

// ---- create and start ----

func TestStartFailureRemovesOwnContainer(t *testing.T) {
	fr := &fakeRunner{rules: []rule{{contains: "docker start agent-dev-1", stderr: "runsc: start failed", code: 125}}}
	r := run(t, fr, &fakeFS{}, agentID, nil)
	if r.code != 2 || !strings.Contains(r.err, "runsc: start failed") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.count("docker rm -f agent-dev-1") != 1 {
		t.Errorf("expected one rm of own container: %v", fr.calls)
	}
	if fr.has("docker pause") || fr.has("/usr/local/bin/create_agent_net arm") {
		t.Error("pause and arming must not run after a start failure")
	}
	noSuccess(t, r)
}

func TestCreateFailureExits2WithoutRemove(t *testing.T) {
	fr := &fakeRunner{rules: []rule{{contains: "docker create", stderr: "no such image layer", code: 1}}}
	r := run(t, fr, &fakeFS{}, agentID, nil)
	if r.code != 2 || fr.has("docker start") || fr.has("docker rm") {
		t.Fatalf("exit %d, calls %v", r.code, fr.calls)
	}
	noSuccess(t, r)
}

// ---- pause gate (SFR-AGC-012) ----

func TestPauseFailureRemovesOwnContainerExits2(t *testing.T) {
	fr := &fakeRunner{rules: []rule{{contains: "docker pause", stderr: "cannot pause", code: 1}}}
	r := run(t, fr, &fakeFS{}, agentID, nil)
	if r.code != 2 || !strings.Contains(r.err, "cannot pause") || !strings.Contains(r.err, "REMOVED") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.count("docker rm -f agent-dev-1") != 1 {
		t.Errorf("expected one rm of own container: %v", fr.calls)
	}
	if fr.has("/usr/local/bin/create_agent_net arm") || fr.has("docker unpause") {
		t.Errorf("no arming and no unpause after a pause failure: %v", fr.calls)
	}
	noSuccess(t, r)
}

func TestPauseNotVerifiedRemovesOwnContainerExits2(t *testing.T) {
	fr := &fakeRunner{mutate: func(s *stateSpec) { s.Paused = false }}
	r := run(t, fr, &fakeFS{}, agentID, nil)
	if r.code != 2 || !strings.Contains(r.err, "State.Paused is false") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.count("docker rm -f agent-dev-1") != 1 || fr.has("/usr/local/bin/create_agent_net arm") {
		t.Errorf("own container removed, no arming: %v", fr.calls)
	}
	noSuccess(t, r)
}

// ---- arming and check-gate (SFR-AGC-007/008) ----

func TestArmFailureDisarmsThenRemovesOwnContainerExits2(t *testing.T) {
	fr := &fakeRunner{rules: []rule{{contains: "create_agent_net arm", stderr: "nft: rule rejected", code: 3}}}
	r := run(t, fr, &fakeFS{}, agentID, nil)
	if r.code != 2 {
		t.Fatalf("exit %d, want 2", r.code)
	}
	for _, want := range []string{"arming", "nft: rule rejected", "REMOVED"} {
		if !strings.Contains(r.err, want) {
			t.Errorf("stderr lacks %q:\n%s", want, r.err)
		}
	}
	disarm := fr.lastIndex("/usr/local/bin/create_agent_net disarm agent-dev-1") // the first one clears leftovers before the create
	rm := fr.index("docker rm -f agent-dev-1")
	if disarm < 0 || rm < 0 || disarm > rm || fr.count("/usr/local/bin/create_agent_net disarm agent-dev-1") != 2 {
		t.Errorf("disarm must precede rm: disarm=%d rm=%d calls=%v", disarm, rm, fr.calls)
	}
	if fr.count("docker rm -f agent-dev-1") != 1 {
		t.Errorf("expected one rm of own container: %v", fr.calls)
	}
	for _, c := range fr.calls {
		if strings.HasPrefix(c.joined(), "docker rm") && c.joined() != "docker rm -f agent-dev-1" {
			t.Errorf("only the own container may be removed: %s", c.joined())
		}
	}
	if fr.has("/usr/local/bin/create_agent_net check-gate") || fr.has("docker exec") {
		t.Errorf("no check-gate or exec after an arming failure: %v", fr.calls)
	}
	// The only unpause is the one inside the removal, after the disarm and right before the rm.
	if up := fr.index("docker unpause agent-dev-1"); fr.count("docker unpause") != 1 || up < disarm || up > rm {
		t.Errorf("unpause only between disarm and rm: %v", fr.calls)
	}
	noSuccess(t, r)
}

func TestArmFailureWithDisarmFailureRemovesNothing(t *testing.T) {
	fr := &fakeRunner{rules: []rule{
		{contains: "create_agent_net arm", stderr: "nft: rule rejected", code: 3},
		{contains: "create_agent_net disarm", stderr: "nft: busy", code: 2, skip: 1}, // the clean-up before the create passes
	}}
	r := run(t, fr, &fakeFS{}, agentID, nil)
	if r.code != 2 || !strings.Contains(r.err, "DISARM_FAILED") || !strings.Contains(r.err, "nft: busy") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.has("docker rm") {
		t.Errorf("nothing may be removed when the disarm failed: %v", fr.calls)
	}
	if fr.has("docker unpause") {
		t.Error("the container must stay paused")
	}
	noSuccess(t, r)
}

func TestCheckGateFailureDisarmsThenRemovesOwnContainer(t *testing.T) {
	for _, code := range []int{1, 2} {
		code := code
		t.Run("exit "+string(rune('0'+code)), func(t *testing.T) {
			fr := &fakeRunner{rules: []rule{{contains: "create_agent_net check-gate", stdout: "control forward-drop: missing\n", code: code}}}
			r := run(t, fr, &fakeFS{}, agentID, nil)
			if r.code != 2 {
				t.Fatalf("exit %d, want 2; stderr %q", r.code, r.err)
			}
			for _, want := range []string{"arming read-back", "FAILED CHECK: control forward-drop: missing", "REMOVED"} {
				if !strings.Contains(r.err, want) {
					t.Errorf("stderr lacks %q:\n%s", want, r.err)
				}
			}
			arm := fr.index("/usr/local/bin/create_agent_net arm agent-dev-1")
			gate := fr.index("/usr/local/bin/create_agent_net check-gate agent-dev-1")
			disarm := fr.lastIndex("/usr/local/bin/create_agent_net disarm agent-dev-1")
			rm := fr.index("docker rm -f agent-dev-1")
			if !(arm >= 0 && arm < gate && gate < disarm && disarm < rm) {
				t.Errorf("order arm=%d check-gate=%d disarm=%d rm=%d", arm, gate, disarm, rm)
			}
			if fr.has("docker exec") {
				t.Errorf("the container must never be released: %v", fr.calls)
			}
			// The only unpause is the one inside the removal, after the disarm and right before the rm.
			if up := fr.index("docker unpause agent-dev-1"); fr.count("docker unpause") != 1 || up < disarm || up > rm {
				t.Errorf("unpause only between disarm and rm: %v", fr.calls)
			}
			noSuccess(t, r)
		})
	}
}

func TestCheckGateFailureWithDisarmFailureRemovesNothing(t *testing.T) {
	fr := &fakeRunner{rules: []rule{
		{contains: "create_agent_net check-gate", stdout: "control forward-drop: missing\n", code: 2},
		{contains: "create_agent_net disarm", stderr: "nft: busy", code: 2, skip: 1}, // the clean-up before the create passes
	}}
	r := run(t, fr, &fakeFS{}, agentID, nil)
	if r.code != 2 || !strings.Contains(r.err, "DISARM_FAILED") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.has("docker rm") || fr.has("docker unpause") {
		t.Errorf("nothing removed and nothing released: %v", fr.calls)
	}
	noSuccess(t, r)
}

func TestArmBinaryMissingExits2(t *testing.T) {
	fr := &fakeRunner{rules: []rule{{contains: "create_agent_net arm", code: -1, err: errors.New("no such file or directory")}}}
	r := run(t, fr, &fakeFS{}, agentID, nil)
	if r.code != 2 || !strings.Contains(r.err, "no such file or directory") || !fr.has("docker rm -f agent-dev-1") {
		t.Fatalf("exit %d, stderr %q, calls %v", r.code, r.err, fr.calls)
	}
}

func TestArmPathIsConfigurable(t *testing.T) {
	fr := &fakeRunner{}
	r := run(t, fr, &fakeFS{}, agentID, map[string]string{"AGENT_SANDBOX_ARM_BIN": "/opt/arm"})
	if r.code != 0 || !fr.has("/opt/arm arm agent-dev-1") || !fr.has("/opt/arm check-gate agent-dev-1") {
		t.Fatalf("exit %d, calls %v", r.code, fr.calls)
	}
}

// ---- release (SFR-AGC-021) ----

func TestUnpauseFailureStillPausedIsContainedNotRemoved(t *testing.T) {
	fr := &fakeRunner{rules: []rule{{contains: "docker unpause", stderr: "cannot unpause", code: 1}}}
	r := run(t, fr, &fakeFS{}, agentID, nil)
	if r.code != 2 || !strings.Contains(r.err, "cannot unpause") || !strings.Contains(r.err, "exit_state: CONTAINED") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.has("docker rm") || fr.has("docker exec") {
		t.Errorf("no removal and no resolv.conf write: %v", fr.calls)
	}
	if fr.count("docker pause") != 1 {
		t.Errorf("a still-paused container needs no second pause: %v", fr.calls)
	}
	noSuccess(t, r)
}

func TestUnpauseFailureNotPausedIsRepaused(t *testing.T) {
	fr := &fakeRunner{rules: []rule{{contains: "docker unpause", stderr: "cannot unpause", code: 1, pausedAfter: boolPtr(false)}}}
	r := run(t, fr, &fakeFS{}, agentID, nil)
	if r.code != 2 || !strings.Contains(r.err, "exit_state: CONTAINED") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.count("docker pause agent-dev-1") != 2 {
		t.Errorf("the container must be re-paused: %v", fr.calls)
	}
	if fr.has("docker rm") || fr.has("docker exec") {
		t.Errorf("no removal and no resolv.conf write: %v", fr.calls)
	}
	noSuccess(t, r)
}

func TestUnpauseFailureNotPausedAndRepauseFailsIsUnsafe(t *testing.T) {
	fr := &fakeRunner{rules: []rule{
		{contains: "docker unpause", stderr: "cannot unpause", code: 1, pausedAfter: boolPtr(false)},
		{contains: "docker pause", skip: 1, stderr: "cannot pause", code: 1},
	}}
	r := run(t, fr, &fakeFS{}, agentID, nil)
	if r.code != 2 || !strings.Contains(r.err, "UNSAFE") || fr.has("docker rm") {
		t.Fatalf("exit %d, stderr %q, calls %v", r.code, r.err, fr.calls)
	}
	noSuccess(t, r)
}

// ---- final verification (SFR-AGC-009) ----

func TestVerifyMismatchContainsExits2(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*stateSpec)
		rules  []rule
		fs     *fakeFS
		want   string
	}{
		{name: "not running", mutate: func(s *stateSpec) { s.Running = false }, want: "State.Running"},
		{name: "wrong runtime", mutate: func(s *stateSpec) { s.Runtime = "runc" }, want: "HostConfig.Runtime"},
		{name: "restart policy", mutate: func(s *stateSpec) { s.Restart = "always" }, want: "RestartPolicy"},
		{name: "no workspace mount", mutate: func(s *stateSpec) { s.Dest = "/other" }, want: "no mount at /workspace"},
		{name: "wrong source", mutate: func(s *stateSpec) { s.Source = "/somewhere/else" }, want: "has source"},
		{name: "read-only", mutate: func(s *stateSpec) { s.RW = false }, want: "not read-write"},
		{name: "mount type", mutate: func(s *stateSpec) { s.MountType = "volume" }, want: `has type "volume"`},
		{name: "extra mount", mutate: func(s *stateSpec) { s.ExtraMounts = []string{"/data"} }, want: "unexpected mount at /data"},
		{name: "config user", mutate: func(s *stateSpec) { s.User = "1001" }, want: "Config.User"},
		{name: "cap drop", mutate: func(s *stateSpec) { s.CapDrop = nil }, want: "HostConfig.CapDrop"},
		{name: "extra cap", mutate: func(s *stateSpec) { s.CapAdd = append(s.CapAdd, "NET_ADMIN") }, want: "HostConfig.CapAdd"},
		{name: "missing cap", mutate: func(s *stateSpec) { s.CapAdd = s.CapAdd[1:] }, want: "HostConfig.CapAdd"},
		{name: "security opt", mutate: func(s *stateSpec) { s.SecurityOpt = nil }, want: "HostConfig.SecurityOpt"},
		{name: "memory", mutate: func(s *stateSpec) { s.Memory = 1073741824 }, want: "HostConfig.Memory"},
		{name: "cpus", mutate: func(s *stateSpec) { s.NanoCpus = 1000000000 }, want: "HostConfig.NanoCpus"},
		{name: "pids limit", mutate: func(s *stateSpec) { s.PidsLimit = 1024 }, want: "HostConfig.PidsLimit is 1024"},
		{name: "tmpfs missing", mutate: func(s *stateSpec) { s.Tmpfs = nil }, want: "HostConfig.Tmpfs"},
		{name: "tmpfs extra", mutate: func(s *stateSpec) { s.Tmpfs["/extra"] = "rw" }, want: "HostConfig.Tmpfs"},
		{name: "extra network", mutate: func(s *stateSpec) { s.Networks = []string{"agents-net", "bridge"} }, want: "NetworkSettings.Networks"},
		{name: "no network", mutate: func(s *stateSpec) { s.Networks = nil }, want: "NetworkSettings.Networks"},
		{name: "wrong uid", rules: []rule{{contains: "agent-dev-1 id", stdout: "uid=0(root) gid=1001(agent) groups=1001(agent),999(docker)\n"}}, want: "want uid 1001"},
		{name: "docker group missing", rules: []rule{{contains: "agent-dev-1 id -nG", stdout: "agent\n"}}, want: "want group docker"},
		{name: "id exec fails", rules: []rule{{contains: "agent-dev-1 id -nG", stderr: "exec failed", code: 1}}, want: "id -nG failed"},
		{name: "resolv content", rules: []rule{{contains: "docker exec -u 1001 agent-dev-1 cat /etc/resolv.conf", stdout: "nameserver 127.0.0.11\n"}}, want: "content is"},
		{name: "claude mount source", mutate: func(s *stateSpec) { s.Std[0].Source = "/somewhere/else" }, want: "mount /home/agent/.claude has source"},
		{name: "claude mount read-only", mutate: func(s *stateSpec) { s.Std[0].RW = false }, want: "mount /home/agent/.claude is not read-write"},
		{name: "resolv mount writable", mutate: func(s *stateSpec) { s.Std[1].RW = true }, want: "mount /etc/resolv.conf is not read-only"},
		{name: "resolv mount missing", mutate: func(s *stateSpec) { s.Std = s.Std[:1] }, want: "no mount at /etc/resolv.conf"},
		{name: "claude env missing", mutate: func(s *stateSpec) { s.Env = nil }, want: "Config.Env lacks CLAUDE_CONFIG_DIR=/home/agent/.claude"},
		{name: "output owner", fs: &fakeFS{owner: func(p string) int {
			if filepath.Base(p) == ".output" {
				return 0
			}
			return 1001
		}}, want: "is owned by uid 0, want 1001"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fr := &fakeRunner{mutate: c.mutate, rules: c.rules}
			fs := c.fs
			if fs == nil {
				fs = &fakeFS{}
			}
			r := run(t, fr, fs, agentID, nil)
			if r.code != 2 || !strings.Contains(r.err, c.want) || !strings.Contains(r.err, "exit_state: CONTAINED") {
				t.Fatalf("exit %d, stderr %q", r.code, r.err)
			}
			if fr.count("docker pause agent-dev-1") != 2 {
				t.Errorf("container must be re-paused: %v", fr.calls)
			}
			if fr.has("docker rm") {
				t.Errorf("a verification mismatch never removes the container: %v", fr.calls)
			}
			noSuccess(t, r)
		})
	}
}

func TestVerifyListsEveryMismatch(t *testing.T) {
	fr := &fakeRunner{mutate: func(s *stateSpec) {
		s.Running, s.Runtime, s.RW = false, "runc", false
		s.ExtraMounts = []string{"/data"}
		s.PidsLimit = 1024
	}}
	r := run(t, fr, &fakeFS{}, agentID, nil)
	if r.code != 2 {
		t.Fatalf("exit %d", r.code)
	}
	for _, want := range []string{"State.Running", "HostConfig.Runtime", "not read-write", "unexpected mount at /data", "HostConfig.PidsLimit"} {
		if !strings.Contains(r.err, want) {
			t.Errorf("stderr lacks %q:\n%s", want, r.err)
		}
	}
}

func TestVerifyEmptyRestartPolicyRejected(t *testing.T) {
	fr := &fakeRunner{mutate: func(s *stateSpec) { s.Restart = "" }}
	if r := run(t, fr, &fakeFS{}, agentID, nil); r.code != 2 || !strings.Contains(r.err, `RestartPolicy.Name is "", want "no"`) {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
}

func TestVerifyUnreadableExits2(t *testing.T) {
	fr := &fakeRunner{rawVerify: "not json"}
	r := run(t, fr, &fakeFS{}, agentID, nil)
	if r.code != 2 || !strings.Contains(r.err, "unreadable") || !strings.Contains(r.err, "exit_state: CONTAINED") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.has("docker rm") {
		t.Errorf("no removal: %v", fr.calls)
	}
	noSuccess(t, r)
}

func TestVerifyInspectFailureExits2(t *testing.T) {
	// The first full inspect (pause check) passes; the final one fails; the containment inspect passes again.
	fr := &fakeRunner{rules: []rule{{contains: "container inspect --format", skip: 1, limit: 1, stderr: "boom", code: 1}}}
	r := run(t, fr, &fakeFS{}, agentID, nil)
	if r.code != 2 || !strings.Contains(r.err, "boom") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.has("docker rm") {
		t.Errorf("no removal: %v", fr.calls)
	}
	noSuccess(t, r)
}

// ---- /etc/resolv.conf read-only bind, check and read-back (SFR-AGC-024, SFR-NET-020) ----

const resolvCheckCmd = "docker exec -u 0:0 agent-dev-1 cat /etc/resolv.conf"

func TestResolvConfCheckedAndVerified(t *testing.T) {
	fr := &fakeRunner{}
	if r := run(t, fr, &fakeFS{}, agentID, nil); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	if fr.count(resolvCheckCmd) != 1 {
		t.Errorf("expected the uid 0 read exec once: %v", fr.calls)
	}
	if fr.has("docker exec -u 0:0 agent-dev-1 sh -c") {
		t.Errorf("nothing may be written into the container: %v", fr.calls)
	}
	for _, p := range []string{
		"docker exec -u 1001 agent-dev-1 id",
		"docker exec -u 1001 agent-dev-1 id -nG",
		"docker exec -u 1001 agent-dev-1 cat /etc/resolv.conf",
		"docker exec -u 1001 agent-dev-1 stat -c",
		"docker exec -u 1001 agent-dev-1 sh -c : >> /etc/resolv.conf",
	} {
		if !fr.has(p) {
			t.Errorf("missing read-back call %q", p)
		}
	}
	if fr.count("docker pause") != 1 {
		t.Error("only the pause gate may pause on success")
	}
}

func TestResolvConfOrdering(t *testing.T) {
	fr := &fakeRunner{}
	if r := run(t, fr, &fakeFS{}, agentID, nil); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	unpause := fr.index("docker unpause agent-dev-1")
	check := fr.index(resolvCheckCmd)
	verify := fr.lastIndex(inspectVerify)
	cat := fr.index("docker exec -u 1001 agent-dev-1 cat")
	if !(unpause >= 0 && unpause < check && check < verify && verify < cat) {
		t.Errorf("order unpause=%d check=%d verify=%d read-back=%d", unpause, check, verify, cat)
	}
}

func TestResolvConfCheckFailureContainsExits2(t *testing.T) {
	cases := map[string]rule{
		"non-zero exit":    {contains: "docker exec -u 0:0", stderr: "cat: not found", code: 1},
		"cannot be run":    {contains: "docker exec -u 0:0", code: -1, err: errors.New("docker: not found")},
		"embedded":         {contains: "docker exec -u 0:0", stdout: "nameserver 127.0.0.11\noptions ndots:0\n"},
		"embedded + valid": {contains: "docker exec -u 0:0", stdout: resolvGood + "nameserver 127.0.0.11\n"},
		"missing second":   {contains: "docker exec -u 0:0", stdout: "nameserver 1.1.1.1\n"},
		"empty":            {contains: "docker exec -u 0:0", stdout: ""},
	}
	for name, rl := range cases {
		t.Run(name, func(t *testing.T) {
			fr := &fakeRunner{rules: []rule{rl}}
			r := run(t, fr, &fakeFS{}, agentID, nil)
			if r.code != 2 || !strings.Contains(r.err, "/etc/resolv.conf") || !strings.Contains(r.err, "CONTAINED") {
				t.Fatalf("exit %d, stderr %q", r.code, r.err)
			}
			if fr.count("docker pause agent-dev-1") != 2 {
				t.Error("container must be re-paused")
			}
			if fr.has("docker rm") || fr.has("docker exec -u 1001 ") {
				t.Errorf("no removal and no read-back after a failed write: %v", fr.calls)
			}
			noSuccess(t, r)
		})
	}
}

func TestResolvConfCheckFailureUnsafeWhenPauseFails(t *testing.T) {
	fr := &fakeRunner{rules: []rule{
		{contains: "docker exec -u 0:0", code: 1},
		{contains: "docker pause", skip: 1, stderr: "cannot pause", code: 1}, // the pause gate passes, the re-pause fails
	}}
	r := run(t, fr, &fakeFS{}, agentID, nil)
	if r.code != 2 || !strings.Contains(r.err, "UNSAFE") || fr.has("docker rm") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	noSuccess(t, r)
}

func TestResolvConfReadBackMismatchContainsExits2(t *testing.T) {
	cases := []struct {
		name string
		rule rule
		want string
	}{
		{"content differs", rule{contains: "-u 1001 agent-dev-1 cat /etc/resolv.conf", stdout: "nameserver 127.0.0.11\n"}, "content is"},
		{"extra line", rule{contains: "-u 1001 agent-dev-1 cat /etc/resolv.conf", stdout: resolvGood + "nameserver 8.8.8.8\n"}, "content is"},
		{"wrong owner", rule{contains: "agent-dev-1 stat -c", stdout: "agent:agent 644\n"}, "owner and mode are"},
		{"wrong mode", rule{contains: "agent-dev-1 stat -c", stdout: "root:root 666\n"}, "owner and mode are"},
		{"agent can write", rule{contains: "agent-dev-1 sh -c : >> /etc/resolv.conf"}, "writable by the agent user"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fr := &fakeRunner{rules: []rule{c.rule}}
			r := run(t, fr, &fakeFS{}, agentID, nil)
			if r.code != 2 || !strings.Contains(r.err, c.want) || !strings.Contains(r.err, "CONTAINED") {
				t.Fatalf("exit %d, stderr %q", r.code, r.err)
			}
			if fr.count("docker pause agent-dev-1") != 2 || fr.has("docker rm") {
				t.Errorf("container must be paused and kept: %v", fr.calls)
			}
			noSuccess(t, r)
		})
	}
}

func TestCreatePassesResolvAndStateMountsAndEnv(t *testing.T) {
	fr := &fakeRunner{}
	r := run(t, fr, &fakeFS{}, agentID, nil)
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	cfg := testConfig("", nil)
	claude := filepath.Join(cfg.StateRoot, agentID, "claude")
	create := fr.calls[fr.index("docker create ")].joined()
	for _, want := range []string{
		"-v " + cfg.HostResolv + ":/etc/resolv.conf:ro",
		"-v " + claude + ":/home/agent/.claude:rw",
		"-e CLAUDE_CONFIG_DIR=/home/agent/.claude",
	} {
		if !strings.Contains(create, want) {
			t.Errorf("create lacks %q: %s", want, create)
		}
	}
	if strings.Count(create, " -v ") != 3 {
		t.Errorf("create must carry exactly the workspace, state and resolv.conf binds: %s", create)
	}
}

func TestStateAndHostResolvPrepared(t *testing.T) {
	base := t.TempDir()
	extra := map[string]string{
		"AGENT_SANDBOX_STATE_ROOT":  filepath.Join(base, "state"),
		"AGENT_SANDBOX_HOST_RESOLV": filepath.Join(base, "etc", "resolv.conf"),
	}
	// A differing host file is rewritten; existing state content is kept.
	if err := os.MkdirAll(filepath.Join(base, "etc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(extra["AGENT_SANDBOX_HOST_RESOLV"], []byte("nameserver 8.8.8.8\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	claude := filepath.Join(base, "state", agentID, "claude")
	if err := os.MkdirAll(claude, 0o755); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(claude, "login.json")
	if err := os.WriteFile(keep, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := run(t, &fakeRunner{}, &fakeFS{}, agentID, extra)
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	got, err := os.ReadFile(extra["AGENT_SANDBOX_HOST_RESOLV"])
	if err != nil || string(got) != resolvGood {
		t.Errorf("host resolv.conf = %q (%v), want %q", got, err, resolvGood)
	}
	if fi, err := os.Stat(extra["AGENT_SANDBOX_HOST_RESOLV"]); err != nil || fi.Mode().Perm() != 0o644 {
		t.Errorf("host resolv.conf mode: %v %v", fi, err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("existing state content must be kept: %v", err)
	}
	for dir, mode := range map[string]os.FileMode{
		filepath.Join(base, "state"):          0o711,
		filepath.Join(base, "state", agentID): 0o711,
		claude:                                0o700,
	} {
		if fi, err := os.Stat(dir); err != nil || fi.Mode().Perm() != mode {
			t.Errorf("%s: mode %v err %v, want %o", dir, fi, err, mode)
		}
	}
}

func TestStateFolderThatIsASymlinkExits2(t *testing.T) {
	base := t.TempDir()
	stateRoot := filepath.Join(base, "state")
	if err := os.MkdirAll(stateRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(stateRoot, agentID)); err != nil {
		t.Fatal(err)
	}
	fr := &fakeRunner{}
	r := run(t, fr, &fakeFS{}, agentID, map[string]string{"AGENT_SANDBOX_STATE_ROOT": stateRoot})
	if r.code != 2 || !strings.Contains(r.err, "not a real directory") || fr.has("docker create") {
		t.Fatalf("exit %d, stderr %q, calls %v", r.code, r.err, fr.calls)
	}
}

// ---- readiness wait and per-agent lock ----

func TestCreateNetNotReady(t *testing.T) {
	cases := map[string]func(dir string) error{
		"marker missing": func(string) error { return nil },
		"stale pid": func(dir string) error {
			return os.WriteFile(filepath.Join(dir, "ready"), []byte("1111\n"), 0o644)
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := setup(dir); err != nil {
				t.Fatal(err)
			}
			fr, fs := &fakeRunner{}, &fakeFS{}
			r := run(t, fr, fs, agentID, map[string]string{"AGENT_SANDBOX_RUNTIME_DIR": dir})
			if r.code != 3 || !strings.Contains(r.err, "exit_state: NET_NOT_READY") || !strings.Contains(r.err, "systemctl status agent-restore.service") {
				t.Fatalf("exit %d, stderr %q", r.code, r.err)
			}
			if fr.count("docker") != 0 || len(fs.mkdirs) != 0 {
				t.Errorf("nothing may touch docker or the workspace: %v", fr.calls)
			}
			noSuccess(t, r)
		})
	}
}

func TestCreateRefusedWhileAgentLockHeld(t *testing.T) {
	dir := t.TempDir()
	lock, err := services.TryLock(dir, agentID)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	if err := os.WriteFile(filepath.Join(dir, "ready"), []byte(dockerPID+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fr := &fakeRunner{}
	r := run(t, fr, &fakeFS{}, agentID, map[string]string{"AGENT_SANDBOX_RUNTIME_DIR": dir})
	if r.code != 1 || !strings.Contains(r.err, "in progress") || fr.count("docker") != 0 {
		t.Fatalf("exit %d, stderr %q, calls %v", r.code, r.err, fr.calls)
	}
}

func TestCreateHoldsAgentLockForTheWholeRun(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ready"), []byte(dockerPID+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The probe runs inside `docker unpause`, i.e. while create is still running.
	var heldAtUnpause, freeAfter bool
	fr := &fakeRunner{}
	probe := probeRunner{inner: fr, onUnpause: func() {
		lock, err := services.TryLock(dir, agentID)
		heldAtUnpause = errors.Is(err, services.ErrLockBusy)
		lock.Release()
	}}
	cfg := testConfig(t.TempDir(), map[string]string{"AGENT_SANDBOX_RUNTIME_DIR": dir})
	fr.ws = filepath.Join(cfg.WorkspaceRoot, agentID)
	fr.claude = filepath.Join(cfg.StateRoot, agentID, "claude")
	fr.resolv = cfg.HostResolv
	var out, errb bytes.Buffer
	code := sequence.Run(context.Background(), sequence.Deps{
		Config: cfg, Runner: probe, FS: &fakeFS{}, AgentID: agentID, Stdout: &out, Stderr: &errb,
	})
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	lock, err := services.TryLock(dir, agentID)
	freeAfter = err == nil
	lock.Release()
	if !heldAtUnpause || !freeAfter {
		t.Errorf("lock held during unpause = %v, free after the run = %v", heldAtUnpause, freeAfter)
	}
}

// probeRunner calls onUnpause when a `docker unpause` is about to run.
type probeRunner struct {
	inner     *fakeRunner
	onUnpause func()
}

func (p probeRunner) Run(ctx context.Context, stdin []byte, args ...string) ([]byte, []byte, int, error) {
	if len(args) > 1 && args[0] == "docker" && args[1] == "unpause" {
		p.onUnpause()
	}
	return p.inner.Run(ctx, stdin, args...)
}

func TestNoHostConfigAccess(t *testing.T) {
	fr := &fakeRunner{}
	if r := run(t, fr, &fakeFS{}, agentID, nil); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	for _, c := range fr.calls {
		j := c.joined()
		if j == "systemctl show -p MainPID --value docker.service" {
			continue // the only systemd call: read-only, for the ready marker
		}
		if strings.Contains(j, "daemon.json") || strings.Contains(j, "systemctl") || strings.Contains(j, "network create") {
			t.Errorf("forbidden call: %s", j)
		}
	}
}
