// Package config holds every tunable value of create_agent_container.
// Fixed design values are constants; the few tunables have a default and an environment override.
package config

import (
	"strings"
	"time"
)

const (
	DockerExe       = "docker"
	ContainerPrefix = "agent-"
	Runtime         = "runsc"
	NetworkName     = "agents-net"
	Image           = "agent-image:latest"
	WorkspaceMount  = "/workspace"
	Memory          = "4g"
	CPUs            = "2"
	PidsLimit       = "512"

	// Bounds the final verification compares against the inspected values (SFR-AGC-009).
	MemoryBytes     = 4294967296
	NanoCPUs        = 2000000000
	PidsLimitNumber = 512

	// tmpfs mounts of SSR-AGC-012: path and options (together 2g + 512m stay below the memory limit).
	TmpfsTmpPath       = "/tmp"
	TmpfsTmpOptions    = "rw,noexec,nosuid,size=512m"
	TmpfsDockerPath    = "/var/lib/docker"
	TmpfsDockerOptions = "rw,nosuid,size=2g"

	// Exact content of the container's /etc/resolv.conf (SFR-AGC-024); path, owner and mode are below.
	ResolvConfContent = "nameserver " + ResolverPrimary + "\nnameserver " + ResolverSecondary + "\n"

	// Agent user inside the container (SSR-AGC-011) and its required group. AgentUser is the uid alone: an
	// explicit group ("1001:1001") makes Docker skip the supplementary groups, so only the uid is given and the
	// gid (1001) and the group docker come from the image's /etc/passwd and /etc/group.
	AgentUser  = "1001"
	AgentUID   = 1001
	AgentGID   = 1001
	AgentGroup = "docker"

	// OutputDir is the workspace subfolder created next to the agent's files (SFR-AGC-004).
	OutputDir = ".output"

	// AgentIDPattern is the SFR-AGC-001 identifier rule.
	AgentIDPattern = `^[a-z][a-z0-9-]{0,30}$`

	// Allowed DNS resolvers, written in this order to the container's /etc/resolv.conf (SFR-NET-020, SFR-AGC-024).
	ResolverPrimary   = "1.1.1.1"
	ResolverSecondary = "9.9.9.9"
	ResolvConfPath    = "/etc/resolv.conf"
	ResolvConfOwner   = "root:root"
	ResolvConfMode    = "644"

	// Owner of the per-agent workspace folder (agent-operator).
	WorkspaceUID = 1001
	WorkspaceGID = 1001

	DefaultWorkspaceRoot = "/home/agent-operator/Workspace-Agents"
	DefaultArmBin        = "/usr/local/bin/create_agent_net"
	DefaultRemoveNetBin  = "/usr/local/bin/remove_agent_net"

	// Ready marker and per-agent locks live in the runtime dir (tmpfs: gone with the VM, like the gate).
	DefaultRuntimeDir = "/run/agent-sandbox"
	ReadyMarkerName   = "ready"
	DockerService     = "docker.service"
	DefaultWait       = 60 * time.Second

	DefaultGuardReconcile = 60 * time.Second

	// Persistent per-agent state: <state root>/<id> (root:root 0711) holds claude/ (1001:1001 0700), which is bind-mounted
	// read-write at AgentHome/.claude. The state survives remove_agent_container unless --purge is given.
	DefaultStateRoot = "/var/lib/agent-sandbox"
	StateClaudeDir   = "claude"
	AgentHome        = "/home/agent"
	ClaudeMount      = AgentHome + "/.claude"
	ClaudeEnvVar     = "CLAUDE_CONFIG_DIR"

	// Host file bind-mounted read-only over the container's /etc/resolv.conf (gVisor does not serve 127.0.0.11).
	DefaultHostResolvPath = "/etc/agent-sandbox/resolv.conf"
)

// Capabilities is the closed allowlist added after --cap-drop ALL (SFR-AGC-006).
var Capabilities = []string{
	"SYS_ADMIN", "SYS_CHROOT", "MKNOD",
	"CHOWN", "DAC_OVERRIDE", "FOWNER", "FSETID",
	"SETUID", "SETGID", "SETPCAP", "SETFCAP",
	"KILL", "AUDIT_WRITE", "NET_BIND_SERVICE", "SYS_RESOURCE",
}

type Config struct {
	LogLevel      string
	WorkspaceRoot string
	ArmBin        string
	RemoveNetBin  string
	RuntimeDir    string
	StateRoot     string
	HostResolv    string
	ReadyTimeout  time.Duration // create waits this long for the ready marker
	DockerWait    time.Duration // restore waits this long for the docker daemon

	// GuardReconcile is the period of the guard's reconcile pass (also run once at guard start): every free agent
	// container is gate-checked and the gate set `released` is cleaned of untagged and orphaned elements.
	GuardReconcile time.Duration
}

// Load builds the configuration from getenv (os.Getenv in production).
func Load(getenv func(string) string) Config {
	return Config{
		LogLevel:      strings.ToUpper(str(getenv, "AGENT_LOG_LEVEL", "INFO")),
		WorkspaceRoot: str(getenv, "AGENT_SANDBOX_WORKSPACE_ROOT", DefaultWorkspaceRoot),
		ArmBin:        str(getenv, "AGENT_SANDBOX_ARM_BIN", DefaultArmBin),
		RemoveNetBin:  str(getenv, "AGENT_SANDBOX_REMOVE_NET_BIN", DefaultRemoveNetBin),
		RuntimeDir:    str(getenv, "AGENT_SANDBOX_RUNTIME_DIR", DefaultRuntimeDir),
		StateRoot:     str(getenv, "AGENT_SANDBOX_STATE_ROOT", DefaultStateRoot),
		HostResolv:    str(getenv, "AGENT_SANDBOX_HOST_RESOLV", DefaultHostResolvPath),
		ReadyTimeout:  dur(getenv, "AGENT_SANDBOX_READY_TIMEOUT", DefaultWait),
		DockerWait:    dur(getenv, "AGENT_SANDBOX_DOCKER_WAIT", DefaultWait),

		GuardReconcile: dur(getenv, "AGENT_SANDBOX_GUARD_RECONCILE", DefaultGuardReconcile),
	}
}

func dur(getenv func(string) string, key string, def time.Duration) time.Duration {
	if v := strings.TrimSpace(getenv(key)); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return def
}

func str(getenv func(string) string, key, def string) string {
	if v := strings.TrimSpace(getenv(key)); v != "" {
		return v
	}
	return def
}
