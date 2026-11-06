// IP-WSL-CLI adapter: the only place that talks to wsl.exe.
// Every operation is pinned to the agent-host distro; no distro parameter exists (SSR-HOST-005).
package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"

	"agent-host/internal/config"
)

// Runner executes wsl.exe with the given arguments. A non-zero exit status is
// reported through exitCode; err is only set when the process could not run.
type Runner interface {
	Run(ctx context.Context, stdin []byte, args ...string) (stdout, stderr []byte, exitCode int, err error)
}

// ExecRunner runs the real executable with WSL_UTF8=1.
type ExecRunner struct {
	Exe string
}

func (r ExecRunner) Run(ctx context.Context, stdin []byte, args ...string) ([]byte, []byte, int, error) {
	cmd := exec.CommandContext(ctx, r.Exe, args...)
	cmd.Env = append(os.Environ(), "WSL_UTF8=1")
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
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
	Inner Runner
	Trace func(args []string)
}

func (t TracingRunner) Run(ctx context.Context, stdin []byte, args ...string) ([]byte, []byte, int, error) {
	t.Trace(args)
	return t.Inner.Run(ctx, stdin, args...)
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

// WSL offers typed operations on the agent-host distro.
type WSL struct {
	Runner Runner
}

func NewWSL(r Runner) *WSL { return &WSL{Runner: r} }

func (w *WSL) run(ctx context.Context, stdin []byte, args ...string) ([]byte, error) {
	stdout, stderr, code, err := w.Runner.Run(ctx, stdin, args...)
	if err != nil {
		return stdout, &WSLError{Args: args, ExitCode: -1, Stderr: decodeText(stderr), Err: err}
	}
	if code != 0 {
		return stdout, &WSLError{Args: args, ExitCode: code, Stderr: decodeText(stderr)}
	}
	return stdout, nil
}

// UnknownWSLVersion is recorded for a listed distro whose version column did not parse.
const UnknownWSLVersion = 0

// FindDistro looks name up case-insensitively (WSL distro names are case-insensitive)
// and returns the name as listed and its version.
func FindDistro(distros map[string]int, name string) (string, int, bool) {
	if v, ok := distros[name]; ok {
		return name, v, true
	}
	for n, v := range distros {
		if strings.EqualFold(n, name) {
			return n, v, true
		}
	}
	return "", 0, false
}

// List runs `wsl.exe -l -v` and returns distro name -> WSL version
// (UnknownWSLVersion if the version column is unparseable).
func (w *WSL) List(ctx context.Context) (map[string]int, error) {
	out, err := w.run(ctx, nil, "-l", "-v")
	if err != nil {
		return nil, err
	}
	return parseDistroList(out), nil
}

// Version runs `wsl.exe --version` and returns the WSL version, the first dotted number on
// the first line (the label is localized, e.g. "WSL version: 2.4.10.0"). Old inbox WSL has no
// --version: it fails or prints usage text, which yields an error here.
func (w *WSL) Version(ctx context.Context) (string, error) {
	out, err := w.run(ctx, nil, "--version")
	if err != nil {
		return "", err
	}
	return parseWSLVersion(out)
}

var dottedVersion = regexp.MustCompile(`\d+(?:\.\d+)+`)

// parseWSLVersion extracts the first dotted version from the first non-empty line of `wsl.exe --version`.
func parseWSLVersion(raw []byte) (string, error) {
	text := strings.TrimPrefix(decodeText(raw), "\uFEFF")
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.ReplaceAll(line, "\x00", ""))
		if line == "" {
			continue
		}
		if v := dottedVersion.FindString(line); v != "" {
			return v, nil
		}
		break
	}
	return "", fmt.Errorf("no version found in %s --version output", config.WSLExe)
}

// VersionAtLeast reports whether the dotted version v is >= min (missing components count as 0).
func VersionAtLeast(v, min string) bool {
	a, b := strings.Split(v, "."), strings.Split(min, ".")
	for i := 0; i < len(a) || i < len(b); i++ {
		var x, y int
		if i < len(a) {
			x, _ = strconv.Atoi(a[i])
		}
		if i < len(b) {
			y, _ = strconv.Atoi(b[i])
		}
		if x != y {
			return x > y
		}
	}
	return true
}

// Import registers agent-host as a WSL2 distro.
func (w *WSL) Import(ctx context.Context, installDir, tarball string) error {
	_, err := w.run(ctx, nil, "--import", config.DistroName, installDir, tarball, "--version", "2")
	return err
}

// Exec runs cmd inside agent-host as user.
func (w *WSL) Exec(ctx context.Context, user string, stdin []byte, cmd ...string) ([]byte, error) {
	args := append([]string{"-d", config.DistroName, "-u", user, "-e"}, cmd...)
	return w.run(ctx, stdin, args...)
}

// ExecDefaultUser runs cmd inside agent-host as the distro's default user (no -u).
func (w *WSL) ExecDefaultUser(ctx context.Context, stdin []byte, cmd ...string) ([]byte, error) {
	args := append([]string{"-d", config.DistroName, "-e"}, cmd...)
	return w.run(ctx, stdin, args...)
}

// Terminate stops agent-host.
func (w *WSL) Terminate(ctx context.Context) error {
	_, err := w.run(ctx, nil, "--terminate", config.DistroName)
	return err
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

// parseDistroList parses `wsl.exe -l -v` output into name -> version.
func parseDistroList(raw []byte) map[string]int {
	res := map[string]int{}
	text := strings.TrimPrefix(decodeText(raw), "\uFEFF")
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(strings.ReplaceAll(line, "\x00", ""))
		if len(fields) > 0 && strings.HasPrefix(fields[0], "*") {
			fields[0] = strings.TrimPrefix(fields[0], "*")
			if fields[0] == "" {
				fields = fields[1:]
			}
		}
		if len(fields) < 2 || strings.EqualFold(fields[0], "NAME") {
			continue
		}
		v, err := strconv.Atoi(fields[len(fields)-1])
		if err != nil {
			// Unparseable version: still record a well-formed row (NAME STATE VERSION)
			// so the collision check sees it; prose lines have more columns.
			if len(fields) != 3 {
				continue
			}
			v = UnknownWSLVersion
		}
		res[fields[0]] = v
	}
	return res
}
