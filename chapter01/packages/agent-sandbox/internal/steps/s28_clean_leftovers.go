// USC-002 (before step 4, the container creation): no state of an earlier container of that name may be inherited.
// `create_agent_net disarm agent-<id>` (name-keyed, idempotent: gate set elements by comment and MAC, port table, tagged
// rules, neighbour and fdb entries) runs first, then the `released` part of the guard's reconcile removes untagged and
// orphaned gate set elements. Both must succeed; otherwise nothing is created. The container does not exist yet, so
// nothing is removed here.
package steps

import (
	"context"

	"agent-sandbox/internal/services"
)

func CleanLeftovers(ctx context.Context, env *Env) *Failure {
	if err := env.Arm.Disarm(ctx, env.Container); err != nil {
		return fail(ExitRuntime, StateNotCreated, "cannot clear leftover network state of "+env.Container+" ("+env.Config.ArmBin+" disarm)", err)
	}
	if err := services.ReconcileReleased(ctx, env.Docker, env.Arm, env.Config.RuntimeDir, env.Log.Err); err != nil {
		return fail(ExitRuntime, StateNotCreated, "cannot clear leftover elements of the agent network gate set", err)
	}
	return nil
}
