// SFR-AGC-001 (USC-002 step 1, err-1): validate agentID before any engine call.
package steps

import (
	"context"
	"path/filepath"
	"regexp"

	"agent-sandbox/internal/config"
)

// Usage is printed for a missing or invalid agentID.
const Usage = "usage: create_agent_container <agentID>\n" +
	"  agentID must match " + config.AgentIDPattern + " (lowercase letters, digits and -; starts with a letter; 1 to 31 characters)"

var agentIDRe = regexp.MustCompile(config.AgentIDPattern)

// ValidAgentID reports whether id matches the agentID pattern.
func ValidAgentID(id string) bool { return agentIDRe.MatchString(id) }

func ValidateID(_ context.Context, env *Env) *Failure {
	if !agentIDRe.MatchString(env.AgentID) {
		return &Failure{ExitCode: ExitPrerequisite, ExitState: StateNotCreated, Message: Usage}
	}
	env.Container = config.ContainerPrefix + env.AgentID
	env.Workspace = filepath.Join(env.Config.WorkspaceRoot, env.AgentID)
	env.StateDir = filepath.Join(env.Config.StateRoot, env.AgentID)
	env.ClaudeDir = filepath.Join(env.StateDir, config.StateClaudeDir)
	return nil
}
