package services

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// HostError is a failed command with its captured stderr.
type HostError struct {
	Args     []string
	ExitCode int
	Stderr   string
	Err      error
}

func (e *HostError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: ", strings.Join(e.Args, " "))
	if e.Err != nil {
		b.WriteString(e.Err.Error())
	} else {
		fmt.Fprintf(&b, "exit code %d", e.ExitCode)
	}
	if s := tail(strings.TrimSpace(e.Stderr), 2000); s != "" {
		b.WriteString("\nstderr: " + s)
	}
	return b.String()
}

func (e *HostError) Unwrap() error { return e.Err }

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "..." + s[len(s)-n:]
}

// Host offers typed operations on the machine this tool runs on (root inside the VM).
type Host struct {
	Runner Runner
}

func NewHost(r Runner) *Host { return &Host{Runner: r} }

// Exec runs cmd.
func (h *Host) Exec(ctx context.Context, cmd ...string) ([]byte, error) {
	return h.ExecStdin(ctx, nil, cmd...)
}

// ExecStdin is Exec with stdin fed to cmd (byte-exact; nil means no stdin).
func (h *Host) ExecStdin(ctx context.Context, stdin []byte, cmd ...string) ([]byte, error) {
	return h.ExecStream(ctx, stdin, nil, cmd...)
}

// ExecStream is ExecStdin that also copies cmd's stdout and stderr to live as they
// arrive (nil means quiet). Output is still captured, so errors carry the stderr tail.
func (h *Host) ExecStream(ctx context.Context, stdin []byte, live io.Writer, cmd ...string) ([]byte, error) {
	stdout, stderr, code, err := h.Runner.Run(ctx, stdin, live, cmd...)
	if err != nil {
		return stdout, &HostError{Args: cmd, ExitCode: -1, Stderr: string(stderr), Err: err}
	}
	if code != 0 {
		return stdout, &HostError{Args: cmd, ExitCode: code, Stderr: string(stderr)}
	}
	return stdout, nil
}
