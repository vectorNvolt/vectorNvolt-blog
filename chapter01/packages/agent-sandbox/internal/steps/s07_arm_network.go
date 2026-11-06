// SFR-AGC-007/008, SSR-AGC-016, SFR-AGC-022 (USC-002 step 6, err-8): create_agent_net arm agent-<id> while the container is
// paused, then create_agent_net check-gate agent-<id> (exit 0 required). On failure disarm first; only a disarm with exit 0
// lets the own container be removed; otherwise nothing is removed and the container stays paused behind its controls.
package steps

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"agent-sandbox/internal/services"
)

func ArmNetwork(ctx context.Context, env *Env) *Failure {
	if err := env.Arm.Apply(ctx, env.Container); err != nil {
		return armFailure(ctx, env, fmt.Sprintf("failed control: network arming (%s arm %s): %v", env.Config.ArmBin, env.Container, err), nil)
	}
	if err := env.Arm.CheckGate(ctx, env.Container); err != nil {
		var checks []string
		var ce *services.CmdError
		if errors.As(err, &ce) {
			for _, l := range strings.Split(ce.Stdout, "\n") {
				if l = strings.TrimSpace(l); l != "" {
					checks = append(checks, l)
				}
			}
		}
		return armFailure(ctx, env, fmt.Sprintf("failed control: arming read-back (%s check-gate %s): %v", env.Config.ArmBin, env.Container, err), checks)
	}
	return nil
}

// armFailure disarms, then removes the own container only when the disarm exited 0 (no prompt).
func armFailure(ctx context.Context, env *Env, msg string, checks []string) *Failure {
	f := &Failure{ExitCode: ExitRuntime, ExitState: StateRemoved, Message: msg, Checks: checks}
	if err := env.Arm.Disarm(context.WithoutCancel(ctx), env.Container); err != nil {
		f.ExitState = StateDisarmFailed
		f.Message += fmt.Sprintf("\ndisarm failed (%s disarm %s): %v\n"+
			"container %s was not removed; it is left paused behind its controls; run remove_agent_container %s",
			env.Config.ArmBin, env.Container, err, env.Container, env.AgentID)
		return f
	}
	if rmErr := removeOwn(ctx, env); rmErr != nil {
		f.ExitState = StateRemoveFailed
		f.Message += fmt.Sprintf("\ncleanup failed, container %s left in place and disarmed, remove it manually: %v", env.Container, rmErr)
	}
	return f
}
