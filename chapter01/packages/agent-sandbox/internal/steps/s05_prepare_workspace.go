// SFR-AGC-004/005 (USC-002 step 3, err-7): create <root>/<id> and <root>/<id>/.output (reuse if present) and chown
// both, not recursively, to agent-operator. Mkdir (not MkdirAll): a missing root is never created.
package steps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"agent-sandbox/internal/config"
)

func PrepareWorkspace(_ context.Context, env *Env) *Failure {
	output := filepath.Join(env.Workspace, config.OutputDir)
	dirs := []string{env.Workspace, output}
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
