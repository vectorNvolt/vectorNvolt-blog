// Package config holds every tunable value of create_agent_host.
// Each value has a default and an environment override; nothing else reads the environment.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	DistroName = "agent-host"
	User       = "agent-operator"
	UserID     = 1001     // UID and GID of User; agent-sandbox chowns workspaces to 1001:1001
	ImageUser  = "ubuntu" // earlier Canonical WSL images shipped this user (GID 1001); the 26.04 image ships none, so removal is conditional
	WSLExe     = "wsl.exe"

	MinWSLVersion = "2.4.10" // minimum WSL release (wsl.exe --version) for the distro import (SFR-HOST-004)

	DefaultRootfsURL = "https://releases.ubuntu.com/26.04.1/ubuntu-26.04.1-wsl-amd64.wsl"
	UserAgent        = "create-agent-host/1.0"

	// UbuntuCDSigningKeyFingerprint pins the Ubuntu CD image signing key (RSA 4096) that signs SHA256SUMS (SFR-HOST-003).
	// The key itself is embedded in internal/services/ubuntu-cdimage-signing-key.asc.
	UbuntuCDSigningKeyFingerprint = "843938DF228D22F7B3742BC0D94AA3F0EFE21092"
	// SHA256SumsSignatureSuffix is appended to the SHA256SUMS URL to get the detached signature URL.
	SHA256SumsSignatureSuffix = ".gpg"
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

// Paths inside agent-host.
const (
	ToolInstallDir = "/usr/local/bin"
	RunscPath      = "/usr/local/bin/runsc"
	RunscPkgPath   = "/usr/bin/runsc" // where the gVisor apt package installs the binary
	DaemonJSONPath = "/etc/docker/daemon.json"
	WSLConfPath    = "/etc/wsl.conf"
	ResolvConfPath = "/etc/resolv.conf"
	HostsPath      = "/etc/hosts"
	TmpfilesDir    = "/etc/tmpfiles.d"
	SystemUnitDir  = "/etc/systemd/system"
)

// ResolvedMasks are symlinks to /dev/null that stop systemd-resolved from replacing /etc/resolv.conf at boot.
// The tmpfiles entry has the same file name as the packaged rule (/usr/lib/tmpfiles.d/systemd-resolve.conf), which masks it.
var ResolvedMasks = []string{
	SystemUnitDir + "/systemd-resolved.service",
	SystemUnitDir + "/systemd-resolved-monitor.socket",
	SystemUnitDir + "/systemd-resolved-varlink.socket",
	TmpfilesDir + "/systemd-resolve.conf",
}

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
	KeepAlive            bool   // keep-alive wsl.exe at Windows logon (default true; main clears it for --no-keepalive)
	AppData              string // %APPDATA%; the keep-alive file goes in its Startup folder
	ToolsDir             string // folder holding the tool builds (IP-DIST-ARTIFACT)
	GVisorKeyFingerprint string
	LogLevel             string
	RootfsURL            string
	SHA256SumsURL        string
	InstallDir           string
	DownloadTimeout      time.Duration
	DownloadAttempt      int
	Backoff              time.Duration
}

// ParseArgs parses the command line (without the program name). The only flag is --no-keepalive;
// anything else is an error.
func ParseArgs(args []string) (noKeepAlive bool, err error) {
	for _, a := range args {
		if a != "--no-keepalive" {
			return false, fmt.Errorf("unknown argument %q", a)
		}
		noKeepAlive = true
	}
	return noKeepAlive, nil
}

// Load builds the configuration from getenv (os.Getenv in production).
func Load(getenv func(string) string) Config {
	c := Config{
		KeepAlive:       true,
		AppData:         strings.TrimSpace(getenv("APPDATA")),
		LogLevel:        strings.ToUpper(str(getenv, "AGENT_LOG_LEVEL", "INFO")),
		RootfsURL:       str(getenv, "AGENT_HOST_ROOTFS_URL", DefaultRootfsURL),
		DownloadTimeout: time.Duration(num(getenv, "AGENT_HOST_DOWNLOAD_TIMEOUT_SECONDS", 300)) * time.Second,
		DownloadAttempt: num(getenv, "AGENT_HOST_DOWNLOAD_ATTEMPTS", 3),
		Backoff:         time.Duration(num(getenv, "AGENT_HOST_DOWNLOAD_BACKOFF_MS", 2000)) * time.Millisecond,
	}
	c.SHA256SumsURL = str(getenv, "AGENT_HOST_ROOTFS_SHA256SUMS_URL", sumsURLFor(c.RootfsURL))
	c.InstallDir = str(getenv, "AGENT_HOST_INSTALL_DIR", defaultInstallDir(getenv))
	c.ToolsDir = str(getenv, "AGENT_HOST_TOOLS_DIR", defaultToolsDir())
	c.GVisorKeyFingerprint = str(getenv, "AGENT_HOST_GVISOR_KEY_FINGERPRINT", DefaultGVisorKeyFingerprint)
	return c
}

// defaultToolsDir is the directory of the running executable (same folder as create_agent_host.exe).
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

func num(getenv func(string) string, key string, def int) int {
	v, err := strconv.Atoi(strings.TrimSpace(getenv(key)))
	if err != nil || v <= 0 {
		return def
	}
	return v
}

// sumsURLFor returns <dir of rootfsURL>/SHA256SUMS.
func sumsURLFor(rootfsURL string) string {
	i := strings.LastIndex(rootfsURL, "/")
	if i < 0 {
		return path.Join(rootfsURL, "SHA256SUMS")
	}
	return rootfsURL[:i] + "/SHA256SUMS"
}

func defaultInstallDir(getenv func(string) string) string {
	if local := strings.TrimSpace(getenv("LOCALAPPDATA")); local != "" {
		return filepath.Join(local, DistroName)
	}
	if cache, err := os.UserCacheDir(); err == nil && cache != "" {
		return filepath.Join(cache, DistroName)
	}
	return filepath.Join(os.TempDir(), DistroName)
}
