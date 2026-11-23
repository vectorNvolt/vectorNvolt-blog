package agentinit

import (
	"io"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"
)

type fakeProc struct {
	spec Spec
	done chan int
	mu   sync.Mutex
	sigs []os.Signal
}

func (p *fakeProc) Wait() (int, error) { return <-p.done, nil }

func (p *fakeProc) Signal(s os.Signal) error {
	p.mu.Lock()
	p.sigs = append(p.sigs, s)
	p.mu.Unlock()
	select {
	case p.done <- 143:
	default:
	}
	return nil
}

func (p *fakeProc) exit(code int) { p.done <- code }

func (p *fakeProc) signals() []os.Signal {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]os.Signal(nil), p.sigs...)
}

type fakeStarter struct{ started chan *fakeProc }

func (s *fakeStarter) Start(spec Spec) (Process, error) {
	p := &fakeProc{spec: spec, done: make(chan int, 1)}
	s.started <- p
	return p, nil
}

func (s *fakeStarter) next(t *testing.T) *fakeProc {
	t.Helper()
	select {
	case p := <-s.started:
		return p
	case <-time.After(5 * time.Second):
		t.Fatal("no process started")
		return nil
	}
}

func newInit(t *testing.T) (*Init, *fakeStarter) {
	t.Helper()
	st := &fakeStarter{started: make(chan *fakeProc, 16)}
	return &Init{
		SocketPath:  filepath.Join(t.TempDir(), "run", "plugins", "ephemeral-authz.sock"),
		PluginPath:  "/plugin",
		PluginUser:  "authz",
		DockerdPath: "/dockerd",
		Starter:     st,
		LookupUser:  func(string) (uint32, uint32, error) { return 990, 991, nil },
		Log:         io.Discard,
		Backoff:     10 * time.Millisecond,
		TermWait:    time.Second,
	}, st
}

// runAsync runs i.Run and returns the channel of its exit code.
func runAsync(i *Init, args []string, sigs <-chan os.Signal) <-chan int {
	out := make(chan int, 1)
	go func() { out <- i.Run(args, sigs) }()
	return out
}

func wantCode(t *testing.T, ch <-chan int, want int) {
	t.Helper()
	select {
	case got := <-ch:
		if got != want {
			t.Fatalf("exit code %d, want %d", got, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return")
	}
}

func TestPluginSpecHasFd3AndAuthzCredential(t *testing.T) {
	i, st := newInit(t)
	code := runAsync(i, nil, make(chan os.Signal))
	plugin := st.next(t)
	dockerd := st.next(t)

	if plugin.spec.Path != "/plugin" || len(plugin.spec.Args) != 0 {
		t.Errorf("plugin spec %+v", plugin.spec)
	}
	if len(plugin.spec.ExtraFiles) != 1 || plugin.spec.ExtraFiles[0] == nil {
		t.Fatalf("plugin must get the socket as the only extra file (fd 3): %+v", plugin.spec.ExtraFiles)
	}
	c := plugin.spec.Cred
	if c == nil || c.Uid != 990 || c.Gid != 991 {
		t.Errorf("plugin credential %+v, want uid 990 gid 991", c)
	}
	if len(plugin.spec.Env) != 1 || plugin.spec.Env[0] != "PATH="+pluginPATH {
		t.Errorf("plugin env %v, want PATH only", plugin.spec.Env)
	}
	if dockerd.spec.Path != "/dockerd" || len(dockerd.spec.Args) != 0 || dockerd.spec.Cred != nil || len(dockerd.spec.ExtraFiles) != 0 {
		t.Errorf("dockerd spec %+v", dockerd.spec)
	}

	fi, err := os.Stat(i.SocketPath)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSocket == 0 || fi.Mode().Perm() != 0o600 {
		t.Errorf("socket mode %v, want a socket with 0600", fi.Mode())
	}
	if di, err := os.Stat(filepath.Dir(i.SocketPath)); err != nil || di.Mode().Perm() != 0o755 {
		t.Errorf("socket directory %v %v, want 0755", di, err)
	}

	dockerd.exit(0)
	wantCode(t, code, 0)
}

func TestNoCredentialRunsPluginAsCaller(t *testing.T) {
	i, st := newInit(t)
	i.NoCredential = true
	i.LookupUser = func(string) (uint32, uint32, error) { t.Error("user must not be looked up"); return 0, 0, nil }
	code := runAsync(i, nil, make(chan os.Signal))
	if p := st.next(t); p.spec.Cred != nil {
		t.Errorf("credential %+v, want none", p.spec.Cred)
	}
	st.next(t).exit(0)
	wantCode(t, code, 0)
}

func TestStaleSocketIsReplaced(t *testing.T) {
	i, st := newInit(t)
	if err := os.MkdirAll(filepath.Dir(i.SocketPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(i.SocketPath, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	code := runAsync(i, nil, make(chan os.Signal))
	st.next(t)
	dockerd := st.next(t)
	if fi, err := os.Stat(i.SocketPath); err != nil || fi.Mode()&os.ModeSocket == 0 {
		t.Errorf("stale file not replaced by a socket: %v %v", fi, err)
	}
	dockerd.exit(0)
	wantCode(t, code, 0)
}

func TestPluginRestartsWithSameSocket(t *testing.T) {
	i, st := newInit(t)
	code := runAsync(i, nil, make(chan os.Signal))
	first := st.next(t)
	dockerd := st.next(t)

	prev := first
	for n := 1; n <= 2; n++ {
		prev.exit(3)
		again := st.next(t)
		if again == prev || again.spec.Path != "/plugin" || len(again.spec.ExtraFiles) != 1 ||
			again.spec.ExtraFiles[0] != first.spec.ExtraFiles[0] || again.spec.Cred == nil {
			t.Fatalf("restart %d: spec %+v", n, again.spec)
		}
		prev = again
	}
	if sigs := dockerd.signals(); len(sigs) != 0 {
		t.Errorf("dockerd must not be signalled by a plugin restart: %v", sigs)
	}
	dockerd.exit(0)
	wantCode(t, code, 0)
}

func TestDockerdExitCodePropagatedAndPluginTerminated(t *testing.T) {
	i, st := newInit(t)
	code := runAsync(i, nil, make(chan os.Signal))
	plugin := st.next(t)
	dockerd := st.next(t)
	dockerd.exit(7)
	wantCode(t, code, 7)
	sigs := plugin.signals()
	if len(sigs) == 0 || sigs[0] != syscall.SIGTERM {
		t.Errorf("plugin signals %v, want SIGTERM first", sigs)
	}
}

func TestDockerdSignalDeathExitsWith1(t *testing.T) {
	i, st := newInit(t)
	code := runAsync(i, nil, make(chan os.Signal))
	st.next(t)
	st.next(t).exit(-1)
	wantCode(t, code, 1)
}

func TestSignalIsForwardedToDockerd(t *testing.T) {
	i, st := newInit(t)
	sigs := make(chan os.Signal, 1)
	code := runAsync(i, nil, sigs)
	plugin := st.next(t)
	dockerd := st.next(t)
	sigs <- syscall.SIGTERM
	// the fake dockerd exits (143) when signalled; agent-init follows with that code
	wantCode(t, code, 143)
	if got := dockerd.signals(); len(got) != 1 || got[0] != syscall.SIGTERM {
		t.Errorf("dockerd signals %v, want [SIGTERM]", got)
	}
	if got := plugin.signals(); len(got) == 0 || got[0] != syscall.SIGTERM {
		t.Errorf("plugin signals %v, want SIGTERM", got)
	}
}

func TestArgumentsRejected(t *testing.T) {
	i, st := newInit(t)
	if got := i.Run([]string{"dockerd"}, make(chan os.Signal)); got != 2 {
		t.Fatalf("exit code %d, want 2", got)
	}
	select {
	case p := <-st.started:
		t.Fatalf("nothing may start, started %+v", p.spec)
	default:
	}
	if _, err := os.Stat(i.SocketPath); err == nil {
		t.Error("no socket may be created")
	}
}
