// SFR-AGC-013 (USC-006 step 1, err-1): validate agentID and the optional flags --purge-workspace and --purge (each at
// most once, after agentID) before any engine call or path construction.
package steps

import (
	"context"
	"path/filepath"

	"agent-sandbox/internal/config"
)

// RemoveUsage is printed for a missing or invalid argument.
const RemoveUsage = "usage: remove_agent_container <agentID> [--purge-workspace] [--purge]\n" +
	"  agentID must match " + config.AgentIDPattern + " (lowercase letters, digits and -; starts with a letter; 1 to 31 characters)\n" +
	"  --purge-workspace deletes the workspace folder; --purge deletes the agent state (Claude login and history)"

// PurgeFlag deletes the workspace folder; PurgeStateFlag deletes the persistent agent state.
const (
	PurgeFlag      = "--purge-workspace"
	PurgeStateFlag = "--purge"
)

func ValidateRemoveArgs(_ context.Context, env *Env) *Failure {
	bad := &Failure{ExitCode: ExitPrerequisite, ExitState: StateNotRemoved, Message: RemoveUsage}
	if len(env.Args) < 1 || len(env.Args) > 3 {
		return bad
	}
	if !agentIDRe.MatchString(env.Args[0]) {
		return bad
	}
	var purge, purgeState bool
	for _, a := range env.Args[1:] {
		switch {
		case a == PurgeFlag && !purge:
			purge = true
		case a == PurgeStateFlag && !purgeState:
			purgeState = true
		default:
			return bad
		}
	}
	env.AgentID = env.Args[0]
	env.Purge, env.PurgeState = purge, purgeState
	env.Container = config.ContainerPrefix + env.AgentID
	env.Workspace = filepath.Join(env.Config.WorkspaceRoot, env.AgentID)
	env.StateDir = filepath.Join(env.Config.StateRoot, env.AgentID)
	env.ClaudeDir = filepath.Join(env.StateDir, config.StateClaudeDir)
	return nil
}
