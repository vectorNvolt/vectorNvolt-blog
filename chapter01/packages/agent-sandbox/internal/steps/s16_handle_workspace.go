// SFR-AGC-019, SSR-AGC-015 (USC-006 step 7, err-7): keep <root>/<id> by default; with --purge-workspace delete exactly
// that directory, and only if it is a real directory directly under the configured root (no symlink, nothing outside).
package steps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

func HandleWorkspace(_ context.Context, env *Env) *Failure {
	if !env.Purge {
		return nil
	}
	purgeFail := func(msg string, err error) *Failure {
		if err != nil {
			return fail(ExitRuntime, StateWorkspacePurgeFailed, msg, err)
		}
		return &Failure{ExitCode: ExitRuntime, ExitState: StateWorkspacePurgeFailed, Message: msg}
	}

	fi, err := env.FS.Lstat(env.Workspace)
	if os.IsNotExist(err) {
		return nil // nothing to purge; reported as a note
	}
	if err != nil {
		return purgeFail(fmt.Sprintf("cannot inspect workspace folder %s", env.Workspace), err)
	}
	if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
		return purgeFail(fmt.Sprintf("refusing to purge %s: not a real directory (symlink or file); nothing deleted", env.Workspace), nil)
	}
	rootInfo, err := env.FS.Lstat(env.Config.WorkspaceRoot)
	if err != nil {
		return purgeFail(fmt.Sprintf("cannot inspect workspace root %s", env.Config.WorkspaceRoot), err)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 {
		return purgeFail(fmt.Sprintf("refusing to purge: workspace root %s is a symlink; nothing deleted", env.Config.WorkspaceRoot), nil)
	}
	root, err := env.FS.EvalSymlinks(env.Config.WorkspaceRoot)
	if err != nil {
		return purgeFail(fmt.Sprintf("cannot resolve workspace root %s", env.Config.WorkspaceRoot), err)
	}
	resolved, err := env.FS.EvalSymlinks(env.Workspace)
	if err != nil {
		return purgeFail(fmt.Sprintf("cannot resolve workspace folder %s", env.Workspace), err)
	}
	if resolved == root || filepath.Dir(resolved) != root {
		return purgeFail(fmt.Sprintf("refusing to purge %s: resolves to %s, not directly under %s; nothing deleted", env.Workspace, resolved, root), nil)
	}
	if err := env.FS.RemoveAll(env.Workspace); err != nil {
		return purgeFail(fmt.Sprintf("cannot delete workspace folder %s", env.Workspace), err)
	}
	env.Purged = true
	return nil
}
