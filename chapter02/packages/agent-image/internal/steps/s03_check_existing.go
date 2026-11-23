// SFR-IMG-006 (USC-003 step 3): does agent-image:<tag> already exist in agent-host?
// The image is absent only on an explicit "no such image" from the engine; any other failure
// is an engine error (err-6): exit 1, no prompt, no build.
package steps

import "context"

func CheckExisting(ctx context.Context, env *Env) *Failure {
	exists, err := env.Host.ImageExists(ctx, env.Image)
	if err != nil {
		return fail(ExitPrerequisite, StateNotCreated, "cannot check for an existing image", err)
	}
	env.Exists = exists
	return nil
}
