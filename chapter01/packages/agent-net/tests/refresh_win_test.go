package tests

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"agent-net/internal/winrefresh"
)

// fakeWSL records the wsl.exe invocation.
type fakeWSL struct {
	calls  int
	args   []string
	stdin  []byte
	code   int
	err    error
	stdout string
	stderr string
}

func (f *fakeWSL) Run(_ context.Context, stdin []byte, stdout, stderr io.Writer, args ...string) (int, error) {
	f.calls++
	f.args = args
	f.stdin = stdin
	io.WriteString(stdout, f.stdout)
	io.WriteString(stderr, f.stderr)
	return f.code, f.err
}

func winDir(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if content != "" {
		if err := os.WriteFile(filepath.Join(dir, "endpoints.json"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func runWin(dir string, w *fakeWSL, args ...string) result {
	var out, errb bytes.Buffer
	code := winrefresh.Run(context.Background(), winrefresh.Deps{Args: args, Dir: dir, WSL: w, Stdout: &out, Stderr: &errb})
	return result{code, out.String(), errb.String()}
}

func TestWinRefreshSendsFileBytesOnStdin(t *testing.T) {
	content := "{\n  \"endpoints\": [\"api.anthropic.com\"]\n}\n"
	w := &fakeWSL{stdout: "ok\n"}
	r := runWin(winDir(t, content), w)
	if r.code != 0 || w.calls != 1 || r.stdout != "ok\n" {
		t.Fatalf("exit %d, calls %d, stdout %q, stderr %q", r.code, w.calls, r.stdout, r.err)
	}
	if string(w.stdin) != content {
		t.Errorf("stdin %q, want the file bytes", w.stdin)
	}
	want := []string{"-d", "agent-host", "-u", "root", "-e", "/usr/local/bin/refresh_endpoints"}
	if !reflect.DeepEqual(w.args, want) {
		t.Errorf("args %v, want %v", w.args, want)
	}
}

func TestWinRefreshForwardsDryRun(t *testing.T) {
	w := &fakeWSL{}
	r := runWin(winDir(t, epJSON), w, "--dry-run")
	if r.code != 0 || len(w.args) == 0 || w.args[len(w.args)-1] != "--dry-run" {
		t.Errorf("exit %d, args %v", r.code, w.args)
	}
}

func TestWinRefreshPropagatesExitCode(t *testing.T) {
	for _, code := range []int{1, 2, 3} {
		w := &fakeWSL{code: code, stderr: "boom\n"}
		r := runWin(winDir(t, epJSON), w)
		if r.code != code || r.err != "boom\n" {
			t.Errorf("remote %d: exit %d, stderr %q", code, r.code, r.err)
		}
	}
}

func TestWinRefreshFileMissingOrInvalidCallsNoWSL(t *testing.T) {
	cases := map[string]string{
		"missing":          "",
		"invalid json":     "not json",
		"empty list":       `{"endpoints": []}`,
		"bad host":         `{"endpoints": ["bad host"]}`,
		"trailing garbage": `{"endpoints":["a.example"]} x`,
		"two objects":      `{"endpoints":["a.example"]}{"endpoints":["b.example"]}`,
		"oversize":         epJSON + strings.Repeat(" ", 64<<10),
	}
	for name, content := range cases {
		w := &fakeWSL{}
		r := runWin(winDir(t, content), w)
		if r.code != 1 || w.calls != 0 || r.err == "" {
			t.Errorf("%s: exit %d, calls %d, stderr %q", name, r.code, w.calls, r.err)
		}
	}
}

func TestWinRefreshBadArgs(t *testing.T) {
	for _, args := range [][]string{{"--config", "x"}, {"extra"}, {"--bogus"}} {
		w := &fakeWSL{}
		r := runWin(winDir(t, epJSON), w, args...)
		if r.code != 1 || w.calls != 0 || !strings.Contains(r.err, "usage: refresh_endpoints.exe [--dry-run]") {
			t.Errorf("args %q: exit %d, calls %d, stderr %q", args, r.code, w.calls, r.err)
		}
	}
}

func TestWinRefreshWSLFailures(t *testing.T) {
	w := &fakeWSL{code: -1, err: exec.ErrNotFound}
	if r := runWin(winDir(t, epJSON), w); r.code != 1 {
		t.Errorf("wsl.exe missing: exit %d, stderr %q", r.code, r.err)
	}
	w = &fakeWSL{code: -1, err: errors.New("pipe broken")}
	if r := runWin(winDir(t, epJSON), w); r.code != 2 {
		t.Errorf("wsl.exe failed to run: exit %d, stderr %q", r.code, r.err)
	}
}
