package tests

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"

	"agent-host/internal/config"
	"agent-host/internal/sequence"
	"agent-host/internal/steps"
)

// opPrefix is what Host.ExecAs puts in front of a command that runs as agent-operator.
const opPrefix = "/usr/sbin/runuser -u agent-operator -- "

// ---- fake host runner ----

type call struct {
	Args  []string
	Stdin []byte
}

func (c call) joined() string { return strings.Join(c.Args, " ") }

type rule struct {
	contains string
	stderr   string
	code     int
}

type outRule struct {
	contains string
	stdout   string
}

type fakeRunner struct {
	mu        sync.Mutex
	calls     []call
	rules     []rule
	outputs   []outRule           // first match (substring of the joined args) overrides the stdout of the default responders
	installed map[string]string   // installed path -> hex SHA-256 of the stdin bytes
	sha256Out map[string]string   // optional override: path -> sha256sum stdout hash
	onCall    func(joined string) // optional hook run before every command (e.g. a host mount added after preflight)
}

func (f *fakeRunner) Run(_ context.Context, stdin []byte, args ...string) ([]byte, []byte, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call{Args: append([]string(nil), args...), Stdin: stdin})
	joined := strings.Join(args, " ")
	if f.onCall != nil {
		f.onCall(joined)
	}
	for _, r := range f.rules {
		// A leading "^" anchors the rule at the start of the command.
		if p, anchored := strings.CutPrefix(r.contains, "^"); (anchored && strings.HasPrefix(joined, p)) || (!anchored && strings.Contains(joined, r.contains)) {
			return nil, []byte(r.stderr), r.code, nil
		}
	}
	for _, o := range f.outputs {
		if strings.Contains(joined, o.contains) {
			return []byte(o.stdout), nil, 0, nil
		}
	}
	// Model the steps.WriteFileCmd stdin write (`/bin/sh -c <script> sh <dst> <mode>`) and `sha256sum <dst>`.
	if n := len(args); n >= 6 && args[n-6] == "/bin/sh" && args[n-4] == steps.WriteFileCmd("", "")[2] && args[n-3] == "sh" {
		sum := sha256.Sum256(stdin)
		if f.installed == nil {
			f.installed = map[string]string{}
		}
		f.installed[args[n-2]] = hex.EncodeToString(sum[:])
		return nil, nil, 0, nil
	}
	if n := len(args); n >= 2 && args[n-2] == "/usr/bin/sha256sum" {
		h := f.installed[args[n-1]]
		if o, ok := f.sha256Out[args[n-1]]; ok {
			h = o
		}
		return []byte(h + "  " + args[n-1] + "\n"), nil, 0, nil
	}
	switch {
	case strings.HasPrefix(joined, "docker info --format"):
		return []byte(`{"runc":{"path":"runc"},"runsc":{"path":"/usr/local/bin/runsc","runtimeArgs":` + argsJSON() + `}}` + "\n"), nil, 0, nil
	case joined == "stat -c %U:%G %a "+config.EndpointsInstallPath:
		return []byte("root:root 644\n"), nil, 0, nil
	}
	if rest, ok := strings.CutPrefix(joined, opPrefix); ok {
		return operatorResponse(rest)
	}
	return nil, nil, 0, nil
}

// operatorResponse answers the verify commands run as agent-operator; unknown ones fail.
func operatorResponse(rest string) ([]byte, []byte, int, error) {
	ok := func(s string) ([]byte, []byte, int, error) { return []byte(s), nil, 0, nil }
	switch {
	case rest == "id -u", rest == "id -g":
		return ok("1001\n")
	case rest == "id -nG":
		return ok("agent-operator\n")
	case rest == "/usr/bin/systemctl is-system-running":
		return ok("running\n")
	case rest == "cat "+config.DaemonJSONPath:
		return ok(string(config.DaemonJSON()))
	case strings.HasPrefix(rest, "stat -c %U:%G:%a /usr/local/bin/"):
		return ok("root:root:755\n")
	case rest == "stat -c %U:%G "+config.DaemonJSONPath:
		return ok("root:root\n")
	case rest == "stat -c %U /usr/local/bin":
		return ok("root\n")
	case strings.HasPrefix(rest, "stat -c %U:%G:%a /etc/systemd/system/"):
		return ok("root:root:644\n")
	case rest == "cat "+steps.AgentRestoreUnitPath:
		return ok(steps.AgentRestoreUnit)
	case rest == "cat "+steps.AgentGuardUnitPath:
		return ok(steps.AgentGuardUnit)
	case strings.HasPrefix(rest, "/usr/bin/systemctl is-enabled "):
		return ok("enabled\n")
	case rest == "/usr/bin/systemctl is-active agent-guard.service":
		return ok("active\n")
	case rest == guardShowCmd:
		return ok(guardShowOut("0", "always"))
	case rest == "cat /proc/"+guardPID+"/cmdline":
		return ok("/usr/local/bin/agent_lifecycle\x00guard\x00")
	case rest == "test -f /run/agent-sandbox/ready":
		return ok("")
	case strings.HasPrefix(rest, "/usr/bin/systemctl show agent-"):
		return ok("ActiveState=active\nSubState=exited\n")
	case rest == "hostname":
		return ok(config.HostName + "\n")
	case rest == "cat "+config.PreserveHostnameCfgPath:
		return ok(steps.PreserveHostnameCfg)
	case rest == "cat "+config.NoDockerUpgradeCfgPath:
		return ok(steps.NoDockerUpgradeCfg)
	case rest == "stat -c %U:%G:%a "+config.NoDockerUpgradeCfgPath:
		return ok("root:root:644\n")
	case rest == "cat "+config.BrNetfilterModulesPath:
		return ok(config.BrNetfilterModulesContent)
	case rest == "cat "+config.BridgeSysctlPath:
		return ok(config.BridgeSysctlContent)
	case rest == "stat -c %U:%G:%a "+config.BrNetfilterModulesPath, rest == "stat -c %U:%G:%a "+config.BridgeSysctlPath:
		return ok("root:root:644\n")
	case rest == "test -d "+config.BrNetfilterSysModuleDir:
		return ok("")
	case rest == "cat "+config.BridgeNfCallIptablesProc:
		return ok("1\n")
	}
	if script, found := strings.CutPrefix(rest, "/bin/sh -c "); found {
		for _, known := range verifyScripts {
			if strings.HasPrefix(script, known) {
				return nil, nil, 0, nil
			}
		}
	}
	return nil, []byte("fake: no responder for verify command"), 127, nil
}

// verifyScripts are the `sh -c` scripts the verify step may run as agent-operator (all succeed by default).
var verifyScripts = []string{
	"command -v docker", "command -v python3", "command -v gnome-keyring-daemon", "command -v git", "command -v tar",
	"test ! -w " + config.DaemonJSONPath, "test ! -w /usr/local/bin", "! sudo -n true",
	steps.HomeWriteScript,
}

const (
	guardShowCmd = "/usr/bin/systemctl show -p NRestarts -p Restart -p MainPID -p ExecStart agent-guard.service"
	guardPID     = "4242"
)

// guardShowOut renders `systemctl show` output for agent-guard.service.
func guardShowOut(nRestarts, restart string) string {
	return "NRestarts=" + nRestarts + "\nRestart=" + restart + "\nMainPID=" + guardPID +
		"\nExecStart={ path=/usr/local/bin/agent_lifecycle ; argv[]=/usr/local/bin/agent_lifecycle guard ; ignore_errors=no ; start_time=[n/a] ; stop_time=[n/a] ; pid=0 ; code=(null) ; status=0/0 }\n"
}

func argsJSON() string {
	b, _ := json.Marshal(config.RunscRuntimeArgs)
	return string(b)
}

// writeCmd is the joined argv of a steps.WriteFileCmd stdin write as root.
func writeCmd(dst, mode string) string {
	return strings.Join(steps.WriteFileCmd(dst, mode), " ")
}

func (f *fakeRunner) has(prefix string) bool {
	for _, c := range f.calls {
		if strings.HasPrefix(c.joined(), prefix) {
			return true
		}
	}
	return false
}

func (f *fakeRunner) index(joined string) int {
	for i, c := range f.calls {
		if c.joined() == joined {
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

const testFingerprint = "0123456789ABCDEF0123456789ABCDEF01234567"

const validEndpoints = "{\"endpoints\":[\"api.anthropic.com\",\"platform.claude.com\"]}\n"

// makeTools writes fake tool builds (distinct content each) except those named in skip.
func makeTools(t *testing.T, skip ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range config.ToolNames {
		skipped := false
		for _, s := range skip {
			skipped = skipped || s == name
		}
		if skipped {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte("ELF fake build of "+name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, config.EndpointsFileName), []byte(validEndpoints), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func sha(b string) string {
	s := sha256.Sum256([]byte(b))
	return hex.EncodeToString(s[:])
}

const (
	goodMarker  = "# written by foundation.sh\nFOUNDATION_VERSION=1\nUBUNTU=26.04\n"
	cleanPasswd = "root:x:0:0:root:/root:/bin/bash\nubuntu:x:1000:1000::/home/ubuntu:/bin/bash\n"
	cleanGroup  = "root:x:0:\nubuntu:x:1000:\n"
	cleanMounts = "proc /proc proc rw 0 0\n/dev/vda1 / ext4 rw 0 0\n"
)

// foundationSystem is a System that passes the preflight: Linux, root, the given marker, no host mounts,
// no agent-operator user or group, no done marker (the files live in a temp dir).
func foundationSystem(t *testing.T, markerContent string) steps.System {
	t.Helper()
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	return steps.System{GOOS: "linux", Euid: 0, MarkerPath: write("agent-foundation", markerContent),
		MountsPath: write("mounts", cleanMounts), PasswdPath: write("passwd", cleanPasswd), GroupPath: write("group", cleanGroup),
		DoneMarkerPath: filepath.Join(dir, "agent-host")}
}

func run(t *testing.T, fr *fakeRunner, env map[string]string) result {
	t.Helper()
	sys := foundationSystem(t, goodMarker)
	return runOn(t, fr, sys, env)
}

func runOn(t *testing.T, fr *fakeRunner, sys steps.System, env map[string]string) result {
	t.Helper()
	values := map[string]string{
		"AGENT_HOST_TOOLS_DIR":              makeTools(t),
		"AGENT_HOST_GVISOR_KEY_FINGERPRINT": testFingerprint,
	}
	for k, v := range env {
		values[k] = v
	}
	var out, errb bytes.Buffer
	code := sequence.Run(context.Background(), sequence.Deps{
		Config: config.Load(func(k string) string { return values[k] }),
		Runner: fr,
		Stdout: &out,
		Stderr: &errb,
		System: &sys,
	})
	return result{code, out.String(), errb.String()}
}

// ---- tests ----

func TestHappyPath(t *testing.T) {
	fr := &fakeRunner{}
	r := run(t, fr, nil)
	if r.code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", r.code, r.err)
	}
	if !strings.Contains(r.stdout, "agent-host created") {
		t.Errorf("missing success report: %q", r.stdout)
	}
}

func TestPipelineOrder(t *testing.T) {
	r := run(t, &fakeRunner{}, nil)
	if r.code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", r.code, r.err)
	}
	re := regexp.MustCompile(`(?m)^\[(\d+)/9\] (.+)$`)
	var nums []string
	var names []string
	for _, m := range re.FindAllStringSubmatch(r.stdout, -1) {
		nums = append(nums, m[1])
		names = append(names, m[2])
	}
	if want := []string{"1", "2", "3", "4", "5", "6", "7", "8", "9"}; !reflect.DeepEqual(nums, want) {
		t.Fatalf("step numbers = %v, want %v", nums, want)
	}
	wantNames := []string{"check foundation", "read tool builds", "create default user", "set hostname", "install dependencies",
		"register runsc runtime", "install agent tools", "verify acceptance criteria", "report success"}
	for i, w := range wantNames {
		if !strings.HasPrefix(names[i], w) {
			t.Errorf("step %s is %q, want prefix %q", nums[i], names[i], w)
		}
	}
}

func TestCommandOrderAcrossSteps(t *testing.T) {
	fr := &fakeRunner{}
	if r := run(t, fr, nil); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	order := []int{
		fr.index(strings.Join(steps.CreateGroupCmd, " ")),
		fr.index(strings.Join(steps.CreateUserCmd, " ")),
		fr.index("/usr/bin/hostnamectl set-hostname agent-host"),
		fr.index(strings.Join(steps.WaitSystemdCmd, " ")),
		fr.index("/usr/bin/systemctl restart docker"),
		fr.index(writeCmd("/usr/local/bin/agent_lifecycle", "755")),
		fr.index("docker run --rm --runtime=runsc hello-world"),
	}
	for i, at := range order {
		if at < 0 || (i > 0 && at < order[i-1]) {
			t.Fatalf("commands out of order or missing: %v", order)
		}
	}
}

// ---- foundation preflight ----

func TestPreflightFailures(t *testing.T) {
	good := foundationSystem(t, goodMarker)
	cases := []struct {
		name string
		sys  steps.System
		want string
	}{
		{"not root", steps.System{GOOS: "linux", Euid: 1000, MarkerPath: good.MarkerPath}, "must run as root"},
		{"not linux", steps.System{GOOS: "darwin", Euid: 0, MarkerPath: good.MarkerPath}, "Linux foundation VM"},
		{"marker missing", steps.System{GOOS: "linux", Euid: 0, MarkerPath: filepath.Join(t.TempDir(), "absent")}, "missing or unreadable"},
		{"wrong version", foundationSystem(t, "FOUNDATION_VERSION=2\n"), "unknown foundation version"},
		{"no version line", foundationSystem(t, "something else\n"), "unknown foundation version"},
		{"empty marker", foundationSystem(t, ""), "unknown foundation version"},
		{"wrong ubuntu", foundationSystem(t, "FOUNDATION_VERSION=1\nUBUNTU=24.04\n"), "must be Ubuntu 26.04"},
		{"no ubuntu line", foundationSystem(t, "FOUNDATION_VERSION=1\n"), "must be Ubuntu 26.04"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			fr := &fakeRunner{}
			r := runOn(t, fr, c.sys, nil)
			if r.code != 1 || !strings.Contains(r.err, "step 1") || !strings.Contains(r.err, "NOT_CREATED") || !strings.Contains(r.err, c.want) {
				t.Fatalf("exit %d, stderr %q", r.code, r.err)
			}
			if c.name != "not root" && c.name != "not linux" && !strings.Contains(r.err, "vm/check-foundation.sh") {
				t.Errorf("message must name vm/check-foundation.sh: %q", r.err)
			}
			if len(fr.calls) != 0 {
				t.Errorf("nothing may run after a failed preflight: %v", fr.calls)
			}
		})
	}
}

func TestPreflightAcceptsMarkerWithOtherLinesAndCRLF(t *testing.T) {
	sys := foundationSystem(t, "# comment\r\nFOUNDATION_VERSION=1\r\nUBUNTU=26.04\r\nOTHER=x\r\n")
	if r := runOn(t, &fakeRunner{}, sys, nil); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
}

func TestRealSystemDescribesThisMachine(t *testing.T) {
	s := steps.RealSystem()
	if s.GOOS == "" || s.MarkerPath != "/etc/agent-foundation" || s.Euid != os.Geteuid() ||
		s.MountsPath != "/proc/mounts" || s.PasswdPath != "/etc/passwd" || s.GroupPath != "/etc/group" || s.DoneMarkerPath != config.DoneMarkerPath {
		t.Errorf("RealSystem() = %+v", s)
	}
}

func TestPreflightRefusesHostMounts(t *testing.T) {
	for _, fstype := range []string{"virtiofs", "9p", "sshfs", "fuse.sshfs", "fuse.reverse-sshfs"} {
		fstype := fstype
		t.Run(fstype, func(t *testing.T) {
			sys := foundationSystem(t, goodMarker)
			if err := os.WriteFile(sys.MountsPath, []byte(cleanMounts+"share /mnt/host "+fstype+" rw 0 0\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			fr := &fakeRunner{}
			r := runOn(t, fr, sys, nil)
			if r.code != 1 || !strings.Contains(r.err, "step 1") || !strings.Contains(r.err, "NOT_CREATED") || !strings.Contains(r.err, "/mnt/host") {
				t.Fatalf("exit %d, stderr %q", r.code, r.err)
			}
			if len(fr.calls) != 0 {
				t.Errorf("nothing may run: %v", fr.calls)
			}
		})
	}
}

func TestPreflightRefusesRerun(t *testing.T) {
	cases := []struct {
		name, file, content, want string
	}{
		{"user exists", "passwd", cleanPasswd + "agent-operator:x:2000:2000::/home/agent-operator:/bin/bash\n", "user agent-operator exists"},
		{"uid taken", "passwd", cleanPasswd + "lima:x:1001:1001::/home/lima:/bin/bash\n", "UID 1001 taken by lima"},
		{"gid taken", "group", cleanGroup + "lima:x:1001:\n", "GID 1001 taken by lima"},
		{"done marker", "done", "AGENT_HOST_VERSION=1\n", "agent-host already created"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			sys := foundationSystem(t, goodMarker)
			p := map[string]string{"passwd": sys.PasswdPath, "group": sys.GroupPath, "done": sys.DoneMarkerPath}[c.file]
			if err := os.WriteFile(p, []byte(c.content), 0o644); err != nil {
				t.Fatal(err)
			}
			fr := &fakeRunner{}
			r := runOn(t, fr, sys, nil)
			if r.code != 1 || !strings.Contains(r.err, "step 1") || !strings.Contains(r.err, "NOT_CREATED") ||
				!strings.Contains(r.err, c.want) || !strings.Contains(r.err, "recreate the VM") {
				t.Fatalf("exit %d, stderr %q", r.code, r.err)
			}
			if len(fr.calls) != 0 {
				t.Errorf("nothing may run: %v", fr.calls)
			}
		})
	}
}

func TestDoneMarkerWrittenBeforeSuccess(t *testing.T) {
	fr := &fakeRunner{}
	r := run(t, fr, nil)
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	i := fr.index(writeCmd(config.DoneMarkerPath, "644"))
	if i < 0 || string(fr.calls[i].Stdin) != config.DoneMarkerContent {
		t.Errorf("done marker not written with the expected content (call %d)", i)
	}
	if i != len(fr.calls)-1 {
		t.Error("done marker must be the last command")
	}
}

func TestDoneMarkerWriteFailureExits2(t *testing.T) {
	fr := &fakeRunner{rules: []rule{{" sh " + config.DoneMarkerPath + " 644", "disk full", 1}}}
	r := run(t, fr, nil)
	if r.code != 2 || !strings.Contains(r.err, "step 9") || !strings.Contains(r.err, "disk full") || strings.Contains(r.stdout, "agent-host created") {
		t.Fatalf("exit %d, stderr %q, stdout %q", r.code, r.err, r.stdout)
	}
}

func TestNoDockerUpgradeBlacklistWrittenAfterInstall(t *testing.T) {
	fr := &fakeRunner{}
	if r := run(t, fr, nil); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	w := fr.index(writeCmd(config.NoDockerUpgradeCfgPath, "644"))
	if w < 0 || string(fr.calls[w].Stdin) != steps.NoDockerUpgradeCfg {
		t.Fatalf("blacklist not written with the expected content (call %d)", w)
	}
	for _, p := range []string{"docker.io", "docker-buildx", "containerd", "runc", "runsc"} {
		if !strings.Contains(steps.NoDockerUpgradeCfg, "\""+p+"\";") {
			t.Errorf("blacklist lacks %s", p)
		}
	}
	install := -1
	for i, c := range fr.calls {
		if strings.HasPrefix(c.joined(), "/bin/bash -s") {
			install = i
		}
	}
	if install < 0 || w < install {
		t.Errorf("blacklist (call %d) must follow install_deps (call %d)", w, install)
	}
}

func TestBridgeNetfilterPersistedAndLoadedBeforeDockerRestart(t *testing.T) {
	fr := &fakeRunner{}
	if r := run(t, fr, nil); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	m := fr.index(writeCmd(config.BrNetfilterModulesPath, "644"))
	s := fr.index(writeCmd(config.BridgeSysctlPath, "644"))
	if m < 0 || string(fr.calls[m].Stdin) != "br_netfilter\n" {
		t.Fatalf("modules-load file not written with the expected content (call %d)", m)
	}
	if s < 0 || string(fr.calls[s].Stdin) != "net.bridge.bridge-nf-call-iptables = 1\nnet.bridge.bridge-nf-call-ip6tables = 1\n" {
		t.Fatalf("sysctl file not written with the expected content (call %d)", s)
	}
	probe := fr.index("modprobe br_netfilter")
	apply := fr.index("sysctl -p " + config.BridgeSysctlPath)
	restart := fr.index("/usr/bin/systemctl restart docker")
	if probe < 0 || apply < 0 || restart < 0 {
		t.Fatalf("modprobe %d, sysctl -p %d, docker restart %d: all must be issued", probe, apply, restart)
	}
	if !(m < probe && s < probe && probe < apply && apply < restart) {
		t.Errorf("order wrong: writes %d,%d modprobe %d sysctl %d restart %d", m, s, probe, apply, restart)
	}
}

// ---- steps 2, 3, 4 ----

func TestMissingToolBuildsExit1NoChanges(t *testing.T) {
	fr := &fakeRunner{}
	dir := makeTools(t, "create_agent_net", "remove_agent_container", "agent_lifecycle")
	r := run(t, fr, map[string]string{"AGENT_HOST_TOOLS_DIR": dir})
	if r.code != 1 || !strings.Contains(r.err, "NOT_CREATED") || !strings.Contains(r.err, "step 2") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	for _, name := range []string{"create_agent_net", "remove_agent_container", "agent_lifecycle"} {
		if !strings.Contains(r.err, filepath.Join(dir, name)) {
			t.Errorf("stderr does not name missing build %s:\n%s", name, r.err)
		}
	}
	for _, name := range []string{"create_agent_container", "remove_agent_net"} {
		if strings.Contains(r.err, filepath.Join(dir, name)) {
			t.Errorf("present build %s reported missing", name)
		}
	}
	if len(fr.calls) != 0 {
		t.Errorf("nothing may change: %v", fr.calls)
	}
}

func TestUserCreationFailureExits2(t *testing.T) {
	fr := &fakeRunner{rules: []rule{{"useradd", "useradd: cannot lock", 9}}}
	r := run(t, fr, nil)
	if r.code != 2 || !strings.Contains(r.err, "PARTIALLY_CONFIGURED") || !strings.Contains(r.err, "useradd: cannot lock") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.has("/usr/bin/hostnamectl") {
		t.Error("sequence must stop at the failing step")
	}
}

func TestGroupCreationFailureExits2(t *testing.T) {
	fr := &fakeRunner{rules: []rule{{"groupadd", "groupadd: GID '1001' already exists", 4}}}
	r := run(t, fr, nil)
	if r.code != 2 || !strings.Contains(r.err, "PARTIALLY_CONFIGURED") || !strings.Contains(r.err, "GID 1001 must be free") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.has("/usr/sbin/useradd") {
		t.Error("useradd must not run after groupadd failed")
	}
}

func TestUserAddHasNoSudoGroupAndNoImageUserRemoval(t *testing.T) {
	fr := &fakeRunner{}
	if r := run(t, fr, nil); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	want := "/usr/sbin/useradd --create-home --shell /bin/bash --uid 1001 --gid 1001 agent-operator"
	if !fr.has("/usr/sbin/groupadd --gid 1001 agent-operator") {
		t.Error("no groupadd call recorded")
	}
	found := false
	for _, c := range fr.calls {
		j := c.joined()
		if strings.Contains(j, "userdel") || strings.Contains(j, "groupdel") {
			t.Errorf("the foundation's login user must never be removed: %s", j)
		}
		if strings.Contains(j, "useradd") {
			found = true
			if j != want {
				t.Errorf("useradd call = %q, want %q", j, want)
			}
			for _, bad := range []string{"sudo", "admin", "-G", "--groups"} {
				for _, a := range c.Args {
					if a == bad {
						t.Errorf("useradd must not use %q", bad)
					}
				}
			}
		}
	}
	if !found {
		t.Error("no useradd call recorded")
	}
}

func TestHostnameSetIdempotently(t *testing.T) {
	fr := &fakeRunner{}
	if r := run(t, fr, nil); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	if !fr.has("/usr/bin/hostnamectl set-hostname agent-host") {
		t.Error("hostnamectl not called")
	}
	cfg := fr.index(writeCmd(config.PreserveHostnameCfgPath, "644"))
	if cfg < 0 || string(fr.calls[cfg].Stdin) != steps.PreserveHostnameCfg || !strings.Contains(steps.PreserveHostnameCfg, "preserve_hostname: true") {
		t.Error("cloud-init preserve_hostname drop-in not written")
	}
	hosts := fr.index(strings.Join(steps.EnsureHostsEntryCmd, " "))
	if hosts < 0 || !strings.Contains(steps.EnsureHostsEntryCmd[2], "grep -qE") || !strings.Contains(steps.EnsureHostsEntryCmd[2], "||") {
		t.Error("/etc/hosts entry must be added only when absent")
	}
}

func TestHostnameFailureExits2(t *testing.T) {
	fr := &fakeRunner{rules: []rule{{"hostnamectl", "Failed to set hostname", 1}}}
	r := run(t, fr, nil)
	if r.code != 2 || !strings.Contains(r.err, "step 4") || !strings.Contains(r.err, "Failed to set hostname") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.has(strings.Join(steps.WaitSystemdCmd, " ")) {
		t.Error("sequence must stop at the hostname failure")
	}
}

// ---- step 5 ----

func TestWaitForSystemdCannotFailTheStep(t *testing.T) {
	cmd := strings.Join(steps.WaitSystemdCmd, " ")
	if !strings.Contains(cmd, "exit 0") {
		t.Errorf("wait command must end in exit 0 so a degraded system does not fail the step: %q", cmd)
	}
	if !strings.Contains(cmd, "systemctl is-system-running --wait") {
		t.Errorf("wait command must wait with systemctl is-system-running --wait: %q", cmd)
	}
}

func TestInstallScriptDeliveredViaStdin(t *testing.T) {
	fr := &fakeRunner{}
	if r := run(t, fr, nil); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	for _, c := range fr.calls {
		if strings.HasPrefix(c.joined(), "/bin/bash -s") {
			for _, want := range []string{"iptables", "nftables", "iproute2", "--show-keys", "fingerprint mismatch"} {
				if !bytes.Contains(c.Stdin, []byte(want)) {
					t.Errorf("install script lacks %q", want)
				}
			}
			if bytes.Contains(c.Stdin, []byte("runsc install")) {
				t.Error("runtime registration must not be in the install script")
			}
			if c.Args[len(c.Args)-1] != testFingerprint {
				t.Errorf("fingerprint not passed as last arg: %v", c.Args)
			}
			s := string(c.Stdin)
			chk := strings.Index(s, "fingerprint mismatch")
			if chk < 0 || chk > strings.Index(s, "gpg --batch --dearmor") || chk > strings.Index(s, "gvisor.list") {
				t.Error("fingerprint check must precede dearmor and apt source")
			}
			return
		}
	}
	t.Error("install script call not recorded")
}

// streamRunner is a fakeRunner that also implements services.LineStreamer: during the install script it emits
// one progress marker and one raw apt line.
type streamRunner struct{ *fakeRunner }

func (s streamRunner) RunStream(ctx context.Context, stdin []byte, onLine func(stream, line string), args ...string) ([]byte, []byte, int, error) {
	if strings.HasPrefix(strings.Join(args, " "), "/bin/bash -s") {
		onLine("stdout", "progress: apt-get update")
		onLine("stdout", "Hit:1 http://archive.example/ubuntu resolute InRelease")
	}
	return s.Run(ctx, stdin, args...)
}

func TestInstallDepsProgressLines(t *testing.T) {
	const raw = "Hit:1 http://archive.example/ubuntu resolute InRelease"
	for _, level := range []string{"INFO", "DEBUG"} {
		var out, errb bytes.Buffer
		sys := foundationSystem(t, goodMarker)
		values := map[string]string{
			"AGENT_HOST_TOOLS_DIR":              makeTools(t),
			"AGENT_HOST_GVISOR_KEY_FINGERPRINT": testFingerprint,
			"AGENT_LOG_LEVEL":                   level,
		}
		code := sequence.Run(context.Background(), sequence.Deps{
			Config: config.Load(func(k string) string { return values[k] }),
			Runner: streamRunner{&fakeRunner{}},
			Stdout: &out,
			Stderr: &errb,
			System: &sys,
		})
		if code != 0 {
			t.Fatalf("%s: exit %d: %s", level, code, errb.String())
		}
		s := out.String()
		if !regexp.MustCompile(`(?m)^ +- apt-get update \(elapsed \d\d:\d\d since step start\)$`).MatchString(s) {
			t.Errorf("%s: no progress line for apt-get update:\n%s", level, s)
		}
		if strings.Contains(s, "progress: apt-get update") {
			t.Errorf("%s: marker must not be printed raw at INFO level (only as progress line)", level)
		}
		if got := strings.Contains(s, "stdout: "+raw); got != (level == "DEBUG") {
			t.Errorf("%s: raw line shown = %v:\n%s", level, got, s)
		}
	}
}

func TestEmptyFingerprintFailsInstall(t *testing.T) {
	fr := &fakeRunner{}
	// An empty override falls back to the pinned default, so use an invalid value to exercise fail-closed.
	r := run(t, fr, map[string]string{"AGENT_HOST_GVISOR_KEY_FINGERPRINT": "not-a-fingerprint"})
	if r.code != 2 || !strings.Contains(r.err, "fingerprint") || !strings.Contains(r.err, "step 5") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.has("/bin/bash -s") {
		t.Error("install script must not run without a pinned fingerprint")
	}
}

// ---- steps 6 and 7 ----

func TestDaemonJSONRuntimeRegistration(t *testing.T) {
	fr := &fakeRunner{}
	if r := run(t, fr, nil); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	wantArgs := []string{"--platform=systrap", "--network=sandbox", "--net-raw=false", "--allow-flag-override=false",
		"--overlay2=root:memory", "--file-access=exclusive", "--file-access-mounts=shared", "--directfs=false"}
	cp := fr.index("/usr/bin/install -o root -g root -m 755 /usr/bin/runsc /usr/local/bin/runsc")
	install := fr.index("/usr/local/bin/runsc install --runtime=runsc -- " + strings.Join(wantArgs, " "))
	own := fr.index("/bin/sh -c chown root:root /etc/docker/daemon.json && chmod 644 /etc/docker/daemon.json")
	restart := fr.index("/usr/bin/systemctl restart docker")
	if cp < 0 || install < cp || own < install || restart < own {
		t.Fatalf("expected runsc copy, runsc install, daemon.json ownership, then restart; got %d %d %d %d", cp, install, own, restart)
	}
	if fr.has(writeCmd("/etc/docker/daemon.json", "644")) {
		t.Error("daemon.json must be registered via runsc install, not written directly")
	}
	if !reflect.DeepEqual(config.RunscRuntimeArgs, wantArgs) {
		t.Errorf("runtimeArgs = %v", config.RunscRuntimeArgs)
	}
}

func TestRuntimeRestartFailureExits2(t *testing.T) {
	fr := &fakeRunner{rules: []rule{{"systemctl restart docker", "Job failed", 1}}}
	r := run(t, fr, nil)
	if r.code != 2 || !strings.Contains(r.err, "step 6") || !strings.Contains(r.err, "restarting docker") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	for _, name := range config.ToolNames {
		if fr.has(writeCmd("/usr/local/bin/"+name, "755")) {
			t.Error("tools must not be installed after runtime registration failed")
		}
	}
}

func TestToolsInstalledWithHashCheck(t *testing.T) {
	fr := &fakeRunner{}
	if r := run(t, fr, nil); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	for _, name := range config.ToolNames {
		if !fr.has(writeCmd("/usr/local/bin/"+name, "755")) {
			t.Errorf("no write of %s", name)
		}
		if !fr.has("/usr/bin/sha256sum /usr/local/bin/" + name) {
			t.Errorf("no sha256sum for %s", name)
		}
		if got := fr.installed["/usr/local/bin/"+name]; got != sha("ELF fake build of "+name) {
			t.Errorf("installed bytes of %s hash %s", name, got)
		}
	}
	if !fr.has("/bin/sh -c chown root:root /usr/local/bin && chmod 755 /usr/local/bin") {
		t.Error("/usr/local/bin ownership not enforced")
	}
}

func TestToolInstallFailureExits2(t *testing.T) {
	fr := &fakeRunner{rules: []rule{{" sh /usr/local/bin/remove_agent_net 755", "install: cannot create", 1}}}
	r := run(t, fr, nil)
	if r.code != 2 || !strings.Contains(r.err, "remove_agent_net") || !strings.Contains(r.err, "install: cannot create") || !strings.Contains(r.err, "PARTIALLY_CONFIGURED") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
}

func TestToolHashMismatchExits2WithBothHashes(t *testing.T) {
	bad := strings.Repeat("ab", 32)
	fr := &fakeRunner{sha256Out: map[string]string{"/usr/local/bin/create_agent_net": bad}}
	r := run(t, fr, nil)
	if r.code != 2 {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	for _, want := range []string{"create_agent_net", bad, sha("ELF fake build of create_agent_net")} {
		if !strings.Contains(r.err, want) {
			t.Errorf("stderr lacks %q:\n%s", want, r.err)
		}
	}
	if fr.has("/usr/bin/sha256sum /usr/local/bin/remove_agent_net") {
		t.Error("must stop at the first mismatch")
	}
}

func ptr(s string) *string { return &s }

func TestInvalidEndpointsFileExit1NoChanges(t *testing.T) {
	cases := map[string]*string{
		"missing":          nil,
		"not json":         ptr("endpoints: api.anthropic.com"),
		"empty list":       ptr(`{"endpoints":[]}`),
		"empty name":       ptr(`{"endpoints":[""]}`),
		"duplicate (case)": ptr(`{"endpoints":["api.anthropic.com","API.Anthropic.com"]}`),
		"unknown key":      ptr(`{"endpoints":["api.anthropic.com"],"extra":1}`),
		"bad hostname":     ptr(`{"endpoints":["bad host"]}`),
		"trailing garbage": ptr(`{"endpoints":["a.example"]} x`),
		"two objects":      ptr(`{"endpoints":["a.example"]}{"endpoints":["b.example"]}`),
		"too large":        ptr(`{"endpoints":["` + strings.Repeat("a", config.EndpointsMaxBytes) + `.com"]}`),
	}
	for name, content := range cases {
		content := content
		t.Run(name, func(t *testing.T) {
			fr := &fakeRunner{}
			dir := makeTools(t)
			p := filepath.Join(dir, config.EndpointsFileName)
			if content == nil {
				os.Remove(p)
			} else if err := os.WriteFile(p, []byte(*content), 0o644); err != nil {
				t.Fatal(err)
			}
			r := run(t, fr, map[string]string{"AGENT_HOST_TOOLS_DIR": dir})
			if r.code != 1 || !strings.Contains(r.err, "NOT_CREATED") || !strings.Contains(r.err, "step 2") || !strings.Contains(r.err, "endpoints") {
				t.Fatalf("exit %d, stderr %q", r.code, r.err)
			}
			if len(fr.calls) != 0 {
				t.Errorf("nothing may change: %v", fr.calls)
			}
		})
	}
}

func TestEndpointsSeededViaStdinAfterUnitsStarted(t *testing.T) {
	fr := &fakeRunner{}
	if r := run(t, fr, nil); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	seed, start, stat := -1, -1, -1
	for i, c := range fr.calls {
		switch c.joined() {
		case "/usr/local/bin/refresh_endpoints":
			seed = i
			if string(c.Stdin) != validEndpoints {
				t.Errorf("stdin = %q, want the exact file bytes %q", c.Stdin, validEndpoints)
			}
		case "/usr/bin/systemctl start agent-guard.service agent-restore.service":
			start = i
		case "stat -c %U:%G %a /etc/agent-net/endpoints.json":
			stat = i
		}
	}
	if start < 0 || seed < start || stat < seed {
		t.Fatalf("expected units start, seed, stat; got %d %d %d", start, seed, stat)
	}
	for _, c := range fr.calls {
		for _, a := range c.Args {
			if strings.Contains(a, "endpoints.json") && a != "/etc/agent-net/endpoints.json" {
				t.Errorf("endpoints file passed by path: %v", c.Args)
			}
		}
	}
}

func TestEndpointsSeedFailureExits2(t *testing.T) {
	fr := &fakeRunner{rules: []rule{{"^/usr/local/bin/refresh_endpoints", "lookup api.anthropic.com: no such host", 2}}}
	r := run(t, fr, nil)
	if r.code != 2 || !strings.Contains(r.err, "step 7") || !strings.Contains(r.err, "PARTIALLY_CONFIGURED") || !strings.Contains(r.err, "lookup api.anthropic.com: no such host") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.has(opPrefix + "/usr/bin/systemctl is-system-running") {
		t.Error("verify must not run after a seed failure")
	}
}

func TestEndpointsFileWrongModeExits2(t *testing.T) {
	fr := &fakeRunner{rules: []rule{{"stat -c %U:%G %a /etc/agent-net/endpoints.json", "", 1}}}
	if r := run(t, fr, nil); r.code != 2 || !strings.Contains(r.err, "step 7") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	fr = &fakeRunner{outputs: []outRule{{"stat -c %U:%G %a /etc/agent-net/endpoints.json", "root:root 664\n"}}}
	if r := run(t, fr, nil); r.code != 2 || !strings.Contains(r.err, "root:root 644") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
}

func TestLifecycleUnitsWrittenEnabledAndStarted(t *testing.T) {
	fr := &fakeRunner{}
	if r := run(t, fr, nil); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	if !strings.Contains(steps.AgentRestoreUnit, "ExecStart=/usr/local/bin/agent_lifecycle restore") ||
		!strings.Contains(steps.AgentGuardUnit, "ExecStart=/usr/local/bin/agent_lifecycle guard") {
		t.Error("unit ExecStart lines wrong")
	}
	for _, u := range steps.LifecycleUnits {
		i := fr.index(writeCmd(u.Path, "644"))
		if i < 0 {
			t.Errorf("no write of %s", u.Path)
		} else if string(fr.calls[i].Stdin) != u.Content {
			t.Errorf("%s content = %q", u.Path, fr.calls[i].Stdin)
		}
	}
	tool := fr.index(writeCmd("/usr/local/bin/agent_lifecycle", "755"))
	unit := fr.index(writeCmd(steps.AgentRestoreUnitPath, "644"))
	reload := fr.index("/usr/bin/systemctl daemon-reload")
	enable := fr.index("/usr/bin/systemctl enable docker.service agent-restore.service agent-guard.service")
	start := fr.index("/usr/bin/systemctl start agent-guard.service agent-restore.service")
	if tool < 0 || unit < tool || reload < unit || enable < reload || start < enable {
		t.Fatalf("expected tool install, unit write, daemon-reload, enable, start; got %d %d %d %d %d", tool, unit, reload, enable, start)
	}
}

func TestLifecycleUnitsNotRewrittenWhenIdentical(t *testing.T) {
	fr := &fakeRunner{outputs: []outRule{
		{"/bin/cat " + steps.AgentRestoreUnitPath, steps.AgentRestoreUnit},
		{"/bin/cat " + steps.AgentGuardUnitPath, steps.AgentGuardUnit},
	}}
	if r := run(t, fr, nil); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	for _, u := range steps.LifecycleUnits {
		if fr.has(writeCmd(u.Path, "644")) {
			t.Errorf("%s rewritten although identical", u.Path)
		}
	}
	if !fr.has("/usr/bin/systemctl daemon-reload") {
		t.Error("daemon-reload must still run")
	}
}

func TestLifecycleUnitsRewrittenWhenContentDiffers(t *testing.T) {
	fr := &fakeRunner{outputs: []outRule{{"/bin/cat " + steps.AgentGuardUnitPath, "[Unit]\nstale\n"}}}
	if r := run(t, fr, nil); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	if !fr.has(writeCmd(steps.AgentGuardUnitPath, "644")) {
		t.Error("differing unit must be rewritten")
	}
}

func TestLifecycleUnitFailuresExit2(t *testing.T) {
	cases := []struct {
		name string
		rule rule
		want string
	}{
		{"unit write", rule{" sh " + steps.AgentGuardUnitPath + " 644", "disk full", 1}, "writing " + steps.AgentGuardUnitPath},
		{"daemon-reload", rule{"systemctl daemon-reload", "boom", 1}, "daemon-reload failed"},
		{"enable", rule{"systemctl enable", "boom", 1}, "enabling"},
		{"start", rule{"systemctl start", "boom", 1}, "starting"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			fr := &fakeRunner{rules: []rule{c.rule}}
			r := run(t, fr, nil)
			if r.code != 2 || !strings.Contains(r.err, "step 7") || !strings.Contains(r.err, "PARTIALLY_CONFIGURED") || !strings.Contains(r.err, c.want) {
				t.Fatalf("exit %d, stderr %q", r.code, r.err)
			}
			if fr.has(opPrefix + "/usr/bin/systemctl is-system-running") {
				t.Error("verify must not run after a unit failure")
			}
		})
	}
}
