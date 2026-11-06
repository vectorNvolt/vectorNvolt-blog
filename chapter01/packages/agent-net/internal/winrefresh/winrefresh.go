// Package winrefresh is the logic of refresh_endpoints.exe (Windows side): it reads endpoints.json from the current
// working directory, validates it, and sends the bytes on stdin to `refresh_endpoints` in the agent-host distro through
// wsl.exe. The endpoints config is never passed by path. Only main.go is Windows-specific in spirit; this package
// compiles everywhere so the tests run on Linux.
package winrefresh

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"agent-net/internal/config"
)

const (
	WSLExe     = "wsl.exe"
	HostDistro = "agent-host"
	RemoteTool = "/usr/local/bin/refresh_endpoints"
	FileName   = "endpoints.json"
	MaxBytes   = 64 << 10 // the remote tool refuses more

	ExitPrerequisite = 1
	ExitRuntime      = 2
)

// Usage is printed for any invocation other than the accepted form.
const Usage = "usage: refresh_endpoints.exe [--dry-run]\n" +
	"  sends " + FileName + " (from the current directory) to refresh_endpoints in the " + HostDistro + " distro"

// WSL runs wsl.exe with stdin and streams its output. A non-zero exit status is returned as exitCode; err is only set
// when the process could not run.
type WSL interface {
	Run(ctx context.Context, stdin []byte, stdout, stderr io.Writer, args ...string) (exitCode int, err error)
}

// Deps are the injected collaborators.
type Deps struct {
	Args   []string // os.Args[1:]
	Dir    string   // the current working directory
	WSL    WSL
	Stdout io.Writer
	Stderr io.Writer
}

// Run validates the invocation and the file, calls the distro and returns the process exit code.
func Run(ctx context.Context, d Deps) int {
	dry := false
	for _, a := range d.Args {
		if a != "--dry-run" {
			fmt.Fprintln(d.Stderr, Usage)
			return ExitPrerequisite
		}
		dry = true
	}

	path := filepath.Join(d.Dir, FileName)
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(d.Stderr, "cannot read %s: %v\n", path, err)
		return ExitPrerequisite
	}
	if len(data) > MaxBytes {
		fmt.Fprintf(d.Stderr, "%s is larger than %d bytes\n", path, MaxBytes)
		return ExitPrerequisite
	}
	if _, err := config.ParseEndpoints(data); err != nil {
		fmt.Fprintf(d.Stderr, "%s: %v\n", path, err)
		return ExitPrerequisite
	}

	args := []string{"-d", HostDistro, "-u", "root", "-e", RemoteTool}
	if dry {
		args = append(args, "--dry-run")
	}
	code, err := d.WSL.Run(ctx, data, d.Stdout, d.Stderr, args...)
	if err != nil {
		fmt.Fprintf(d.Stderr, "%s: %v\n", WSLExe, err)
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			return ExitPrerequisite
		}
		return ExitRuntime
	}
	if code < 0 {
		return ExitRuntime
	}
	return code
}

// ExecWSL runs the real wsl.exe with WSL_UTF8=1.
type ExecWSL struct {
	Exe string
}

func (r ExecWSL) Run(ctx context.Context, stdin []byte, stdout, stderr io.Writer, args ...string) (int, error) {
	cmd := exec.CommandContext(ctx, r.Exe, args...)
	cmd.Env = append(os.Environ(), "WSL_UTF8=1")
	cmd.WaitDelay = 5 * time.Second
	cmd.Stdin = bytes.NewReader(stdin)
	cmd.Stdout = &crlfWriter{w: stdout} // the Windows console needs CRLF
	cmd.Stderr = &crlfWriter{w: stderr}
	err := cmd.Run()
	if err == nil {
		return 0, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), nil
	}
	return -1, err
}

// crlfWriter rewrites every bare LF to CRLF; an LF already preceded by CR (even across two writes) is left alone.
// Write reports the input length.
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
