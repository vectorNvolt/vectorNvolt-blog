// SFR-IMG-002 (USC-003 step 2, err-3): the agent-host engine reachable before any build.
package steps

import "context"

func CheckEngine(ctx context.Context, env *Env) *Failure {
	if err := env.Host.Version(ctx); err != nil {
		return fail(ExitPrerequisite, StateNotCreated, "docker engine in agent-host unreachable", err)
	}
	return nil
}
