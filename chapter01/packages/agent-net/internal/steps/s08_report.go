// SFR-NET-007 (USC-004 step 6): report success on stdout, stating that arm, disarm and check-gate are available;
// the sequence then returns exit 0.
package steps

import (
	"context"
	"fmt"

	"agent-net/internal/config"
)

func ReportCreated(_ context.Context, env *Env) *Failure {
	env.Log.Success(
		fmt.Sprintf("agent network %s created and verified (driver %s, subnet %s, not internal)", config.NetworkName, config.NetworkDriver, config.Subnet),
		"daemon.json and the docker engine are unchanged",
		"available: create_agent_net arm <container>, create_agent_net disarm <container>, create_agent_net check-gate <container>",
	)
	return nil
}
