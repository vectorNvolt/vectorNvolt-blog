// SFR-IMG-016 (USC-003 step 6, err-5): after the build the image must be inspectable in agent-host.
// SSR-IMG-005: its env, labels and build history must hold no credential (nothing is started).
package steps

import (
	"context"
	"fmt"
)

func VerifyImage(ctx context.Context, env *Env) *Failure {
	if err := env.Host.Inspect(ctx, env.Image); err != nil {
		return fail(ExitRuntime, StateUnverified, "image missing after docker build", err)
	}

	vars, err := env.Host.ImageEnv(ctx, env.Image)
	if err != nil {
		return fail(ExitRuntime, StateUnverified, "cannot read image environment for the credential check", err)
	}
	labels, err := env.Host.ImageLabels(ctx, env.Image)
	if err != nil {
		return fail(ExitRuntime, StateUnverified, "cannot read image labels for the credential check", err)
	}
	history, err := env.Host.History(ctx, env.Image)
	if err != nil {
		return fail(ExitRuntime, StateUnverified, "cannot read image history for the credential check", err)
	}
	if finding := ScanCredentials(vars, labels, history); finding != "" {
		return &Failure{
			ExitCode:  ExitRuntime,
			ExitState: StateUnverified,
			Message:   fmt.Sprintf("image may contain a credential (%s); the matched value is not shown", finding),
		}
	}
	return nil
}
