// SFR-AGC-016, SSR-AGC-016 (USC-006 step 4, err-3/err-4): create_agent_net disarm agent-<id> before anything is removed.
// A binary that cannot run is a prerequisite failure; a non-zero exit stops the run and the container stays behind the gate.
package steps

import (
	"context"
	"errors"
	"fmt"

	"agent-sandbox/internal/services"
)

func DisarmNetwork(ctx context.Context, env *Env) *Failure {
	if env.Absent {
		return nil
	}
	err := env.Arm.Disarm(ctx, env.Container)
	if err == nil {
		return nil
	}
	if env.TolerantDisarm {
		// restore: veths and gate state are already gone and the gate is rebuilt right after the removals.
		fmt.Fprintf(env.Log.Err, "warning: disarm of %s failed, removing it anyway: %v\n", env.Container, err)
		return nil
	}
	var ce *services.CmdError
	if errors.As(err, &ce) && ce.Err != nil {
		return fail(ExitPrerequisite, StateNotRemoved, fmt.Sprintf("cannot run %s disarm; nothing was removed", env.Config.ArmBin), err)
	}
	return &Failure{
		ExitCode:  ExitRuntime,
		ExitState: StateDisarmFailed,
		Message: fmt.Sprintf("failed control: network disarm (%s disarm %s): %v\n"+
			"container %s was not removed; run again to retry", env.Config.ArmBin, env.Container, err, env.Container),
	}
}
