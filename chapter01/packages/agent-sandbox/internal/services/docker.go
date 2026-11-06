// IP-DOCKER-ENGINE adapter: the only place that talks to the docker CLI.
// The tool runs as root inside agent-host, so docker is called directly (no wsl.exe).
package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"agent-sandbox/internal/config"
)

// Runner executes a program; args[0] is the executable. A non-zero exit status is
// reported through exitCode; err is only set when the process could not run.
type Runner interface {
	Run(ctx context.Context, stdin []byte, args ...string) (stdout, stderr []byte, exitCode int, err error)
}

// ExecRunner runs the real executable.
type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, stdin []byte, args ...string) ([]byte, []byte, int, error) {
	if len(args) == 0 {
		return nil, nil, -1, errors.New("no command given")
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
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

// CmdError is a failed invocation with its captured stderr. Err is nil for a non-zero exit.
type CmdError struct {
	Args     []string
	ExitCode int
	Stderr   string
	Stdout   string // captured stdout of a non-zero exit (the per-control report lines of check-gate)
	Err      error
}

func (e *CmdError) Error() string {
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

func (e *CmdError) Unwrap() error { return e.Err }

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "..." + s[len(s)-n:]
}

func runCmd(ctx context.Context, r Runner, args ...string) ([]byte, error) {
	stdout, stderr, code, err := r.Run(ctx, nil, args...)
	if err != nil {
		return stdout, &CmdError{Args: args, ExitCode: -1, Stderr: string(stderr), Err: err}
	}
	if code != 0 {
		return stdout, &CmdError{Args: args, ExitCode: code, Stderr: string(stderr), Stdout: string(stdout)}
	}
	return stdout, nil
}

// Mount is one entry of the inspected container's Mounts.
type Mount struct {
	Type        string `json:"Type"`
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
	RW          bool   `json:"RW"`
}

// ContainerInfo holds the fields of `docker container inspect` that SFR-AGC-009 reads.
type ContainerInfo struct {
	State struct {
		Running bool `json:"Running"`
		Paused  bool `json:"Paused"`
	} `json:"State"`
	Config struct {
		User string   `json:"User"`
		Env  []string `json:"Env"`
	} `json:"Config"`
	HostConfig struct {
		Runtime       string `json:"Runtime"`
		CapDrop       []string
		CapAdd        []string
		SecurityOpt   []string
		Memory        int64
		NanoCpus      int64
		PidsLimit     int64
		Tmpfs         map[string]string
		RestartPolicy struct {
			Name string `json:"Name"`
		} `json:"RestartPolicy"`
	} `json:"HostConfig"`
	Mounts          []Mount `json:"Mounts"`
	NetworkSettings struct {
		Networks map[string]json.RawMessage `json:"Networks"`
	} `json:"NetworkSettings"`
}

// Docker offers typed operations on the docker engine.
type Docker struct {
	Runner Runner
}

func NewDocker(r Runner) *Docker { return &Docker{Runner: r} }

func (d *Docker) run(ctx context.Context, args ...string) ([]byte, error) {
	return runCmd(ctx, d.Runner, append([]string{config.DockerExe}, args...)...)
}

// exists maps only a "No such container/object/network/image" answer on stderr to "absent";
// any other non-zero exit and a process that cannot run are errors (engine errors are not absence).
func (d *Docker) exists(ctx context.Context, args ...string) (bool, error) {
	_, err := d.run(ctx, args...)
	if err == nil {
		return true, nil
	}
	var ce *CmdError
	if errors.As(err, &ce) && ce.Err == nil && isNoSuch(ce.Stderr) {
		return false, nil
	}
	return false, err
}

func isNoSuch(stderr string) bool {
	s := strings.ToLower(stderr)
	for _, m := range []string{"no such container", "no such object", "no such network", "no such image"} {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}

// RuntimeRegistered reports whether the engine registers the named runtime (SFR-RTC-001).
func (d *Docker) RuntimeRegistered(ctx context.Context, name string) (bool, error) {
	out, err := d.run(ctx, "info", "--format", "{{json .Runtimes}}")
	if err != nil {
		return false, err
	}
	var runtimes map[string]json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(out), &runtimes); err != nil {
		return false, fmt.Errorf("cannot parse docker runtimes %q: %w", strings.TrimSpace(string(out)), err)
	}
	_, ok := runtimes[name]
	return ok, nil
}

// Reachable reports an error when the engine does not answer (USC-006 step 2; runsc is not required).
func (d *Docker) Reachable(ctx context.Context) error {
	_, err := d.run(ctx, "info", "--format", "{{.ServerVersion}}")
	return err
}

func (d *Docker) ContainerExists(ctx context.Context, name string) (bool, error) {
	return d.exists(ctx, "container", "inspect", name)
}

// ContainerName resolves a container ID (as carried by network events) to its name; found is false when the
// container does not exist.
func (d *Docker) ContainerName(ctx context.Context, id string) (name string, found bool, err error) {
	out, err := d.run(ctx, "container", "inspect", "--format", "{{.Name}}", id)
	if err != nil {
		var ce *CmdError
		if errors.As(err, &ce) && ce.Err == nil && isNoSuch(ce.Stderr) {
			return "", false, nil
		}
		return "", false, err
	}
	return strings.TrimPrefix(strings.TrimSpace(string(out)), "/"), true, nil
}

// State returns State.Status of the container (running, exited, paused, created, dead, restarting, removing) and ""
// when it does not exist.
func (d *Docker) State(ctx context.Context, name string) (string, error) {
	out, err := d.run(ctx, "container", "inspect", "--format", "{{.State.Status}}", name)
	if err != nil {
		var ce *CmdError
		if errors.As(err, &ce) && ce.Err == nil && isNoSuch(ce.Stderr) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (d *Docker) NetworkExists(ctx context.Context, name string) (bool, error) {
	return d.exists(ctx, "network", "inspect", name)
}

func (d *Docker) ImageExists(ctx context.Context, ref string) (bool, error) {
	return d.exists(ctx, "image", "inspect", ref)
}

// ListAgentContainers returns the names of all containers (any state) whose name starts with the agent prefix.
func (d *Docker) ListAgentContainers(ctx context.Context) ([]string, error) {
	out, err := d.run(ctx, "ps", "-a", "--filter", "name="+config.ContainerPrefix, "--format", "{{.Names}}")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, n := range strings.Fields(string(out)) {
		if strings.HasPrefix(n, config.ContainerPrefix) {
			names = append(names, n)
		}
	}
	return names, nil
}

// CreateArgs returns the arguments after `docker` for the SFR-AGC-006 container creation. claudeDir is the host
// state directory mounted read-write at config.ClaudeMount; hostResolv is mounted read-only at /etc/resolv.conf.
func CreateArgs(name, workspace, claudeDir, hostResolv string) []string {
	args := []string{
		"create", "--name", name,
		"--runtime=" + config.Runtime,
		"--restart", "no", // never auto-restart: after a WSL2 restart the gate is gone
		"--network", config.NetworkName, // attached at create; the agents-net gate keeps the port closed until create_agent_net arm
		"--cap-drop", "ALL",
	}
	for _, c := range config.Capabilities {
		args = append(args, "--cap-add", c)
	}
	return append(args,
		"--security-opt", "no-new-privileges",
		"--memory", config.Memory,
		"--cpus", config.CPUs,
		"--pids-limit", config.PidsLimit,
		"--tmpfs", config.TmpfsTmpPath+":"+config.TmpfsTmpOptions,
		"--tmpfs", config.TmpfsDockerPath+":"+config.TmpfsDockerOptions,
		"-v", workspace+":"+config.WorkspaceMount+":rw",
		"-v", claudeDir+":"+config.ClaudeMount+":rw",
		"-v", hostResolv+":"+config.ResolvConfPath+":ro",
		"-e", config.ClaudeEnvVar+"="+config.ClaudeMount,
		config.Image,
	)
}

func (d *Docker) Create(ctx context.Context, name, workspace, claudeDir, hostResolv string) error {
	_, err := d.run(ctx, CreateArgs(name, workspace, claudeDir, hostResolv)...)
	return err
}

func (d *Docker) Start(ctx context.Context, name string) error {
	_, err := d.run(ctx, "start", name)
	return err
}

// Exec runs cmd inside the container as user (uid:gid) and returns its stdout.
func (d *Docker) Exec(ctx context.Context, name, user string, cmd ...string) ([]byte, error) {
	return d.run(ctx, append([]string{"exec", "-u", user, name}, cmd...)...)
}

// ExecSucceeds reports whether cmd exits 0 in the container; a non-zero exit is false, not an error.
func (d *Docker) ExecSucceeds(ctx context.Context, name, user string, cmd ...string) (bool, error) {
	_, err := d.run(ctx, append([]string{"exec", "-u", user, name}, cmd...)...)
	if err == nil {
		return true, nil
	}
	var ce *CmdError
	if errors.As(err, &ce) && ce.Err == nil {
		return false, nil
	}
	return false, err
}

// Pause pauses the container (pause gate SFR-AGC-012 and containment SFR-AGC-009).
func (d *Docker) Pause(ctx context.Context, name string) error {
	_, err := d.run(ctx, "pause", name)
	return err
}

// Unpause releases the container once the arming is verified (SFR-AGC-021).
func (d *Docker) Unpause(ctx context.Context, name string) error {
	_, err := d.run(ctx, "unpause", name)
	return err
}

// Remove force-removes the named container (only ever the one this run created).
func (d *Docker) Remove(ctx context.Context, name string) error {
	// runsc: a paused sandbox cannot be killed, so `rm -f` reports "is zombie and can not be killed"; unpause first.
	out, err := d.run(ctx, "inspect", "-f", "{{.State.Paused}}", name)
	if err != nil {
		var ce *CmdError
		if !(errors.As(err, &ce) && ce.Err == nil && isNoSuch(ce.Stderr)) {
			return err
		}
	} else if strings.TrimSpace(string(out)) == "true" {
		if _, err := d.run(ctx, "unpause", name); err != nil {
			return fmt.Errorf("unpause before remove: %w", err)
		}
	}
	_, err = d.run(ctx, "rm", "-f", name)
	return err
}

// Inspect reads the container state, runtime, restart policy and mounts.
func (d *Docker) Inspect(ctx context.Context, name string) (ContainerInfo, error) {
	var info ContainerInfo
	out, err := d.run(ctx, "container", "inspect", "--format", "{{json .}}", name)
	if err != nil {
		return info, err
	}
	if err := json.Unmarshal(bytes.TrimSpace(out), &info); err != nil {
		return info, fmt.Errorf("cannot parse inspect output of %s: %w", name, err)
	}
	return info, nil
}
