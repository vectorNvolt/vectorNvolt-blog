// SFR-AGC-010 (USC-002 step 10), reached only after the pause, arm read-back, release, resolv.conf write and final verification passed: report success on stdout; the sequence then returns exit 0.
package steps

import (
	"context"
	"fmt"

	"agent-sandbox/internal/config"
)

func ReportSuccess(_ context.Context, env *Env) *Failure {
	env.Log.Success(
		fmt.Sprintf("agent container %s created and running (runtime runsc, network armed)", env.Container),
		fmt.Sprintf("workspace: %s -> /workspace", env.Workspace),
		fmt.Sprintf("claude state: %s -> %s", env.ClaudeDir, config.ClaudeMount),
	)
	return nil
}
