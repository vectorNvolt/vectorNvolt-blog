package tests

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"unicode/utf16"

	"github.com/ProtonMail/go-crypto/openpgp"

	"agent-host/internal/config"
	"agent-host/internal/sequence"
	"agent-host/internal/steps"
)

// ---- fake wsl.exe ----

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
	imported  bool
	rules     []rule
	outputs   []outRule         // first match (substring of the joined args) overrides the stdout of the default responders
	installed map[string]string // installed path -> hex SHA-256 of the stdin bytes
	sha256Out map[string]string // optional override: path -> sha256sum stdout hash
	verOut    string            // optional override: stdout of `wsl.exe --version`
}

func utf16le(s string) []byte {
	b := []byte{0xFF, 0xFE}
	for _, u := range utf16.Encode([]rune(s)) {
		b = append(b, byte(u), byte(u>>8))
	}
	return b
}

func (f *fakeRunner) Run(_ context.Context, stdin []byte, args ...string) ([]byte, []byte, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call{Args: append([]string(nil), args...), Stdin: stdin})
	joined := strings.Join(args, " ")
	for _, r := range f.rules {
		if strings.Contains(joined, r.contains) {
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
	case len(args) == 1 && args[0] == "--version":
		if f.verOut != "" {
			return utf16le(f.verOut), nil, 0, nil
		}
		return utf16le("WSL version: 2.6.1.0\r\nKernel version: 6.6.87.2-1\r\n"), nil, 0, nil
	case len(args) >= 2 && args[0] == "-l":
		listing := "  NAME      STATE     VERSION\r\n* Ubuntu    Running   2\r\n"
		if f.imported {
			listing += "  agent-host   Running   2\r\n"
		}
		return utf16le(listing), nil, 0, nil
	case args[0] == "--import":
		f.imported = true
		return nil, nil, 0, nil
	case args[0] == "--terminate":
		return nil, nil, 0, nil
	}
	switch {
	case strings.HasSuffix(joined, "-e id -un"):
		return []byte("agent-operator\n"), nil, 0, nil
	case strings.HasSuffix(joined, "-e id -u"), strings.HasSuffix(joined, "-e id -g"):
		return []byte("1001\n"), nil, 0, nil
	case strings.HasSuffix(joined, "-e id -nG"):
		return []byte("agent-operator\n"), nil, 0, nil
	case strings.HasSuffix(joined, "-e printenv PATH"):
		return []byte("/usr/local/bin:/usr/bin\n"), nil, 0, nil
	case strings.HasSuffix(joined, "-e /usr/bin/systemctl is-system-running"):
		return []byte("running\n"), nil, 0, nil
	case strings.Contains(joined, "-u root -e docker info --format"):
		return []byte(`{"runc":{"path":"runc"},"runsc":{"path":"/usr/local/bin/runsc","runtimeArgs":` + argsJSON() + `}}` + "\n"), nil, 0, nil
	case strings.Contains(joined, "-u agent-operator -e cat /etc/docker/daemon.json"):
		return config.DaemonJSON(), nil, 0, nil
	case strings.Contains(joined, "-u agent-operator -e stat -c %U:%G:%a /usr/local/bin/"):
		return []byte("root:root:755\n"), nil, 0, nil
	case strings.Contains(joined, "-u agent-operator -e stat -c %U:%G /etc/"):
		return []byte("root:root\n"), nil, 0, nil
	case strings.HasSuffix(joined, "-u agent-operator -e stat -c %U /usr/local/bin"):
		return []byte("root\n"), nil, 0, nil
	case strings.Contains(joined, "-u agent-operator -e stat -c %U:%G:%a /etc/systemd/system/"):
		return []byte("root:root:644\n"), nil, 0, nil
	case strings.HasSuffix(joined, "-u root -e stat -c %U:%G %a "+config.EndpointsInstallPath):
		return []byte("root:root 644\n"), nil, 0, nil
	case strings.HasSuffix(joined, "-u agent-operator -e cat "+steps.AgentRestoreUnitPath):
		return []byte(steps.AgentRestoreUnit), nil, 0, nil
	case strings.HasSuffix(joined, "-u agent-operator -e cat "+steps.AgentGuardUnitPath):
		return []byte(steps.AgentGuardUnit), nil, 0, nil
	case strings.Contains(joined, "-u agent-operator -e /usr/bin/systemctl is-enabled "):
		return []byte("enabled\n"), nil, 0, nil
	case strings.HasSuffix(joined, "-u agent-operator -e /usr/bin/systemctl is-active agent-guard.service"):
		return []byte("active\n"), nil, 0, nil
	case strings.Contains(joined, "-u agent-operator -e /usr/bin/systemctl show agent-"):
		return []byte("ActiveState=active\nSubState=exited\n"), nil, 0, nil
	case strings.HasSuffix(joined, "-u agent-operator -e stat -c %U:%G:%a "+steps.IsolateScriptPath):
		return []byte("root:root:755\n"), nil, 0, nil
	case strings.HasSuffix(joined, "-u agent-operator -e hostname"):
		return []byte("agent-host\n"), nil, 0, nil
	case strings.HasSuffix(joined, "-u agent-operator -e cat "+config.WSLConfPath):
		return []byte(steps.WSLConf), nil, 0, nil
	case strings.HasSuffix(joined, "-u agent-operator -e cat "+config.ResolvConfPath):
		return []byte(steps.ResolvConf), nil, 0, nil
	case strings.HasSuffix(joined, "-u agent-operator -e cat "+config.HostsPath):
		return []byte(steps.Hosts), nil, 0, nil
	}
	for _, f := range steps.IsolationFiles {
		if strings.HasSuffix(joined, "-u agent-operator -e cat "+f.Path) {
			return []byte(f.Content), nil, 0, nil
		}
	}
	// Verify commands run as agent-operator need an explicit responder: unknown ones fail.
	if i := strings.Index(joined, "-d agent-host -u agent-operator -e /bin/sh -c "); i >= 0 {
		script := joined[i+len("-d agent-host -u agent-operator -e /bin/sh -c "):]
		for _, known := range verifyScripts {
			if strings.HasPrefix(script, known) {
				return nil, nil, 0, nil
			}
		}
	}
	if strings.Contains(joined, "-d agent-host -u agent-operator -e ") {
		return nil, []byte("fake: no responder for verify command"), 127, nil
	}
	return nil, nil, 0, nil
}

// verifyScripts are the `sh -c` scripts the verify step may run as agent-operator (all succeed by default).
var verifyScripts = []string{
	"command -v docker", "command -v python3", "command -v gnome-keyring-daemon", "command -v git", "command -v tar",
	"test ! -w /etc/wsl.conf", "test ! -w /etc/docker/daemon.json", "test ! -w /usr/local/bin",
	"test ! -e /mnt/c", "awk ", "! command -v powershell.exe", "test ! -e /proc/sys/fs/binfmt_misc/WSLInterop",
	"! sudo -n true",
	"test ! -w " + config.ResolvConfPath, "test ! -w " + config.HostsPath,
	"test -f " + config.ResolvConfPath + " && test ! -L " + config.ResolvConfPath,
	"! /usr/bin/mountpoint -q ", `test ! -e /dev/dxg || test "$(stat -c %t:%T /dev/dxg)" = "1:3"`,
	`test -z "$(find /usr/lib/wsl/lib /usr/lib/wsl/drivers -mindepth 1 -print -quit 2>/dev/null)"`,
}

func argsJSON() string {
	b, _ := json.Marshal(config.RunscRuntimeArgs)
	return string(b)
}

// writeCmd is the joined wsl.exe args of a steps.WriteFileCmd stdin write as root.
func writeCmd(dst, mode string) string {
	return "-d agent-host -u root -e " + strings.Join(steps.WriteFileCmd(dst, mode), " ")
}

func (f *fakeRunner) has(prefix string) bool {
	for _, c := range f.calls {
		if strings.HasPrefix(c.joined(), prefix) {
			return true
		}
	}
	return false
}

// ---- fake rootfs server ----

type rootfsServer struct {
	srv       *httptest.Server
	body      []byte
	sumsHash  string
	failFirst int32 // number of 5xx answers for the archive before success
	hits      int32
	badSig    bool // serve a SHA256SUMS.gpg made by a key outside the trusted keyring
}

var (
	keyOnce           sync.Once
	trustedKey, rogue *openpgp.Entity
)

// testKeys returns the throwaway key injected as the trusted keyring and a second, untrusted one.
func testKeys(t *testing.T) (*openpgp.Entity, *openpgp.Entity) {
	t.Helper()
	keyOnce.Do(func() {
		var err error
		if trustedKey, err = openpgp.NewEntity("t", "", "t@example", nil); err != nil {
			panic(err)
		}
		if rogue, err = openpgp.NewEntity("r", "", "r@example", nil); err != nil {
			panic(err)
		}
	})
	return trustedKey, rogue
}

func newRootfsServer(t *testing.T) *rootfsServer {
	t.Helper()
	trusted, untrusted := testKeys(t)
	rs := &rootfsServer{body: []byte("fake rootfs archive")}
	sum := sha256.Sum256(rs.body)
	rs.sumsHash = strings.ToUpper(hex.EncodeToString(sum[:])) // case-insensitive compare
	mux := http.NewServeMux()
	mux.HandleFunc("/rootfs.tar.gz", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "create-agent-host/1.0" {
			http.Error(w, "bad user agent", http.StatusForbidden)
			return
		}
		if atomic.AddInt32(&rs.hits, 1) <= atomic.LoadInt32(&rs.failFirst) {
			http.Error(w, "boom", http.StatusServiceUnavailable)
			return
		}
		w.Write(rs.body)
	})
	mux.HandleFunc("/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s *rootfs.tar.gz\n", rs.sumsHash)
	})
	mux.HandleFunc("/SHA256SUMS.gpg", func(w http.ResponseWriter, r *http.Request) {
		signer := trusted
		if rs.badSig {
			signer = untrusted
		}
		msg := fmt.Sprintf("%s *rootfs.tar.gz\n", rs.sumsHash)
		if err := openpgp.ArmoredDetachSign(w, signer, strings.NewReader(msg), nil); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	rs.srv = httptest.NewTLSServer(mux)
	t.Cleanup(rs.srv.Close)
	return rs
}

// ---- harness ----

type result struct {
	code        int
	stdout, err string
}

const testFingerprint = "0123456789ABCDEF0123456789ABCDEF01234567"

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

const validEndpoints = "{\"endpoints\":[\"api.anthropic.com\",\"platform.claude.com\"]}\n"

func sha(b string) string {
	s := sha256.Sum256([]byte(b))
	return hex.EncodeToString(s[:])
}

// fakeStarter records detached starts instead of launching processes.
type fakeStarter struct {
	cmds [][]string // exe followed by its args
	err  error
}

func (f *fakeStarter) Start(exe string, args ...string) error {
	f.cmds = append(f.cmds, append([]string{exe}, args...))
	return f.err
}

// lastStarter is the fakeStarter of the most recent run().
var lastStarter *fakeStarter

func run(t *testing.T, rs *rootfsServer, fr *fakeRunner, env map[string]string) result {
	t.Helper()
	return runWith(t, rs, fr, env, nil)
}

// runWith is run with a hook that adjusts the loaded Config (e.g. KeepAlive=false for --no-keepalive).
func runWith(t *testing.T, rs *rootfsServer, fr *fakeRunner, env map[string]string, tweak func(*config.Config)) result {
	t.Helper()
	values := map[string]string{
		"APPDATA":                           t.TempDir(),
		"AGENT_HOST_TOOLS_DIR":              makeTools(t),
		"AGENT_HOST_GVISOR_KEY_FINGERPRINT": testFingerprint,
		"AGENT_HOST_ROOTFS_URL":             rs.srv.URL + "/rootfs.tar.gz",
		"AGENT_HOST_ROOTFS_SHA256SUMS_URL":  rs.srv.URL + "/SHA256SUMS",
		"AGENT_HOST_INSTALL_DIR":            t.TempDir(),
		"AGENT_HOST_DOWNLOAD_BACKOFF_MS":    "1",
	}
	for k, v := range env {
		values[k] = v
	}
	cfg := config.Load(func(k string) string { return values[k] })
	if tweak != nil {
		tweak(&cfg)
	}
	lastStarter = &fakeStarter{}
	var out, errb bytes.Buffer
	code := sequence.Run(context.Background(), sequence.Deps{
		Config:     cfg,
		Runner:     fr,
		HTTPClient: rs.srv.Client(),
		Stdout:     &out,
		Stderr:     &errb,
		Keyring:    openpgp.EntityList{trustedKey},
		Starter:    lastStarter,
	})
	return result{code, out.String(), errb.String()}
}

// ---- tests ----

func TestHappyPath(t *testing.T) {
	rs, fr := newRootfsServer(t), &fakeRunner{}
	r := run(t, rs, fr, nil)
	if r.code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", r.code, r.err)
	}
	if !strings.Contains(r.stdout, "agent-host created") {
		t.Errorf("missing success report: %q", r.stdout)
	}
	if !fr.has("--import agent-host ") || !fr.has("--terminate agent-host") {
		t.Errorf("expected import and terminate calls: %v", fr.calls)
	}
}

func TestExistingDistroExits1WithoutChanges(t *testing.T) {
	rs, fr := newRootfsServer(t), &fakeRunner{imported: true}
	r := run(t, rs, fr, nil)
	if r.code != 1 {
		t.Fatalf("exit %d, want 1", r.code)
	}
	if !strings.Contains(r.err, "already registered") || !strings.Contains(r.err, "NOT_CREATED") {
		t.Errorf("stderr: %q", r.err)
	}
	for _, c := range fr.calls {
		if c.joined() != "-l -v" {
			t.Errorf("unexpected call after collision: %s", c.joined())
		}
	}
	if atomic.LoadInt32(&rs.hits) != 0 {
		t.Error("rootfs must not be downloaded after a collision")
	}
}

func TestWSLUnavailableExits1(t *testing.T) {
	rs := newRootfsServer(t)
	fr := &fakeRunner{rules: []rule{{"-l -v", "WSL is not installed", 1}}}
	r := run(t, rs, fr, nil)
	if r.code != 1 || !strings.Contains(r.err, "WSL is not installed") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
}

func TestWSLVersionAccepted(t *testing.T) {
	for _, out := range []string{"WSL version: 2.4.10.0\r\n", "WSL-Version: 2.6.1.0\r\nKernel version: 6.6.87.2-1\r\n"} {
		rs := newRootfsServer(t)
		fr := &fakeRunner{verOut: out}
		if r := run(t, rs, fr, nil); r.code != 0 {
			t.Fatalf("version output %q: exit %d, stderr:\n%s", out, r.code, r.err)
		}
	}
}

func TestWSLVersionTooOldExits1NoImport(t *testing.T) {
	rs := newRootfsServer(t)
	fr := &fakeRunner{verOut: "WSL version: 2.3.26.0\r\n"}
	r := run(t, rs, fr, nil)
	if r.code != 1 || !strings.Contains(r.err, "2.4.10") || !strings.Contains(r.err, "wsl --update") || !strings.Contains(r.err, "NOT_CREATED") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.has("--import") || atomic.LoadInt32(&rs.hits) != 0 {
		t.Error("nothing may be downloaded or imported on an old WSL")
	}
}

func TestWSLVersionUnsupportedExits1NoImport(t *testing.T) {
	rs := newRootfsServer(t)
	fr := &fakeRunner{rules: []rule{{"--version", "Invalid command line option: --version", 1}}}
	r := run(t, rs, fr, nil)
	if r.code != 1 || !strings.Contains(r.err, "2.4.10") || !strings.Contains(r.err, "wsl --update") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.has("--import") {
		t.Error("import must not run")
	}
}

func TestChecksumMismatchExits1NoImport(t *testing.T) {
	rs, fr := newRootfsServer(t), &fakeRunner{}
	rs.sumsHash = strings.Repeat("0", 64)
	r := run(t, rs, fr, nil)
	if r.code != 1 || !strings.Contains(r.err, "mismatch") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.has("--import") {
		t.Error("import must not run after a checksum mismatch")
	}
}

func TestBadSignatureExits1NoImport(t *testing.T) {
	rs, fr := newRootfsServer(t), &fakeRunner{}
	rs.badSig = true
	r := run(t, rs, fr, nil)
	if r.code != 1 || !strings.Contains(r.err, "signature") || !strings.Contains(r.err, "NOT_CREATED") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.has("--import") {
		t.Error("import must not run after a bad SHA256SUMS signature")
	}
}

func TestServerErrorsAreRetried(t *testing.T) {
	rs, fr := newRootfsServer(t), &fakeRunner{}
	rs.failFirst = 2
	r := run(t, rs, fr, nil)
	if r.code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", r.code, r.err)
	}
	if got := atomic.LoadInt32(&rs.hits); got != 3 {
		t.Errorf("archive requests = %d, want 3", got)
	}
}

func TestRetriesExhaustedExits1(t *testing.T) {
	rs, fr := newRootfsServer(t), &fakeRunner{}
	rs.failFirst = 100
	r := run(t, rs, fr, nil)
	if r.code != 1 || fr.has("--import") {
		t.Fatalf("exit %d, calls %v", r.code, fr.calls)
	}
	if got := atomic.LoadInt32(&rs.hits); got != 3 {
		t.Errorf("archive requests = %d, want 3", got)
	}
}

func TestNonHTTPSRefused(t *testing.T) {
	rs, fr := newRootfsServer(t), &fakeRunner{}
	r := run(t, rs, fr, map[string]string{
		"AGENT_HOST_ROOTFS_URL":            "http://example.invalid/rootfs.tar.gz",
		"AGENT_HOST_ROOTFS_SHA256SUMS_URL": "http://example.invalid/SHA256SUMS",
	})
	if r.code != 1 || !strings.Contains(r.err, "https") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.has("--import") {
		t.Error("import must not run")
	}
}

func TestImportFailureExits2WithWSLStderr(t *testing.T) {
	rs := newRootfsServer(t)
	fr := &fakeRunner{rules: []rule{{"--import", "Wsl/Service/E_ACCESSDENIED", 1}}}
	r := run(t, rs, fr, nil)
	if r.code != 2 {
		t.Fatalf("exit %d, want 2", r.code)
	}
	if !strings.Contains(r.err, "Wsl/Service/E_ACCESSDENIED") || !strings.Contains(r.err, "NOT_CREATED") {
		t.Errorf("stderr: %q", r.err)
	}
}

func TestUserCreationFailureExits2(t *testing.T) {
	rs := newRootfsServer(t)
	fr := &fakeRunner{rules: []rule{{"useradd", "useradd: cannot lock", 9}}}
	r := run(t, rs, fr, nil)
	if r.code != 2 || !strings.Contains(r.err, "PARTIALLY_CONFIGURED") || !strings.Contains(r.err, "useradd: cannot lock") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.has("--terminate") {
		t.Error("sequence must stop at the failing step")
	}
}

func TestGroupCreationFailureExits2(t *testing.T) {
	rs := newRootfsServer(t)
	fr := &fakeRunner{rules: []rule{{"groupadd", "groupadd: GID '1001' already exists", 4}}}
	r := run(t, rs, fr, nil)
	if r.code != 2 || !strings.Contains(r.err, "PARTIALLY_CONFIGURED") || !strings.Contains(r.err, "GID 1001 must be free") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.has("-d agent-host -u root -e /usr/sbin/useradd") {
		t.Error("useradd must not run after groupadd failed")
	}
}

func TestImageUserRemovedAsRootBeforeGroupadd(t *testing.T) {
	rs, fr := newRootfsServer(t), &fakeRunner{}
	if r := run(t, rs, fr, nil); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	want := "-d agent-host -u root -e " + strings.Join(steps.RemoveImageUserCmd, " ")
	rm, add := -1, -1
	for i, c := range fr.calls {
		j := c.joined()
		if j == want && rm < 0 {
			rm = i
		}
		if strings.HasPrefix(j, "-d agent-host -u root -e /usr/sbin/groupadd") && add < 0 {
			add = i
		}
	}
	if rm < 0 {
		t.Fatalf("no %q call recorded", want)
	}
	if add < 0 || rm > add {
		t.Errorf("image user removal (call %d) must precede groupadd (call %d)", rm, add)
	}
	for _, s := range []string{
		"if /usr/bin/getent passwd ubuntu >/dev/null; then /usr/sbin/userdel --remove ubuntu; fi",
		"if /usr/bin/getent group ubuntu >/dev/null; then /usr/sbin/groupdel ubuntu; fi",
	} {
		if !strings.Contains(want, s) {
			t.Errorf("removal command lacks %q", s)
		}
	}
}

func TestImageUserRemovalFailureStopsBeforeGroupadd(t *testing.T) {
	rs := newRootfsServer(t)
	fr := &fakeRunner{rules: []rule{{"userdel", "userdel: user ubuntu is currently used by process 1", 8}}}
	r := run(t, rs, fr, nil)
	if r.code != 2 || !strings.Contains(r.err, "PARTIALLY_CONFIGURED") || !strings.Contains(r.err, "removing image default user ubuntu failed") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.has("-d agent-host -u root -e /usr/sbin/groupadd") {
		t.Error("groupadd must not run after image user removal failed")
	}
}

func TestVerifyFailsOnWrongUID(t *testing.T) {
	rs := newRootfsServer(t)
	fr := &fakeRunner{rules: []rule{{"-e id -u", "", 1}}}
	r := run(t, rs, fr, nil)
	if r.code != 2 || !strings.Contains(r.err, "has UID and GID 1001") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
}

func TestVerifyCollectsAllFailures(t *testing.T) {
	rs := newRootfsServer(t)
	fr := &fakeRunner{rules: []rule{
		{"command -v git", "", 1},
		{"command -v tar", "", 1},
	}}
	r := run(t, rs, fr, nil)
	if r.code != 2 {
		t.Fatalf("exit %d, want 2", r.code)
	}
	for _, want := range []string{"tool available: git", "tool available: tar", "UNVERIFIED"} {
		if !strings.Contains(r.err, want) {
			t.Errorf("stderr lacks %q:\n%s", want, r.err)
		}
	}
	if strings.Contains(r.err, "tool available: docker") {
		t.Errorf("docker check should have passed:\n%s", r.err)
	}
	if !fr.has("-d agent-host -u root -e docker run --rm --runtime=runsc hello-world") {
		t.Error("checks after the first failure must still run")
	}
	if strings.Contains(r.stdout, "agent-host created") {
		t.Error("success must not be reported")
	}
}

func TestEveryInvocationTargetsAgentHost(t *testing.T) {
	rs, fr := newRootfsServer(t), &fakeRunner{}
	if r := run(t, rs, fr, nil); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	for _, c := range fr.calls {
		j := c.joined()
		ok := j == "-l -v" || j == "--version" ||
			strings.HasPrefix(j, "--import agent-host ") ||
			strings.HasPrefix(j, "--terminate agent-host") ||
			strings.HasPrefix(j, "-d agent-host ")
		if !ok {
			t.Errorf("invocation not pinned to agent-host: %s", j)
		}
	}
}

func TestWSLConfDeliveredViaStdin(t *testing.T) {
	rs, fr := newRootfsServer(t), &fakeRunner{}
	if r := run(t, rs, fr, nil); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	found := false
	for _, c := range fr.calls {
		if c.joined() == writeCmd("/etc/wsl.conf", "644") {
			found = true
			want := "# /etc/wsl.conf for agent-host.\n[boot]\nsystemd=true\n\n" +
				"# No /mnt/c, /mnt/d: Windows drives are not mounted. fstab cannot re-add drvfs mounts.\n[automount]\nenabled=false\nmountFsTab=false\n\n" +
				"# No Windows executables from Linux, no Windows directories in PATH.\n[interop]\nenabled=false\nappendWindowsPath=false\n\n" +
				"# /etc/hosts and /etc/resolv.conf are not generated from Windows. Provisioning writes both.\n[network]\nhostname=agent-host\ngenerateHosts=false\ngenerateResolvConf=false\n\n" +
				"# Requests no GPU passthrough. /dev/dxg stays (shared kernel device); the isolate service blocks it and unmounts /usr/lib/wsl.\n[gpu]\nenabled=false\n\n[time]\nuseWindowsTimezone=false\n\n[user]\ndefault=agent-operator\n"
			if string(c.Stdin) != want {
				t.Errorf("wsl.conf = %q, want %q", c.Stdin, want)
			}
			if string(c.Stdin) != steps.WSLConf {
				t.Error("stdin differs from steps.WSLConf")
			}
		}
	}
	if !found {
		t.Error("no wsl.conf write recorded")
	}
}

func TestIsolateScriptMakesDevPrivateBeforeBindingDxg(t *testing.T) {
	s := steps.IsolateScript
	priv := strings.Index(s, "mount --make-private /dev\n")
	bind := strings.Index(s, "mount --bind /dev/null /dev/dxg\n")
	if priv < 0 || bind < 0 {
		t.Fatalf("isolate script lacks make-private (%d) or bind (%d)", priv, bind)
	}
	if priv > bind {
		t.Error("mount --make-private /dev must come before mount --bind /dev/null /dev/dxg")
	}
	if !strings.Contains(s, "[ -e /dev/dxg ]") {
		t.Error("isolate script must bind only when /dev/dxg exists")
	}
}

func TestNetworkAndIsolationFilesDeliveredBeforeTerminate(t *testing.T) {
	rs, fr := newRootfsServer(t), &fakeRunner{}
	if r := run(t, rs, fr, nil); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	terminateAt := -1
	at := map[string]int{}
	stdin := map[string]string{}
	linkCmd := "-d agent-host -u root -e /bin/ln -sf ../agent-host-isolate.service /etc/systemd/system/multi-user.target.wants/agent-host-isolate.service"
	want := []struct{ key, cmd, content string }{
		{"hosts", writeCmd("/etc/hosts", "644"), "127.0.0.1 localhost\n127.0.1.1 agent-host\n::1 localhost ip6-localhost ip6-loopback\n"},
		{"script", writeCmd("/usr/local/libexec/agent-host-isolate", "755"), steps.IsolateScript},
		{"unit", writeCmd("/etc/systemd/system/agent-host-isolate.service", "644"), steps.IsolateUnit},
		{"drop-in", writeCmd("/etc/systemd/system/docker.service.d/agent-host-isolate.conf", "644"), steps.IsolateDropIn},
		{"link", linkCmd, ""},
	}
	for i, c := range fr.calls {
		j := c.joined()
		if strings.HasPrefix(j, "--terminate agent-host") {
			terminateAt = i
		}
		for _, w := range want {
			if j == w.cmd {
				at[w.key] = i
				stdin[w.key] = string(c.Stdin)
			}
		}
	}
	if terminateAt < 0 {
		t.Fatal("no terminate call")
	}
	for _, w := range want {
		i, ok := at[w.key]
		if !ok {
			t.Errorf("%s not written", w.key)
			continue
		}
		if i > terminateAt {
			t.Errorf("%s written after terminate", w.key)
		}
		if stdin[w.key] != w.content {
			t.Errorf("%s = %q, want %q", w.key, stdin[w.key], w.content)
		}
	}
	if at["link"] < at["unit"] {
		t.Error("enable symlink created before the unit file")
	}
	if steps.Hosts != want[0].content {
		t.Error("steps.Hosts differs from the expected content")
	}
	if steps.ResolvConf != "nameserver 1.1.1.1\nnameserver 9.9.9.9\n" {
		t.Error("steps.ResolvConf differs from the expected content")
	}
	// resolv.conf is written in step 9, after the restart, not before the terminate.
	for i, c := range fr.calls {
		if c.joined() == writeCmd("/etc/resolv.conf", "644") && i < terminateAt {
			t.Error("resolv.conf written before terminate")
		}
	}
	for _, d := range []string{"/usr/local/libexec", "/etc/systemd/system/docker.service.d", "/etc/systemd/system/multi-user.target.wants"} {
		if !fr.has("-d agent-host -u root -e /bin/mkdir -p " + d) {
			t.Errorf("mkdir -p %s not issued", d)
		}
	}
}

func TestResolvedMaskedBeforeTerminate(t *testing.T) {
	rs, fr := newRootfsServer(t), &fakeRunner{}
	if r := run(t, rs, fr, nil); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	wantMasks := []string{
		"/etc/systemd/system/systemd-resolved.service",
		"/etc/systemd/system/systemd-resolved-monitor.socket",
		"/etc/systemd/system/systemd-resolved-varlink.socket",
		"/etc/tmpfiles.d/systemd-resolve.conf",
	}
	if !reflect.DeepEqual(config.ResolvedMasks, wantMasks) {
		t.Errorf("config.ResolvedMasks = %v", config.ResolvedMasks)
	}
	terminateAt := -1
	at := map[string]int{}
	for i, c := range fr.calls {
		j := c.joined()
		if strings.HasPrefix(j, "--terminate agent-host") {
			terminateAt = i
		}
		for _, m := range wantMasks {
			if j == "-d agent-host -u root -e /bin/ln -sfn /dev/null "+m {
				at[m] = i
			}
		}
	}
	for _, m := range wantMasks {
		i, ok := at[m]
		if !ok {
			t.Errorf("%s not masked", m)
		} else if terminateAt < 0 || i > terminateAt {
			t.Errorf("%s masked after terminate", m)
		}
	}
	if !fr.has("-d agent-host -u root -e /bin/mkdir -p /etc/tmpfiles.d /etc/systemd/system") {
		t.Error("mask directories not created")
	}
}

func TestResolvedMaskFailureExits2(t *testing.T) {
	rs := newRootfsServer(t)
	fr := &fakeRunner{rules: []rule{{"/bin/ln -sfn /dev/null /etc/tmpfiles.d/systemd-resolve.conf", "ln: read-only file system", 1}}}
	r := run(t, rs, fr, nil)
	if r.code != 2 || !strings.Contains(r.err, "step 7") || !strings.Contains(r.err, "masking /etc/tmpfiles.d/systemd-resolve.conf failed") || !strings.Contains(r.err, "ln: read-only file system") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.has("--terminate") {
		t.Error("sequence must stop at mask failure")
	}
}

func TestUserAddHasNoSudoGroup(t *testing.T) {
	rs, fr := newRootfsServer(t), &fakeRunner{}
	if r := run(t, rs, fr, nil); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	want := "-d agent-host -u root -e /usr/sbin/useradd --create-home --shell /bin/bash --uid 1001 --gid 1001 agent-operator"
	wantGroup := "-d agent-host -u root -e /usr/sbin/groupadd --gid 1001 agent-operator"
	if !fr.has(wantGroup) {
		t.Errorf("no %q call recorded", wantGroup)
	}
	found := false
	for _, c := range fr.calls {
		j := c.joined()
		if strings.Contains(j, "useradd") {
			found = true
			if j != want {
				t.Errorf("useradd call = %q, want %q", j, want)
			}
			for _, bad := range []string{"sudo", "admin", "-G", "--groups", "-g"} {
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

func TestLeftoverDriveDirsRemovedAsRootBeforeInstallScript(t *testing.T) {
	rs, fr := newRootfsServer(t), &fakeRunner{}
	if r := run(t, rs, fr, nil); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	want := "-d agent-host -u root -e " + strings.Join(steps.RemoveLeftoverDriveDirsCmd, " ")
	rm, install := -1, -1
	for i, c := range fr.calls {
		j := c.joined()
		if j == want && rm < 0 {
			rm = i
		}
		if strings.HasPrefix(j, "-d agent-host -u root -e /bin/bash -s") && install < 0 {
			install = i
		}
	}
	if rm < 0 {
		t.Fatalf("no %q call recorded", want)
	}
	if install < 0 || rm > install {
		t.Errorf("leftover drive dir removal (call %d) must precede install script (call %d)", rm, install)
	}
}

func TestResolvConfWrittenInStep9AfterWaitBeforeInstallScript(t *testing.T) {
	rs, fr := newRootfsServer(t), &fakeRunner{}
	if r := run(t, rs, fr, nil); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	wait := "-d agent-host -u root -e " + strings.Join(steps.WaitSystemdCmd, " ")
	terminate, waitAt, resolv, install := -1, -1, -1, -1
	for i, c := range fr.calls {
		j := c.joined()
		switch {
		case strings.HasPrefix(j, "--terminate agent-host"):
			terminate = i
		case j == wait && waitAt < 0:
			waitAt = i
		case j == writeCmd(config.ResolvConfPath, "644") && resolv < 0:
			resolv = i
			if string(c.Stdin) != steps.ResolvConf {
				t.Errorf("resolv.conf = %q, want %q", c.Stdin, steps.ResolvConf)
			}
		case strings.HasPrefix(j, "-d agent-host -u root -e /bin/bash -s") && install < 0:
			install = i
		}
	}
	if waitAt < 0 || resolv < 0 || install < 0 {
		t.Fatalf("calls missing: wait %d, resolv.conf write %d, install script %d", waitAt, resolv, install)
	}
	if !(terminate < waitAt && waitAt < resolv && resolv < install) {
		t.Errorf("want terminate (%d) < wait (%d) < resolv.conf write (%d) < install script (%d)", terminate, waitAt, resolv, install)
	}
}

func TestResolvConfWriteFailureExits2BeforeInstallScript(t *testing.T) {
	rs := newRootfsServer(t)
	fr := &fakeRunner{rules: []rule{{"sh " + config.ResolvConfPath + " 644", "install: disk full", 1}}}
	r := run(t, rs, fr, nil)
	if r.code != 2 || !strings.Contains(r.err, "PARTIALLY_CONFIGURED") || !strings.Contains(r.err, "step 9") ||
		!strings.Contains(r.err, "writing "+config.ResolvConfPath+" failed") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.has("-d agent-host -u root -e /bin/bash -s") {
		t.Error("install script must not run after the resolv.conf write failed")
	}
}

func TestWaitForSystemdCannotFailTheStep(t *testing.T) {
	cmd := strings.Join(steps.WaitSystemdCmd, " ")
	if !strings.Contains(cmd, "exit 0") {
		t.Errorf("wait command must end in exit 0 so a degraded system does not fail the step: %q", cmd)
	}
	if !strings.Contains(cmd, "systemctl is-system-running --wait") {
		t.Errorf("wait command must wait with systemctl is-system-running --wait: %q", cmd)
	}
}

func TestLeftoverDriveDirsFailureStopsBeforeInstallScript(t *testing.T) {
	rs := newRootfsServer(t)
	fr := &fakeRunner{rules: []rule{{"/bin/rmdir", "rmdir: failed to remove '/mnt/c': Directory not empty", 1}}}
	r := run(t, rs, fr, nil)
	if r.code != 2 || !strings.Contains(r.err, "PARTIALLY_CONFIGURED") || !strings.Contains(r.err, "removing leftover drive mount directories failed") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.has("-d agent-host -u root -e /bin/bash -s") {
		t.Error("install script must not run after leftover drive dir removal failed")
	}
}

func TestInstallScriptDeliveredViaStdin(t *testing.T) {
	rs, fr := newRootfsServer(t), &fakeRunner{}
	if r := run(t, rs, fr, nil); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	for _, c := range fr.calls {
		if strings.HasPrefix(c.joined(), "-d agent-host -u root -e /bin/bash -s") {
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
			// the fingerprint check must precede dearmor and the apt source
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

func TestEmptyFingerprintFailsInstall(t *testing.T) {
	rs, fr := newRootfsServer(t), &fakeRunner{}
	// An empty override falls back to the pinned default, so use an invalid value to exercise fail-closed.
	r := run(t, rs, fr, map[string]string{"AGENT_HOST_GVISOR_KEY_FINGERPRINT": "not-a-fingerprint"})
	if r.code != 2 || !strings.Contains(r.err, "fingerprint") || !strings.Contains(r.err, "step 9") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.has("-d agent-host -u root -e /bin/bash -s") {
		t.Error("install script must not run without a pinned fingerprint")
	}
}

func TestMissingToolBuildsExit1NoChanges(t *testing.T) {
	rs, fr := newRootfsServer(t), &fakeRunner{}
	dir := makeTools(t, "create_agent_net", "remove_agent_container", "agent_lifecycle")
	r := run(t, rs, fr, map[string]string{"AGENT_HOST_TOOLS_DIR": dir})
	if r.code != 1 || !strings.Contains(r.err, "NOT_CREATED") || !strings.Contains(r.err, "step 3") {
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
	if fr.has("--import") || fr.has("-d agent-host") {
		t.Errorf("nothing may change: %v", fr.calls)
	}
	if atomic.LoadInt32(&rs.hits) != 0 {
		t.Error("rootfs must not be downloaded")
	}
}

func TestToolsInstalledWithHashCheck(t *testing.T) {
	rs, fr := newRootfsServer(t), &fakeRunner{}
	if r := run(t, rs, fr, nil); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	for _, name := range config.ToolNames {
		want := writeCmd("/usr/local/bin/"+name, "755")
		if !fr.has(want) {
			t.Errorf("no %q call", want)
		}
		if !fr.has("-d agent-host -u root -e /usr/bin/sha256sum /usr/local/bin/" + name) {
			t.Errorf("no sha256sum for %s", name)
		}
		if got := fr.installed["/usr/local/bin/"+name]; got != sha("ELF fake build of "+name) {
			t.Errorf("installed bytes of %s hash %s", name, got)
		}
	}
	if !fr.has("-d agent-host -u root -e /bin/sh -c chown root:root /usr/local/bin && chmod 755 /usr/local/bin") {
		t.Error("/usr/local/bin ownership not enforced")
	}
}

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
			rs, fr := newRootfsServer(t), &fakeRunner{}
			dir := makeTools(t)
			p := filepath.Join(dir, config.EndpointsFileName)
			if content == nil {
				os.Remove(p)
			} else if err := os.WriteFile(p, []byte(*content), 0o644); err != nil {
				t.Fatal(err)
			}
			r := run(t, rs, fr, map[string]string{"AGENT_HOST_TOOLS_DIR": dir})
			if r.code != 1 || !strings.Contains(r.err, "NOT_CREATED") || !strings.Contains(r.err, "step 3") || !strings.Contains(r.err, "endpoints") {
				t.Fatalf("exit %d, stderr %q", r.code, r.err)
			}
			if fr.has("--import") || fr.has("-d agent-host") {
				t.Errorf("nothing may change: %v", fr.calls)
			}
			if atomic.LoadInt32(&rs.hits) != 0 {
				t.Error("rootfs must not be downloaded")
			}
		})
	}
}

func ptr(s string) *string { return &s }

func TestEndpointsSeededViaStdinAfterUnitsStarted(t *testing.T) {
	rs, fr := newRootfsServer(t), &fakeRunner{}
	if r := run(t, rs, fr, nil); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	seed, start, stat := -1, -1, -1
	for i, c := range fr.calls {
		switch c.joined() {
		case rootPrefix + "/usr/local/bin/refresh_endpoints":
			seed = i
			if string(c.Stdin) != validEndpoints {
				t.Errorf("stdin = %q, want the exact file bytes %q", c.Stdin, validEndpoints)
			}
		case rootPrefix + "/usr/bin/systemctl start agent-guard.service agent-restore.service":
			start = i
		case rootPrefix + "stat -c %U:%G %a /etc/agent-net/endpoints.json":
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
	rs := newRootfsServer(t)
	fr := &fakeRunner{rules: []rule{{"-u root -e /usr/local/bin/refresh_endpoints", "lookup api.anthropic.com: no such host", 2}}}
	r := run(t, rs, fr, nil)
	if r.code != 2 || !strings.Contains(r.err, "step 11") || !strings.Contains(r.err, "PARTIALLY_CONFIGURED") || !strings.Contains(r.err, "lookup api.anthropic.com: no such host") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.has(opPrefix + "/usr/bin/systemctl is-system-running") {
		t.Error("verify must not run after a seed failure")
	}
}

func TestEndpointsFileWrongModeExits2(t *testing.T) {
	rs := newRootfsServer(t)
	fr := &fakeRunner{rules: []rule{{"-u root -e stat -c %U:%G %a /etc/agent-net/endpoints.json", "", 1}}}
	if r := run(t, rs, fr, nil); r.code != 2 || !strings.Contains(r.err, "step 11") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	fr = &fakeRunner{outputs: []outRule{{rootPrefix + "stat -c %U:%G %a /etc/agent-net/endpoints.json", "root:root 664\n"}}}
	if r := run(t, newRootfsServer(t), fr, nil); r.code != 2 || !strings.Contains(r.err, "root:root 644") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
}

func TestToolInstallFailureExits2(t *testing.T) {
	rs := newRootfsServer(t)
	fr := &fakeRunner{rules: []rule{{" sh /usr/local/bin/remove_agent_net 755", "install: cannot create", 1}}}
	r := run(t, rs, fr, nil)
	if r.code != 2 || !strings.Contains(r.err, "remove_agent_net") || !strings.Contains(r.err, "install: cannot create") || !strings.Contains(r.err, "PARTIALLY_CONFIGURED") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
}

func TestToolHashMismatchExits2WithBothHashes(t *testing.T) {
	rs := newRootfsServer(t)
	bad := strings.Repeat("ab", 32)
	fr := &fakeRunner{sha256Out: map[string]string{"/usr/local/bin/create_agent_net": bad}}
	r := run(t, rs, fr, nil)
	if r.code != 2 {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	for _, want := range []string{"create_agent_net", bad, sha("ELF fake build of create_agent_net")} {
		if !strings.Contains(r.err, want) {
			t.Errorf("stderr lacks %q:\n%s", want, r.err)
		}
	}
	if fr.has("-d agent-host -u root -e /usr/bin/sha256sum /usr/local/bin/remove_agent_net") {
		t.Error("must stop at the first mismatch")
	}
}

func TestLifecycleUnitsWrittenEnabledAndStarted(t *testing.T) {
	rs, fr := newRootfsServer(t), &fakeRunner{}
	if r := run(t, rs, fr, nil); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	if !strings.Contains(steps.AgentRestoreUnit, "ExecStart=/usr/local/bin/agent_lifecycle restore") ||
		!strings.Contains(steps.AgentGuardUnit, "ExecStart=/usr/local/bin/agent_lifecycle guard") {
		t.Error("unit ExecStart lines wrong")
	}
	idx := func(s string) int {
		for i, c := range fr.calls {
			if c.joined() == s {
				return i
			}
		}
		return -1
	}
	for _, u := range steps.LifecycleUnits {
		found := false
		for _, c := range fr.calls {
			if c.joined() == writeCmd(u.Path, "644") {
				found = true
				if string(c.Stdin) != u.Content {
					t.Errorf("%s content = %q", u.Path, c.Stdin)
				}
			}
		}
		if !found {
			t.Errorf("no write of %s", u.Path)
		}
	}
	tool := idx(writeCmd("/usr/local/bin/agent_lifecycle", "755"))
	unit := idx(writeCmd(steps.AgentRestoreUnitPath, "644"))
	reload := idx(rootPrefix + "/usr/bin/systemctl daemon-reload")
	enable := idx(rootPrefix + "/usr/bin/systemctl enable docker.service agent-restore.service agent-guard.service")
	start := idx(rootPrefix + "/usr/bin/systemctl start agent-guard.service agent-restore.service")
	if tool < 0 || unit < tool || reload < unit || enable < reload || start < enable {
		t.Fatalf("expected tool install, unit write, daemon-reload, enable, start; got %d %d %d %d %d", tool, unit, reload, enable, start)
	}
}

func TestLifecycleUnitsNotRewrittenWhenIdentical(t *testing.T) {
	rs := newRootfsServer(t)
	fr := &fakeRunner{outputs: []outRule{
		{rootPrefix + "/bin/cat " + steps.AgentRestoreUnitPath, steps.AgentRestoreUnit},
		{rootPrefix + "/bin/cat " + steps.AgentGuardUnitPath, steps.AgentGuardUnit},
	}}
	if r := run(t, rs, fr, nil); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	for _, u := range steps.LifecycleUnits {
		if fr.has(writeCmd(u.Path, "644")) {
			t.Errorf("%s rewritten although identical", u.Path)
		}
	}
	if !fr.has(rootPrefix + "/usr/bin/systemctl daemon-reload") {
		t.Error("daemon-reload must still run")
	}
}

func TestLifecycleUnitsRewrittenWhenContentDiffers(t *testing.T) {
	rs := newRootfsServer(t)
	fr := &fakeRunner{outputs: []outRule{{rootPrefix + "/bin/cat " + steps.AgentGuardUnitPath, "[Unit]\nstale\n"}}}
	if r := run(t, rs, fr, nil); r.code != 0 {
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
			rs := newRootfsServer(t)
			fr := &fakeRunner{rules: []rule{c.rule}}
			r := run(t, rs, fr, nil)
			if r.code != 2 || !strings.Contains(r.err, "step 11") || !strings.Contains(r.err, "PARTIALLY_CONFIGURED") || !strings.Contains(r.err, c.want) {
				t.Fatalf("exit %d, stderr %q", r.code, r.err)
			}
			if fr.has(opPrefix + "/usr/bin/systemctl is-system-running") {
				t.Error("verify must not run after a unit failure")
			}
		})
	}
}

func TestDaemonJSONRuntimeRegistration(t *testing.T) {
	rs, fr := newRootfsServer(t), &fakeRunner{}
	if r := run(t, rs, fr, nil); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	wantArgs := []string{"--platform=systrap", "--network=sandbox", "--net-raw=false", "--allow-flag-override=false",
		"--overlay2=root:memory", "--file-access=exclusive", "--file-access-mounts=shared", "--directfs=false"}
	wantInstall := "-d agent-host -u root -e /usr/local/bin/runsc install --runtime=runsc -- " + strings.Join(wantArgs, " ")
	install, own, restart, cp := -1, -1, -1, -1
	for i, c := range fr.calls {
		switch c.joined() {
		case wantInstall:
			install = i
		case "-d agent-host -u root -e /bin/sh -c chown root:root /etc/docker/daemon.json && chmod 644 /etc/docker/daemon.json":
			own = i
		case "-d agent-host -u root -e /usr/bin/systemctl restart docker":
			restart = i
		case "-d agent-host -u root -e /usr/bin/install -o root -g root -m 755 /usr/bin/runsc /usr/local/bin/runsc":
			cp = i
		case writeCmd("/etc/docker/daemon.json", "644"):
			t.Error("daemon.json must be registered via runsc install, not written directly")
		}
	}
	if cp < 0 || install < cp || own < install || restart < own {
		t.Fatalf("expected runsc copy, runsc install, daemon.json ownership, then restart; got %d %d %d %d", cp, install, own, restart)
	}
	if !reflect.DeepEqual(config.RunscRuntimeArgs, wantArgs) {
		t.Errorf("runtimeArgs = %v", config.RunscRuntimeArgs)
	}
}

func TestRuntimeRestartFailureExits2(t *testing.T) {
	rs := newRootfsServer(t)
	fr := &fakeRunner{rules: []rule{{"systemctl restart docker", "Job failed", 1}}}
	r := run(t, rs, fr, nil)
	if r.code != 2 || !strings.Contains(r.err, "step 10") || !strings.Contains(r.err, "restarting docker") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	for _, name := range config.ToolNames {
		if fr.has(writeCmd("/usr/local/bin/"+name, "755")) {
			t.Error("tools must not be installed after runtime registration failed")
		}
	}
}

func TestTerminateFailureExits2(t *testing.T) {
	rs := newRootfsServer(t)
	fr := &fakeRunner{rules: []rule{{"--terminate", "terminate failed", 1}}}
	r := run(t, rs, fr, nil)
	if r.code != 2 || !strings.Contains(r.err, "step 8") || !strings.Contains(r.err, "terminate failed") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.has("-d agent-host -u root -e /bin/bash -s") {
		t.Error("deps must not install after terminate failed")
	}
}

func TestWSLConfWriteFailureExits2(t *testing.T) {
	rs := newRootfsServer(t)
	fr := &fakeRunner{rules: []rule{{"/etc/wsl.conf", "install: disk full", 1}}}
	r := run(t, rs, fr, nil)
	if r.code != 2 || !strings.Contains(r.err, "step 7") || !strings.Contains(r.err, "install: disk full") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.has("--terminate") {
		t.Error("sequence must stop at wsl.conf failure")
	}
}
