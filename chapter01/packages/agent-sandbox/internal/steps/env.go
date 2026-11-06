// Shared step environment and failure type. Steps depend on services, never the reverse.
package steps

import (
	"context"
	"fmt"

	"agent-sandbox/internal/config"
	"agent-sandbox/internal/services"
)

// Exit codes of IP-CLI-CREATE-AGENT-CONTAINER.
const (
	ExitPrerequisite = 1
	ExitRuntime      = 2
	// ExitNetNotReady: the ready marker written by `agent_lifecycle restore` is missing or stale.
	ExitNetNotReady = 3
)

// Exit states from the USC-002 exception flows.
const (
	// The agents-net gate was not (re)built since the last docker start; see agent-restore.service.
	StateNetNotReady = "NET_NOT_READY"
	StateNotCreated  = "NOT_CREATED"
	StateRemoved     = "REMOVED"
	StateUnverified  = "UNVERIFIED"
	// Cleanup of our own container failed; it may still exist.
	StateCreatedCleanupFailed = "CREATED_CLEANUP_FAILED"
)

// Containment outcomes of a failed /etc/resolv.conf write or read-back (SFR-AGC-009, SFR-AGC-024).
const (
	StateContained = "CONTAINED" // re-paused and verified paused; not removed
	StateUnsafe    = "UNSAFE"    // could not be paused and verified; possibly running
)

// Env carries the injected dependencies and the data handed from step to step.
type Env struct {
	Config    config.Config
	Docker    *services.Docker
	Arm       *services.Arm
	FS        services.FS
	Log       *services.Renderer
	AgentID   string
	Container string // agent-<id>, set by step 1
	Workspace string // <workspace root>/<id>, set by step 1
	StateDir  string // <state root>/<id>, set by step 1
	ClaudeDir string // <state root>/<id>/claude, set by step 1

	// Runner is the raw process runner (systemctl); Unlock frees the per-agent lock taken by AcquireLock.
	Runner services.Runner
	Unlock func()

	// remove_agent_container only (USC-006).
	Args        []string // command-line arguments after the program name
	Purge       bool     // --purge-workspace, set by its step 1
	PurgeState  bool     // --purge: also delete the agent state directory, set by its step 1
	Absent      bool     // container agent-<id> does not exist, set by its step 3
	Purged      bool     // the workspace folder was deleted, set by its step 7
	StatePurged bool     // the state directory was deleted
	// TolerantDisarm continues with the removal when the disarm fails (agent_lifecycle restore: the gate is rebuilt next).
	TolerantDisarm bool
	// LockHeld: the caller already holds the per-agent lock (the guard keeps it from its check to the removal).
	LockHeld bool

	// agent_lifecycle restore only.
	IsRoot      bool
	DockerPID   string                                                         // dockerd MainPID seen when the daemon answered
	RemoveAgent func(ctx context.Context, id string) (code int, detail string) // runs the remove path for one agent
}

// Exit states of USC-006 (remove_agent_container).
const (
	// Nothing was removed; the container, if any, is untouched and still behind the gate.
	StateNotRemoved = "NOT_REMOVED"
	// No container agent-<id> existed; nothing to remove (exit 0).
	StateNotPresent = "NOT_PRESENT"
	// The port was not closed; the container was left in place.
	StateDisarmFailed = "DISARM_FAILED"
	// The container was disarmed but may still exist.
	StateRemoveFailed = "REMOVE_FAILED"
	// The container is gone but the requested workspace purge did not complete.
	StateWorkspacePurgeFailed = "WORKSPACE_PURGE_FAILED"
	// The container is gone but the requested --purge of the agent state did not complete.
	StateStatePurgeFailed = "STATE_PURGE_FAILED"
)

// Exit states of agent_lifecycle restore.
const (
	StateRestored      = "RESTORED"
	StateNotRestored   = "NOT_RESTORED"   // a precondition failed; nothing was changed
	StateRestoreFailed = "RESTORE_FAILED" // part of the rebuild ran; no ready marker is written
)

// Failure is returned by a step that did not succeed; nil means success.
type Failure struct {
	ExitCode  int
	ExitState string
	Message   string
	Checks    []string
}

// fail builds a Failure whose message is "<what>: <err>"; command errors already carry stderr.
func fail(code int, state, what string, err error) *Failure {
	return &Failure{ExitCode: code, ExitState: state, Message: fmt.Sprintf("%s: %v", what, err)}
}

// containFailure re-pauses the container and verifies the paused state; it never removes it.
// It returns the exit state and a sentence for the failure message.
func containFailure(ctx context.Context, env *Env) (string, string) {
	ctx = context.WithoutCancel(ctx)
	if err := env.Docker.Pause(ctx, env.Container); err != nil {
		return StateUnsafe, fmt.Sprintf("UNSAFE: %s could not be paused and may be running: %v", env.Container, err)
	}
	info, err := env.Docker.Inspect(ctx, env.Container)
	if err != nil {
		return StateUnsafe, fmt.Sprintf("UNSAFE: paused state of %s could not be verified, it may be running: %v", env.Container, err)
	}
	if !info.State.Paused {
		return StateUnsafe, fmt.Sprintf("UNSAFE: %s is not paused after the pause request and may be running", env.Container)
	}
	return StateContained, fmt.Sprintf("container %s contained (paused, not removed)", env.Container)
}

// removeOwn removes the container this run created and nothing else. It ignores
// cancellation so an interrupt does not leave the container behind.
func removeOwn(ctx context.Context, env *Env) error {
	return env.Docker.Remove(context.WithoutCancel(ctx), env.Container)
}
