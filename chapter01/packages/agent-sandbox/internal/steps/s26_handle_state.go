// Agent state (remove step after the workspace): keep <state root>/<id> (Claude login and history) by default; with
// --purge delete exactly that directory, and only for a valid agentID, a real directory directly under the real state
// root (no symlink, nothing outside). Only create-remove runs reach this step; restore and guard never set --purge.
package steps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

func HandleState(_ context.Context, env *Env) *Failure {
	if !env.PurgeState {
		return nil
	}
	purgeFail := func(msg string, err error) *Failure {
		if err != nil {
			return fail(ExitRuntime, StateStatePurgeFailed, msg, err)
		}
		return &Failure{ExitCode: ExitRuntime, ExitState: StateStatePurgeFailed, Message: msg}
	}

	// The path is built from the validated id only; re-check before anything is deleted.
	if !agentIDRe.MatchString(env.AgentID) || env.StateDir != filepath.Join(env.Config.StateRoot, env.AgentID) {
		return purgeFail(fmt.Sprintf("refusing to purge state %q: not <state root>/<validated id>; nothing deleted", env.StateDir), nil)
	}
	fi, err := env.FS.Lstat(env.StateDir)
	if os.IsNotExist(err) {
		return nil // nothing to purge; reported as a note
	}
	if err != nil {
		return purgeFail(fmt.Sprintf("cannot inspect state folder %s", env.StateDir), err)
	}
	if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
		return purgeFail(fmt.Sprintf("refusing to purge %s: not a real directory (symlink or file); nothing deleted", env.StateDir), nil)
	}
	rootInfo, err := env.FS.Lstat(env.Config.StateRoot)
	if err != nil {
		return purgeFail(fmt.Sprintf("cannot inspect state root %s", env.Config.StateRoot), err)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 {
		return purgeFail(fmt.Sprintf("refusing to purge: state root %s is a symlink; nothing deleted", env.Config.StateRoot), nil)
	}
	root, err := env.FS.EvalSymlinks(env.Config.StateRoot)
	if err != nil {
		return purgeFail(fmt.Sprintf("cannot resolve state root %s", env.Config.StateRoot), err)
	}
	resolved, err := env.FS.EvalSymlinks(env.StateDir)
	if err != nil {
		return purgeFail(fmt.Sprintf("cannot resolve state folder %s", env.StateDir), err)
	}
	if resolved == root || filepath.Dir(resolved) != root {
		return purgeFail(fmt.Sprintf("refusing to purge %s: resolves to %s, not directly under %s; nothing deleted", env.StateDir, resolved, root), nil)
	}
	if err := env.FS.RemoveAll(env.StateDir); err != nil {
		return purgeFail(fmt.Sprintf("cannot delete state folder %s", env.StateDir), err)
	}
	env.StatePurged = true
	return nil
}
