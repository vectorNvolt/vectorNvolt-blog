// Per-agent lock (create and remove): flock on <runtime dir>/<id>.lock held for the whole run, so that the guard of
// `agent_lifecycle guard` can tell a tool-driven start/unpause from one made outside the tools. A second create/remove
// for the same agent fails fast instead of interleaving.
package steps

import (
	"context"
	"errors"
	"fmt"

	"agent-sandbox/internal/services"
)

func acquireLock(env *Env, state string) *Failure {
	lock, err := services.TryLock(env.Config.RuntimeDir, env.AgentID)
	if err != nil {
		if errors.Is(err, services.ErrLockBusy) {
			return &Failure{ExitCode: ExitPrerequisite, ExitState: state,
				Message: fmt.Sprintf("another create or remove of agent %s is in progress (lock %s/%s.lock is held)", env.AgentID, env.Config.RuntimeDir, env.AgentID)}
		}
		return fail(ExitRuntime, state, "cannot take the per-agent lock", err)
	}
	env.Unlock = lock.Release
	return nil
}

// AcquireLockCreate is the create_agent_container variant.
func AcquireLockCreate(_ context.Context, env *Env) *Failure {
	return acquireLock(env, StateNotCreated)
}

// AcquireLockRemove is the remove_agent_container variant.
// With LockHeld the caller (the guard) already holds the agent lock and releases it itself: nothing is taken here.
func AcquireLockRemove(_ context.Context, env *Env) *Failure {
	if env.LockHeld {
		return nil
	}
	return acquireLock(env, StateNotRemoved)
}
