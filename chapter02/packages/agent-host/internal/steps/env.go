// Shared step environment and failure type. Steps depend on services, never the reverse.
package steps

import (
	"fmt"
	"os"
	"runtime"

	"agent-host/internal/config"
	"agent-host/internal/services"
)

// Exit codes of IP-CLI-CREATE-AGENT-HOST.
const (
	ExitPrerequisite = 1
	ExitRuntime      = 2
)

// Exit states from the USC-001 exception flows.
const (
	StateNotCreated          = "NOT_CREATED"
	StatePartiallyConfigured = "PARTIALLY_CONFIGURED"
	StateUnverified          = "UNVERIFIED"
)

// System is what the foundation preflight inspects; tests replace it so they run unprivileged.
type System struct {
	GOOS       string
	Euid       int
	MarkerPath string

	MountsPath     string // /proc/mounts: host mounts are refused
	PasswdPath     string // /etc/passwd: rerun refusal (user / UID taken)
	GroupPath      string // /etc/group: rerun refusal (GID taken)
	DoneMarkerPath string // written at the end of a successful run
}

// RealSystem describes the machine the process runs on.
func RealSystem() System {
	return System{GOOS: runtime.GOOS, Euid: os.Geteuid(), MarkerPath: config.FoundationMarkerPath,
		MountsPath: "/proc/mounts", PasswdPath: "/etc/passwd", GroupPath: "/etc/group", DoneMarkerPath: config.DoneMarkerPath}
}

// Env carries the injected dependencies and the data handed from step to step.
type Env struct {
	Config config.Config
	Host   *services.Host
	Log    *services.Renderer
	System System

	// Set by the reference-hash step (USC-001 step 2), consumed by the tool install step (7) and verify (8).
	ToolBuilds map[string][]byte // tool name -> Linux build bytes
	ToolHashes map[string]string // tool name -> lowercase hex SHA-256 computed on the host

	EndpointsJSON []byte // validated endpoints.json from the tools folder (step 2), sent to refresh_endpoints in step 7
}

// Failure is returned by a step that did not succeed; nil means success.
type Failure struct {
	ExitCode  int
	ExitState string
	Message   string
	Checks    []string
}

// fail builds a Failure whose message is "<what>: <err>"; host errors already carry stderr.
func fail(code int, state, what string, err error) *Failure {
	return &Failure{ExitCode: code, ExitState: state, Message: fmt.Sprintf("%s: %v", what, err)}
}
