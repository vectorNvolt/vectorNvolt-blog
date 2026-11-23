package tests

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-exec/internal/app"
	"agent-exec/internal/cli"
	"agent-exec/internal/engine"
	"agent-exec/internal/sequence"
	"agent-exec/internal/wsfs"
)

var imageID = "sha256:" + strings.Repeat("a", 64)

const codeProfile = `
name: python
kind: code
image_id: IMAGEID
ref: exec-python:1
entry: agent
env_dirs: { .venv: ro }
env: { PATH: /workspace/.venv/bin:/usr/bin }
inputs: { data: { path_under: [.input/, .agent/, src/], multiple: true, optional: true } }
limits: { mem: 512m, cpus: 1, pids: 128, timeout_s: 60, out_max: 1m }
`

const installProfile = `
name: install-python
kind: install
image_id: IMAGEID
entry: fixed-args
fetch_argv: [pip, download, --only-binary=:all:, -d, "/workspace/.output/exec/<execId>/dl"]
argv: [install-into-venv, install, --no-index, --find-links, "/workspace/.output/exec/<execId>/dl"]
env_dirs: { .venv: rw }
tmp_exec: true
limits: { mem: 512m, cpus: 1, pids: 128, timeout_s: 60, out_max: 1m }
`

const buildProfile = `
name: c-make
kind: build
image_id: IMAGEID
entry: fixed-args
argv: [make]
build_dir: true
tmp_exec: true
limits: { mem: 512m, cpus: 1, pids: 128, timeout_s: 60, out_max: 1m }
`

type world struct {
	t    *testing.T
	root string
	ws   *wsfs.Workspace
	eng  *fakeEngine
}

func newWorld(t *testing.T) *world {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{"src", "build", ".venv", ".packages", ".input", ".output", ".tools", ".agent"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	w := &world{t: t, root: root, ws: wsfs.New(root), eng: newFakeEngine()}
	w.eng.images[imageID] = true
	w.write("src/requirements.txt", "requests\n")
	w.write("src/main.py", "print(1)\n")
	w.write(".input/data.csv", "a,b\n1,2\n")
	w.write(".tools/python/profile.yml", codeProfile)
	w.write(".tools/install-python/profile.yml", installProfile)
	w.write(".tools/c-make/profile.yml", buildProfile)
	return w
}

func (w *world) write(rel, content string) {
	w.t.Helper()
	p := filepath.Join(w.root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		w.t.Fatal(err)
	}
	content = strings.ReplaceAll(content, "IMAGEID", imageID)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		w.t.Fatal(err)
	}
}

func (w *world) path(rel string) string { return filepath.Join(w.root, rel) }

func (w *world) read(rel string) string {
	w.t.Helper()
	b, err := os.ReadFile(w.path(rel))
	if err != nil {
		w.t.Fatal(err)
	}
	return string(b)
}

func (w *world) exists(rel string) bool {
	_, err := os.Lstat(w.path(rel))
	return err == nil
}

func (w *world) run(args ...string) (sequence.Outcome, string, string) {
	return w.runWith(nil, args...)
}

func (w *world) runWith(mod func(*sequence.Deps), args ...string) (sequence.Outcome, string, string) {
	w.t.Helper()
	req, err := cli.Parse(args)
	if err != nil {
		w.t.Fatalf("parse %v: %v", args, err)
	}
	var so, se bytes.Buffer
	d := &sequence.Deps{
		WS: w.ws, Engine: w.eng, Stdout: &so, Stderr: &se,
		Poll: 10 * time.Millisecond, LockWait: 100 * time.Millisecond, Pid: 4242,
		PidAlive: func(pid int) bool { return pid == 4242 },
	}
	if mod != nil {
		mod(d)
	}
	out := sequence.Execute(d, req)
	return out, so.String(), se.String()
}

func (w *world) result(id string) sequence.Result {
	w.t.Helper()
	var r sequence.Result
	if err := json.Unmarshal([]byte(w.read(".output/exec/"+id+"/result.json")), &r); err != nil {
		w.t.Fatal(err)
	}
	return r
}

func (w *world) noContainers() {
	w.t.Helper()
	if n := w.eng.remaining(); n != 0 {
		w.t.Fatalf("%d containers left behind", n)
	}
}

func TestRunOK(t *testing.T) {
	w := newWorld(t)
	w.eng.behave = func(string) behavior { return behavior{stdout: "hello\n", stderr: "warn\n"} }
	out, so, se := w.run("run", "python", "--exec-id", "e1", "--label", "agent-graph.run=r1", "--", "python", "/workspace/src/main.py")
	if out != (sequence.Outcome{Exit: 0, Cause: "ok"}) {
		t.Fatalf("outcome %+v stderr %q", out, se)
	}
	if so != "hello\n" {
		t.Fatalf("stdout %q", so)
	}
	first := strings.SplitN(se, "\n", 2)[0]
	if first != "run_ephemeral_container: exec_dir="+w.path(".output/exec/e1") {
		t.Fatalf("first stderr line %q", first)
	}
	if !strings.Contains(se, "warn") {
		t.Fatalf("stderr %q", se)
	}
	if w.read(".output/exec/e1/stdout.log") != "hello\n" || w.read(".output/exec/e1/stderr.log") != "warn\n" {
		t.Fatal("logs not written")
	}
	r := w.result("e1")
	if r.Exit != 0 || r.Cause != "ok" || r.ImageID != imageID || r.ExecDir != w.path(".output/exec/e1") || r.Start == "" || r.End == "" || r.Phase != "" {
		t.Fatalf("result %+v", r)
	}
	if !w.exists(".output/exec/e1/meta.json") || !w.exists(".output/exec/e1/in") || !w.exists(".output/exec/e1/out") || w.exists(".output/exec/e1/dl") {
		t.Fatal("exec dir layout")
	}
	body := w.eng.bodies["ephemeral-e1"]
	if body.Labels["ephemeral.exec"] != "e1" || body.Labels["ephemeral.pid"] != "4242" || body.Labels["agent-graph.run"] != "r1" {
		t.Fatalf("labels %v", body.Labels)
	}
	if body.User != "1001:1001" || body.HostConfig.NetworkMode != "none" || !body.HostConfig.AutoRemove {
		t.Fatalf("settings %+v", body)
	}
	w.noContainers()
}

func TestRunNonZeroExit(t *testing.T) {
	w := newWorld(t)
	w.eng.behave = func(string) behavior { return behavior{exit: 3} }
	out, _, _ := w.run("run", "python", "--exec-id", "e1", "--", "false")
	if out != (sequence.Outcome{Exit: 3, Cause: "exit"}) {
		t.Fatalf("outcome %+v", out)
	}
	if r := w.result("e1"); r.Exit != 3 || r.Cause != "exit" {
		t.Fatalf("result %+v", r)
	}
	w.noContainers()
}

func TestRunTimeout(t *testing.T) {
	w := newWorld(t)
	w.eng.behave = func(string) behavior { return behavior{hang: true} }
	out, _, _ := w.runWith(func(d *sequence.Deps) { d.TimeoutOverride = 100 * time.Millisecond },
		"run", "python", "--exec-id", "e1", "--", "sleep", "100")
	if out != (sequence.Outcome{Exit: 124, Cause: "timeout"}) {
		t.Fatalf("outcome %+v", out)
	}
	if r := w.result("e1"); r.Exit != 124 || r.Cause != "timeout" {
		t.Fatalf("result %+v", r)
	}
	w.noContainers()
}

func TestRunOOM(t *testing.T) {
	w := newWorld(t)
	w.eng.behave = func(string) behavior { return behavior{oom: true} }
	out, _, _ := w.run("run", "python", "--exec-id", "e1", "--", "alloc")
	if out != (sequence.Outcome{Exit: 137, Cause: "oom"}) {
		t.Fatalf("outcome %+v", out)
	}
	if r := w.result("e1"); r.Cause != "oom" {
		t.Fatalf("result %+v", r)
	}
	w.noContainers()
}

func TestRunOutCap(t *testing.T) {
	w := newWorld(t)
	w.eng.behave = func(string) behavior {
		return behavior{hang: true, before: func() {
			_ = os.WriteFile(w.path(".output/exec/e1/out/big"), make([]byte, 2<<20), 0o644)
		}}
	}
	out, _, _ := w.run("run", "python", "--exec-id", "e1", "--", "fill")
	if out != (sequence.Outcome{Exit: 137, Cause: "out_cap"}) {
		t.Fatalf("outcome %+v", out)
	}
	if !w.exists(".output/exec/e1/out/big") {
		t.Fatal("output must be kept")
	}
	w.noContainers()
}

func TestRunCancel(t *testing.T) {
	w := newWorld(t)
	w.eng.behave = func(string) behavior { return behavior{hang: true} }
	cancel := make(chan struct{})
	time.AfterFunc(100*time.Millisecond, func() { close(cancel) })
	out, _, _ := w.runWith(func(d *sequence.Deps) { d.Cancel = cancel }, "run", "python", "--exec-id", "e1", "--", "sleep", "100")
	if out != (sequence.Outcome{Exit: 130, Cause: "cancel"}) {
		t.Fatalf("outcome %+v", out)
	}
	if r := w.result("e1"); r.Exit != 130 || r.Cause != "cancel" {
		t.Fatalf("result %+v", r)
	}
	w.noContainers()
}

func TestDenied(t *testing.T) {
	w := newWorld(t)
	w.eng.createErr = func(string) error {
		return &engine.APIError{Status: 403, Message: "authorization denied by plugin ephemeral-authz: nope"}
	}
	out, _, se := w.run("run", "python", "--exec-id", "e1", "--", "x")
	if out != (sequence.Outcome{Exit: 125, Cause: "denied"}) {
		t.Fatalf("outcome %+v stderr %q", out, se)
	}
	if r := w.result("e1"); r.Cause != "denied" {
		t.Fatalf("result %+v", r)
	}
	if len(w.eng.startedNames()) != 0 {
		t.Fatal("nothing may start")
	}
	w.noContainers()
}

func TestVerifyMismatch(t *testing.T) {
	w := newWorld(t)
	w.eng.mutate = func(i *engine.Inspect) { i.HostConfig.CapAdd = []string{"SYS_ADMIN"} }
	out, _, se := w.run("run", "python", "--exec-id", "e1", "--", "x")
	if out != (sequence.Outcome{Exit: 125, Cause: "verify"}) {
		t.Fatalf("outcome %+v", out)
	}
	if !strings.Contains(se, "CapAdd") {
		t.Fatalf("diagnostic must name the mismatch: %q", se)
	}
	if len(w.eng.startedNames()) != 0 {
		t.Fatal("a container that fails verify must never start")
	}
	if r := w.result("e1"); r.Cause != "verify" {
		t.Fatalf("result %+v", r)
	}
	w.noContainers()
}

func TestInstallOK(t *testing.T) {
	w := newWorld(t)
	out, _, se := w.run("install", "install-python", "--exec-id", "i1", "--manifest", w.path("src/requirements.txt"))
	if out != (sequence.Outcome{Exit: 0, Cause: "ok"}) {
		t.Fatalf("outcome %+v stderr %q", out, se)
	}
	if got := w.eng.startedNames(); len(got) != 2 || got[0] != "ephemeral-i1-fetch" || got[1] != "ephemeral-i1" {
		t.Fatalf("start order %v", got)
	}
	fetch, inst := w.eng.bodies["ephemeral-i1-fetch"], w.eng.bodies["ephemeral-i1"]
	if fetch.HostConfig.NetworkMode != "host" || inst.HostConfig.NetworkMode != "none" {
		t.Fatalf("networks %q %q", fetch.HostConfig.NetworkMode, inst.HostConfig.NetworkMode)
	}
	if strings.Join(fetch.Cmd, " ") != "pip download --only-binary=:all: -d /workspace/.output/exec/i1/dl -r /workspace/src/requirements.txt" {
		t.Fatalf("fetch cmd %v", fetch.Cmd)
	}
	if !strings.HasSuffix(strings.Join(inst.Cmd, " "), "--find-links /workspace/.output/exec/i1/dl -r /workspace/src/requirements.txt") {
		t.Fatalf("install cmd %v", inst.Cmd)
	}
	if r := w.result("i1"); r.Phase != "install" || r.Cause != "ok" {
		t.Fatalf("result %+v", r)
	}
	if !w.exists(".output/exec/i1/dl") || w.exists(".output/exec/i1/in") {
		t.Fatal("install exec dir has dl/ and no in/")
	}
	w.noContainers()
}

func TestInstallFetchFailure(t *testing.T) {
	w := newWorld(t)
	w.eng.behave = func(name string) behavior {
		if strings.HasSuffix(name, "-fetch") {
			return behavior{exit: 1, stderr: "ERROR: no matching distribution\n"}
		}
		return behavior{}
	}
	out, _, se := w.run("install", "install-python", "--exec-id", "i1", "--manifest", w.path("src/requirements.txt"))
	if out != (sequence.Outcome{Exit: 1, Cause: "fetch"}) {
		t.Fatalf("outcome %+v", out)
	}
	if !strings.Contains(se, "run_ephemeral_container: "+sequence.FetchMessage) {
		t.Fatalf("stderr lacks the fetch message: %q", se)
	}
	if r := w.result("i1"); r.Phase != "fetch" || r.Cause != "fetch" || r.Exit != 1 {
		t.Fatalf("result %+v", r)
	}
	if got := w.eng.startedNames(); len(got) != 1 {
		t.Fatalf("only fetch may start: %v", got)
	}
	w.noContainers() // the install container was created and is removed again
}

func TestBuildKind(t *testing.T) {
	w := newWorld(t)
	out, _, se := w.run("build", "c-make", "--exec-id", "b1", "--", "-C", "/workspace/src")
	if out.Cause != "ok" {
		t.Fatalf("outcome %+v %q", out, se)
	}
	body := w.eng.bodies["ephemeral-b1"]
	if strings.Join(body.Cmd, " ") != "make -C /workspace/src" {
		t.Fatalf("cmd %v", body.Cmd)
	}
	rw := map[string]bool{}
	for _, m := range body.HostConfig.Mounts {
		rw[m.Target] = !m.ReadOnly
	}
	if !rw["/workspace/build"] || rw["/workspace/src"] || !rw["/workspace/.output/exec/b1/out"] || rw["/workspace/.output/exec/b1/in"] {
		t.Fatalf("mount modes %v", rw)
	}
}

func TestUsageErrorsCreateNothing(t *testing.T) {
	w := newWorld(t)
	w.write(".process/secret.csv", "x")
	w.write(".input/a/data.csv", "x")
	w.write(".input/b/data.csv", "y")
	if err := os.Symlink("/etc/hostname", w.path(".input/evil.csv")); err != nil {
		t.Fatal(err)
	}
	cases := map[string][]string{
		"unknown profile":      {"run", "nope", "--", "x"},
		"kind mismatch":        {"build", "python", "--", "x"},
		"timeout above":        {"run", "python", "--timeout", "9999", "--", "x"},
		"undeclared input":     {"run", "python", "--in", "other=" + w.path(".input/data.csv"), "--", "x"},
		"input outside":        {"run", "python", "--in", "data=/etc/hostname", "--", "x"},
		"input not allowed":    {"run", "python", "--in", "data=" + w.path(".process/secret.csv"), "--", "x"},
		"input symlink":        {"run", "python", "--in", "data=" + w.path(".input/evil.csv"), "--", "x"},
		"input missing":        {"run", "python", "--in", "data=" + w.path(".input/none.csv"), "--", "x"},
		"duplicate basename":   {"run", "python", "--in", "data=" + w.path(".input/a/data.csv"), "--in", "data=" + w.path(".input/b/data.csv"), "--", "x"},
		"manifest outside src": {"install", "install-python", "--manifest", w.path(".input/data.csv")},
		"manifest missing":     {"install", "install-python", "--manifest", w.path("src/none.txt")},
	}
	for name, args := range cases {
		out, _, se := w.run(args...)
		if out != (sequence.Outcome{Exit: 125, Cause: "usage"}) {
			t.Errorf("%s: outcome %+v stderr %q", name, out, se)
		}
		if strings.Contains(se, "exec_dir=") {
			t.Errorf("%s: exec dir announced", name)
		}
	}
	if w.exists(".output/exec") {
		t.Fatal("a usage error must create nothing")
	}
	// An existing exec dir is a usage error too.
	w.write(".output/exec/dup/keep", "x")
	if out, _, _ := w.run("run", "python", "--exec-id", "dup", "--", "x"); out.Cause != "usage" {
		t.Fatalf("exec-id exists: %+v", out)
	}
}

func TestStagingAndExpansion(t *testing.T) {
	w := newWorld(t)
	w.write(".agent/r1/other.csv", "other\n")
	out, _, se := w.run("run", "python", "--exec-id", "s1",
		"--in", "data="+w.path(".input/data.csv"),
		"--in", "data="+w.path(".agent/r1/other.csv"),
		"--in", "data="+w.path("src/main.py"),
		"--", "cat", "{in.data}")
	if out.Cause != "ok" {
		t.Fatalf("outcome %+v stderr %q", out, se)
	}
	if w.read(".output/exec/s1/in/data/data.csv") != "a,b\n1,2\n" || w.read(".output/exec/s1/in/data/other.csv") != "other\n" {
		t.Fatal("inputs not staged as in/<name>/<basename>")
	}
	if w.exists(".output/exec/s1/in/data/main.py") {
		t.Fatal("a path under src/ is used in place, not copied")
	}
	want := []string{"cat",
		"/workspace/.output/exec/s1/in/data/data.csv",
		"/workspace/.output/exec/s1/in/data/other.csv",
		"/workspace/src/main.py"}
	if got := w.eng.bodies["ephemeral-s1"].Cmd; strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("cmd %v", got)
	}
	var m struct {
		Inputs []struct{ Name, Source, Path, SHA256 string } `json:"inputs"`
	}
	if err := json.Unmarshal([]byte(w.read(".output/exec/s1/meta.json")), &m); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("a,b\n1,2\n"))
	if len(m.Inputs) != 3 || m.Inputs[0].Name != "data" || m.Inputs[0].SHA256 != hex.EncodeToString(sum[:]) ||
		m.Inputs[0].Path != "/workspace/.output/exec/s1/in/data/data.csv" || m.Inputs[0].Source != w.path(".input/data.csv") {
		t.Fatalf("meta inputs %+v", m.Inputs)
	}
}

func TestSingleInputStagedAsName(t *testing.T) {
	w := newWorld(t)
	w.write(".tools/python/profile.yml", strings.Replace(codeProfile, "multiple: true, ", "", 1))
	out, _, se := w.run("run", "python", "--exec-id", "s2", "--in", "data="+w.path(".input/data.csv"), "--", "cat", "{in.data}")
	if out.Cause != "ok" {
		t.Fatalf("outcome %+v %q", out, se)
	}
	if w.read(".output/exec/s2/in/data") != "a,b\n1,2\n" {
		t.Fatal("single input must be staged as in/<name>")
	}
}

func TestBusyWhenLocked(t *testing.T) {
	w := newWorld(t)
	l, err := w.ws.Lock(".venv", true, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	out, _, se := w.run("run", "python", "--exec-id", "e1", "--", "x")
	if out != (sequence.Outcome{Exit: 125, Cause: "busy"}) {
		t.Fatalf("outcome %+v", out)
	}
	if strings.Contains(se, "exec_dir=") || w.exists(".output/exec") {
		t.Fatal("busy must create nothing")
	}
	l.Release()
	if out, _, _ := w.run("run", "python", "--exec-id", "e1", "--", "x"); out.Cause != "ok" {
		t.Fatalf("after release: %+v", out)
	}
}

func TestBusyWhenNoSlot(t *testing.T) {
	w := newWorld(t)
	w.eng.addExisting("ephemeral-other", "running", map[string]string{"ephemeral.exec": "other", "ephemeral.pid": "4242"}, 4<<30)
	out, _, _ := w.run("run", "python", "--exec-id", "e1", "--", "x")
	if out != (sequence.Outcome{Exit: 125, Cause: "busy"}) {
		t.Fatalf("outcome %+v", out)
	}
	if w.exists(".output/exec") {
		t.Fatal("busy must create nothing")
	}
}

func TestStaleContainersRemoved(t *testing.T) {
	w := newWorld(t)
	w.eng.addExisting("ephemeral-dead", "running", map[string]string{"ephemeral.exec": "dead", "ephemeral.pid": "1"}, 4<<30)
	out, _, se := w.run("run", "python", "--exec-id", "e1", "--", "x")
	if out.Cause != "ok" {
		t.Fatalf("outcome %+v stderr %q", out, se)
	}
	found := false
	for _, n := range w.eng.removed {
		if n == "ephemeral-dead" {
			found = true
		}
	}
	if !found {
		t.Fatalf("stale container not removed: %v", w.eng.removed)
	}
}

func TestSetupFailuresBeforeExecDir(t *testing.T) {
	t.Run("engine unreachable", func(t *testing.T) {
		w := newWorld(t)
		w.eng.pingErr = errors.New("connection refused")
		out, _, _ := w.run("run", "python", "--", "x")
		if out != (sequence.Outcome{Exit: 125, Cause: "setup"}) || w.exists(".output/exec") {
			t.Fatalf("outcome %+v", out)
		}
	})
	t.Run("missing env dir", func(t *testing.T) {
		w := newWorld(t)
		if err := os.Remove(w.path(".venv")); err != nil {
			t.Fatal(err)
		}
		out, _, _ := w.run("run", "python", "--", "x")
		if out != (sequence.Outcome{Exit: 125, Cause: "setup"}) || w.exists(".output/exec") || w.exists(".venv") {
			t.Fatalf("outcome %+v (the tool must not create the folder)", out)
		}
	})
	t.Run("requires_build with empty build", func(t *testing.T) {
		w := newWorld(t)
		w.write(".tools/python/profile.yml", codeProfile+"build_dir: true\nrequires_build: true\n")
		out, _, _ := w.run("run", "python", "--", "x")
		if out != (sequence.Outcome{Exit: 125, Cause: "setup"}) || w.exists(".output/exec") {
			t.Fatalf("outcome %+v", out)
		}
		w.write("build/bin/app", "x")
		if out, _, se := w.run("run", "python", "--", "x"); out.Cause != "ok" {
			t.Fatalf("with build: %+v %q", out, se)
		}
	})
	t.Run("invalid profile", func(t *testing.T) {
		w := newWorld(t)
		w.write(".tools/python/profile.yml", codeProfile+"mount: /\n")
		out, _, _ := w.run("run", "python", "--", "x")
		if out != (sequence.Outcome{Exit: 125, Cause: "setup"}) || w.exists(".output/exec") {
			t.Fatalf("outcome %+v", out)
		}
	})
}

func tarWith(t *testing.T, id string) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	manifest := []byte(`[{"Config":"` + strings.TrimPrefix(id, "sha256:") + `.json","RepoTags":[],"Layers":[]}]`)
	if err := tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0o644, Size: int64(len(manifest))}); err != nil {
		t.Fatal(err)
	}
	tw.Write(manifest)
	tw.Close()
	return buf.Bytes()
}

// ociTarWith builds a containerd-store docker-save tarball: index.json names indexID, manifest.json names the config.
func ociTarWith(t *testing.T, indexID, configHex string) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	files := []struct{ name, body string }{
		{"oci-layout", `{"imageLayoutVersion":"1.0.0"}`},
		{"index.json", `{"schemaVersion":2,"manifests":[{"mediaType":"application/vnd.oci.image.index.v1+json","digest":"` + indexID + `","size":1}]}`},
		{"manifest.json", `[{"Config":"blobs/sha256/` + configHex + `","RepoTags":["exec-python:1"],"Layers":[]}]`},
	}
	for _, f := range files {
		if err := tw.WriteHeader(&tar.Header{Name: f.name, Mode: 0o644, Size: int64(len(f.body))}); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte(f.body))
	}
	tw.Close()
	return buf.Bytes()
}

func TestImageLoadOCILayout(t *testing.T) {
	cfg := strings.Repeat("c", 64)
	t.Run("profile pins the index digest", func(t *testing.T) {
		w := newWorld(t)
		delete(w.eng.images, imageID)
		w.write(".tools/python/image.tar", string(ociTarWith(t, imageID, cfg)))
		out, _, se := w.run("run", "python", "--exec-id", "e1", "--", "x")
		if out.Cause != "ok" || w.eng.loads != 1 {
			t.Fatalf("outcome %+v loads %d stderr %q", out, w.eng.loads, se)
		}
	})
	t.Run("list reports tar match", func(t *testing.T) {
		w := newWorld(t)
		w.write(".tools/python/image.tar", string(ociTarWith(t, imageID, cfg)))
		es, err := app.Entries(context.Background(), w.ws, nil)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, e := range es {
			if e.Name == "python" {
				found = true
				if !e.TarPresent || !e.TarMatches {
					t.Fatalf("entry %+v", e)
				}
			}
		}
		if !found {
			t.Fatalf("python not listed: %+v", es)
		}
	})
	t.Run("neither digest matches", func(t *testing.T) {
		w := newWorld(t)
		delete(w.eng.images, imageID)
		w.write(".tools/python/image.tar", string(ociTarWith(t, "sha256:"+strings.Repeat("b", 64), cfg)))
		out, _, _ := w.run("run", "python", "--exec-id", "e1", "--", "x")
		if out.Cause != "setup" || w.eng.loads != 0 {
			t.Fatalf("outcome %+v loads %d", out, w.eng.loads)
		}
	})
}

func TestImageLoad(t *testing.T) {
	t.Run("loads when absent", func(t *testing.T) {
		w := newWorld(t)
		delete(w.eng.images, imageID)
		w.write(".tools/python/image.tar", string(tarWith(t, imageID)))
		out, _, se := w.run("run", "python", "--exec-id", "e1", "--", "x")
		if out.Cause != "ok" || w.eng.loads != 1 {
			t.Fatalf("outcome %+v loads %d stderr %q", out, w.eng.loads, se)
		}
	})
	t.Run("not loaded again when present", func(t *testing.T) {
		w := newWorld(t)
		w.write(".tools/python/image.tar", string(tarWith(t, imageID)))
		if out, _, _ := w.run("run", "python", "--exec-id", "e1", "--", "x"); out.Cause != "ok" || w.eng.loads != 0 {
			t.Fatalf("outcome %+v loads %d", out, w.eng.loads)
		}
	})
	t.Run("missing tarball", func(t *testing.T) {
		w := newWorld(t)
		delete(w.eng.images, imageID)
		out, _, _ := w.run("run", "python", "--exec-id", "e1", "--", "x")
		if out != (sequence.Outcome{Exit: 125, Cause: "setup"}) || w.result("e1").Cause != "setup" {
			t.Fatalf("outcome %+v", out)
		}
		w.noContainers()
	})
	t.Run("tarball of another image", func(t *testing.T) {
		w := newWorld(t)
		delete(w.eng.images, imageID)
		w.write(".tools/python/image.tar", string(tarWith(t, "sha256:"+strings.Repeat("b", 64))))
		out, _, _ := w.run("run", "python", "--exec-id", "e1", "--", "x")
		if out.Cause != "setup" || w.eng.loads != 0 {
			t.Fatalf("outcome %+v loads %d", out, w.eng.loads)
		}
	})
}

func TestSweepRemovesOnlyLabelled(t *testing.T) {
	w := newWorld(t)
	w.eng.addExisting("ephemeral-a", "running", map[string]string{"ephemeral.exec": "a", "agent-graph.run": "r1"}, 1)
	w.eng.addExisting("ephemeral-b", "running", map[string]string{"ephemeral.exec": "b", "agent-graph.run": "r2"}, 1)
	w.eng.addExisting("ephemeral-c", "created", map[string]string{"ephemeral.exec": "c"}, 1)
	d := &sequence.Deps{WS: w.ws, Engine: w.eng}
	if out := sequence.Sweep(d, "agent-graph.run", "r1"); out.Exit != 0 {
		t.Fatalf("outcome %+v", out)
	}
	if len(w.eng.removed) != 1 || w.eng.removed[0] != "ephemeral-a" || w.eng.remaining() != 2 {
		t.Fatalf("removed %v remaining %d", w.eng.removed, w.eng.remaining())
	}
}

func TestExecDirIsNeverReused(t *testing.T) {
	w := newWorld(t)
	if out, _, _ := w.run("run", "python", "--exec-id", "e1", "--", "x"); out.Cause != "ok" {
		t.Fatalf("first: %+v", out)
	}
	if out, _, _ := w.run("run", "python", "--exec-id", "e1", "--", "x"); out != (sequence.Outcome{Exit: 125, Cause: "usage"}) {
		t.Fatalf("second: %+v", out)
	}
	// result.json of the first run is untouched.
	if r := w.result("e1"); r.Cause != "ok" {
		t.Fatalf("result %+v", r)
	}
}

func TestGeneratedExecID(t *testing.T) {
	w := newWorld(t)
	_, _, se := w.run("run", "python", "--", "x")
	line := strings.SplitN(se, "\n", 2)[0]
	id := filepath.Base(strings.TrimPrefix(line, "run_ephemeral_container: exec_dir="))
	if !cli.ExecIDRe.MatchString(id) || !w.exists(".output/exec/"+id+"/result.json") {
		t.Fatalf("generated exec id %q (stderr %q)", id, se)
	}
}
