// SFR-AGC-006 (USC-002 step 4, err-9): docker create (runsc, --network agents-net, closed capability allowlist, the
// workspace bind, the per-agent Claude state bind with CLAUDE_CONFIG_DIR and the read-only resolv.conf bind) then
// docker start. The container is attached to agents-net from the create (gVisor does not pick up interfaces added after
// the start) and gated closed by the agents-net gate until it is armed. If start fails, only the container created by
// this run is removed; it was never armed, so its MAC is not in the gate and no disarm is needed.
package steps

import (
	"context"
	"fmt"
)

func CreateAndStart(ctx context.Context, env *Env) *Failure {
	if err := env.Docker.Create(ctx, env.Container, env.Workspace, env.ClaudeDir, env.Config.HostResolv); err != nil {
		return fail(ExitRuntime, StateNotCreated, "container create failed", err)
	}
	if err := env.Docker.Start(ctx, env.Container); err != nil {
		f := fail(ExitRuntime, StateRemoved, "container start failed", err)
		if rmErr := removeOwn(ctx, env); rmErr != nil {
			f.ExitState = StateCreatedCleanupFailed
			f.Message += fmt.Sprintf("\ncleanup failed, remove %s manually: %v", env.Container, rmErr)
		}
		return f
	}
	return nil
}
