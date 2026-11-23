// USC-001 step 1 (preflight): create_agent_host runs natively on the foundation VM, as root. Refuse anything else
// before a single command is run. Changes nothing.
package steps

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"agent-host/internal/config"
)

// HostMounts returns the mount points in the mounts file (/proc/mounts format) whose fstype is a host-share type
// (same set as vm/check-foundation.sh).
func HostMounts(mountsPath string) ([]string, error) {
	b, err := os.ReadFile(mountsPath)
	if err != nil {
		return nil, err
	}
	var bad []string
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		switch f[2] {
		case "virtiofs", "9p", "sshfs", "fuse.sshfs", "fuse.reverse-sshfs":
			bad = append(bad, f[1])
		}
	}
	return bad, nil
}

// idTaken returns the name of the first entry of a passwd/group file (name:x:id:...) whose id field equals id.
func idTaken(path string, id int) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Split(line, ":")
		if len(f) >= 3 && f[2] == strconv.Itoa(id) {
			return f[0], nil
		}
	}
	return "", nil
}

// userExists reports whether name is an entry of a passwd file.
func userExists(path, name string) (bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(string(b), "\n") {
		if n, _, _ := strings.Cut(line, ":"); n == name {
			return true, nil
		}
	}
	return false, nil
}

// LoginUser is a passwd entry of a login user: name and home directory.
type LoginUser struct{ Name, Home string }

// loginUsers returns the entries of a passwd file with 1000 <= UID < 65534, except the user named except.
func loginUsers(path, except string) ([]LoginUser, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var users []LoginUser
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Split(line, ":")
		if len(f) < 6 || f[0] == except {
			continue
		}
		if uid, err := strconv.Atoi(f[2]); err == nil && uid >= 1000 && uid < 65534 && f[5] != "" {
			users = append(users, LoginUser{f[0], f[5]})
		}
	}
	return users, nil
}

// PreflightFoundation checks GOOS linux, euid 0, the foundation marker (FOUNDATION_VERSION=1, UBUNTU=26.04),
// no host mounts, and that agent-host was not already created on this VM.
func PreflightFoundation(_ context.Context, env *Env) *Failure {
	hint := fmt.Sprintf("run %s on this machine to see what the foundation lacks", config.CheckFoundationScript)
	notCreated := func(msg string) *Failure {
		return &Failure{ExitCode: ExitPrerequisite, ExitState: StateNotCreated, Message: msg}
	}
	if env.System.GOOS != "linux" {
		return notCreated(fmt.Sprintf("create_agent_host must run on the Linux foundation VM, not on %s; build it natively inside the VM (%s)", env.System.GOOS, hint))
	}
	if env.System.Euid != 0 {
		return notCreated(fmt.Sprintf("create_agent_host must run as root (euid 0, found %d): run it with sudo", env.System.Euid))
	}
	b, err := os.ReadFile(env.System.MarkerPath)
	if err != nil {
		return notCreated(fmt.Sprintf("foundation marker %s is missing or unreadable (%v); this is not a foundation VM (%s)", env.System.MarkerPath, err, hint))
	}
	hasVersion, hasUbuntu := false, false
	for _, line := range strings.Split(string(b), "\n") {
		switch strings.TrimSpace(line) {
		case config.FoundationVersionLine:
			hasVersion = true
		case config.FoundationUbuntuLine:
			hasUbuntu = true
		}
	}
	if !hasVersion {
		return notCreated(fmt.Sprintf("foundation marker %s does not contain %s: unknown foundation version (%s)", env.System.MarkerPath, config.FoundationVersionLine, hint))
	}
	if !hasUbuntu {
		return notCreated(fmt.Sprintf("foundation marker %s does not contain %s: the foundation must be Ubuntu 26.04 (%s)", env.System.MarkerPath, config.FoundationUbuntuLine, hint))
	}
	mounts, err := HostMounts(env.System.MountsPath)
	if err != nil {
		return notCreated(fmt.Sprintf("cannot read %s: %v", env.System.MountsPath, err))
	}
	if len(mounts) > 0 {
		return notCreated(fmt.Sprintf("the VM has host mount(s) %s (virtiofs, 9p or sshfs); the foundation must have none (%s)", strings.Join(mounts, ", "), hint))
	}
	rerun := func(why string) *Failure {
		return notCreated(fmt.Sprintf("%s; recreate the VM (create_agent_host has no repair on rerun)", why))
	}
	if _, err := os.Stat(env.System.DoneMarkerPath); err == nil {
		return rerun(fmt.Sprintf("agent-host already created (%s exists)", env.System.DoneMarkerPath))
	}
	if exists, err := userExists(env.System.PasswdPath, config.User); err != nil {
		return notCreated(fmt.Sprintf("cannot read %s: %v", env.System.PasswdPath, err))
	} else if exists {
		return rerun(fmt.Sprintf("agent-host already created (user %s exists)", config.User))
	}
	if name, err := idTaken(env.System.PasswdPath, config.UserID); err != nil {
		return notCreated(fmt.Sprintf("cannot read %s: %v", env.System.PasswdPath, err))
	} else if name != "" {
		return rerun(fmt.Sprintf("UID %d taken by %s", config.UserID, name))
	}
	if name, err := idTaken(env.System.GroupPath, config.UserID); err != nil {
		return notCreated(fmt.Sprintf("cannot read %s: %v", env.System.GroupPath, err))
	} else if name != "" {
		return rerun(fmt.Sprintf("GID %d taken by %s", config.UserID, name))
	}
	return nil
}
