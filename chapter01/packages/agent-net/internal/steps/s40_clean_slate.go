// SFR-NET-008 (USC-004 alt-1): arm builds from empty state. After the container preconditions and before the port is
// identified and closed, the name-keyed cleanup of `disarm` runs in-process (DisarmInspect .. DisarmVerify): it removes
// whatever an earlier container or run of that name left (elements of `released`, port table, tagged iptables rules,
// neighbour entry, fdb entries). Nothing is left to inherit, and a leftover that cannot be removed fails the arm with
// the container still paused and gated. env.Port and env.Inspected are restored afterwards.
package steps

import "context"

// ctlCleanSlate names the control "state of an earlier run removed" in the arming failure report.
const ctlCleanSlate = "clean slate"

func CleanSlate(ctx context.Context, env *Env) *Failure {
	port, inspected := env.Port, env.Inspected
	defer func() { env.Port, env.Inspected = port, inspected }()
	for _, step := range []func(context.Context, *Env) *Failure{DisarmInspect, DisarmFindPorts, DisarmCloseGate, DisarmDeleteTable, DisarmRemoveRules, DisarmVerify} {
		if f := step(ctx, env); f != nil {
			if f.ExitCode == ExitPrerequisite {
				return f // a precondition of the mode (docker unreadable, port table of another container)
			}
			wrapped := failControl(ctlCleanSlate, f.Message, nil)
			wrapped.Checks = f.Checks
			return wrapped
		}
	}
	return nil
}
