// SFR-AGC-020 (USC-006 step 8): report the outcome on stdout; the sequence then returns exit 0.
package steps

import (
	"context"
	"fmt"
	"os"
)

func ReportRemoved(_ context.Context, env *Env) *Failure {
	var lines []string
	if env.Absent {
		lines = append(lines, fmt.Sprintf("no container %s", env.Container), "exit_state: "+StateNotPresent)
	} else {
		lines = append(lines, fmt.Sprintf("agent container %s disarmed and removed", env.Container), "exit_state: "+StateRemoved)
	}
	switch {
	case env.Purged:
		lines = append(lines, fmt.Sprintf("workspace purged: %s", env.Workspace))
	case env.Purge:
		lines = append(lines, fmt.Sprintf("workspace not present, nothing to purge: %s", env.Workspace))
	default:
		if _, err := env.FS.Lstat(env.Workspace); os.IsNotExist(err) {
			lines = append(lines, fmt.Sprintf("workspace absent: %s", env.Workspace))
		} else {
			lines = append(lines, fmt.Sprintf("workspace kept: %s", env.Workspace))
		}
	}
	switch {
	case env.StatePurged:
		lines = append(lines, fmt.Sprintf("agent state purged: %s", env.StateDir))
	case env.PurgeState:
		lines = append(lines, fmt.Sprintf("agent state not present, nothing to purge: %s", env.StateDir))
	default:
		if _, err := env.FS.Lstat(env.StateDir); os.IsNotExist(err) {
			lines = append(lines, fmt.Sprintf("agent state absent: %s", env.StateDir))
		} else {
			lines = append(lines, fmt.Sprintf("agent state kept: %s", env.StateDir))
		}
	}
	env.Log.Success(lines...)
	return nil
}
