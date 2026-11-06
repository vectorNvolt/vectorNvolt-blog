// SFR-NET-018 (USC-007 step 10): report success on stdout; exit 0. The count is the number of containers this run
// actually removed; a container that was already gone is not counted.
package steps

import (
	"context"
	"fmt"

	"agent-net/internal/config"
)

func ReportRemoved(_ context.Context, env *Env) *Failure {
	env.Log.Success(fmt.Sprintf("agent network %s removed: %d agent container(s) removed (workspaces kept), %s* tables, the %s table and %s rules gone",
		config.NetworkName, len(env.Removed), config.PortTablePfx, config.GateTable, config.CommentPrefix))
	return nil
}
