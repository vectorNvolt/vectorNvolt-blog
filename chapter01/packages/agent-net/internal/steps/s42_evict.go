// `create_agent_net evict <mac>`: remove one element of the gate set `released` by MAC, for an element that carries
// no container name (untagged) or whose container is gone. Only the set element is deleted; the gate table, its chains
// and its rule are never touched. An element that is not there is not an error. The set is read again afterwards.
// Exit 0 evicted or not present, 2 (EVICT_INCOMPLETE) when the element could not be deleted or is still there.
package steps

import (
	"context"
	"fmt"

	"agent-net/internal/config"
)

func EvictElement(ctx context.Context, env *Env) *Failure {
	n, err := env.Net.RemoveGateElements(ctx, []string{env.EvictMAC}, "")
	if err != nil {
		return fail(ExitRuntime, StateEvictIncomplete, "MAC "+env.EvictMAC+" could not be removed from "+config.GateSet, err)
	}
	elems, _, err := env.Net.GateElements(ctx)
	if err != nil {
		return fail(ExitRuntime, StateEvictIncomplete, "set "+config.GateSet+" of "+config.GateTable+" unreadable", err)
	}
	for _, e := range elems {
		if e.MAC == env.EvictMAC {
			return &Failure{ExitCode: ExitRuntime, ExitState: StateEvictIncomplete,
				Message: fmt.Sprintf("MAC %s is still in set %s of %s", env.EvictMAC, config.GateSet, config.GateTable)}
		}
	}
	if n == 0 {
		env.Log.Success(fmt.Sprintf("MAC %s is not in set %s", env.EvictMAC, config.GateSet))
		return nil
	}
	env.Log.Success(fmt.Sprintf("MAC %s evicted from set %s", env.EvictMAC, config.GateSet))
	return nil
}
