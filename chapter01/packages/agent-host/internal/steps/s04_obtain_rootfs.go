// SFR-HOST-003 (USC-001 step 4): obtain the Ubuntu 26.04 rootfs and accept it only if its SHA-256 verifies.
package steps

import "context"

func ObtainRootfs(ctx context.Context, env *Env) *Failure {
	path, err := env.Rootfs.Fetch(ctx, env.Config.RootfsURL, env.Config.SHA256SumsURL)
	if err != nil {
		return fail(ExitPrerequisite, StateNotCreated, "rootfs not obtainable", err)
	}
	env.RootfsPath = path
	return nil
}
