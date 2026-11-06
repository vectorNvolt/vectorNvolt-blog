// Package config holds every tunable value of create_agent_net.
// Fixed design values are constants; the few tunables have a default and an environment override.
package config

import (
	"errors"
	"io/fs"
	"os"
	"strings"
)

const (
	DockerExe   = "docker"
	IPExe       = "ip"
	BridgeExe   = "bridge"
	NftExe      = "nft"
	IptablesExe = "iptables"
	NsenterExe  = "nsenter"

	NetworkName   = "agents-net"
	NetworkDriver = "bridge"
	Subnet        = "172.30.0.0/24"

	// Per-port nftables table (USS-004): `table netdev as_port_<id>` with an ingress
	// and an egress chain bound to the container's veth (the WSL2 kernel has no nf_tables bridge family).
	NftFamily    = "netdev"
	PortTablePfx = "as_port_"
	ChainIngress = "ingress" // frames the agent sends, before the bridge sees them
	ChainEgress  = "egress"  // frames sent to the agent
	AgentNamePfx = "agent-"  // container names made by create_agent_container are agent-<id>
	// SandboxRuntime is the docker runtime create_agent_container gives every agent (its config.Runtime); arm and
	// check-gate refuse a container that reports another one.
	SandboxRuntime = "runsc"

	BroadcastMAC = "ff:ff:ff:ff:ff:ff"

	// Network-wide gate, installed by create_agent_net: `table netdev agents_gate` on the agents-net bridge device.
	// Both chains (ingress and egress) have policy drop; only frames from / to a MAC in the set `released` pass. A
	// container is attached to agents-net at `docker create`, so this gate keeps its port closed until `arm` adds its MAC.
	GateTable = "agents_gate"
	GateSet   = "released"
	// GateComment tags the DOCKER-USER rule that drops port-to-port (bridged) traffic on agents-net.
	GateComment = CommentPrefix + "gate"
	// PortMark is the one packet mark bit (bit 28) that ties a frame to its per-port table. The accept rules of the port
	// table (ingress chain on the veth) set it; the gate ingress rule on the bridge requires it and clears it again, so it
	// never reaches host routing or iptables. A restarted container gets a new veth without a port table: its frames are
	// unmarked and the gate drops them even if its MAC is still in `released`. The bit is in the high half of the mark,
	// away from the low bits used by Docker (swarm ingress 0x100), kube-proxy (0x4000/0x8000), firewalld and Cilium
	// defaults; nothing on the agent-host uses it. (Calico's default mask 0xffff0000 would cover it; Calico is not used.)
	// The other bits of the mark are preserved. Probed on 6.6.87.2-microsoft-standard-WSL2 with nft 1.0.9: a mark set at
	// the veth's netdev ingress is still present at the bridge's netdev ingress.
	PortMark    = "0x10000000"
	PortMarkInv = "0xefffffff" // ^PortMark in 32 bits: the mask that clears the bit

	// BrNfCallIptables must read 1 so that bridged frames traverse iptables (read only, never written).
	BrNfCallIptables = "/proc/sys/net/bridge/bridge-nf-call-iptables"

	// The two fixed DNS resolvers (SFR-NET-009 DNS rule (a)): primary and secondary.
	ResolverPrimary   = "1.1.1.1"
	ResolverSecondary = "9.9.9.9"

	ChainDockerUser = "DOCKER-USER"
	ChainInput      = "INPUT"

	// CommentPrefix tags every iptables rule the arming unit adds.
	CommentPrefix = "agent-net:"

	// ContainerPattern is the docker container name rule accepted by `arm`.
	ContainerPattern = `^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`

	// AgentContainerPattern is the name of a container made by create_agent_container (`agent-<id>`; the id rule is
	// SFR-AGC-001 of agent-sandbox, copied here). remove_agent_net removes only containers with such a name.
	AgentContainerPattern = `^agent-[a-z][a-z0-9-]{0,30}$`

	// CapNetAdmin is the bit of CAP_NET_ADMIN in CapEff of /proc/self/status.
	CapNetAdmin = 12
	ProcStatus  = "/proc/self/status"

	// Network namespace of this process and of the agent host's init (pid 1): they must be the same (SFR-NET-002).
	ProcSelfNetNS = "/proc/self/ns/net"
	ProcInitNetNS = "/proc/1/ns/net"

	DNSPort      = "53"
	ModelAPIPort = "443"

	DefaultDockerSocket  = "/var/run/docker.sock"
	DefaultDaemonJSON    = "/etc/docker/daemon.json"
	DefaultDockerPIDFile = "/var/run/docker.pid"
	DefaultModelAPIs     = "api.anthropic.com,platform.claude.com" // SFR-NET-009
)

// PortID is the <id> of table as_port_<id>: the container name without the agent- prefix.
func PortID(container string) string { return strings.TrimPrefix(container, AgentNamePfx) }

// PortTable is the name of the container's per-port table.
func PortTable(container string) string { return PortTablePfx + PortID(container) }

type Config struct {
	LogLevel      string
	DockerSocket  string
	DaemonJSON    string
	DockerPIDFile string
	ModelAPIs     []string // host names or IPv4 literals, port 443

	EndpointsFile string // path of the endpoints file that was looked at
	EndpointsErr  error  // the endpoints file exists (or was named) but is unusable: arm and check-gate fail on it
}

// Load builds the configuration from getenv (os.Getenv in production).
func Load(getenv func(string) string) Config {
	return LoadWith(getenv, os.ReadFile, StatFile)
}

// LoadWith is Load with the file access injected.
func LoadWith(getenv func(string) string, read func(string) ([]byte, error), stat func(string) (int, fs.FileMode, error)) Config {
	c := Config{
		LogLevel:      strings.ToUpper(str(getenv, "AGENT_LOG_LEVEL", "INFO")),
		DockerSocket:  str(getenv, "AGENT_NET_DOCKER_SOCKET", DefaultDockerSocket),
		DaemonJSON:    str(getenv, "AGENT_NET_DAEMON_JSON", DefaultDaemonJSON),
		DockerPIDFile: str(getenv, "AGENT_NET_DOCKER_PID_FILE", DefaultDockerPIDFile),
		// SFR-NET-009 names the two endpoints; they stay the default. An endpoints file (DefaultEndpointsFile or
		// AGENT_NET_ENDPOINTS_FILE), maintained with refresh_endpoints, replaces them when it exists.
		ModelAPIs:     list(DefaultModelAPIs),
		EndpointsFile: str(getenv, EnvEndpointsFile, DefaultEndpointsFile),
	}
	explicit := strings.TrimSpace(getenv(EnvEndpointsFile)) != ""
	apis, err := ReadEndpointsFile(c.EndpointsFile, read, stat)
	switch {
	case err == nil:
		c.ModelAPIs = apis
	case errors.Is(err, fs.ErrNotExist) && !explicit:
		// no file: the defaults stay
	default:
		c.EndpointsErr = err
	}
	return c
}

// list splits a comma separated value, dropping blanks.
func list(s string) []string {
	var out []string
	for _, v := range strings.Split(s, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func str(getenv func(string) string, key, def string) string {
	if v := strings.TrimSpace(getenv(key)); v != "" {
		return v
	}
	return def
}
