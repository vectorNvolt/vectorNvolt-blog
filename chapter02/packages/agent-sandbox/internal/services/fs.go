// IP-AGENT-WORKSPACE-FS adapter: filesystem operations behind an interface so tests can use a temp dir.
package services

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// FS is the filesystem surface the tool needs.
type FS interface {
	IsDir(path string) bool
	MkdirAll(path string, perm os.FileMode) error
	// Mkdir creates one directory and fails with an exist error if present (never creates a missing parent).
	Mkdir(path string, perm os.FileMode) error
	Chown(path string, uid, gid int) error
	// Owner returns the uid and gid owning path without following a final symlink.
	Owner(path string) (uid, gid int, err error)
	// Lstat, EvalSymlinks and RemoveAll serve the workspace purge of remove_agent_container.
	Lstat(path string) (os.FileInfo, error)
	EvalSymlinks(path string) (string, error)
	RemoveAll(path string) error
	// Chmod, ReadFile and WriteFile serve the agent state directories and the host resolv.conf of create_agent_container.
	Chmod(path string, perm os.FileMode) error
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, perm os.FileMode) error
	// Glob lists the paths matching pattern (enter_agent looks for conversation files).
	Glob(pattern string) ([]string, error)
}

// OSFS is the real filesystem.
type OSFS struct{}

func (OSFS) IsDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

func (OSFS) MkdirAll(path string, perm os.FileMode) error { return os.MkdirAll(path, perm) }

func (OSFS) Mkdir(path string, perm os.FileMode) error { return os.Mkdir(path, perm) }

func (OSFS) Chown(path string, uid, gid int) error { return os.Chown(path, uid, gid) }

func (OSFS) Owner(path string) (int, int, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return 0, 0, err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, fmt.Errorf("owner of %s is not available on this platform", path)
	}
	return int(st.Uid), int(st.Gid), nil
}

func (OSFS) Lstat(path string) (os.FileInfo, error) { return os.Lstat(path) }

func (OSFS) EvalSymlinks(path string) (string, error) { return filepath.EvalSymlinks(path) }

func (OSFS) RemoveAll(path string) error { return os.RemoveAll(path) }

func (OSFS) Chmod(path string, perm os.FileMode) error { return os.Chmod(path, perm) }

func (OSFS) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

func (OSFS) Glob(pattern string) ([]string, error) { return filepath.Glob(pattern) }

func (OSFS) WriteFile(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}
