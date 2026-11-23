// SFR-AGC-014 (USC-006 step 2, err-2): the engine must answer; runsc is not required to remove a container.
package steps

import "context"

func CheckEngine(ctx context.Context, env *Env) *Failure {
	if err := env.Docker.Reachable(ctx); err != nil {
		return fail(ExitPrerequisite, StateNotRemoved, "docker engine unavailable", err)
	}
	return nil
}
