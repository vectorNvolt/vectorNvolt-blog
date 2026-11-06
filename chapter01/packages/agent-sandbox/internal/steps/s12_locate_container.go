// SFR-AGC-015 (USC-006 step 3): look up agent-<id>; if absent, steps 4-6 are skipped and the run continues at step 7.
package steps

import "context"

func LocateContainer(ctx context.Context, env *Env) *Failure {
	exists, err := env.Docker.ContainerExists(ctx, env.Container)
	if err != nil {
		return fail(ExitRuntime, StateNotRemoved, "cannot check for the container", err)
	}
	env.Absent = !exists
	return nil
}
