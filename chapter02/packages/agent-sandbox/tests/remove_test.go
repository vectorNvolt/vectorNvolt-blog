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
)

// ---- fakeFS purge surface (real temp-dir calls, removals recorded) ----

func (f *fakeFS) Lstat(path string) (os.FileInfo, error)   { return os.Lstat(path) }
func (f *fakeFS) EvalSymlinks(path string) (string, error) { return filepath.EvalSymlinks(path) }

func (f *fakeFS) RemoveAll(path string) error {
	f.removed = append(f.removed, path)
	return os.RemoveAll(path)
}

// ---- harness ----

// goneAfterRm makes the container disappear once `docker rm -f` ran, like the engine would.
type goneAfterRm struct{ fr *fakeRunner }

func (g goneAfterRm) Run(ctx context.Context, stdin []byte, args ...string) ([]byte, []byte, int, error) {
	out, errb, code, err := g.fr.Run(ctx, stdin, args...)
	if len(args) > 2 && args[0] == "docker" && args[1] == "rm" && code == 0 && err == nil {
		g.fr.existing = false
	}
	return out, errb, code, err
}

type removeResult struct {
	code        int
	stdout, err string
	root, ws    string
}

// runRemove runs the pipeline with the workspace root `root` (a fresh temp dir when empty).
func runRemove(t *testing.T, fr *fakeRunner, fs *fakeFS, root string, args ...string) removeResult {
	t.Helper()
	return runRemoveEnv(t, fr, fs, root, nil, args...)
}

// runRemoveEnv is runRemove with configuration overrides (e.g. a private state root).
func runRemoveEnv(t *testing.T, fr *fakeRunner, fs *fakeFS, root string, extra map[string]string, args ...string) removeResult {
	t.Helper()
	if root == "" {
		root = t.TempDir()
	}
	cfg := testConfig(root, extra)
	var out, errb bytes.Buffer
	code := sequence.RunRemove(context.Background(), sequence.RemoveDeps{
		Config: cfg, Runner: goneAfterRm{fr}, FS: fs, Args: args, Stdout: &out, Stderr: &errb,
	})
	if len(fr.unexpected) > 0 {
		t.Errorf("commands without a fake answer: %v", fr.unexpected)
	}
	return removeResult{code, out.String(), errb.String(), root, filepath.Join(root, agentID)}
}

func mkWorkspace(t *testing.T, root string) string {
	t.Helper()
	ws := filepath.Join(root, agentID)
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return ws
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// ---- tests ----

func TestRemoveHappyPathKeepsWorkspace(t *testing.T) {
	root := t.TempDir()
	ws := mkWorkspace(t, root)
	fr, fs := &fakeRunner{existing: true}, &fakeFS{}
	r := runRemove(t, fr, fs, root, agentID)
	if r.code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", r.code, r.err)
	}
	inspect := fr.index("docker container inspect agent-dev-1")
	disarm := fr.index("/usr/local/bin/create_agent_net disarm agent-dev-1")
	rm := fr.index("docker rm -f agent-dev-1")
	verify := fr.count("docker container inspect agent-dev-1")
	if !(inspect >= 0 && inspect < disarm && disarm < rm) || verify != 2 {
		t.Errorf("order inspect=%d disarm=%d rm=%d inspects=%d", inspect, disarm, rm, verify)
	}
	last := fr.calls[len(fr.calls)-1].joined()
	if last != "docker container inspect agent-dev-1" {
		t.Errorf("verify must come last, got %q", last)
	}
	if !exists(ws) || len(fs.removed) != 0 {
		t.Error("workspace must be kept by default")
	}
	for _, want := range []string{"[1/10] validate arguments", "disarmed and removed", "exit_state: REMOVED", "workspace kept: " + ws} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("stdout lacks %q: %q", want, r.stdout)
		}
	}
	if r.err != "" {
		t.Errorf("stderr should be empty: %q", r.err)
	}
	if fr.has("docker create") || fr.has("docker start") || fr.has("docker network") {
		t.Errorf("nothing but inspect and rm may touch the engine: %v", fr.calls)
	}
}

func TestPurgeRemovesOnlyOwnWorkspace(t *testing.T) {
	root := t.TempDir()
	ws := mkWorkspace(t, root)
	other := filepath.Join(root, "other-agent")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	fr, fs := &fakeRunner{existing: true}, &fakeFS{}
	r := runRemove(t, fr, fs, root, agentID, "--purge-workspace")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	if exists(ws) || !exists(other) || !exists(root) {
		t.Errorf("only %s may be deleted", ws)
	}
	if len(fs.removed) != 1 || fs.removed[0] != ws {
		t.Errorf("removals = %v", fs.removed)
	}
	if !strings.Contains(r.stdout, "workspace purged: "+ws) {
		t.Errorf("stdout: %q", r.stdout)
	}
}

func TestPurgeRefusesSymlinkedWorkspace(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "keep"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, agentID)); err != nil {
		t.Fatal(err)
	}
	fs := &fakeFS{}
	r := runRemove(t, &fakeRunner{existing: true}, fs, root, agentID, "--purge-workspace")
	if r.code != 2 || !strings.Contains(r.err, "WORKSPACE_PURGE_FAILED") || !strings.Contains(r.err, "refusing to purge") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if len(fs.removed) != 0 || !exists(filepath.Join(outside, "keep")) {
		t.Error("nothing may be deleted")
	}
}

func TestPurgeRefusesSymlinkedRoot(t *testing.T) {
	real, link := t.TempDir(), filepath.Join(t.TempDir(), "root-link")
	mkWorkspace(t, real)
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	fs := &fakeFS{}
	r := runRemove(t, &fakeRunner{existing: true}, fs, link, agentID, "--purge-workspace")
	if r.code != 2 || !strings.Contains(r.err, "workspace root") || len(fs.removed) != 0 {
		t.Fatalf("exit %d, stderr %q, removed %v", r.code, r.err, fs.removed)
	}
}

// outsideFS resolves every workspace to a path outside the root, as a bind-mounted or nested link would.
type outsideFS struct{ *fakeFS }

func (o outsideFS) EvalSymlinks(path string) (string, error) {
	if filepath.Base(path) == agentID {
		return "/elsewhere/" + agentID, nil
	}
	return filepath.EvalSymlinks(path)
}

func TestPurgeRefusesPathResolvingOutsideRoot(t *testing.T) {
	root := t.TempDir()
	ws := mkWorkspace(t, root)
	inner := &fakeFS{}
	cfg := testConfig(root, nil)
	var out, errb bytes.Buffer
	code := sequence.RunRemove(context.Background(), sequence.RemoveDeps{
		Config: cfg, Runner: goneAfterRm{&fakeRunner{existing: true}}, FS: outsideFS{inner},
		Args: []string{agentID, "--purge-workspace"}, Stdout: &out, Stderr: &errb,
	})
	if code != 2 || !strings.Contains(errb.String(), "not directly under") || len(inner.removed) != 0 || !exists(ws) {
		t.Fatalf("exit %d, stderr %q", code, errb.String())
	}
}

func TestPurgeAbsentWorkspaceIsNotAnError(t *testing.T) {
	r := runRemove(t, &fakeRunner{existing: true}, &fakeFS{}, "", agentID, "--purge-workspace")
	if r.code != 0 || !strings.Contains(r.stdout, "nothing to purge") {
		t.Fatalf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.err)
	}
}

func TestContainerAbsentExits0NotPresent(t *testing.T) {
	root := t.TempDir()
	ws := mkWorkspace(t, root)
	fr := &fakeRunner{}
	r := runRemove(t, fr, &fakeFS{}, root, agentID)
	if r.code != 0 || !strings.Contains(r.stdout, "no container agent-dev-1") || !strings.Contains(r.stdout, "exit_state: NOT_PRESENT") {
		t.Fatalf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.err)
	}
	if fr.has("/usr/local/bin/create_agent_net") || fr.has("docker rm") {
		t.Errorf("no disarm or rm for an absent container: %v", fr.calls)
	}
	if !exists(ws) {
		t.Error("workspace must be kept")
	}
	// purge is still honored when the container is gone (re-run after a failed purge)
	fs := &fakeFS{}
	r = runRemove(t, &fakeRunner{}, fs, root, agentID, "--purge-workspace")
	if r.code != 0 || exists(ws) || len(fs.removed) != 1 {
		t.Fatalf("exit %d, stderr %q, removed %v", r.code, r.err, fs.removed)
	}
}

func TestDisarmFailureRemovesNothingExits2(t *testing.T) {
	root := t.TempDir()
	ws := mkWorkspace(t, root)
	fr := &fakeRunner{existing: true, rules: []rule{{contains: "create_agent_net disarm", stderr: "nft: busy", code: 2}}}
	fs := &fakeFS{}
	r := runRemove(t, fr, fs, root, agentID, "--purge-workspace")
	if r.code != 2 {
		t.Fatalf("exit %d, want 2", r.code)
	}
	for _, want := range []string{"nft: busy", "DISARM_FAILED"} {
		if !strings.Contains(r.err, want) {
			t.Errorf("stderr lacks %q:\n%s", want, r.err)
		}
	}
	if fr.has("docker rm") || len(fs.removed) != 0 || !exists(ws) {
		t.Error("nothing may be removed after a disarm failure")
	}
	if strings.Contains(r.stdout, "removed") {
		t.Errorf("no success text: %q", r.stdout)
	}
}

func TestDisarmBinaryMissingExits1(t *testing.T) {
	fr := &fakeRunner{existing: true, rules: []rule{{contains: "create_agent_net disarm", code: -1, err: errors.New("no such file or directory")}}}
	r := runRemove(t, fr, &fakeFS{}, "", agentID)
	if r.code != 1 || !strings.Contains(r.err, "no such file or directory") || !strings.Contains(r.err, "NOT_REMOVED") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.has("docker rm") {
		t.Error("nothing may be removed")
	}
}

func TestDisarmPathIsConfigurable(t *testing.T) {
	fr := &fakeRunner{existing: true}
	root := t.TempDir()
	cfg := testConfig(root, map[string]string{"AGENT_SANDBOX_ARM_BIN": "/opt/arm"})
	var out, errb bytes.Buffer
	code := sequence.RunRemove(context.Background(), sequence.RemoveDeps{
		Config: cfg, Runner: goneAfterRm{fr}, FS: &fakeFS{}, Args: []string{agentID}, Stdout: &out, Stderr: &errb,
	})
	if code != 0 || !fr.has("/opt/arm disarm agent-dev-1") {
		t.Fatalf("exit %d, calls %v", code, fr.calls)
	}
}

func TestRemoveFailureExits2(t *testing.T) {
	root := t.TempDir()
	ws := mkWorkspace(t, root)
	fr := &fakeRunner{existing: true, rules: []rule{{contains: "docker rm -f agent-dev-1", stderr: "device busy", code: 1}}}
	fs := &fakeFS{}
	r := runRemove(t, fr, fs, root, agentID, "--purge-workspace")
	if r.code != 2 || !strings.Contains(r.err, "device busy") || !strings.Contains(r.err, "REMOVE_FAILED") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if len(fs.removed) != 0 || !exists(ws) {
		t.Error("workspace must not be purged after a failed removal")
	}
}

func TestStillPresentAfterRemoveExits2(t *testing.T) {
	// rm "succeeds" but the engine still lists the container
	fr := &fakeRunner{existing: true}
	cfg := testConfig("", nil)
	var out, errb bytes.Buffer
	code := sequence.RunRemove(context.Background(), sequence.RemoveDeps{
		Config: cfg, Runner: fr, FS: &fakeFS{}, Args: []string{agentID}, Stdout: &out, Stderr: &errb,
	})
	if code != 2 || !strings.Contains(errb.String(), "still exists") {
		t.Fatalf("exit %d, stderr %q", code, errb.String())
	}
}

func TestLocateEngineErrorIsNotAbsence(t *testing.T) {
	fr := &fakeRunner{rules: []rule{{contains: "docker container inspect agent-dev-1", stderr: "Cannot connect to the Docker daemon", code: 1}}}
	root := t.TempDir()
	ws := mkWorkspace(t, root)
	fs := &fakeFS{}
	r := runRemove(t, fr, fs, root, agentID, "--purge-workspace")
	if r.code != 2 || !strings.Contains(r.err, "cannot check for the container") || !strings.Contains(r.err, "NOT_REMOVED") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.has("/usr/local/bin/create_agent_net") || fr.has("docker rm") || len(fs.removed) != 0 || !exists(ws) {
		t.Errorf("nothing may be disarmed, removed or purged: %v", fr.calls)
	}
	if strings.Contains(r.stdout, "NOT_PRESENT") {
		t.Errorf("an engine error must not be reported as absence: %q", r.stdout)
	}
}

func TestVerifyRemovedEngineErrorExits2NotRemoved(t *testing.T) {
	// The locate step passes (skip 1); the verification read-back fails.
	fr := &fakeRunner{existing: true, rules: []rule{{contains: "docker container inspect agent-dev-1", skip: 1, stderr: "Cannot connect to the Docker daemon", code: 1}}}
	root := t.TempDir()
	ws := mkWorkspace(t, root)
	fs := &fakeFS{}
	r := runRemove(t, fr, fs, root, agentID, "--purge-workspace")
	if r.code != 2 || !strings.Contains(r.err, "could not be verified") || !strings.Contains(r.err, "NOT_REMOVED") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if len(fs.removed) != 0 || !exists(ws) {
		t.Error("workspace must not be purged when the removal is unverified")
	}
	if strings.Contains(r.stdout, "disarmed and removed") {
		t.Errorf("no success text: %q", r.stdout)
	}
}

func TestWorkspaceAbsentWording(t *testing.T) {
	// Container present, workspace folder missing, no purge requested: reported as absent, not kept.
	root := t.TempDir()
	r := runRemove(t, &fakeRunner{existing: true}, &fakeFS{}, root, agentID)
	if r.code != 0 || !strings.Contains(r.stdout, "workspace absent: "+r.ws) || strings.Contains(r.stdout, "workspace kept") {
		t.Fatalf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.err)
	}
	// Container absent too.
	r = runRemove(t, &fakeRunner{}, &fakeFS{}, root, agentID)
	if r.code != 0 || !strings.Contains(r.stdout, "exit_state: NOT_PRESENT") || !strings.Contains(r.stdout, "workspace absent: "+r.ws) {
		t.Fatalf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.err)
	}
}

func TestRemoveInvalidInvocationExits1(t *testing.T) {
	cases := [][]string{
		nil,
		{""},
		{"Agent"},
		{"1abc"},
		{"a/b"},
		{strings.Repeat("a", 32)},
		{agentID, "--force"},
		{agentID, "--purge", "--purge"},
		{agentID, "--purge-workspace", "--purge", "--purge-workspace"},
		{agentID, "--purge", "extra"},
		{agentID, "--purge-workspace", "--purge-workspace"},
		{"--purge-workspace", agentID},
		{agentID, "extra", "--purge-workspace"},
	}
	for _, args := range cases {
		fr, fs := &fakeRunner{existing: true}, &fakeFS{}
		r := runRemove(t, fr, fs, "", args...)
		if r.code != 1 || !strings.Contains(r.err, "usage: remove_agent_container") {
			t.Errorf("args %q: exit %d, stderr %q", args, r.code, r.err)
		}
		if len(fr.calls) != 0 || len(fs.removed) != 0 {
			t.Errorf("args %q: no engine or fs call allowed, got %v", args, fr.calls)
		}
	}
}

func TestRemoveEngineDownExits1(t *testing.T) {
	fr := &fakeRunner{rules: []rule{{contains: "docker info", stderr: "Cannot connect to the Docker daemon", code: 1}}}
	r := runRemove(t, fr, &fakeFS{}, "", agentID)
	if r.code != 1 || !strings.Contains(r.err, "Cannot connect to the Docker daemon") || len(fr.calls) != 1 {
		t.Fatalf("exit %d, stderr %q, calls %v", r.code, r.err, fr.calls)
	}
}
