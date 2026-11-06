// USC-002 step 2: runtime check (SFR-RTC-001), duplicate refusal (SFR-AGC-002), shared prerequisites and the
// Workspace-Agents root (SFR-AGC-003/023), in this order; the first failure stops with no side effect.
package steps

import "context"

func CheckPreconditions(ctx context.Context, env *Env) *Failure {
	for _, check := range []func(context.Context, *Env) *Failure{CheckRuntime, RefuseDuplicate, ConfirmPrerequisites} {
		if f := check(ctx, env); f != nil {
			return f
		}
	}
	return nil
}
