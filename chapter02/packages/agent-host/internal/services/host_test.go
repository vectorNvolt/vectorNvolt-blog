package services

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestExecRunnerRunsCommandAsIs(t *testing.T) {
	out, errb, code, err := ExecRunner{}.Run(context.Background(), nil, "/bin/echo", "hello", "world")
	if err != nil || code != 0 {
		t.Fatalf("code %d, err %v", code, err)
	}
	if string(out) != "hello world\n" || len(errb) != 0 {
		t.Errorf("stdout %q, stderr %q", out, errb)
	}
}

func TestExecRunnerPassesStdin(t *testing.T) {
	out, _, code, err := ExecRunner{}.Run(context.Background(), []byte("payload"), "/bin/cat")
	if err != nil || code != 0 || string(out) != "payload" {
		t.Fatalf("stdout %q, code %d, err %v", out, code, err)
	}
}

func TestExecRunnerReportsExitCodeWithoutError(t *testing.T) {
	_, errb, code, err := ExecRunner{}.Run(context.Background(), nil, "/bin/sh", "-c", "echo boom >&2; exit 3")
	if err != nil || code != 3 || strings.TrimSpace(string(errb)) != "boom" {
		t.Fatalf("code %d, err %v, stderr %q", code, err, errb)
	}
}

func TestExecRunnerMissingBinaryIsAnError(t *testing.T) {
	_, _, code, err := ExecRunner{}.Run(context.Background(), nil, "/nonexistent/binary")
	if err == nil || code != -1 {
		t.Fatalf("code %d, err %v", code, err)
	}
	if _, _, _, err := (ExecRunner{}).Run(context.Background(), nil); err == nil {
		t.Error("empty command must fail")
	}
}

func TestExecRunnerRunStreamReportsLinesAndKeepsOutput(t *testing.T) {
	var got []string
	onLine := func(stream, line string) { got = append(got, stream+":"+line) }
	out, errb, code, err := ExecRunner{}.RunStream(context.Background(), nil, onLine, "/bin/sh", "-c", "echo a; echo b >&2; echo c")
	if err != nil || code != 0 {
		t.Fatalf("code %d, err %v", code, err)
	}
	if string(out) != "a\nc\n" || string(errb) != "b\n" {
		t.Errorf("stdout %q, stderr %q", out, errb)
	}
	var so, se []string
	for _, g := range got {
		if v, ok := strings.CutPrefix(g, "stdout:"); ok {
			so = append(so, v)
		} else if v, ok := strings.CutPrefix(g, "stderr:"); ok {
			se = append(se, v)
		}
	}
	if strings.Join(so, ",") != "a,c" || strings.Join(se, ",") != "b" {
		t.Errorf("lines = %v", got)
	}

	got = nil
	out, errb, code, err = ExecRunner{}.RunStream(context.Background(), nil, onLine, "/bin/sh", "-c", "echo x; echo y >&2; exit 4")
	if err != nil || code != 4 || string(out) != "x\n" || string(errb) != "y\n" || len(got) != 2 {
		t.Fatalf("code %d, err %v, out %q, err %q, lines %v", code, err, out, errb, got)
	}
}

type recordingRunner struct {
	args   []string
	stderr string
	code   int
	err    error
}

func (r *recordingRunner) Run(_ context.Context, _ []byte, args ...string) ([]byte, []byte, int, error) {
	r.args = append([]string(nil), args...)
	return []byte("out"), []byte(r.stderr), r.code, r.err
}

func TestHostExecAddsNoPrefix(t *testing.T) {
	r := &recordingRunner{}
	out, err := NewHost(r).Exec(context.Background(), nil, "/usr/bin/id", "-un")
	if err != nil || string(out) != "out" {
		t.Fatalf("out %q, err %v", out, err)
	}
	if got := strings.Join(r.args, " "); got != "/usr/bin/id -un" {
		t.Errorf("args = %q", got)
	}
}

func TestHostExecAsUsesRunuser(t *testing.T) {
	r := &recordingRunner{}
	if _, err := NewHost(r).ExecAs(context.Background(), "agent-operator", nil, "id", "-u"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(r.args, " "); got != "/usr/sbin/runuser -u agent-operator -- id -u" {
		t.Errorf("args = %q", got)
	}
}

func TestHostErrorCarriesStderrAndExitCode(t *testing.T) {
	r := &recordingRunner{stderr: "useradd: cannot lock", code: 9}
	_, err := NewHost(r).Exec(context.Background(), nil, "/usr/sbin/useradd", "x")
	var he *HostError
	if !errors.As(err, &he) || he.ExitCode != 9 {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "useradd: cannot lock") || !strings.Contains(err.Error(), "exit code 9") {
		t.Errorf("message %q", err.Error())
	}
	cause := errors.New("no such file")
	_, err = NewHost(&recordingRunner{err: cause}).Exec(context.Background(), nil, "/x")
	if !errors.Is(err, cause) {
		t.Errorf("start failure must unwrap to its cause: %v", err)
	}
}

func TestTracingRunnerReportsArgs(t *testing.T) {
	var traced string
	inner := &recordingRunner{}
	tr := TracingRunner{Inner: inner, Trace: func(a []string) { traced = strings.Join(a, " ") }}
	if _, _, _, err := tr.Run(context.Background(), nil, "/bin/true", "x"); err != nil {
		t.Fatal(err)
	}
	if traced != "/bin/true x" || strings.Join(inner.args, " ") != "/bin/true x" {
		t.Errorf("traced %q, delegated %v", traced, inner.args)
	}
}
