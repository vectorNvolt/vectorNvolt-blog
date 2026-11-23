// SFR-AGC-004/005 (USC-002 step 3, err-7): create <root>/<id> and its subfolders (config.WorkspaceSubdirs: .output, build, .venv,
// .npm-env, .conda-env, .packages; reuse if present) and chown all of them, not recursively, to agent-operator. Mkdir (not MkdirAll): a missing root is never created.
package steps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"agent-sandbox/internal/config"
)

func PrepareWorkspace(_ context.Context, env *Env) *Failure {
	dirs := []string{env.Workspace}
	for _, s := range config.WorkspaceSubdirs {
		dirs = append(dirs, filepath.Join(env.Workspace, s))
	}
	for _, d := range dirs {
		if err := env.FS.Mkdir(d, 0o755); err != nil {
			if !os.IsExist(err) {
				return fail(ExitRuntime, StateNotCreated, fmt.Sprintf("cannot create folder %s", d), err)
			}
			fi, lerr := env.FS.Lstat(d)
			if lerr != nil {
				return fail(ExitRuntime, StateNotCreated, fmt.Sprintf("cannot inspect existing folder %s", d), lerr)
			}
			if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
				return fail(ExitRuntime, StateNotCreated, fmt.Sprintf("existing %s is not a real directory", d), os.ErrExist)
			}
		}
	}
	for _, d := range dirs {
		if err := env.FS.Chown(d, config.WorkspaceUID, config.WorkspaceGID); err != nil {
			return fail(ExitRuntime, StateNotCreated, fmt.Sprintf("cannot hand folder %s to %d:%d", d, config.WorkspaceUID, config.WorkspaceGID), err)
		}
	}
	return nil
}
