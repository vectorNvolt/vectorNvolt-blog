// Package config holds every tunable value of create_agent_host.
// Each value has a default and an environment override; nothing else reads the environment.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	HostName = "agent-host" // hostname set on the VM (SFR-HOST-015)
	User     = "agent-operator"
	UserID   = 1001 // UID and GID of User; agent-sandbox chowns workspaces to 1001:1001

	// FoundationMarkerPath is written by vm/foundation.sh; create_agent_host refuses to run without it.
	FoundationMarkerPath = "/etc/agent-foundation"
	// FoundationVersionLine is the only marker content this build accepts.
	FoundationVersionLine = "FOUNDATION_VERSION=1"
	// FoundationUbuntuLine is the marker line that pins the foundation OS release.
	FoundationUbuntuLine = "UBUNTU=26.04"
	// DoneMarkerPath is written as the last step; its presence makes a rerun refuse (no repair on rerun).
	DoneMarkerPath    = "/etc/agent-host"
	DoneMarkerContent = "AGENT_HOST_VERSION=1\n"
	// CheckFoundationScript is named in the preflight failure message.
	CheckFoundationScript = "vm/check-foundation.sh"
)

// DefaultGVisorKeyFingerprint is the pinned fingerprint (40 hex chars) of the gVisor apt signing key.
// Primary key fingerprint of https://gvisor.dev/archive.key, pinned trust-on-first-use on 2026-10-30
// (corroborated by an identical key in github.com/google/gvisor; no official doc publishes it).
// AGENT_HOST_GVISOR_KEY_FINGERPRINT still overrides.
const DefaultGVisorKeyFingerprint = "6F1DF85E3A71C24918E727D56FC6D554E32BD943"

// Agent tools (SFR-HOST-013 / SFR-HOST-011): Linux builds read from the folder the executable runs from.
var ToolNames = []string{"create_agent_container", "remove_agent_container", "create_agent_net", "remove_agent_net", "agent_lifecycle", "enter_agent", "refresh_endpoints"}

// Model endpoints file (SFR-HOST-001 / SFR-HOST-011): read from the tools folder, sent to refresh_endpoints on stdin.
const (
	EndpointsFileName    = "endpoints.json"
	EndpointsMaxBytes    = 64 * 1024 // refresh_endpoints refuses more
	EndpointsInstallPath = "/etc/agent-net/endpoints.json"
)

// Paths on the host.
const (
	ToolInstallDir = "/usr/local/bin"
	RunscPath      = "/usr/local/bin/runsc"
	RunscPkgPath   = "/usr/bin/runsc" // where the gVisor apt package installs the binary
	DaemonJSONPath = "/etc/docker/daemon.json"
	HostsPath      = "/etc/hosts"
	CloudCfgDir    = "/etc/cloud/cloud.cfg.d"
	// PreserveHostnameCfgPath keeps cloud-init from resetting the hostname at the next boot.
	PreserveHostnameCfgPath = CloudCfgDir + "/99-agent-host-hostname.cfg"
	// NoDockerUpgradeCfgPath keeps unattended-upgrades from upgrading (and restarting) Docker and its runtime.
	NoDockerUpgradeCfgPath = "/etc/apt/apt.conf.d/51agent-host-no-docker-upgrade"
	// BrNetfilterModulesPath loads br_netfilter at boot (stock cloud images do not load it); agent-net only reads the sysctl.
	BrNetfilterModulesPath    = "/etc/modules-load.d/agent-host.conf"
	BrNetfilterModulesContent = "br_netfilter\n"
	// BridgeSysctlPath sets bridge-nf-call-iptables/ip6tables to 1 (systemd-modules-load runs before systemd-sysctl).
	BridgeSysctlPath    = "/etc/sysctl.d/90-agent-host-bridge.conf"
	BridgeSysctlContent = "net.bridge.bridge-nf-call-iptables = 1\nnet.bridge.bridge-nf-call-ip6tables = 1\n"
	// BridgeNfCallIptablesProc is the live value agent-net requires to be 1.
	BridgeNfCallIptablesProc = "/proc/sys/net/bridge/bridge-nf-call-iptables"
	BrNetfilterSysModuleDir  = "/sys/module/br_netfilter"
)

// RunscRuntimeArgs are the exactly eight runtimeArgs of runsc, in order (SSR-HOST-006).
var RunscRuntimeArgs = []string{
	"--platform=systrap",
	"--network=sandbox",
	"--net-raw=false",
	"--allow-flag-override=false",
	"--overlay2=root:memory",
	"--file-access=exclusive",
	"--file-access-mounts=shared",
	"--directfs=false",
}

// DaemonJSON renders /etc/docker/daemon.json deterministically (map keys are sorted by encoding/json).
func DaemonJSON() []byte {
	doc := map[string]any{
		"runtimes": map[string]any{
			"runsc": map[string]any{
				"path":        RunscPath,
				"runtimeArgs": RunscRuntimeArgs,
			},
		},
	}
	b, _ := json.MarshalIndent(doc, "", "  ")
	return append(b, '\n')
}

type Config struct {
	ToolsDir             string // folder holding the tool builds (IP-DIST-ARTIFACT)
	GVisorKeyFingerprint string
	LogLevel             string
}

// ParseArgs parses the command line (without the program name). There are no flags; any argument is an error.
func ParseArgs(args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("unknown argument %q", args[0])
	}
	return nil
}

// Load builds the configuration from getenv (os.Getenv in production).
func Load(getenv func(string) string) Config {
	return Config{
		LogLevel:             strings.ToUpper(str(getenv, "AGENT_LOG_LEVEL", "INFO")),
		ToolsDir:             str(getenv, "AGENT_HOST_TOOLS_DIR", defaultToolsDir()),
		GVisorKeyFingerprint: str(getenv, "AGENT_HOST_GVISOR_KEY_FINGERPRINT", DefaultGVisorKeyFingerprint),
	}
}

// defaultToolsDir is the directory of the running executable (same folder as create_agent_host).
func defaultToolsDir() string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Dir(exe)
	}
	return "."
}

func str(getenv func(string) string, key, def string) string {
	if v := strings.TrimSpace(getenv(key)); v != "" {
		return v
	}
	return def
}
