// SFR-HOST-007 (USC-001 step 8): stop agent-host so wsl.conf takes effect on the next start.
package steps

import "context"

func Terminate(ctx context.Context, env *Env) *Failure {
	if err := env.WSL.Terminate(ctx); err != nil {
		return fail(ExitRuntime, StatePartiallyConfigured, "terminating agent-host failed", err)
	}
	return nil
}
