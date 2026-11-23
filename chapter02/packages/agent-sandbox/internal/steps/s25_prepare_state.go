// Persistent agent state and DNS file (create, before the container exists): <state root> and <state root>/<id> are
// root:root 0711, <state root>/<id>/claude is 1001:1001 0700 (kept, with its content, when present), and the host file
// config.HostResolv holds exactly the two allowed resolvers (root:root 0644, rewritten when it differs).
package steps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"agent-sandbox/internal/config"
)

func PrepareState(_ context.Context, env *Env) *Failure {
	dirs := []struct {
		path     string
		uid, gid int
		mode     os.FileMode
	}{
		{env.Config.StateRoot, 0, 0, 0o711},
		{env.StateDir, 0, 0, 0o711},
		{env.ClaudeDir, config.AgentUID, config.AgentGID, 0o700},
	}
	for i, d := range dirs {
		var err error
		if i == 0 {
			err = env.FS.MkdirAll(d.path, d.mode)
		} else {
			err = env.FS.Mkdir(d.path, d.mode)
		}
		if err != nil {
			if !os.IsExist(err) {
				return fail(ExitRuntime, StateNotCreated, fmt.Sprintf("cannot create state folder %s", d.path), err)
			}
			fi, lerr := env.FS.Lstat(d.path)
			if lerr != nil {
				return fail(ExitRuntime, StateNotCreated, fmt.Sprintf("cannot inspect existing state folder %s", d.path), lerr)
			}
			if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
				return fail(ExitRuntime, StateNotCreated, fmt.Sprintf("existing %s is not a real directory", d.path), os.ErrExist)
			}
		}
		if err := env.FS.Chown(d.path, d.uid, d.gid); err != nil {
			return fail(ExitRuntime, StateNotCreated, fmt.Sprintf("cannot hand state folder %s to %d:%d", d.path, d.uid, d.gid), err)
		}
		if err := env.FS.Chmod(d.path, d.mode); err != nil {
			return fail(ExitRuntime, StateNotCreated, fmt.Sprintf("cannot set mode %o on state folder %s", d.mode, d.path), err)
		}
	}

	file := env.Config.HostResolv
	if err := env.FS.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return fail(ExitRuntime, StateNotCreated, fmt.Sprintf("cannot create folder %s", filepath.Dir(file)), err)
	}
	if got, err := env.FS.ReadFile(file); err != nil || string(got) != config.ResolvConfContent {
		if err := env.FS.WriteFile(file, []byte(config.ResolvConfContent), 0o644); err != nil {
			return fail(ExitRuntime, StateNotCreated, fmt.Sprintf("cannot write %s", file), err)
		}
	}
	if err := env.FS.Chown(file, 0, 0); err != nil {
		return fail(ExitRuntime, StateNotCreated, fmt.Sprintf("cannot hand %s to root", file), err)
	}
	if err := env.FS.Chmod(file, 0o644); err != nil {
		return fail(ExitRuntime, StateNotCreated, fmt.Sprintf("cannot set mode 644 on %s", file), err)
	}
	return nil
}
