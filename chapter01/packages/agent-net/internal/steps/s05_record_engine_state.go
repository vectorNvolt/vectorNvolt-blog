// SSR-NET-001 (USC-004 step 5): record the engine configuration and start identity before anything is created.
// This tool never writes daemon.json and never reloads or restarts docker. The same step starts the arm, disarm,
// check-gate and remove pipelines (the not-done state of the mode); VerifyEngineUnchanged compares at the end.
package steps

import "context"

func RecordEngineState(_ context.Context, env *Env) *Failure {
	st, err := snapshotEngine(env)
	if err != nil {
		return fail(ExitPrerequisite, notDone(env), "engine state could not be recorded", err)
	}
	env.Before = st
	return nil
}
