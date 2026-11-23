// SSR-NET-001 / SSR-NET-004 (every run of arm, disarm, check-gate and remove): compare the engine state recorded at the
// start of the run (RecordEngineState: daemon.json hash and engine start identity) with the state now, before the
// success report. A change fails the run with the exit state of the mode; nothing is rolled back (the tool never
// touches the engine, so a change comes from outside and is reported).
//
//	arm         exit 2, GATED_UNARMED, failed control "engine state" (the caller removes the paused container)
//	disarm      exit 2, DISARM_INCOMPLETE
//	check-gate  exit 2, GATE_INCONSISTENT
//	remove      exit 2, REMOVE_INCOMPLETE
package steps

import (
	"context"
	"fmt"
)

// ctlEngine names the engine-state control of the arming failure report.
const ctlEngine = "engine state"

func VerifyEngineUnchanged(_ context.Context, env *Env) *Failure {
	var bad []string
	after, err := snapshotEngine(env)
	if err != nil {
		bad = append(bad, fmt.Sprintf("engine state unreadable after the run: %v", err))
	} else {
		if after.DaemonHash != env.Before.DaemonHash {
			bad = append(bad, fmt.Sprintf("daemon.json changed (SHA-256 %s -> %s)", env.Before.DaemonHash, after.DaemonHash))
		}
		if after.Identity != env.Before.Identity {
			bad = append(bad, fmt.Sprintf("docker engine was restarted (%s -> %s)", env.Before.Identity, after.Identity))
		}
	}
	if len(bad) == 0 {
		return nil
	}
	const msg = "the container engine configuration or process changed during the run"
	switch env.Mode {
	case ModeArm:
		f := failControl(ctlEngine, msg, nil)
		f.Checks = bad
		return f
	case ModeDisarm:
		return &Failure{ExitCode: ExitRuntime, ExitState: StateDisarmIncomplete, Message: msg, Checks: bad}
	case ModeEvict:
		return &Failure{ExitCode: ExitRuntime, ExitState: StateEvictIncomplete, Message: msg, Checks: bad}
	case ModeCheckGate, ModeReleased:
		return &Failure{ExitCode: ExitRuntime, ExitState: StateGateInconsistent, Message: msg, Checks: bad}
	case ModeRemove:
		return &Failure{ExitCode: ExitRuntime, ExitState: StateRemoveIncomplete, Message: msg, Checks: bad}
	}
	return &Failure{ExitCode: ExitRuntime, ExitState: StateCreatedUnverified, Message: msg, Checks: bad}
}
