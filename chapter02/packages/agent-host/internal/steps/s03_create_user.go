// SFR-HOST-005 (USC-001 step 3): create the non-root user agent-operator (no sudo/admin group) with UID/GID 1001.
// The foundation's login user (ubuntu, or the Lima host user) is never touched.
package steps

import (
	"context"
	"fmt"
	"strconv"

	"agent-host/internal/config"
)

var userID = strconv.Itoa(config.UserID)

// Absolute paths are used so the result does not depend on root's PATH.

// CreateGroupCmd is run first; it fails if GID 1001 is already taken.
var CreateGroupCmd = []string{"/usr/sbin/groupadd", "--gid", userID, config.User}

// CreateUserCmd deliberately passes no -G / supplementary group options.
// It fails if UID 1001 is already taken.
var CreateUserCmd = []string{"/usr/sbin/useradd", "--create-home", "--shell", "/bin/bash", "--uid", userID, "--gid", userID, config.User}

func CreateUser(ctx context.Context, env *Env) *Failure {
	if _, err := env.Host.Exec(ctx, nil, CreateGroupCmd...); err != nil {
		return fail(ExitRuntime, StatePartiallyConfigured, fmt.Sprintf("group creation failed (GID %s must be free on the host)", userID), err)
	}
	if _, err := env.Host.Exec(ctx, nil, CreateUserCmd...); err != nil {
		return fail(ExitRuntime, StatePartiallyConfigured, fmt.Sprintf("user creation failed (UID %s must be free on the host)", userID), err)
	}
	return nil
}
