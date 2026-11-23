// IP-DOCKER-ENGINE adapter: runs commands directly (root inside the VM), no wrapper executable.
package services

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"sync"
	"time"
)

// Runner executes a command (args[0] is the executable). A non-zero exit status is
// reported through exitCode; err is only set when the process could not run.
// A non-nil live also receives stdout and stderr as they are produced, in
// addition to the capture (raw bytes).
type Runner interface {
	Run(ctx context.Context, stdin []byte, live io.Writer, args ...string) (stdout, stderr []byte, exitCode int, err error)
}

// ExecRunner runs the command as-is (`docker ...`).
type ExecRunner struct{}

func (r ExecRunner) command(ctx context.Context, args []string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.WaitDelay = 5 * time.Second // SFR-IMG-018: after the kill, Run cannot hang on output pipes
	return cmd
}

func (r ExecRunner) Run(ctx context.Context, stdin []byte, live io.Writer, args ...string) ([]byte, []byte, int, error) {
	if len(args) == 0 {
		return nil, nil, -1, errors.New("no command")
	}
	cmd := r.command(ctx, args)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if live != nil {
		// One shared writer for both streams: os/exec copies each in its own goroutine.
		lw := &lockedWriter{w: live}
		cmd.Stdout = io.MultiWriter(&out, lw)
		cmd.Stderr = io.MultiWriter(&errb, lw)
	}
	code, err := exitStatus(cmd.Run())
	return out.Bytes(), errb.Bytes(), code, err
}

// lockedWriter serializes writes from the stdout and stderr copy goroutines.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// exitStatus maps a command error to (exit code, error): a non-zero exit is not an error.
func exitStatus(err error) (int, error) {
	if err == nil {
		return 0, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), nil
	}
	return -1, err
}
