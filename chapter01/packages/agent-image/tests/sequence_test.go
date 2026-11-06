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
	"agent-image/internal/steps"
)

// ---- fake wsl.exe runner ----

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

	onCall func(joined string) // called for every call after it is recorded (interrupt tests)
}

const hostPrefix = "-d agent-host -u root -e "

func (f *fakeRunner) Run(_ context.Context, stdin []byte, live io.Writer, args ...string) ([]byte, []byte, int, error) {
	joined := strings.Join(args, " ")
	f.mu.Lock()
	f.calls = append(f.calls, joined)
	f.mu.Unlock()
	if f.onCall != nil {
		f.onCall(joined)
	}
	if strings.Contains(joined, "docker build") {
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

// contextDir makes a temp dir holding every build file and the current directory for the test.
func contextDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range config.BuildContextFiles {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("content of "+name+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	chdir(t, dir)
	return dir
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
	var out, errb bytes.Buffer
	code := sequence.Run(ctx, sequence.Deps{
		Config: config.Load(func(string) string { return "" }),
		Runner: fr,
		Tag:    tag,
		Stdin:  stdin,
		Stdout: &out,
		Stderr: &errb,
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
	r := run(t, fr, "latest", "")
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
		hostPrefix + "docker build --progress=plain -f Dockerfile.agent -t agent-image:latest -",
	}
	last := -1
	for _, want := range order {
		i := fr.index(want)
		if i <= last {
			t.Errorf("%q out of order or missing (index %d after %d): %v", want, i, last, fr.calls)
		}
		last = i
	}
	if len(fr.calls) != 7 || fr.calls[3] != hostPrefix+"docker image inspect agent-image:latest" {
		t.Errorf("expected version, inspect, build, inspect, env, labels, history: %v", fr.calls)
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
		if !strings.HasPrefix(c, hostPrefix) {
			t.Errorf("every command must be wsl root in agent-host: %s", c)
		}
		if strings.Count(c, "-d ") != 1 {
			t.Errorf("exactly one distro per call: %s", c)
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
	if r.code != 0 || !fr.has("docker build --progress=plain -f Dockerfile.agent -t agent-image:v1.2_rc-1 -") {
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
	for _, bad := range [][]string{{"extra"}, {"--tag", "v1", "extra"}, {"--bogus"}, {"--tag"},
		{"--dev-distro", "Ubuntu"}, {"--dev-distro"}} {
		if _, err := steps.ParseArgs(bad); err == nil {
			t.Errorf("args %v must be rejected", bad)
		}
	}
}

func TestEngineUnreachableExits1WithoutBuild(t *testing.T) {
	cases := map[string]rule{
		"agent-host": {contains: hostPrefix + "docker version", stderr: "Cannot connect to the Docker daemon", code: 1},
		"wsl.exe":    {contains: "docker version", code: -1, err: errors.New("executable file not found")},
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
	if fr.count("docker image inspect") != 1 {
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
		"other exit":  {contains: "docker image inspect", stderr: "Cannot connect to the Docker daemon", code: 1},
		"wsl failure": {contains: "docker image inspect", code: -1, err: errors.New("wsl.exe vanished")},
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
	for _, contains := range []string{"{{json .Config.Env}}", "{{json .Config.Labels}}", "docker history"} {
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
			if len(args) != 2 || args[0] != "requirements.lock" {
				t.Errorf("only requirements.lock may be copied: %s", line)
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
	data, err := os.ReadFile(filepath.Join("..", config.DockerfileName))
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

	fromRe := regexp.MustCompile(`^FROM \S+:\S+@sha256:[0-9a-f]{64}$`)
	froms := 0
	for _, in := range ins {
		if strings.HasPrefix(strings.ToUpper(in), "FROM ") {
			froms++
			if !fromRe.MatchString(in) {
				t.Errorf("FROM must name a tag and an @sha256 digest: %s", in)
			}
		}
	}
	if froms == 0 {
		t.Error("no FROM instruction")
	}

	snapRe := regexp.MustCompile(`^ARG UBUNTU_SNAPSHOT=\d{8}T\d{6}Z$`)
	hasSnapArg := false
	for _, in := range ins {
		hasSnapArg = hasSnapArg || snapRe.MatchString(in)
	}
	if !hasSnapArg {
		t.Error("Dockerfile.agent must set ARG UBUNTU_SNAPSHOT=<YYYYMMDDTHHMMSSZ>")
	}

	pkgRe := regexp.MustCompile(`^[a-z0-9][a-z0-9.+-]*=[0-9][^=\s]*$`)
	installs, snapUpdate, snapInstall := 0, false, false
	for _, c := range cmds {
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

func TestNoWindowsFileWritten(t *testing.T) {
	dir := contextDir(t)
	if r := runHere(t, &fakeRunner{}, "latest", ""); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != len(config.BuildContextFiles) {
		t.Errorf("working directory must hold only the build files: %v %v", entries, err)
	}
}

func TestMissingBuildFileExits1WithoutWSLCall(t *testing.T) {
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

// ---- SFR-IMG-018: interrupt handling ----

// wantInterrupted asserts exit 130, one stderr line naming the step, no success text and the exact wsl calls.
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
		t.Fatalf("wsl calls %v, want %d calls matching %v", fr.calls, len(calls), calls)
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
	wantInterrupted(t, r, fr, 5, "docker version", "docker image inspect agent-image:latest", "docker build")
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
	wantInterrupted(t, r, fr, 6, "docker version", "docker image inspect", "docker build", "docker image inspect",
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
	wantInterrupted(t, r, fr, 6, "docker version", "docker image inspect", "docker build", "docker image inspect",
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
		want, _ := os.ReadFile(filepath.Join(dir, hdr.Name))
		if !bytes.Equal(body, want) {
			t.Errorf("%s: content %q, want %q", hdr.Name, body, want)
		}
		if hdr.Mode != 0644 || hdr.Uid != 0 || hdr.Gid != 0 || !hdr.ModTime.Equal(time.Unix(0, 0)) {
			t.Errorf("%s: header not deterministic: %+v", hdr.Name, hdr)
		}
	}
	want := append([]string(nil), config.BuildContextFiles...)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("tar entries %v, want %v", got, want)
	}
}
