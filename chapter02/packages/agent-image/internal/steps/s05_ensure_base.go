// USC-003 step 5 (ensure base image): agent-base:<hash of Dockerfile.base> must exist in agent-host before the
// agent build. The base holds the snapshot-pinned apt packages, so only its (rare) build needs
// snapshot.ubuntu.com. Order: present with a matching label, else load agent-base-<hash>.tar from the current
// directory, else build it. --rebuild-base skips straight to the build. The exe never writes the archive.
package steps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"agent-image/internal/config"
	"agent-image/internal/services"
)

func EnsureBase(ctx context.Context, env *Env) *Failure {
	archive := config.BaseArchiveName(env.BaseHash)

	if !env.RebuildBase {
		present, err := baseMatches(ctx, env)
		if err != nil {
			return fail(ExitPrerequisite, StateNotCreated, "cannot check for the base image "+env.BaseImage, err)
		}
		if present {
			env.Log.Infof("base image %s present", env.BaseImage)
			return nil
		}

		archivePath := filepath.Join(env.ContextDir, archive)
		if fi, err := os.Stat(archivePath); err == nil && fi.Mode().IsRegular() {
			data, err := os.ReadFile(archivePath)
			if err != nil {
				return fail(ExitPrerequisite, StateNotCreated, "cannot read "+archive, err)
			}
			if err := env.Host.Load(ctx, data); err != nil {
				return fail(ExitPrerequisite, StateNotCreated, "docker load of "+archive+" failed", err)
			}
			if f := requireBase(ctx, env, "after docker load of "+archive); f != nil {
				return f
			}
			env.Log.Infof("base image loaded from %s", archive)
			return nil
		}
	}

	tar, err := services.BuildContextTar(env.ContextDir, []string{config.BaseDockerfileName})
	if err != nil {
		return fail(ExitPrerequisite, StateNotCreated, "cannot assemble the base build context", err)
	}
	env.Log.Infof("building base image %s (needs snapshot.ubuntu.com)", env.BaseImage)
	if err := env.Host.Build(ctx, config.BaseDockerfileName, env.BaseImage, tar,
		"--label", config.BaseHashLabel+"="+env.BaseHash); err != nil {
		return &Failure{ExitCode: ExitPrerequisite, ExitState: StateNotCreated, Message: fmt.Sprintf(
			"docker build of the base image %s failed: %v\n"+
				"the base build needs snapshot.ubuntu.com; run again later to retry the agent build, or load the base from %s in this folder",
			env.BaseImage, err, archive)}
	}
	if f := requireBase(ctx, env, "after docker build"); f != nil {
		return f
	}
	env.Log.Infof("base image %s built", env.BaseImage)
	return nil
}

// baseMatches reports whether env.BaseImage exists and carries the base hash label equal to env.BaseHash.
func baseMatches(ctx context.Context, env *Env) (bool, error) {
	got, exists, err := env.Host.LabelOf(ctx, env.BaseImage, config.BaseHashLabel)
	if err != nil {
		return false, err
	}
	return exists && got == env.BaseHash, nil
}

// requireBase re-inspects the base image and fails unless it exists with the matching label.
func requireBase(ctx context.Context, env *Env, when string) *Failure {
	ok, err := baseMatches(ctx, env)
	if err != nil {
		return fail(ExitPrerequisite, StateNotCreated, "cannot inspect the base image "+env.BaseImage+" "+when, err)
	}
	if !ok {
		return &Failure{ExitCode: ExitPrerequisite, ExitState: StateNotCreated, Message: fmt.Sprintf(
			"base image %s is missing or does not carry label %s=%s %s", env.BaseImage, config.BaseHashLabel, env.BaseHash, when)}
	}
	return nil
}
