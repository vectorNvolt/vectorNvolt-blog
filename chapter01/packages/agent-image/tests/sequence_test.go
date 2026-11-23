package tests

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"agent-image/internal/config"
	"agent-image/internal/sequence"
	"agent-image/internal/services"
	"agent-image/internal/steps"
)

// ---- fake runner ----

type rule struct {
	contains string
	stderr   string
	code     int
	err      error
}

type fakeRunner struct {
	mu    sync.Mutex
	calls []string
	rules []rule

	existing    bool // agent-image:<tag> already exists in agent-host
	inspectFail bool // docker image inspect fails after docker build
	built       bool
	buildStdin  []byte // stdin passed to docker build (the tar build context)
	buildOut    string // stdout of docker build
	buildErr    string // stderr of docker build

	envOut     string // stdout of the Config.Env inspect (default: a clean env)
	labelsOut  string // stdout of the Config.Labels inspect (default: null)
	historyOut string // stdout of docker history

	// agent-base:<hash> image: present in agent-host unless baseMissing; a docker load or a base build makes it present.
	baseMissing    bool
	baseLabelWrong bool   // the base image carries a different hash label
	baseLoaded     bool   // docker load ran
	baseBuilt      bool   // docker build -f Dockerfile.base ran
	loadStdin      []byte // stdin passed to docker load
	baseStdin      []byte // stdin passed to the base docker build

	onCall func(joined string) // called for every call after it is recorded (interrupt tests)
}

// hostPrefix precedes every command in the joined call strings (the commands run directly: none).
const hostPrefix = ""

func (f *fakeRunner) Run(_ context.Context, stdin []byte, live io.Writer, args ...string) ([]byte, []byte, int, error) {
	joined := strings.Join(args, " ")
	f.mu.Lock()
	f.calls = append(f.calls, joined)
	f.mu.Unlock()
	if f.onCall != nil {
		f.onCall(joined)
	}
	isBaseBuild := strings.Contains(joined, "docker build") && strings.Contains(joined, "-f Dockerfile.base")
	if isBaseBuild {
		f.baseStdin = stdin
	} else if strings.Contains(joined, "docker build") {
		f.buildStdin = stdin
		if live != nil { // like ExecRunner: live copy in addition to the capture
			io.WriteString(live, f.buildOut+f.buildErr)
		}
		if f.buildOut != "" || f.buildErr != "" {
			f.built = true
			return []byte(f.buildOut), []byte(f.buildErr), 0, nil
		}
	}
	for _, r := range f.rules {
		if strings.Contains(joined, r.contains) {
			return nil, []byte(r.stderr), r.code, r.err
		}
	}
	switch {
	case isBaseBuild:
		f.baseBuilt = true
	case joined == hostPrefix+"docker load":
		f.baseLoaded = true
		f.loadStdin = stdin
	case strings.HasPrefix(joined, hostPrefix+"docker image inspect --format {{json .Config.Labels}} agent-base:"):
		if f.baseMissing && !f.baseLoaded && !f.baseBuilt {
			return nil, []byte("Error: No such image"), 1, nil
		}
		hash := joined[strings.LastIndex(joined, "agent-base:")+len("agent-base:"):]
		if f.baseLabelWrong {
			hash = "deadbeefdeadbeef"
		}
		return []byte(`{"` + config.BaseHashLabel + `":"` + hash + `"}`), nil, 0, nil
	case strings.HasPrefix(joined, hostPrefix+"docker image inspect --format {{json .Config.Env}}"):
		if f.envOut != "" {
			return []byte(f.envOut), nil, 0, nil
		}
		return []byte(`["PATH=/opt/venv/bin:/usr/bin","CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1"]`), nil, 0, nil
	case strings.HasPrefix(joined, hostPrefix+"docker image inspect --format {{json .Config.Labels}}"):
		if f.labelsOut != "" {
			return []byte(f.labelsOut), nil, 0, nil
		}
		return []byte("null"), nil, 0, nil
	case strings.HasPrefix(joined, hostPrefix+"docker history --no-trunc --format {{.CreatedBy}}"):
		return []byte(f.historyOut), nil, 0, nil
	case strings.HasPrefix(joined, hostPrefix+"docker build"):
		f.built = true
	case strings.HasPrefix(joined, hostPrefix+"docker image inspect"):
		if f.built {
			if f.inspectFail {
				return nil, []byte("Error: No such image"), 1, nil
			}
			return []byte("[{}]"), nil, 0, nil
		}
		if f.existing {
			return []byte("[{}]"), nil, 0, nil
		}
		return nil, []byte("Error: No such image"), 1, nil
	}
	return nil, nil, 0, nil
}

func (f *fakeRunner) has(substr string) bool { return f.count(substr) > 0 }

func (f *fakeRunner) count(substr string) int {
	n := 0
	for _, c := range f.calls {
		if strings.Contains(c, substr) {
			n++
		}
	}
	return n
}

func (f *fakeRunner) index(substr string) int {
	for i, c := range f.calls {
		if strings.Contains(c, substr) {
			return i
		}
	}
	return -1
}

// ---- harness ----

type result struct {
	code        int
	stdout, err string
}

// chdir changes into dir for the test and restores the old directory afterwards.
func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(old) })
}

// agentGraphFiles are the package files contextDir writes into agent_graph/ that belong in the
// build context; agentGraphIgnored are written too but must stay out of it.
var (
	agentGraphFiles = []string{
		"agent_graph/__init__.py",
		"agent_graph/__main__.py",
		"agent_graph/nodes/__init__.py",
		"agent_graph/nodes/n01_start.py",
		"agent_graph/schemas/process_v1.schema.yml",
	}
	agentGraphIgnored = []string{
		"agent_graph/__pycache__/main.cpython-314.pyc",
		"agent_graph/nodes/__pycache__/n01_start.cpython-314.pyc",
		"agent_graph/main.pyc",
		"agent_graph/agent_graph.egg-info/PKG-INFO",
		"agent_graph/agent_graph.egg-info/top_level.py",
		"agent_graph/notes.yml",
		"agent_graph/schemas/README.md",
	}
)

// wantContextFiles is the sorted list of tar entries for a contextDir.
func wantContextFiles() []string {
	want := append(append([]string(nil), config.BuildContextFiles...), config.BinFiles...)
	want = append(want, agentGraphFiles...)
	sort.Strings(want)
	return want
}

// writeFile writes a slash path under dir, creating its parent directories.
func writeFile(t *testing.T, dir, name string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("content of "+name+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// contextDir makes a temp dir holding every build file and the agent_graph package directory
// (including files that must be left out) and the current directory for the test.
func contextDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range append(append([]string(nil), config.BuildContextFiles...), config.BinFiles...) {
		writeFile(t, dir, name)
	}
	for _, name := range append(append([]string(nil), agentGraphFiles...), agentGraphIgnored...) {
		writeFile(t, dir, name)
	}
	writeFile(t, dir, config.BaseDockerfileName) // not part of the agent build context tar
	chdir(t, dir)
	return dir
}

// baseHashOf is the base image hash of the Dockerfile.base in dir.
func baseHashOf(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, config.BaseDockerfileName))
	if err != nil {
		t.Fatal(err)
	}
	return services.BaseHash(data)
}

// run executes the sequence in a fresh directory holding the build files.
func run(t *testing.T, fr *fakeRunner, tag, stdin string) result {
	t.Helper()
	contextDir(t)
	return runHere(t, fr, tag, stdin)
}

// runHere is run in the current directory as it is.
func runHere(t *testing.T, fr *fakeRunner, tag, stdin string) result {
	t.Helper()
	return runCtx(t, context.Background(), fr, tag, strings.NewReader(stdin))
}

// runCtx is runHere with an explicit context and stdin reader.
func runCtx(t *testing.T, ctx context.Context, fr *fakeRunner, tag string, stdin io.Reader) result {
	t.Helper()
	return runOpts(t, ctx, fr, tag, stdin, false)
}

// runOpts is runCtx with the --rebuild-base flag.
func runOpts(t *testing.T, ctx context.Context, fr *fakeRunner, tag string, stdin io.Reader, rebuildBase bool) result {
	t.Helper()
	var out, errb bytes.Buffer
	code := sequence.Run(ctx, sequence.Deps{
		Config:      config.Load(func(string) string { return "" }),
		Runner:      fr,
		Tag:         tag,
		RebuildBase: rebuildBase,
		Stdin:       stdin,
		Stdout:      &out,
		Stderr:      &errb,
	})
	return result{code, out.String(), errb.String()}
}

const successText = "agent image agent-image:latest built in agent-host"

func noSuccess(t *testing.T, r result) {
	t.Helper()
	if strings.Contains(r.stdout, "built in agent-host") {
		t.Errorf("stdout must carry no success text: %q", r.stdout)
	}
}

// ---- tests ----

func TestHappyPath(t *testing.T) {
	fr := &fakeRunner{}
	dir := contextDir(t)
	hash := baseHashOf(t, dir)
	r := runHere(t, fr, "latest", "")
	if r.code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", r.code, r.err)
	}
	if !strings.Contains(r.stdout, successText) {
		t.Errorf("missing success report: %q", r.stdout)
	}
	if r.err != "" {
		t.Errorf("stderr should be empty: %q", r.err)
	}
	if strings.Contains(r.stdout, "overwrite agent-image:") {
		t.Error("no prompt when the image is absent")
	}
	order := []string{
		hostPrefix + "docker version",
		hostPrefix + "docker image inspect agent-image:latest",
		hostPrefix + "docker image inspect --format {{json .Config.Labels}} agent-base:" + hash,
		hostPrefix + "docker build --progress=plain -f Dockerfile.agent --build-arg BASE_IMAGE=agent-base:" + hash + " -t agent-image:latest -",
	}
	last := -1
	for _, want := range order {
		i := fr.index(want)
		if i <= last {
			t.Errorf("%q out of order or missing (index %d after %d): %v", want, i, last, fr.calls)
		}
		last = i
	}
	if len(fr.calls) != 8 || fr.calls[4] != hostPrefix+"docker image inspect agent-image:latest" {
		t.Errorf("expected version, inspect, base inspect, build, inspect, env, labels, history: %v", fr.calls)
	}
	if fr.has("-f Dockerfile.base") || fr.has("docker load") {
		t.Errorf("a present base image must not be built or loaded: %v", fr.calls)
	}
	if !strings.Contains(r.stdout, "[5/8] ensure base image in agent-host") ||
		!strings.Contains(r.stdout, "base image agent-base:"+hash+" present") {
		t.Errorf("base progress missing: %q", r.stdout)
	}
	if len(fr.buildStdin) == 0 {
		t.Error("the build context tar must be sent on stdin")
	}
}

func TestOnlyAgentHostIsContacted(t *testing.T) {
	fr := &fakeRunner{existing: true}
	if r := run(t, fr, "latest", "y\n"); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	for _, c := range fr.calls {
		if !strings.HasPrefix(c, "docker ") {
			t.Errorf("every command must be a direct docker call: %s", c)
		}
		for _, bad := range []string{"docker run", "docker create", "docker start", "docker save", "docker load",
			"daemon.json", "systemctl", "docker exec", " cat ", "install ", " rm "} {
			if strings.Contains(c, bad) {
				t.Errorf("forbidden call %q in: %s", bad, c)
			}
		}
	}
}

func TestCustomTag(t *testing.T) {
	fr := &fakeRunner{}
	r := run(t, fr, "v1.2_rc-1", "")
	if r.code != 0 || !fr.has("docker build --progress=plain -f Dockerfile.agent --build-arg BASE_IMAGE=agent-base:") ||
		!fr.has(" -t agent-image:v1.2_rc-1 -") {
		t.Fatalf("exit %d, calls %v", r.code, fr.calls)
	}
}

func TestInvalidTagExits1WithNothingRun(t *testing.T) {
	for _, tag := range []string{"", ".x", "-x", "a b", "a/b", "a:b", strings.Repeat("a", 129)} {
		fr := &fakeRunner{}
		r := run(t, fr, tag, "")
		if r.code != 1 || !strings.Contains(r.err, "usage: create_agent_image") {
			t.Errorf("tag %q: exit %d, stderr %q", tag, r.code, r.err)
		}
		if len(fr.calls) != 0 {
			t.Errorf("tag %q: nothing may run, got %v", tag, fr.calls)
		}
		noSuccess(t, r)
	}
	if _, err := steps.ParseArgs([]string{"--tag", strings.Repeat("a", 128)}); err != nil {
		t.Errorf("128 chars must parse: %v", err)
	}
}

func TestParseArgs(t *testing.T) {
	if tag, err := steps.ParseArgs(nil); err != nil || tag != "latest" {
		t.Errorf("default: %q %v", tag, err)
	}
	if tag, err := steps.ParseArgs([]string{"--tag", "v1"}); err != nil || tag != "v1" {
		t.Errorf("--tag: %q %v", tag, err)
	}
	for _, bad := range [][]string{{"extra"}, {"--tag", "v1", "extra"}, {"--bogus"}, {"--tag"}} {
		if _, err := steps.ParseArgs(bad); err == nil {
			t.Errorf("args %v must be rejected", bad)
		}
	}
}

func TestEngineUnreachableExits1WithoutBuild(t *testing.T) {
	cases := map[string]rule{
		"engine down":    {contains: hostPrefix + "docker version", stderr: "Cannot connect to the Docker daemon", code: 1},
		"docker missing": {contains: "docker version", code: -1, err: errors.New("executable file not found")},
	}
	for name, rl := range cases {
		t.Run(name, func(t *testing.T) {
			fr := &fakeRunner{rules: []rule{rl}}
			r := run(t, fr, "latest", "")
			if r.code != 1 || r.err == "" {
				t.Fatalf("exit %d, stderr %q", r.code, r.err)
			}
			if fr.has("docker build") || fr.has("docker image inspect") {
				t.Errorf("nothing may follow an unreachable engine: %v", fr.calls)
			}
			noSuccess(t, r)
		})
	}
}

func TestBuildFailureExits1(t *testing.T) {
	fr := &fakeRunner{rules: []rule{{contains: "docker build", stderr: "failed to resolve source", code: 1}}}
	r := run(t, fr, "latest", "")
	if r.code != 1 || !strings.Contains(r.err, "failed to resolve source") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.count("docker image inspect") != 2 { // existence check and base check only
		t.Errorf("no verification may follow a failed build: %v", fr.calls)
	}
	noSuccess(t, r)
}

func TestBuildOutputStreamsToStderrOnly(t *testing.T) {
	fr := &fakeRunner{buildOut: "#5 RUN apt-get update\n", buildErr: "#6 DONE 1.2s\n"}
	r := run(t, fr, "latest", "")
	if r.code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", r.code, r.err)
	}
	for _, want := range []string{"#5 RUN apt-get update", "#6 DONE 1.2s"} {
		if !strings.Contains(r.err, want) {
			t.Errorf("build output %q must reach stderr: %q", want, r.err)
		}
		if strings.Contains(r.stdout, want) {
			t.Errorf("build output %q must not reach stdout: %q", want, r.stdout)
		}
	}
	// docker version / image inspect output stays captured.
	if strings.Contains(r.err, "[{}]") || strings.Contains(r.stdout, "[{}]") {
		t.Errorf("inspect output must not be streamed: %q %q", r.err, r.stdout)
	}
}

func TestBuildFailureKeepsStderrTail(t *testing.T) {
	fr := &fakeRunner{rules: []rule{{contains: "docker build", stderr: "failed to resolve source", code: 1}}}
	r := run(t, fr, "latest", "")
	if r.code != 1 || !strings.Contains(r.err, "stderr: failed to resolve source") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
}

func TestExistingImageDeclinedExits0WithoutBuild(t *testing.T) {
	for _, answer := range []string{"", "\n", "n\n", "no\n", "yes\n", "Y es\n"} {
		fr := &fakeRunner{existing: true}
		r := run(t, fr, "latest", answer)
		if r.code != 0 {
			t.Fatalf("answer %q: exit %d, stderr %q", answer, r.code, r.err)
		}
		if !strings.Contains(r.stdout, "overwrite agent-image:latest? [y/N] ") {
			t.Errorf("answer %q: prompt missing: %q", answer, r.stdout)
		}
		if !strings.Contains(r.stdout, "kept") {
			t.Errorf("answer %q: kept message missing: %q", answer, r.stdout)
		}
		if fr.has("docker build") {
			t.Errorf("answer %q: no build allowed: %v", answer, fr.calls)
		}
		noSuccess(t, r)
	}
}

func TestExistingImageConfirmedBuilds(t *testing.T) {
	for _, answer := range []string{"y\n", "Y\n", "y"} {
		fr := &fakeRunner{existing: true}
		r := run(t, fr, "latest", answer)
		if r.code != 0 || !strings.Contains(r.stdout, successText) {
			t.Fatalf("answer %q: exit %d, stdout %q, stderr %q", answer, r.code, r.stdout, r.err)
		}
		if !strings.Contains(r.stdout, "overwrite agent-image:latest? [y/N] ") {
			t.Errorf("answer %q: prompt missing: %q", answer, r.stdout)
		}
		if !fr.has("docker build") {
			t.Errorf("answer %q: build expected", answer)
		}
	}
}

func TestVerifyFailureExits2(t *testing.T) {
	fr := &fakeRunner{inspectFail: true}
	r := run(t, fr, "latest", "")
	if r.code != 2 || !strings.Contains(r.err, "UNVERIFIED") || !strings.Contains(r.err, "No such image") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	noSuccess(t, r)
}

func TestExistenceCheckEngineErrorExits1WithoutPromptOrBuild(t *testing.T) {
	cases := map[string]rule{
		"other exit":     {contains: "docker image inspect", stderr: "Cannot connect to the Docker daemon", code: 1},
		"docker failure": {contains: "docker image inspect", code: -1, err: errors.New("docker vanished")},
	}
	for name, rl := range cases {
		t.Run(name, func(t *testing.T) {
			fr := &fakeRunner{existing: true, rules: []rule{rl}}
			r := run(t, fr, "latest", "y\n")
			if r.code != 1 || r.err == "" {
				t.Fatalf("exit %d, stderr %q", r.code, r.err)
			}
			if strings.Contains(r.stdout, "overwrite agent-image:") {
				t.Errorf("no prompt allowed: %q", r.stdout)
			}
			if fr.has("docker build") {
				t.Errorf("no build allowed: %v", fr.calls)
			}
			noSuccess(t, r)
		})
	}
}

func TestCredentialFindingExits2WithoutPrintingValue(t *testing.T) {
	cases := map[string]*fakeRunner{
		"env name":  {envOut: `["MY_API_KEY=abc"]`},
		"env value": {envOut: `["FOO=gh` + `p_TOPSECRETVALUE"]`},
		"label":     {labelsOut: `{"note":"github_` + `pat_TOPSECRETVALUE"}`},
		"history":   {historyOut: "RUN /bin/sh -c echo sk-" + "ant-TOPSECRETVALUE\nRUN true\n"},
	}
	for name, fr := range cases {
		t.Run(name, func(t *testing.T) {
			r := run(t, fr, "latest", "")
			if r.code != 2 || !strings.Contains(r.err, "UNVERIFIED") {
				t.Fatalf("exit %d, stderr %q", r.code, r.err)
			}
			if strings.Contains(r.err, "TOPSECRETVALUE") || strings.Contains(r.stdout, "TOPSECRETVALUE") {
				t.Errorf("matched value must never be printed: %q", r.err)
			}
			noSuccess(t, r)
		})
	}
	r := run(t, &fakeRunner{envOut: `["MY_API_KEY=abc"]`}, "latest", "")
	if !strings.Contains(r.err, "MY_API_KEY") {
		t.Errorf("message must name the variable: %q", r.err)
	}
	r = run(t, &fakeRunner{historyOut: "RUN a\nRUN echo gh" + "p_x\n"}, "latest", "")
	if !strings.Contains(r.err, "step 2") {
		t.Errorf("message must name the history step: %q", r.err)
	}
}

func TestCredentialCheckEngineErrorExits2(t *testing.T) {
	for _, contains := range []string{"{{json .Config.Env}}", "{{json .Config.Labels}} agent-image:", "docker history"} {
		fr := &fakeRunner{rules: []rule{{contains: contains, stderr: "engine gone", code: 1}}}
		r := run(t, fr, "latest", "")
		if r.code != 2 || !strings.Contains(r.err, "UNVERIFIED") {
			t.Errorf("%s: exit %d, stderr %q", contains, r.code, r.err)
		}
		noSuccess(t, r)
	}
}

func TestDockerfileHoldsNoCredentials(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", config.DockerfileName))
	if err != nil {
		t.Fatal(err)
	}
	foundTraffic := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		fields := strings.Fields(line)
		if len(fields) < 2 || strings.HasPrefix(line, "#") {
			continue
		}
		switch strings.ToUpper(fields[0]) {
		case "ENV", "ARG":
			name, _, _ := strings.Cut(fields[1], "=")
			if steps.CredentialName(name) {
				t.Errorf("credential-like %s name: %s", fields[0], name)
			}
			if fields[0] == "ENV" && line == "ENV CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1" {
				foundTraffic = true
			}
		case "COPY", "ADD":
			var args []string
			for _, a := range fields[1:] {
				if !strings.HasPrefix(a, "--") {
					args = append(args, a)
				}
			}
			ok := len(args) == 2 && (args[0] == "requirements.lock" || args[0] == config.AgentGraphDir)
			for _, b := range config.BinFiles {
				if len(args) == 2 && args[0] == b {
					ok = true
				}
			}
			if !ok {
				t.Errorf("only requirements.lock, %s and %v may be copied: %s", config.AgentGraphDir, config.BinFiles, line)
			}
		}
	}
	if !foundTraffic {
		t.Error("Dockerfile.agent must contain ENV CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1")
	}
}

// dockerfileInstructions returns the instructions of Dockerfile.agent with comment lines
// dropped and backslash continuations joined into one line each.
func dockerfileInstructions(t *testing.T) []string {
	t.Helper()
	return instructionsOf(t, config.DockerfileName)
}

// instructionsOf is dockerfileInstructions for the named Dockerfile in the package directory.
func instructionsOf(t *testing.T, name string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", name))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	cur := ""
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || (line == "" && cur == "") {
			continue
		}
		cont := strings.HasSuffix(line, "\\")
		cur += " " + strings.TrimSpace(strings.TrimSuffix(line, "\\"))
		if !cont {
			out = append(out, strings.TrimSpace(cur))
			cur = ""
		}
	}
	if cur != "" {
		out = append(out, strings.TrimSpace(cur))
	}
	return out
}

// runCommands splits every RUN instruction into its "&&"-separated shell commands.
func runCommands(instructions []string) []string {
	var cmds []string
	for _, in := range instructions {
		if f := strings.Fields(in); len(f) > 0 && strings.EqualFold(f[0], "RUN") {
			for _, c := range strings.Split(strings.TrimSpace(in[len(f[0]):]), "&&") {
				cmds = append(cmds, strings.TrimSpace(c))
			}
		}
	}
	return cmds
}

// SSR-IMG-004: every component pinned to an exact version, every download checked by hash.
func TestDockerfilePinsEveryComponent(t *testing.T) {
	ins := dockerfileInstructions(t)
	cmds := runCommands(ins)
	baseIns := instructionsOf(t, config.BaseDockerfileName)
	baseCmds := runCommands(baseIns)

	// Dockerfile.agent: the only FROM is the BASE_IMAGE build arg (no default) and it holds no apt or snapshot.
	froms := 0
	for _, in := range ins {
		if strings.HasPrefix(strings.ToUpper(in), "FROM ") {
			froms++
			if in != "FROM ${BASE_IMAGE}" {
				t.Errorf("Dockerfile.agent FROM must be ${BASE_IMAGE}: %s", in)
			}
		}
		if in != "ARG BASE_IMAGE" && strings.Contains(in, "BASE_IMAGE=") {
			t.Errorf("BASE_IMAGE must have no default: %s", in)
		}
		if strings.Contains(in, "UBUNTU_SNAPSHOT") || strings.Contains(in, "apt-get") || strings.Contains(in, "snapshot.ubuntu.com") {
			t.Errorf("apt and the snapshot belong to Dockerfile.base: %s", in)
		}
	}
	if froms != 1 {
		t.Errorf("Dockerfile.agent needs exactly one FROM, got %d", froms)
	}
	if len(ins) < 2 || ins[0] != "ARG BASE_IMAGE" || ins[1] != "FROM ${BASE_IMAGE}" {
		t.Error("Dockerfile.agent must start with ARG BASE_IMAGE and FROM ${BASE_IMAGE}")
	}

	// Dockerfile.base: digest-pinned FROM, snapshot ARG, exact apt versions from the snapshot.
	fromRe := regexp.MustCompile(`^FROM \S+:\S+@sha256:[0-9a-f]{64}$`)
	baseFroms := 0
	for _, in := range baseIns {
		if strings.HasPrefix(strings.ToUpper(in), "FROM ") {
			baseFroms++
			if !fromRe.MatchString(in) {
				t.Errorf("Dockerfile.base FROM must name a tag and an @sha256 digest: %s", in)
			}
		}
	}
	if baseFroms != 1 {
		t.Errorf("Dockerfile.base needs exactly one FROM, got %d", baseFroms)
	}

	snapRe := regexp.MustCompile(`^ARG UBUNTU_SNAPSHOT=\d{8}T\d{6}Z$`)
	hasSnapArg := false
	for _, in := range baseIns {
		hasSnapArg = hasSnapArg || snapRe.MatchString(in)
	}
	if !hasSnapArg {
		t.Error("Dockerfile.base must set ARG UBUNTU_SNAPSHOT=<YYYYMMDDTHHMMSSZ>")
	}
	baseText := strings.Join(baseIns, "\n")
	for _, want := range []string{`'Acquire::Retries "10";'`, `'Acquire::Retries::Delay::Maximum "60";'`} {
		if !strings.Contains(baseText, want) {
			t.Errorf("Dockerfile.base apt retry config must contain %s", want)
		}
	}

	pkgRe := regexp.MustCompile(`^[a-z0-9][a-z0-9.+-]*=[0-9][^=\s]*$`)
	installs, snapUpdate, snapInstall := 0, false, false
	for _, c := range baseCmds {
		f := strings.Fields(c)
		if len(f) == 0 || (f[0] != "apt-get" && f[0] != "apt") {
			continue
		}
		snap := strings.Contains(c, `--snapshot "${UBUNTU_SNAPSHOT}"`)
		isInstall := false
		for i := 1; i < len(f); i++ {
			switch {
			case f[i] == "-o" || f[i] == "--snapshot" || f[i] == "-S":
				i++
			case strings.HasPrefix(f[i], "-"):
			case !isInstall && f[i] == "update":
				snapUpdate = snapUpdate || snap
			case !isInstall && f[i] == "install":
				isInstall = true
				installs++
				snapInstall = snapInstall || snap
			case isInstall && !pkgRe.MatchString(f[i]):
				t.Errorf("apt package without an exact =version pin: %q in %s", f[i], c)
			}
		}
	}
	if installs == 0 {
		t.Error("no apt-get install found")
	}
	if !snapUpdate || !snapInstall {
		t.Error(`apt-get update and apt-get install must both use --snapshot "${UBUNTU_SNAPSHOT}"`)
	}

	// claude CLI: each tarball fetched with curl -o <file> is listed with a sha512 literal, checked
	// with sha512sum -c, and npm installs offline from local files only.
	var claude string
	for _, in := range ins {
		if strings.Contains(in, "npm install") {
			claude = in
		}
	}
	if claude == "" {
		t.Fatal("no npm install step for the claude CLI")
	}
	if !strings.Contains(claude, "sha512sum -c") {
		t.Error("claude CLI tarballs must be checked with sha512sum -c")
	}
	files := regexp.MustCompile(`curl [^&]*-o (\S+)`).FindAllStringSubmatch(claude, -1)
	if len(files) < 2 {
		t.Errorf("expected the wrapper and the native tarball to be downloaded, got %d", len(files))
	}
	for _, m := range files {
		if !regexp.MustCompile(`\b[0-9a-f]{128} ` + regexp.QuoteMeta(m[1]) + `\b`).MatchString(claude) {
			t.Errorf("downloaded %s has no sha512 literal", m[1])
		}
	}
	for _, c := range runCommands([]string{claude}) {
		if !strings.HasPrefix(c, "npm install") {
			continue
		}
		if !strings.Contains(c, "--offline") {
			t.Errorf("npm must install offline from the verified tarballs: %s", c)
		}
		for _, a := range strings.Fields(c)[2:] {
			if !strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "/") && !strings.HasPrefix(a, "./") {
				t.Errorf("npm install argument is not a local file: %s", a)
			}
		}
	}
	versionRe := regexp.MustCompile(`^ARG CLAUDE_CODE_VERSION=\d+\.\d+\.\d+$`)
	hasVersion := false
	for _, in := range ins {
		hasVersion = hasVersion || versionRe.MatchString(in)
	}
	if !hasVersion {
		t.Error("CLAUDE_CODE_VERSION must be an exact x.y.z version")
	}

	pips := 0
	for _, c := range cmds {
		if strings.Contains(c, "pip install") {
			pips++
			if !strings.Contains(c, "--require-hashes") {
				t.Errorf("pip install must use --require-hashes: %s", c)
			}
		}
	}
	if pips == 0 {
		t.Error("no pip install found")
	}
}

func TestNoLocalFileWritten(t *testing.T) {
	dir := contextDir(t)
	if r := runHere(t, &fakeRunner{}, "latest", ""); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != len(config.BuildContextFiles)+3 {
		t.Errorf("working directory must hold only the build files, %s, bin and %s: %v %v", config.BaseDockerfileName, config.AgentGraphDir, entries, err)
	}
}

func TestMissingAgentGraphExits1WithoutDockerCall(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, dir string)
		want  string
	}{
		{"no directory", func(t *testing.T, dir string) {
			if err := os.RemoveAll(filepath.Join(dir, config.AgentGraphDir)); err != nil {
				t.Fatal(err)
			}
		}, "build directory agent_graph not found as a directory in "},
		{"file instead of directory", func(t *testing.T, dir string) {
			if err := os.RemoveAll(filepath.Join(dir, config.AgentGraphDir)); err != nil {
				t.Fatal(err)
			}
			writeFile(t, dir, config.AgentGraphDir)
		}, "build directory agent_graph not found as a directory in "},
		{"no __init__.py", func(t *testing.T, dir string) {
			if err := os.Remove(filepath.Join(dir, config.AgentGraphDir, "__init__.py")); err != nil {
				t.Fatal(err)
			}
		}, "build file agent_graph/__init__.py not found as a regular file in "},
		{"no __main__.py", func(t *testing.T, dir string) {
			if err := os.Remove(filepath.Join(dir, config.AgentGraphDir, "__main__.py")); err != nil {
				t.Fatal(err)
			}
		}, "build file agent_graph/__main__.py not found as a regular file in "},
		{"selected name is a directory", func(t *testing.T, dir string) {
			if err := os.MkdirAll(filepath.Join(dir, config.AgentGraphDir, "x.py"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(dir, config.AgentGraphDir, "x.py"), filepath.Join(dir, config.AgentGraphDir, "y.py")); err != nil {
				t.Skip("symlinks unavailable:", err)
			}
		}, "build file agent_graph/y.py is not a regular file"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := contextDir(t)
			c.setup(t, dir)
			fr := &fakeRunner{}
			r := runHere(t, fr, "latest", "")
			if r.code != 1 || !strings.Contains(r.err, c.want) {
				t.Fatalf("exit %d, stderr %q", r.code, r.err)
			}
			if len(fr.calls) != 0 {
				t.Errorf("nothing may run: %v", fr.calls)
			}
			noSuccess(t, r)
		})
	}
}

// Dockerfile.agent installs agent_graph from the build context with no build backend: a .pth in
// the venv, a /usr/local/bin wrapper and build-time import and CLI checks.
func TestDockerfileInstallsAgentGraph(t *testing.T) {
	ins := dockerfileInstructions(t)
	cmds := runCommands(ins)
	has := func(list []string, s string) bool {
		for _, x := range list {
			if strings.Contains(x, s) {
				return true
			}
		}
		return false
	}
	if !has(ins, "COPY agent_graph /opt/agent_graph/agent_graph") {
		t.Error("Dockerfile.agent must COPY agent_graph /opt/agent_graph/agent_graph")
	}
	for _, want := range []string{
		"printf '%s\\n' /opt/agent_graph > /opt/venv/lib/python3.14/site-packages/agent_graph.pth",
		`'exec /opt/venv/bin/python -m agent_graph "$@"' > /usr/local/bin/agent_graph`,
		"chmod 0755 /usr/local/bin/agent_graph",
		`/opt/venv/bin/python -c "import agent_graph`,
		"agent_graph --help",
	} {
		if !has(cmds, want) {
			t.Errorf("Dockerfile.agent RUN must contain %q", want)
		}
	}
	for _, c := range cmds {
		if strings.Contains(c, "pip install") && strings.Contains(c, "agent_graph") {
			t.Errorf("agent_graph must not be pip-installed (no build backend): %s", c)
		}
	}
}

func TestMissingBuildFileExits1WithoutDockerCall(t *testing.T) {
	for _, missing := range config.BuildContextFiles {
		t.Run(missing, func(t *testing.T) {
			dir := contextDir(t)
			if err := os.Remove(filepath.Join(dir, missing)); err != nil {
				t.Fatal(err)
			}
			fr := &fakeRunner{}
			r := runHere(t, fr, "latest", "")
			if r.code != 1 || !strings.Contains(r.err, missing) || !strings.Contains(r.err, dir) {
				t.Fatalf("exit %d, stderr %q", r.code, r.err)
			}
			if len(fr.calls) != 0 {
				t.Errorf("nothing may run: %v", fr.calls)
			}
			noSuccess(t, r)
		})
	}
}

func TestMissingBinFileExits1WithoutDockerCall(t *testing.T) {
	for _, missing := range config.BinFiles {
		for _, empty := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s empty=%v", missing, empty), func(t *testing.T) {
				dir := contextDir(t)
				p := filepath.Join(dir, filepath.FromSlash(missing))
				if empty {
					if err := os.WriteFile(p, nil, 0o600); err != nil {
						t.Fatal(err)
					}
				} else if err := os.Remove(p); err != nil {
					t.Fatal(err)
				}
				fr := &fakeRunner{}
				r := runHere(t, fr, "latest", "")
				if r.code != 1 || !strings.Contains(r.err, missing) || !strings.Contains(r.err, dir) || !strings.Contains(r.err, "P8") {
					t.Fatalf("exit %d, stderr %q", r.code, r.err)
				}
				if len(fr.calls) != 0 {
					t.Errorf("nothing may run: %v", fr.calls)
				}
				noSuccess(t, r)
			})
		}
	}
}

// ---- SFR-IMG-018: interrupt handling ----

// wantInterrupted asserts exit 130, one stderr line naming the step, no success text and the exact docker calls.
func wantInterrupted(t *testing.T, r result, fr *fakeRunner, step int, calls ...string) {
	t.Helper()
	if r.code != 130 {
		t.Fatalf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.err)
	}
	if strings.Count(r.err, "\n") != 1 || !strings.HasSuffix(r.err, "\n") ||
		!strings.Contains(r.err, fmt.Sprintf("interrupted at step %d", step)) ||
		!strings.Contains(r.err, "exit_state: INTERRUPTED, exit_code: 130") {
		t.Errorf("stderr must be the one interrupt line for step %d: %q", step, r.err)
	}
	noSuccess(t, r)
	if len(fr.calls) != len(calls) {
		t.Fatalf("docker calls %v, want %d calls matching %v", fr.calls, len(calls), calls)
	}
	for i, want := range calls {
		if !strings.Contains(fr.calls[i], want) {
			t.Errorf("call %d = %q, want it to contain %q", i, fr.calls[i], want)
		}
	}
}

func TestInterruptBeforeStartRunsNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fr := &fakeRunner{}
	contextDir(t)
	wantInterrupted(t, runCtx(t, ctx, fr, "latest", strings.NewReader("")), fr, 1)
}

func TestInterruptDuringBuildExits130(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fr := &fakeRunner{
		rules: []rule{{contains: "docker build", stderr: "killed", code: 1}},
		onCall: func(c string) {
			if strings.Contains(c, "docker build") {
				cancel()
			}
		},
	}
	contextDir(t)
	r := runCtx(t, ctx, fr, "latest", strings.NewReader(""))
	wantInterrupted(t, r, fr, 6, "docker version", "docker image inspect agent-image:latest", "agent-base:", "docker build")
	if strings.Contains(r.err, "failed") {
		t.Errorf("no failure report after an interrupt: %q", r.err)
	}
}

func TestInterruptDuringVerifyExits130(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fr := &fakeRunner{
		rules: []rule{{contains: "docker history", stderr: "killed", code: 1}},
		onCall: func(c string) {
			if strings.Contains(c, "docker history") {
				cancel()
			}
		},
	}
	contextDir(t)
	r := runCtx(t, ctx, fr, "latest", strings.NewReader(""))
	wantInterrupted(t, r, fr, 7, "docker version", "docker image inspect", "agent-base:", "docker build", "docker image inspect",
		"{{json .Config.Env}}", "{{json .Config.Labels}}", "docker history")
	if strings.Contains(r.err, "UNVERIFIED") || strings.Contains(r.err, "failed") {
		t.Errorf("no step 6 failure report after an interrupt: %q", r.err)
	}
}

// The interrupt lands at the end of step 6 (docker history returns success after the cancel);
// the post-step check stops the run before step 7 (the success report) runs.
func TestInterruptAfterVerifySkipsSuccessReport(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fr := &fakeRunner{onCall: func(c string) {
		if strings.Contains(c, "docker history") {
			cancel()
		}
	}}
	contextDir(t)
	r := runCtx(t, ctx, fr, "latest", strings.NewReader(""))
	wantInterrupted(t, r, fr, 7, "docker version", "docker image inspect", "agent-base:", "docker build", "docker image inspect",
		"{{json .Config.Env}}", "{{json .Config.Labels}}", "docker history")
	if strings.Contains(r.stdout, successText) {
		t.Errorf("success line must not be printed: %q", r.stdout)
	}
}

func TestInterruptAtOverwritePromptExits130(t *testing.T) {
	pr, pw := io.Pipe() // never written: the read blocks until the interrupt
	t.Cleanup(func() { pw.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(150*time.Millisecond, cancel)
	fr := &fakeRunner{existing: true}
	contextDir(t)
	r := runCtx(t, ctx, fr, "latest", pr)
	wantInterrupted(t, r, fr, 4, "docker version", "docker image inspect")
	if !strings.Contains(r.stdout, "overwrite agent-image:latest? [y/N] ") {
		t.Errorf("prompt missing: %q", r.stdout)
	}
	if strings.Contains(r.stdout, "kept") {
		t.Errorf("an interrupt is not a decline: %q", r.stdout)
	}
}

// cancelThenEOF cancels the context on its first Read and returns EOF, like a console read
// that ends on Ctrl-C before the signal handler has run.
type cancelThenEOF struct{ cancel func() }

func (c cancelThenEOF) Read([]byte) (int, error) {
	c.cancel()
	return 0, io.EOF
}

func TestPromptEOFWithInterruptExits130(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fr := &fakeRunner{existing: true}
	contextDir(t)
	r := runCtx(t, ctx, fr, "latest", cancelThenEOF{cancel})
	wantInterrupted(t, r, fr, 4, "docker version", "docker image inspect")
}

func TestPromptEOFWithoutInterruptStillDeclines(t *testing.T) {
	fr := &fakeRunner{existing: true}
	r := run(t, fr, "latest", "")
	if r.code != 0 || !strings.Contains(r.stdout, "kept") || r.err != "" {
		t.Fatalf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.err)
	}
	if fr.has("docker build") {
		t.Errorf("no build allowed: %v", fr.calls)
	}
}

// ---- base image (agent-base:<hash of Dockerfile.base>) ----

func baseBuildCall(hash string) string {
	return hostPrefix + "docker build --progress=plain -f Dockerfile.base --label " + config.BaseHashLabel + "=" + hash +
		" -t agent-base:" + hash + " -"
}

func TestBasePresentSkipsBaseBuild(t *testing.T) {
	dir := contextDir(t)
	hash := baseHashOf(t, dir)
	writeFile(t, dir, config.BaseArchiveName(hash)) // an archive is ignored when the image is present
	fr := &fakeRunner{}
	r := runHere(t, fr, "latest", "")
	if r.code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", r.code, r.err)
	}
	if fr.has("-f Dockerfile.base") || fr.has("docker load") {
		t.Errorf("no base build or load expected: %v", fr.calls)
	}
	if !strings.Contains(r.stdout, "base image agent-base:"+hash+" present") {
		t.Errorf("present message missing: %q", r.stdout)
	}
}

func TestBaseMissingWithArchiveLoadsIt(t *testing.T) {
	dir := contextDir(t)
	hash := baseHashOf(t, dir)
	archive := config.BaseArchiveName(hash)
	writeFile(t, dir, archive)
	fr := &fakeRunner{baseMissing: true}
	r := runHere(t, fr, "latest", "")
	if r.code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", r.code, r.err)
	}
	if fr.count(hostPrefix+"docker load") != 1 || fr.has("-f Dockerfile.base") {
		t.Errorf("expected one docker load and no base build: %v", fr.calls)
	}
	want, _ := os.ReadFile(filepath.Join(dir, archive))
	if !bytes.Equal(fr.loadStdin, want) || len(want) == 0 {
		t.Errorf("the archive must be streamed to docker load on stdin: %q", fr.loadStdin)
	}
	if fr.count("{{json .Config.Labels}} agent-base:"+hash) != 2 {
		t.Errorf("the base image must be inspected before and after the load: %v", fr.calls)
	}
	if !strings.Contains(r.stdout, "base image loaded from "+archive) {
		t.Errorf("loaded message missing: %q", r.stdout)
	}
	if fr.index("docker load") > fr.index("-f Dockerfile.agent") {
		t.Errorf("load must precede the agent build: %v", fr.calls)
	}
}

func TestBaseMissingBuildsIt(t *testing.T) {
	dir := contextDir(t)
	hash := baseHashOf(t, dir)
	fr := &fakeRunner{baseMissing: true}
	r := runHere(t, fr, "latest", "")
	if r.code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", r.code, r.err)
	}
	if fr.count(baseBuildCall(hash)) != 1 || fr.has("docker load") {
		t.Errorf("expected the labelled base build and no load: %v", fr.calls)
	}
	tr := tar.NewReader(bytes.NewReader(fr.baseStdin))
	var got []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("base stdin is not a tar: %v", err)
		}
		got = append(got, hdr.Name)
	}
	if strings.Join(got, ",") != config.BaseDockerfileName {
		t.Errorf("base context must hold only %s, got %v", config.BaseDockerfileName, got)
	}
	agent := hostPrefix + "docker build --progress=plain -f Dockerfile.agent --build-arg BASE_IMAGE=agent-base:" + hash + " -t agent-image:latest -"
	if !fr.has(agent) {
		t.Errorf("agent build must get BASE_IMAGE=agent-base:%s: %v", hash, fr.calls)
	}
	if fr.index(baseBuildCall(hash)) > fr.index(agent) {
		t.Errorf("base build must precede the agent build: %v", fr.calls)
	}
	if fr.count("{{json .Config.Labels}} agent-base:"+hash) != 2 {
		t.Errorf("the base image must be inspected before and after the build: %v", fr.calls)
	}
}

func TestRebuildBaseForcesBuild(t *testing.T) {
	dir := contextDir(t)
	hash := baseHashOf(t, dir)
	writeFile(t, dir, config.BaseArchiveName(hash))
	fr := &fakeRunner{} // base present and an archive exists: both are skipped
	r := runOpts(t, context.Background(), fr, "latest", strings.NewReader(""), true)
	if r.code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", r.code, r.err)
	}
	if fr.count(baseBuildCall(hash)) != 1 || fr.has("docker load") {
		t.Errorf("--rebuild-base must build the base and not load: %v", fr.calls)
	}
	if fr.index("{{json .Config.Labels}} agent-base:") < fr.index(baseBuildCall(hash)) {
		t.Errorf("--rebuild-base must not look for an existing base first: %v", fr.calls)
	}
}

func TestBaseBuildFailureExits1WithoutAgentBuild(t *testing.T) {
	dir := contextDir(t)
	hash := baseHashOf(t, dir)
	fr := &fakeRunner{baseMissing: true,
		rules: []rule{{contains: "-f Dockerfile.base", stderr: "503 Service Unavailable", code: 1}}}
	r := runHere(t, fr, "latest", "")
	if r.code != 1 || !strings.Contains(r.err, "exit_state: NOT_CREATED") || !strings.Contains(r.err, "503 Service Unavailable") ||
		!strings.Contains(r.err, "snapshot.ubuntu.com") || !strings.Contains(r.err, config.BaseArchiveName(hash)) {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if !strings.Contains(r.err, "step 5 (ensure base image in agent-host) failed") {
		t.Errorf("step 5 must be named: %q", r.err)
	}
	if fr.has("-f Dockerfile.agent") {
		t.Errorf("the agent build must not run: %v", fr.calls)
	}
	noSuccess(t, r)
}

func TestBaseLabelMismatchExits1(t *testing.T) {
	contextDir(t)
	fr := &fakeRunner{baseLabelWrong: true}
	r := runHere(t, fr, "latest", "")
	if r.code != 1 || !strings.Contains(r.err, "does not carry label "+config.BaseHashLabel) {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if !fr.has("-f Dockerfile.base") || fr.has("-f Dockerfile.agent") {
		t.Errorf("a wrong label means rebuild, then fail before the agent build: %v", fr.calls)
	}
}

func TestMissingBaseDockerfileExits1WithoutDockerCall(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(fmt.Sprintf("empty=%v", empty), func(t *testing.T) {
			dir := contextDir(t)
			p := filepath.Join(dir, config.BaseDockerfileName)
			if empty {
				if err := os.WriteFile(p, nil, 0o600); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Remove(p); err != nil {
				t.Fatal(err)
			}
			fr := &fakeRunner{}
			r := runHere(t, fr, "latest", "")
			if r.code != 1 || !strings.Contains(r.err, config.BaseDockerfileName) || !strings.Contains(r.err, dir) {
				t.Fatalf("exit %d, stderr %q", r.code, r.err)
			}
			if len(fr.calls) != 0 {
				t.Errorf("nothing may run: %v", fr.calls)
			}
			noSuccess(t, r)
		})
	}
}

func TestBaseHashIgnoresLineEndings(t *testing.T) {
	lf := services.BaseHash([]byte("FROM x\nRUN a\n"))
	crlf := services.BaseHash([]byte("FROM x\r\nRUN a\r\n"))
	if lf != crlf || len(lf) != 16 || strings.Trim(lf, "0123456789abcdef") != "" {
		t.Errorf("hash must be 16 hex and equal for LF and CRLF: %q %q", lf, crlf)
	}
	if lf == services.BaseHash([]byte("FROM x\nRUN b\n")) {
		t.Error("different content must give a different hash")
	}
	if config.BaseArchiveName(lf) != "agent-base-"+lf+".tar" || config.BaseImageRef(lf) != "agent-base:"+lf {
		t.Error("archive name or image reference pattern changed")
	}
}

func TestParseOptionsRebuildBase(t *testing.T) {
	if tag, rb, err := steps.ParseOptions(nil); err != nil || tag != "latest" || rb {
		t.Errorf("default: %q %v %v", tag, rb, err)
	}
	if tag, rb, err := steps.ParseOptions([]string{"--rebuild-base", "--tag", "v1"}); err != nil || tag != "v1" || !rb {
		t.Errorf("--rebuild-base: %q %v %v", tag, rb, err)
	}
	if _, _, err := steps.ParseOptions([]string{"--rebuild-base=maybe"}); err == nil {
		t.Error("a non-boolean value must be rejected")
	}
}

func TestBaseDockerfileHoldsOnlyAptLayer(t *testing.T) {
	for _, in := range instructionsOf(t, config.BaseDockerfileName) {
		f := strings.Fields(in)
		switch strings.ToUpper(f[0]) {
		case "FROM", "RUN":
		case "ARG", "ENV":
			name, _, _ := strings.Cut(f[1], "=")
			if steps.CredentialName(name) {
				t.Errorf("credential-like %s name: %s", f[0], name)
			}
		default:
			t.Errorf("Dockerfile.base may hold only FROM, ARG and RUN: %s", in)
		}
	}
}

func TestBuildContextTarOnStdin(t *testing.T) {
	dir := contextDir(t)
	fr := &fakeRunner{}
	if r := runHere(t, fr, "latest", ""); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	if !strings.Contains(fr.calls[fr.index("docker build")], "-f Dockerfile.agent") {
		t.Errorf("Dockerfile.agent must be named from the tar root: %v", fr.calls)
	}
	tr := tar.NewReader(bytes.NewReader(fr.buildStdin))
	var got []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("stdin is not a tar: %v", err)
		}
		got = append(got, hdr.Name)
		body, _ := io.ReadAll(tr)
		want, _ := os.ReadFile(filepath.Join(dir, filepath.FromSlash(hdr.Name)))
		if !bytes.Equal(body, want) {
			t.Errorf("%s: content %q, want %q", hdr.Name, body, want)
		}
		wantMode := int64(0644)
		for _, b := range config.BinFiles {
			if hdr.Name == b {
				wantMode = 0755
			}
		}
		if hdr.Mode != wantMode || hdr.Uid != 0 || hdr.Gid != 0 || !hdr.ModTime.Equal(time.Unix(0, 0)) {
			t.Errorf("%s: header not deterministic: %+v", hdr.Name, hdr)
		}
	}
	want := wantContextFiles()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("tar entries %v, want %v", got, want)
	}
}
