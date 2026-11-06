// Shared step environment and failure type. Steps depend on services, never the reverse.
package steps

import (
	"fmt"

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

// Env carries the injected dependencies and the data handed from step to step.
type Env struct {
	Config     config.Config
	WSL        *services.WSL
	Rootfs     *services.RootfsProvider
	Log        *services.Renderer
	Starter    services.Starter
	RootfsPath string // set by the rootfs step, consumed by the register step

	// Set by the reference-hash step (USC-001 step 3), consumed by the tool install step (11) and verify (12).
	ToolBuilds map[string][]byte // tool name -> Linux build bytes
	ToolHashes map[string]string // tool name -> lowercase hex SHA-256 computed on the host

	EndpointsJSON []byte // validated endpoints.json from the tools folder (step 3), sent to refresh_endpoints in step 11
}

// Failure is returned by a step that did not succeed; nil means success.
type Failure struct {
	ExitCode  int
	ExitState string
	Message   string
	Checks    []string
}

// fail builds a Failure whose message is "<what>: <err>"; WSL errors already carry stderr.
func fail(code int, state, what string, err error) *Failure {
	return &Failure{ExitCode: code, ExitState: state, Message: fmt.Sprintf("%s: %v", what, err)}
}
