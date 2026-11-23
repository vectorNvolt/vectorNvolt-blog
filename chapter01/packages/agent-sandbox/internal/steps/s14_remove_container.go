// SFR-AGC-017 (USC-006 step 5, err-5): docker rm -f agent-<id> after a successful disarm; nothing else is removed.
package steps

import "context"

func RemoveContainer(ctx context.Context, env *Env) *Failure {
	if env.Absent {
		return nil
	}
	if err := removeOwn(ctx, env); err != nil {
		return fail(ExitRuntime, StateRemoveFailed, "cannot remove container "+env.Container, err)
	}
	return nil
}
