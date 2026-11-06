// IP-DOCKER-ENGINE adapter: the only place that talks to the docker CLI.
// The tool runs as root inside agent-host, so docker is called directly (no wsl.exe).
package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os/exec"
	"strings"

	"agent-net/internal/config"
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
	return runCmdIn(ctx, r, nil, args...)
}

func runCmdIn(ctx context.Context, r Runner, stdin []byte, args ...string) ([]byte, error) {
	stdout, stderr, code, err := r.Run(ctx, stdin, args...)
	if err != nil {
		return stdout, &CmdError{Args: args, ExitCode: -1, Stderr: string(stderr), Err: err}
	}
	if code != 0 {
		return stdout, &CmdError{Args: args, ExitCode: code, Stderr: string(stderr)}
	}
	return stdout, nil
}

// isExitError reports a command that ran and exited non-zero (as opposed to one that could not run).
func isExitError(err error) bool {
	var ce *CmdError
	return errors.As(err, &ce) && ce.Err == nil
}

// isNotFound reports docker's own "does not exist" answer: a command that ran, exited non-zero and said on stderr that
// the network, container or object is not there. Any other failure (engine unreachable, permission denied, ...) is
// not an absence.
func isNotFound(err error) bool {
	var ce *CmdError
	if !errors.As(err, &ce) || ce.Err != nil {
		return false
	}
	s := strings.ToLower(ce.Stderr)
	for _, m := range []string{"no such network", "no such container", "no such object", "not found"} {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}

// NetworkInfo holds the fields of `docker network inspect` that SFR-NET-003..006 and SFR-NET-009 read.
type NetworkInfo struct {
	ID       string `json:"Id"`
	Name     string `json:"Name"`
	Driver   string `json:"Driver"`
	Internal bool   `json:"Internal"`
	IPAM     struct {
		Config []struct {
			Subnet  string `json:"Subnet"`
			Gateway string `json:"Gateway"`
		} `json:"Config"`
	} `json:"IPAM"`
	Options    map[string]string `json:"Options"`
	Containers map[string]struct {
		Name       string `json:"Name"`
		MacAddress string `json:"MacAddress"`
	} `json:"Containers"`
}

// Subnets lists the IPAM subnets of the network.
func (n NetworkInfo) Subnets() []string {
	var out []string
	for _, c := range n.IPAM.Config {
		if c.Subnet != "" {
			out = append(out, c.Subnet)
		}
	}
	return out
}

// Gateway is the IP of the host bridge interface on the network: the IPAM gateway, else the first host address of
// the subnet (docker's default).
func (n NetworkInfo) Gateway() string {
	for _, c := range n.IPAM.Config {
		if c.Gateway != "" {
			return c.Gateway
		}
	}
	for _, s := range n.Subnets() {
		if p, err := netip.ParsePrefix(s); err == nil && p.Addr().Is4() {
			return p.Masked().Addr().Next().String()
		}
	}
	return ""
}

// BridgeName is the host bridge device of the network: the explicit option, else br-<first 12 of id>.
func (n NetworkInfo) BridgeName() string {
	if v := n.Options["com.docker.network.bridge.name"]; v != "" {
		return v
	}
	id := n.ID
	if len(id) > 12 {
		id = id[:12]
	}
	return "br-" + id
}

// Endpoint is one network attachment of an inspected container.
type Endpoint struct {
	IPAddress  string `json:"IPAddress"`
	MacAddress string `json:"MacAddress"`
}

// Mount is one entry of `.Mounts` of an inspected container.
type Mount struct {
	Type        string `json:"Type"`
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
	RW          bool   `json:"RW"`
}

// ContainerInfo holds the fields of `docker container inspect` that SFR-NET-009, SFR-NET-020 and SSR-NET-002 read.
type ContainerInfo struct {
	State struct {
		Pid     int  `json:"Pid"`
		Running bool `json:"Running"`
		Paused  bool `json:"Paused"`
	} `json:"State"`
	Mounts     []Mount `json:"Mounts"`
	HostConfig struct {
		NetworkMode   string `json:"NetworkMode"`
		Runtime       string `json:"Runtime"`
		RestartPolicy struct {
			Name string `json:"Name"`
		} `json:"RestartPolicy"`
	} `json:"HostConfig"`
	NetworkSettings struct {
		Networks map[string]Endpoint `json:"Networks"`
	} `json:"NetworkSettings"`
}

// Docker offers typed operations on the docker engine.
type Docker struct {
	Runner Runner
	Socket string // control socket the CLI talks to (`-H unix://<socket>`); empty is the CLI default
}

func NewDocker(r Runner) *Docker { return &Docker{Runner: r} }

func (d *Docker) run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := []string{config.DockerExe}
	if d.Socket != "" {
		host := d.Socket
		if !strings.Contains(host, "://") {
			host = "unix://" + host
		}
		cmd = append(cmd, "-H", host)
	}
	return runCmd(ctx, d.Runner, append(cmd, args...)...)
}

// Version succeeds only when the engine answers (the client alone is not enough).
func (d *Docker) Version(ctx context.Context) error {
	_, err := d.run(ctx, "version", "--format", "{{.Server.Version}}")
	return err
}

// InspectNetwork maps docker's not-found answer to "absent"; any other failure is an error carrying the stderr.
func (d *Docker) InspectNetwork(ctx context.Context, name string) (NetworkInfo, bool, error) {
	var info NetworkInfo
	out, err := d.run(ctx, "network", "inspect", "--format", "{{json .}}", name)
	if err != nil {
		if isNotFound(err) {
			return info, false, nil
		}
		return info, false, err
	}
	if err := json.Unmarshal(bytes.TrimSpace(out), &info); err != nil {
		return info, true, fmt.Errorf("cannot parse inspect output of network %s: %w", name, err)
	}
	return info, true, nil
}

// LiveEndpointMACs lists the lowercased MACs of the containers currently attached to agents-net (the Containers map of
// `docker network inspect`), without the container named exclude. An absent network has none.
func (d *Docker) LiveEndpointMACs(ctx context.Context, exclude string) ([]string, error) {
	info, found, err := d.InspectNetwork(ctx, config.NetworkName)
	if err != nil || !found {
		return nil, err
	}
	var macs []string
	for _, c := range info.Containers {
		if c.Name != exclude && c.MacAddress != "" {
			macs = append(macs, strings.ToLower(c.MacAddress))
		}
	}
	return macs, nil
}

// ListNetworks inspects every existing network (`network ls -q`, then inspect by id).
func (d *Docker) ListNetworks(ctx context.Context) ([]NetworkInfo, error) {
	out, err := d.run(ctx, "network", "ls", "-q")
	if err != nil {
		return nil, err
	}
	var all []NetworkInfo
	for _, id := range strings.Fields(string(out)) {
		info, found, err := d.InspectNetwork(ctx, id)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("network %s listed but not inspectable", id)
		}
		all = append(all, info)
	}
	return all, nil
}

// CreateNetwork runs the SFR-NET-005 command.
func (d *Docker) CreateNetwork(ctx context.Context) error {
	_, err := d.run(ctx, "network", "create", "--driver", config.NetworkDriver, "--subnet", config.Subnet, config.NetworkName)
	return err
}

// RemoveNetwork deletes agents-net (it must have no attached container).
func (d *Docker) RemoveNetwork(ctx context.Context) error {
	_, err := d.run(ctx, "network", "rm", config.NetworkName)
	return err
}

// RemoveContainer force-removes one container (the workspace on the host is not touched). removed is false when docker
// answers that the container is not there: it is already gone, which is not an error.
func (d *Docker) RemoveContainer(ctx context.Context, name string) (removed bool, err error) {
	// runsc: a paused sandbox cannot be killed, so `rm -f` reports "is zombie and can not be killed"; unpause first.
	out, err := d.run(ctx, "inspect", "-f", "{{.State.Paused}}", name)
	if err != nil && !isNotFound(err) {
		return false, err // a missing container falls through to `rm -f`, which answers not-found as before
	}
	if err == nil && strings.TrimSpace(string(out)) == "true" {
		if _, err := d.run(ctx, "unpause", name); err != nil && !isNotFound(err) {
			return false, fmt.Errorf("unpause before remove: %w", err)
		}
	}
	if _, err := d.run(ctx, "rm", "-f", name); err != nil {
		if isNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// NetworkContainers lists the names of all containers attached to agents-net, stopped ones included. Containers are
// attached at `docker create`, so this also lists created containers that were never started or armed.
func (d *Docker) NetworkContainers(ctx context.Context) ([]string, error) {
	out, err := d.run(ctx, "ps", "-a", "--filter", "network="+config.NetworkName, "--format", "{{.Names}}")
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(out)), nil
}

// LookupContainer is InspectContainer that maps docker's not-found answer to "absent"; any other failure is an error.
func (d *Docker) LookupContainer(ctx context.Context, name string) (ContainerInfo, bool, error) {
	info, err := d.InspectContainer(ctx, name)
	if err != nil {
		if isNotFound(err) {
			return ContainerInfo{}, false, nil
		}
		return ContainerInfo{}, false, err
	}
	return info, true, nil
}

// InspectContainer reads the PID and the network endpoints docker reports for the container.
func (d *Docker) InspectContainer(ctx context.Context, name string) (ContainerInfo, error) {
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
