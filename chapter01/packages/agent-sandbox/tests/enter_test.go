package tests

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-sandbox/internal/sequence"
)

func (f *fakeFS) Glob(pattern string) ([]string, error) { return filepath.Glob(pattern) }

const enterID = "a1"

type enterResult struct {
	code        int
	stdout, err string
	execs       []string // joined argv of every captured exec
	execPath    string
}

// runEnter runs enter_agent with a private state root; history creates one conversation file in it before the run.
func runEnter(t *testing.T, fr *fakeRunner, isRoot, interactive bool, history bool, args ...string) enterResult {
	t.Helper()
	root, state := t.TempDir(), t.TempDir()
	cfg := testConfig(root, map[string]string{"AGENT_SANDBOX_STATE_ROOT": state})
	fr.ws = filepath.Join(root, enterID)
	fr.claude = filepath.Join(state, enterID, "claude")
	fr.resolv = cfg.HostResolv
	if history {
		dir := filepath.Join(fr.claude, "projects", "-workspace")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "s.jsonl"), []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var out, errb bytes.Buffer
	var res enterResult
	res.code = sequence.RunEnter(context.Background(), sequence.EnterDeps{
		Config: cfg, Runner: goneAfterRm{fr}, FS: &fakeFS{}, Args: args,
		IsRoot: isRoot, Interactive: interactive, Stdout: &out, Stderr: &errb,
		LookPath: func(string) (string, error) { return "/usr/bin/docker", nil },
		Exec: func(path string, argv []string, _ []string) error {
			res.execPath = path
			res.execs = append(res.execs, strings.Join(argv, " "))
			return nil
		},
	})
	res.stdout, res.err = out.String(), errb.String()
	if len(fr.unexpected) > 0 {
		t.Errorf("commands without a fake answer: %v", fr.unexpected)
	}
	return res
}

func statusRule(status string) rule {
	return rule{contains: "{{.State.Status}}", stdout: status + "\n"}
}

func absentRule() rule {
	return rule{contains: "{{.State.Status}}", stderr: "Error: No such container: agent-a1", code: 1}
}

func wantExec(t *testing.T, r enterResult, want string) {
	t.Helper()
	if len(r.execs) != 1 || r.execs[0] != want {
		t.Fatalf("execs = %q, want [%q]; stderr:\n%s", r.execs, want, r.err)
	}
	if r.execPath != "/usr/bin/docker" {
		t.Errorf("exec path = %q", r.execPath)
	}
}

func TestEnterRunningGateOkNoHistory(t *testing.T) {
	fr := &fakeRunner{rules: []rule{statusRule("running")}}
	r := runEnter(t, fr, true, true, false, enterID)
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	wantExec(t, r, "docker exec -it -u 1001 agent-a1 claude")
	if !strings.Contains(r.err, "entering agent-a1 (claude)") {
		t.Errorf("stderr: %q", r.err)
	}
	if fr.index("/usr/local/bin/create_agent_net check-gate agent-a1") < 0 {
		t.Error("gate must be checked")
	}
	if fr.has("docker rm") || fr.has("docker create") {
		t.Errorf("a running container must be left alone: %v", fr.calls)
	}
}

func TestEnterRunningWithHistoryContinues(t *testing.T) {
	fr := &fakeRunner{rules: []rule{statusRule("running")}}
	r := runEnter(t, fr, true, true, true, enterID)
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	wantExec(t, r, "docker exec -it -u 1001 agent-a1 claude --continue")
	if !strings.Contains(r.err, "entering agent-a1 (claude --continue)") {
		t.Errorf("stderr: %q", r.err)
	}
}

func TestEnterNonTerminalUsesDashI(t *testing.T) {
	fr := &fakeRunner{rules: []rule{statusRule("running")}}
	r := runEnter(t, fr, true, false, false, enterID)
	wantExec(t, r, "docker exec -i -u 1001 agent-a1 claude")
}

func TestEnterRunningGateFailPropagatesAndDoesNotRemove(t *testing.T) {
	fr := &fakeRunner{rules: []rule{
		statusRule("running"),
		{contains: "check-gate", stdout: "control nft-released: missing\n", stderr: "gate bad", code: 2},
	}}
	r := runEnter(t, fr, true, true, false, enterID)
	if r.code != 2 || len(r.execs) != 0 {
		t.Fatalf("exit %d, execs %v", r.code, r.execs)
	}
	for _, want := range []string{"control nft-released: missing", "gate bad", "remove_agent_container a1"} {
		if !strings.Contains(r.err, want) {
			t.Errorf("stderr lacks %q: %q", want, r.err)
		}
	}
	if fr.has("docker rm") || fr.has("docker create") {
		t.Errorf("nothing may be removed or created: %v", fr.calls)
	}
}

func TestEnterGateFailExitCode1(t *testing.T) {
	fr := &fakeRunner{rules: []rule{statusRule("running"), {contains: "check-gate", code: 1}}}
	r := runEnter(t, fr, true, true, false, enterID)
	if r.code != 1 || len(r.execs) != 0 {
		t.Fatalf("exit %d, execs %v", r.code, r.execs)
	}
}

func TestEnterAbsentCreatesThenExecs(t *testing.T) {
	fr := &fakeRunner{rules: []rule{absentRule()}}
	r := runEnter(t, fr, true, true, false, enterID)
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	wantExec(t, r, "docker exec -it -u 1001 agent-a1 claude")
	if fr.index("docker create ") < 0 || fr.has("docker rm") {
		t.Errorf("absent container: create without remove expected: %v", fr.calls)
	}
	if !strings.Contains(r.stdout, "agent container agent-a1 created") {
		t.Errorf("create output must be forwarded: %q", r.stdout)
	}
}

func TestEnterExitedRemovesThenCreatesThenExecs(t *testing.T) {
	for _, state := range []string{"exited", "paused", "created", "dead", "restarting"} {
		fr := &fakeRunner{existing: true, rules: []rule{statusRule(state)}}
		r := runEnter(t, fr, true, true, false, enterID)
		if r.code != 0 {
			t.Fatalf("%s: exit %d: %s", state, r.code, r.err)
		}
		wantExec(t, r, "docker exec -it -u 1001 agent-a1 claude")
		rm, create := fr.index("docker rm -f agent-a1"), fr.index("docker create ")
		if rm < 0 || create < 0 || rm > create {
			t.Fatalf("%s: want rm (%d) before create (%d)", state, rm, create)
		}
	}
}

func TestEnterRemoveFailureStopsBeforeCreate(t *testing.T) {
	fr := &fakeRunner{existing: true, rules: []rule{statusRule("exited"), {contains: "docker rm", stderr: "busy", code: 1}}}
	r := runEnter(t, fr, true, true, false, enterID)
	if r.code == 0 || len(r.execs) != 0 || fr.has("docker create") {
		t.Fatalf("exit %d, execs %v, calls %v", r.code, r.execs, fr.calls)
	}
}

func TestEnterCreateFailureDoesNotExec(t *testing.T) {
	fr := &fakeRunner{rules: []rule{absentRule(), {contains: "docker create", stderr: "no such image layer", code: 1}}}
	r := runEnter(t, fr, true, true, false, enterID)
	if r.code != 2 || len(r.execs) != 0 {
		t.Fatalf("exit %d, execs %v", r.code, r.execs)
	}
}

func TestEnterCommandOverride(t *testing.T) {
	fr := &fakeRunner{rules: []rule{statusRule("running")}}
	r := runEnter(t, fr, true, true, true, enterID, "--", "bash", "-lc", "ls")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	wantExec(t, r, "docker exec -it -u 1001 agent-a1 bash -lc ls")
	if !strings.Contains(r.err, "entering agent-a1 (bash -lc ls)") {
		t.Errorf("stderr: %q", r.err)
	}
}

func TestEnterUsageErrors(t *testing.T) {
	long := strings.Repeat("a", 32)
	for _, args := range [][]string{
		nil, {"Agent"}, {"1abc"}, {long}, {enterID, "extra"}, {enterID, "--"}, {enterID, "bash"}, {enterID, "-x", "bash"},
	} {
		fr := &fakeRunner{}
		r := runEnter(t, fr, true, true, false, args...)
		if r.code != 1 || !strings.Contains(r.err, "usage: enter_agent") || len(r.execs) != 0 || len(fr.calls) != 0 {
			t.Errorf("args %q: exit %d, stderr %q, calls %v", args, r.code, r.err, fr.calls)
		}
	}
}

func TestEnterRefusedWhenNotRoot(t *testing.T) {
	fr := &fakeRunner{}
	r := runEnter(t, fr, false, true, false, enterID)
	if r.code != 1 || !strings.Contains(r.err, "must run as root") || len(r.execs) != 0 || len(fr.calls) != 0 {
		t.Fatalf("exit %d, stderr %q, calls %v", r.code, r.err, fr.calls)
	}
}
