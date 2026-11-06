// SFR-AGC-012, SSR-AGC-002, SFR-AGC-022 (USC-002 step 5, err-10): docker pause, then require State.Paused true on the
// inspected container before it is armed. The container is attached to agents-net but gated closed by the agents-net
// gate until armed. If not verified, only the own container (never armed, its MAC is not in the gate, so no disarm is
// needed) is removed.
package steps

import (
	"context"
	"errors"
	"fmt"
)

func PauseContainer(ctx context.Context, env *Env) *Failure {
	err := env.Docker.Pause(ctx, env.Container)
	if err == nil {
		info, ierr := env.Docker.Inspect(ctx, env.Container)
		switch {
		case ierr != nil:
			err = fmt.Errorf("paused state unreadable: %w", ierr)
		case !info.State.Paused:
			err = errors.New("State.Paused is false after docker pause")
		}
	}
	if err == nil {
		return nil
	}
	f := fail(ExitRuntime, StateRemoved, "pause not verified; container not armed and not unpaused", err)
	if rmErr := removeOwn(ctx, env); rmErr != nil {
		f.ExitState = StateRemoveFailed
		f.Message += fmt.Sprintf("\ncleanup failed, container %s left in place (attached to agents-net, not armed), remove it manually: %v", env.Container, rmErr)
	}
	return f
}
