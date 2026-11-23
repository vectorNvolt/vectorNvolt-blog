package services

import (
	"context"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestHelperProcess is the child of TestExecRunnerKillsChildOnCancel: it only sleeps.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	time.Sleep(60 * time.Second)
	os.Exit(0)
}

// SFR-IMG-018: cancelling the context kills the child and Run returns promptly.
func TestExecRunnerKillsChildOnCancel(t *testing.T) {
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(100*time.Millisecond, cancel)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ExecRunner{}.Run(ctx, nil, nil, os.Args[0], "-test.run=^TestHelperProcess$")
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}

func TestExecRunnerRunsCommandAsIs(t *testing.T) {
	cmd := ExecRunner{}.command(context.Background(), []string{"docker", "image", "inspect", "x"})
	if want := []string{"docker", "image", "inspect", "x"}; !reflect.DeepEqual(cmd.Args, want) {
		t.Errorf("args = %v, want %v", cmd.Args, want)
	}
}

// ---- stub runner: records the last call ----

type stubRunner struct {
	stdout string
	stderr []byte
	code   int
	got    []string
	live   io.Writer
}

func (s *stubRunner) Run(_ context.Context, _ []byte, live io.Writer, args ...string) ([]byte, []byte, int, error) {
	s.got = args
	s.live = live
	return []byte(s.stdout), s.stderr, s.code, nil
}

func TestExecArgsAreTheCommand(t *testing.T) {
	sr := &stubRunner{}
	if _, err := NewHost(sr).Exec(context.Background(), "docker", "version"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(sr.got, " ") != "docker version" {
		t.Errorf("args = %v", sr.got)
	}
}

func TestExecNonZeroCarriesStderr(t *testing.T) {
	sr := &stubRunner{code: 1, stderr: []byte("boom")}
	_, err := NewHost(sr).Exec(context.Background(), "docker", "version")
	var he *HostError
	if !errors.As(err, &he) || he.ExitCode != 1 || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v", err)
	}
}

func TestErrorMessageIsTheCommand(t *testing.T) {
	e := &HostError{Args: []string{"docker", "info"}, ExitCode: 1}
	if got := e.Error(); got != "docker info: exit code 1" {
		t.Errorf("got %q", got)
	}
}
