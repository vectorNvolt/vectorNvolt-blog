// SFR-IMG-003 (USC-003 step 5, err-4): build agent-image:<tag> in agent-host.
// The build context is a tar of the build files, sent to docker build on stdin.
// FROM is the agent-base image made by the ensure-base step (BASE_IMAGE build arg); no apt snapshot access here.
package steps

import (
	"context"

	"agent-image/internal/config"
	"agent-image/internal/services"
)

func BuildImage(ctx context.Context, env *Env) *Failure {
	tar, err := services.BuildContextTar(env.ContextDir, env.ContextFiles)
	if err != nil {
		return fail(ExitPrerequisite, StateNotCreated, "cannot assemble the build context", err)
	}
	if err := env.Host.Build(ctx, config.DockerfileName, env.Image, tar,
		"--build-arg", "BASE_IMAGE="+env.BaseImage); err != nil {
		return fail(ExitPrerequisite, StateNotCreated, "docker build failed", err)
	}
	return nil
}
