// SFR-NET-012 + SSR-NET-003 (USC-004 alt-2): first deletion of the disarm order, before the port table: take the
// container out of the set `released` of the agents-net gate, so that the gate closes the port again before anything
// else of it is removed. Name-keyed: every element whose comment is the container name goes, and every element with
// one of the collected MACs (docker, the accept rules of the port table) even when it carries no or another comment.
// Only set elements are deleted; the gate table, its chains and its DOCKER-USER rule are never touched. An absent gate
// table or element is not an error (a container that was created but never armed has none). Runs per container inside
// remove_agent_net too.
package steps

import "context"

func DisarmCloseGate(ctx context.Context, env *Env) *Failure {
	n, err := env.Net.RemoveGateElements(ctx, env.Disarm.MACs, env.Container)
	env.Disarm.Deleted += n
	if err != nil {
		return fail(ExitRuntime, incomplete(env), "MAC could not be removed from the agents-net gate", err)
	}
	return nil
}
