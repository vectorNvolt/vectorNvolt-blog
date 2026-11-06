// SFR-HOST-005 (USC-001 step 6): create the non-root user agent-operator (no sudo/admin group) with UID/GID 1001.
package steps

import (
	"context"
	"fmt"
	"strconv"

	"agent-host/internal/config"
)

var userID = strconv.Itoa(config.UserID)

// Absolute paths are used because `wsl -e` does not search /usr/sbin on a fresh distro.

// RemoveImageUserCmd is run as root before CreateGroupCmd and removes the image's default user and group only if present.
// Earlier Canonical WSL images shipped user `ubuntu` with primary GID 1001, which would collide with agent-operator;
// the 26.04 image ships none, so the removal is conditional.
var RemoveImageUserCmd = []string{"/bin/sh", "-c",
	"if /usr/bin/getent passwd " + config.ImageUser + " >/dev/null; then /usr/sbin/userdel --remove " + config.ImageUser + "; fi; " +
		"if /usr/bin/getent group " + config.ImageUser + " >/dev/null; then /usr/sbin/groupdel " + config.ImageUser + "; fi"}

// CreateGroupCmd is run as root next; it fails if GID 1001 is already taken.
var CreateGroupCmd = []string{"/usr/sbin/groupadd", "--gid", userID, config.User}

// CreateUserCmd is the command run as root; it deliberately passes no -G / supplementary group options.
// It fails if UID 1001 is already taken.
var CreateUserCmd = []string{"/usr/sbin/useradd", "--create-home", "--shell", "/bin/bash", "--uid", userID, "--gid", userID, config.User}

func CreateUser(ctx context.Context, env *Env) *Failure {
	if _, err := env.WSL.Exec(ctx, "root", nil, RemoveImageUserCmd...); err != nil {
		return fail(ExitRuntime, StatePartiallyConfigured, fmt.Sprintf("removing image default user %s failed", config.ImageUser), err)
	}
	if _, err := env.WSL.Exec(ctx, "root", nil, CreateGroupCmd...); err != nil {
		return fail(ExitRuntime, StatePartiallyConfigured, fmt.Sprintf("group creation failed (GID %s must be free in the rootfs)", userID), err)
	}
	if _, err := env.WSL.Exec(ctx, "root", nil, CreateUserCmd...); err != nil {
		return fail(ExitRuntime, StatePartiallyConfigured, fmt.Sprintf("user creation failed (UID %s must be free in the rootfs)", userID), err)
	}
	return nil
}
