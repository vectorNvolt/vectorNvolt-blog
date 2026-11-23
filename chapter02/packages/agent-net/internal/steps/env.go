// Shared step environment and failure type. Steps depend on services, never the reverse.
package steps

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strconv"
	"strings"

	"agent-net/internal/config"
	"agent-net/internal/services"
)

// Exit codes of IP-CLI-CREATE-AGENT-NET (also used by `arm`).
const (
	ExitPrerequisite = 1
	ExitRuntime      = 2
)

// Exit states from the USC-004 exception flows.
const (
	StateNotCreated        = "NOT_CREATED"
	StateAlreadyExists     = "ALREADY_EXISTS"
	StateCreateFailed      = "CREATE_FAILED"
	StateCreatedUnverified = "CREATED_UNVERIFIED"
	StateGatedUnarmed      = "GATED_UNARMED"
)

// Exit states of `disarm` (USC-004 alt-2), `check-gate` (alt-3) and remove_agent_net (USC-007).
const (
	StateNotDisarmed      = "NOT_DISARMED"      // a precondition failed; nothing was changed
	StateDisarmIncomplete = "DISARM_INCOMPLETE" // the table or rules of the port may be left; run disarm again
	StateNotChecked       = "NOT_CHECKED"       // check-gate: nothing was checked (invalid invocation, unknown container, missing privilege)
	StateGateInconsistent = "GATE_INCONSISTENT" // check-gate: a control is missing or mismatched, or a read-back failed
	StateNotRemoved       = "NOT_REMOVED"       // a precondition failed; nothing was changed
	StateRemoveIncomplete = "REMOVE_INCOMPLETE" // part of the setup is left; run remove_agent_net again
	StateNotEvicted       = "NOT_EVICTED"       // evict: a precondition failed; nothing was changed
	StateEvictIncomplete  = "EVICT_INCOMPLETE"  // evict: the element may still be in the set; run it again
)

// Invocation modes (SFR-NET-001).
const (
	ModeCreate    = "create"
	ModeArm       = "arm"
	ModeCheckGate = "check-gate" // read-only report of one container's port controls
	ModeDisarm    = "disarm"     // delete the port table and the other port rules of one container
	ModeRemove    = "remove"     // remove_agent_net: containers, leftover port tables and rules, network
	ModeRefresh   = "refresh"    // refresh_endpoints: re-resolve the endpoints, update the armed containers' accept rules
	ModeReleased  = "released"   // read-only list of the elements of the gate set `released`
	ModeEvict     = "evict"      // remove one element of the gate set `released` by MAC
)

// Exit states of refresh_endpoints.
const (
	StateNotRefreshed      = "NOT_REFRESHED"      // a precondition failed; nothing was changed
	StateRefreshIncomplete = "REFRESH_INCOMPLETE" // a rule change failed; run refresh_endpoints again
)

// EngineState is what SSR-NET-001 compares before and after the run.
type EngineState struct {
	DaemonHash string // SHA-256 of daemon.json, or "absent"
	Identity   string // dockerd pid and process start time
}

// Port is the host-side attachment of the armed container (SSR-NET-002).
type Port struct {
	PID  int
	IP   string
	MAC  string
	Veth string
}

// Allowed are the destinations the container may reach besides the two fixed DNS resolvers: api.anthropic.com and
// platform.claude.com on TCP 443, resolved host-side at arm time.
type Allowed struct {
	ModelIPs []string
}

// Env carries the injected dependencies and the data handed from step to step.
type Env struct {
	Config    config.Config
	Docker    *services.Docker
	Net       *services.Net
	Host      services.Host
	Log       *services.Renderer
	Args      []string
	Mode      string // set by step 1
	Container string // arm, disarm and check-gate, set by step 1
	EvictMAC  string // evict: the MAC to take out of `released` (lower case), set by step 1
	Before    EngineState
	Network   services.NetworkInfo   // agents-net as inspected
	Bridge    string                 // host bridge device of agents-net
	Inspected services.ContainerInfo // arm: the container as inspected (IdentifyPort)
	Port      Port
	Allowed   Allowed
	Disarm    Disarm   // disarm and remove: the port being closed, set per container
	Agents    []string // remove: agent containers attached to agents-net
	Tables    []string // remove: as_port_* tables found by the survey
	Left      []string // remove: failures of steps 6-8 (the later steps still run); reported by the verification
	DryRun    bool     // refresh: print the plan, change nothing
	Finished  bool     // a step has reported the final outcome (nothing to do); the sequence stops with exit 0

	Stdin         io.Reader // refresh: the endpoints JSON arrives here
	EndpointsData []byte    // refresh: the stdin bytes as validated (what gets installed)

	Removed        []string // remove: the agent containers `docker rm -f` actually removed (not those already gone)
	NetworkPresent bool     // remove: the survey found agents-net (a read failure stops the run, it is never "absent")
}

// Disarm is what is known about the port being disarmed (Env.Container is the container).
type Disarm struct {
	Missing bool     // the container does not exist
	Pid     int      // 0 when it is not running
	MACs    []string // lower case; as docker reports them (empty for a stopped container)
	IPs     []string // from docker and from the container's tagged iptables rules
	Veths   []string // the live veth and the veth the container's port table is bound to
	Live    string   // the live veth of a running container ("" when it was not identified)
	Table   string   // the veth the container's port table is bound to ("" when there is no table)
	Deleted int      // tables, rules and entries removed
}

// PortTable is the name of the container's per-port table, table netdev as_port_<id>.
func (e *Env) PortTable() string { return config.PortTable(e.Container) }

// Failure is returned by a step that did not succeed; nil means success.
// Control names the failed control for the arming steps (SFR-NET-011).
type Failure struct {
	ExitCode  int
	ExitState string
	Message   string
	Control   string
	Checks    []string
}

// fail builds a Failure whose message is "<what>: <err>"; command errors already carry stderr.
func fail(code int, state, what string, err error) *Failure {
	return &Failure{ExitCode: code, ExitState: state, Message: fmt.Sprintf("%s: %v", what, err)}
}

// failControl builds the arming failure that names the control; the container stays gated.
func failControl(control, what string, err error) *Failure {
	msg := what
	if err != nil {
		msg = fmt.Sprintf("%s: %v", what, err)
	}
	return &Failure{ExitCode: ExitRuntime, ExitState: StateGatedUnarmed, Control: control, Message: msg}
}

// notDone is the exit state of a precondition failure in the current mode.
func notDone(env *Env) string {
	switch env.Mode {
	case ModeArm:
		return StateGatedUnarmed
	case ModeDisarm:
		return StateNotDisarmed
	case ModeCheckGate:
		return StateNotChecked
	case ModeRemove:
		return StateNotRemoved
	case ModeRefresh:
		return StateNotRefreshed
	case ModeReleased:
		return StateNotChecked
	case ModeEvict:
		return StateNotEvicted
	}
	return StateNotCreated
}

// incomplete is the exit state of a runtime failure while closing a port: the disarm steps also run inside remove_agent_net.
func incomplete(env *Env) string {
	if env.Mode == ModeRemove {
		return StateRemoveIncomplete
	}
	return StateDisarmIncomplete
}

// snapshotEngine records the SSR-NET-001 state: daemon.json hash (absent is a state) and the engine
// start identity. The identity is the dockerd pid from its pid file plus that process' start time
// (field 22 of /proc/<pid>/stat); a restart changes it. A SIGHUP reload keeps it (see README).
func snapshotEngine(env *Env) (EngineState, error) {
	var st EngineState
	data, err := env.Host.ReadFile(env.Config.DaemonJSON)
	switch {
	case err == nil:
		sum := sha256.Sum256(data)
		st.DaemonHash = hex.EncodeToString(sum[:])
	case errors.Is(err, fs.ErrNotExist):
		st.DaemonHash = "absent"
	default:
		return st, fmt.Errorf("cannot read %s: %w", env.Config.DaemonJSON, err)
	}

	raw, err := env.Host.ReadFile(env.Config.DockerPIDFile)
	if err != nil {
		return st, fmt.Errorf("cannot read docker pid file %s: %w", env.Config.DockerPIDFile, err)
	}
	pid := strings.TrimSpace(string(raw))
	if _, err := strconv.Atoi(pid); err != nil {
		return st, fmt.Errorf("docker pid file %s holds %q, not a pid", env.Config.DockerPIDFile, pid)
	}
	stat, err := env.Host.ReadFile("/proc/" + pid + "/stat")
	if err != nil {
		return st, fmt.Errorf("cannot read start time of dockerd pid %s: %w", pid, err)
	}
	// Fields after the ")" that closes the command name start at field 3; start time is field 22.
	s := string(stat)
	i := strings.LastIndex(s, ")")
	if i < 0 {
		return st, fmt.Errorf("unexpected /proc/%s/stat format", pid)
	}
	fields := strings.Fields(s[i+1:])
	if len(fields) < 20 {
		return st, fmt.Errorf("unexpected /proc/%s/stat format", pid)
	}
	st.Identity = "pid " + pid + " start " + fields[19]
	return st, nil
}
