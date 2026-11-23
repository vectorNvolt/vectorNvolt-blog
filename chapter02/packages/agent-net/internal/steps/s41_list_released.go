// `create_agent_net released`: read-only list of the elements of the gate set `released`, one line per element on
// stdout, `<mac> <comment>` (the comment is empty for an element that carries none, so the line ends after the
// blank). The guard of agent-sandbox uses it to find untagged and orphaned elements. A missing gate table lists
// nothing. Exit 0 when the set was read, 2 when it could not be read.
package steps

import (
	"context"

	"agent-net/internal/config"
)

func ListReleased(ctx context.Context, env *Env) *Failure {
	elems, _, err := env.Net.GateElements(ctx)
	if err != nil {
		return fail(ExitRuntime, StateGateInconsistent, "set "+config.GateSet+" of "+config.GateTable+" unreadable", err)
	}
	lines := make([]string, 0, len(elems))
	for _, e := range elems {
		lines = append(lines, e.MAC+" "+e.Comment)
	}
	env.Log.Success(lines...)
	return nil
}
