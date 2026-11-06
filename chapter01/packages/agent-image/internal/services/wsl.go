// IP-DOCKER-ENGINE adapter: the only place that talks to wsl.exe.
// Every command runs as root in agent-host via `wsl.exe -d <distro> -u root -e <cmd>`.
package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"agent-image/internal/config"
)

// Runner executes wsl.exe with the given arguments. A non-zero exit status is
// reported through exitCode; err is only set when the process could not run.
// A non-nil live also receives stdout and stderr as they are produced, in
// addition to the capture (raw bytes, not decoded).
type Runner interface {
	Run(ctx context.Context, stdin []byte, live io.Writer, args ...string) (stdout, stderr []byte, exitCode int, err error)
}

// ExecRunner runs the real executable with WSL_UTF8=1.
type ExecRunner struct {
	Exe string
}

func (r ExecRunner) command(ctx context.Context, args []string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, r.Exe, args...)
	cmd.Env = append(os.Environ(), "WSL_UTF8=1")
	cmd.WaitDelay = 5 * time.Second // SFR-IMG-018: after the kill, Run cannot hang on output pipes
	return cmd
}

func (r ExecRunner) Run(ctx context.Context, stdin []byte, live io.Writer, args ...string) ([]byte, []byte, int, error) {
	cmd := r.command(ctx, args)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if live != nil {
		// One shared writer for both streams: os/exec copies each in its own goroutine.
		lw := &lockedWriter{w: &crlfWriter{w: live}} // the Windows console needs CRLF
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

// crlfWriter rewrites every bare LF to CRLF; an LF already preceded by CR (even
// across two writes) is left alone. Write reports the input length.
type crlfWriter struct {
	w      io.Writer
	lastCR bool
}

func (c *crlfWriter) Write(p []byte) (int, error) {
	buf := make([]byte, 0, len(p)+len(p)/8)
	prev := c.lastCR
	for _, b := range p {
		if b == '\n' && !prev {
			buf = append(buf, '\r')
		}
		buf = append(buf, b)
		prev = b == '\r'
	}
	c.lastCR = prev
	if _, err := c.w.Write(buf); err != nil {
		return 0, err
	}
	return len(p), nil
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

// WSLError is a failed wsl.exe invocation with its captured stderr.
type WSLError struct {
	Args     []string
	ExitCode int
	Stderr   string
	Err      error
}

func (e *WSLError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s: ", config.WSLExe, strings.Join(e.Args, " "))
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

func (e *WSLError) Unwrap() error { return e.Err }

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "..." + s[len(s)-n:]
}

// WSL offers typed operations on a distro; every command runs as root.
type WSL struct {
	Runner Runner
}

func NewWSL(r Runner) *WSL { return &WSL{Runner: r} }

// rootArgs builds the wsl.exe arguments that run cmd as root in distro.
func rootArgs(distro string, cmd ...string) []string {
	return append([]string{"-d", distro, "-u", "root", "-e"}, cmd...)
}

// Exec runs cmd as root inside distro.
func (w *WSL) Exec(ctx context.Context, distro string, cmd ...string) ([]byte, error) {
	return w.ExecStdin(ctx, nil, distro, cmd...)
}

// ExecStdin is Exec with stdin fed to cmd (byte-exact; nil means no stdin).
func (w *WSL) ExecStdin(ctx context.Context, stdin []byte, distro string, cmd ...string) ([]byte, error) {
	return w.ExecStream(ctx, stdin, nil, distro, cmd...)
}

// ExecStream is ExecStdin that also copies cmd's stdout and stderr to live as they
// arrive (nil means quiet). Output is still captured, so errors carry the stderr tail.
func (w *WSL) ExecStream(ctx context.Context, stdin []byte, live io.Writer, distro string, cmd ...string) ([]byte, error) {
	args := rootArgs(distro, cmd...)
	stdout, stderr, code, err := w.Runner.Run(ctx, stdin, live, args...)
	if err != nil {
		return stdout, &WSLError{Args: args, ExitCode: -1, Stderr: decodeText(stderr), Err: err}
	}
	if code != 0 {
		return stdout, &WSLError{Args: args, ExitCode: code, Stderr: decodeText(stderr)}
	}
	return stdout, nil
}

// decodeText converts wsl.exe output to UTF-8 text: handles BOMs and UTF-16LE.
func decodeText(b []byte) string {
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
	if bytes.HasPrefix(b, []byte{0xFF, 0xFE}) || bytes.IndexByte(b, 0) >= 0 {
		b = bytes.TrimPrefix(b, []byte{0xFF, 0xFE})
		if len(b)%2 == 1 {
			b = b[:len(b)-1]
		}
		u := make([]uint16, len(b)/2)
		for i := range u {
			u[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
		}
		return string(utf16.Decode(u))
	}
	return string(b)
}
