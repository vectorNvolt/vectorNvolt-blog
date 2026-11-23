// Host adapter: the only place that starts processes. Commands run directly on the machine create_agent_host
// runs on (the foundation VM, as root); nothing is wrapped or redirected.
package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

// RunuserPath drops from root to another user without a login shell or password.
const RunuserPath = "/usr/sbin/runuser"

// HostRunner executes argv[0] with the remaining arguments. A non-zero exit status is
// reported through exitCode; err is only set when the process could not run.
type HostRunner interface {
	Run(ctx context.Context, stdin []byte, args ...string) (stdout, stderr []byte, exitCode int, err error)
}

// ExecRunner runs the command as-is with os/exec.
type ExecRunner struct{}

func (e ExecRunner) Run(ctx context.Context, stdin []byte, args ...string) ([]byte, []byte, int, error) {
	return e.RunStream(ctx, stdin, nil, args...)
}

// LineStreamer is an optional HostRunner capability: like Run, but onLine is called for every complete
// output line as it arrives (stream is "stdout" or "stderr"). Calls are serialized.
type LineStreamer interface {
	RunStream(ctx context.Context, stdin []byte, onLine func(stream, line string), args ...string) (stdout, stderr []byte, exitCode int, err error)
}

// lineWriter splits written bytes into lines and reports each complete line.
type lineWriter struct {
	mu     *sync.Mutex
	stream string
	onLine func(stream, line string)
	buf    []byte
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		w.emit(string(bytes.TrimSuffix(w.buf[:i], []byte("\r"))))
		w.buf = w.buf[i+1:]
	}
	return len(p), nil
}

// flush reports a trailing line that has no newline.
func (w *lineWriter) flush() {
	if len(w.buf) > 0 {
		w.emit(string(w.buf))
		w.buf = nil
	}
}

func (w *lineWriter) emit(line string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onLine(w.stream, line)
}

// RunStream is Run plus a callback per output line (nil onLine behaves exactly like Run).
func (ExecRunner) RunStream(ctx context.Context, stdin []byte, onLine func(stream, line string), args ...string) ([]byte, []byte, int, error) {
	if len(args) == 0 {
		return nil, nil, -1, errors.New("empty command")
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if onLine != nil {
		var mu sync.Mutex
		so := &lineWriter{mu: &mu, stream: "stdout", onLine: onLine}
		se := &lineWriter{mu: &mu, stream: "stderr", onLine: onLine}
		cmd.Stdout = io.MultiWriter(&out, so)
		cmd.Stderr = io.MultiWriter(&errb, se)
		defer func() { so.flush(); se.flush() }()
	}
	err := cmd.Run()
	if err == nil {
		return out.Bytes(), errb.Bytes(), 0, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return out.Bytes(), errb.Bytes(), ee.ExitCode(), nil
	}
	return out.Bytes(), errb.Bytes(), -1, err
}

// TracingRunner reports each invocation before delegating (DEBUG log level).
type TracingRunner struct {
	Inner HostRunner
	Trace func(args []string)
}

func (t TracingRunner) Run(ctx context.Context, stdin []byte, args ...string) ([]byte, []byte, int, error) {
	t.Trace(args)
	return t.Inner.Run(ctx, stdin, args...)
}

// RunStream forwards to Inner's RunStream if it has one (trace first), else to Run.
func (t TracingRunner) RunStream(ctx context.Context, stdin []byte, onLine func(stream, line string), args ...string) ([]byte, []byte, int, error) {
	if ls, ok := t.Inner.(LineStreamer); ok {
		t.Trace(args)
		return ls.RunStream(ctx, stdin, onLine, args...)
	}
	return t.Run(ctx, stdin, args...)
}

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

// Host offers typed operations on the machine create_agent_host runs on.
type Host struct {
	Runner HostRunner
}

func NewHost(r HostRunner) *Host { return &Host{Runner: r} }

func (h *Host) run(ctx context.Context, stdin []byte, args ...string) ([]byte, error) {
	stdout, stderr, code, err := h.Runner.Run(ctx, stdin, args...)
	if err != nil {
		return stdout, &HostError{Args: args, ExitCode: -1, Stderr: string(stderr), Err: err}
	}
	if code != 0 {
		return stdout, &HostError{Args: args, ExitCode: code, Stderr: string(stderr)}
	}
	return stdout, nil
}

// ExecProgress is Exec with a per-line output callback when the Runner supports it (LineStreamer); otherwise
// it behaves like Exec and onLine is never called.
func (h *Host) ExecProgress(ctx context.Context, stdin []byte, onLine func(stream, line string), cmd ...string) ([]byte, error) {
	ls, ok := h.Runner.(LineStreamer)
	if !ok {
		return h.run(ctx, stdin, cmd...)
	}
	stdout, stderr, code, err := ls.RunStream(ctx, stdin, onLine, cmd...)
	if err != nil {
		return stdout, &HostError{Args: cmd, ExitCode: -1, Stderr: string(stderr), Err: err}
	}
	if code != 0 {
		return stdout, &HostError{Args: cmd, ExitCode: code, Stderr: string(stderr)}
	}
	return stdout, nil
}

// Exec runs cmd as the current user (root).
func (h *Host) Exec(ctx context.Context, stdin []byte, cmd ...string) ([]byte, error) {
	return h.run(ctx, stdin, cmd...)
}

// ExecAs runs cmd as user through runuser (used by verify to check what the unprivileged user can do).
func (h *Host) ExecAs(ctx context.Context, user string, stdin []byte, cmd ...string) ([]byte, error) {
	return h.run(ctx, stdin, append([]string{RunuserPath, "-u", user, "--"}, cmd...)...)
}
