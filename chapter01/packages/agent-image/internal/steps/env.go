// Shared step environment and failure type. Steps depend on services, never the reverse.
package steps

import (
	"bufio"
	"fmt"

	"agent-image/internal/config"
	"agent-image/internal/services"
)

// Exit codes of IP-CLI-CREATE-AGENT-IMAGE.
const (
	ExitPrerequisite = 1   // steps 1-5 (build failures included)
	ExitRuntime      = 2   // step 6
	ExitInterrupted  = 130 // SFR-IMG-018, any step
)

// Exit states from the USC-003 exception flows.
const (
	StateNotCreated  = "NOT_CREATED"
	StateUnverified  = "UNVERIFIED"
	StateInterrupted = "INTERRUPTED"
)

// Env carries the injected dependencies and the data handed from step to step.
type Env struct {
	Config config.Config
	Host   *services.Docker // docker engine in agent-host (check, build, inspect)
	Log    *services.Renderer
	In     *bufio.Reader // console stdin for the overwrite prompt
	Tag    string
	Exists bool   // agent-image:<tag> already exists in agent-host, set by step 3
	Image  string // agent-image:<tag>, set by step 1

	ContextDir string // Windows directory (cwd) holding the build files, set by step 1
}

// Failure is returned by a step that did not succeed; nil means success.
// ExitCode 0 is a normal early stop (declined overwrite): Message goes to stdout.
type Failure struct {
	ExitCode  int
	ExitState string
	Message   string
}

// fail builds a Failure whose message is "<what>: <err>"; command errors already carry stderr.
func fail(code int, state, what string, err error) *Failure {
	return &Failure{ExitCode: code, ExitState: state, Message: fmt.Sprintf("%s: %v", what, err)}
}
