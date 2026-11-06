// SFR-IMG-003 (USC-003 step 5, err-4): build agent-image:<tag> in agent-host.
// The build context is a tar of the Windows-side build files, sent to docker build on stdin.
// Pulling the base image and installing packages happen inside docker build; its failures surface here.
package steps

import (
	"context"

	"agent-image/internal/config"
	"agent-image/internal/services"
)

func BuildImage(ctx context.Context, env *Env) *Failure {
	tar, err := services.BuildContextTar(env.ContextDir, config.BuildContextFiles)
	if err != nil {
		return fail(ExitPrerequisite, StateNotCreated, "cannot assemble the build context", err)
	}
	if err := env.Host.Build(ctx, config.DockerfileName, env.Image, tar); err != nil {
		return fail(ExitPrerequisite, StateNotCreated, "docker build failed", err)
	}
	return nil
}
