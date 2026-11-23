// Sequence assembly for remove_agent_container: runs the USC-006 steps in order and maps the first failure to an exit code.
// The disarm precedes the removal; a failed disarm leaves the container in place behind the gate.
package sequence

import (
	"context"
	"io"
	"strings"

	"agent-sandbox/internal/config"
	"agent-sandbox/internal/services"
	"agent-sandbox/internal/steps"
)

// RemoveDeps are the injected collaborators; tests replace Runner and FS.
type RemoveDeps struct {
	Config config.Config
	Runner services.Runner
	FS     services.FS
	Args   []string // arguments after the program name: <agentID> [--purge-workspace] [--purge]
	Stdout io.Writer
	Stderr io.Writer
	// TolerantDisarm continues with the removal when the disarm fails (agent_lifecycle restore only).
	TolerantDisarm bool
	// LockHeld: the caller holds the per-agent lock and keeps it; step 2 takes none (the guard).
	LockHeld bool
}

const removeTotalSteps = 10

var removePipeline = []stepDef{
	{1, "validate arguments", steps.ValidateRemoveArgs},
	{2, "acquire agent lock", steps.AcquireLockRemove},
	{3, "check docker engine", steps.CheckEngine},
	{4, "locate container", steps.LocateContainer},
	{5, "disarm agent network", steps.DisarmNetwork},
	{6, "remove container", steps.RemoveContainer},
	{7, "verify container removed", steps.VerifyRemoved},
	{8, "handle workspace folder", steps.HandleWorkspace},
	{9, "handle agent state", steps.HandleState},
	{10, "report result", steps.ReportRemoved},
}

// RunRemove executes the removal pipeline and returns the process exit code.
func RunRemove(ctx context.Context, d RemoveDeps) int {
	log := services.NewRenderer(d.Stdout, d.Stderr, d.Config.LogLevel)
	log.Prog = "remove_agent_container"
	runner := d.Runner
	if log.IsDebug() {
		runner = services.TracingRunner{Inner: runner, Trace: func(args []string) {
			log.Debugf("%s", strings.Join(args, " "))
		}}
	}
	env := &steps.Env{
		Config: d.Config,
		Docker: services.NewDocker(runner),
		Arm:    &services.Arm{Runner: runner, Bin: d.Config.ArmBin},
		FS:     d.FS,
		Log:    log,
		Args:   d.Args,
		Runner: runner,

		TolerantDisarm: d.TolerantDisarm,
		LockHeld:       d.LockHeld,
	}
	// The per-agent lock (step 2) is held for the whole run.
	defer func() {
		if env.Unlock != nil {
			env.Unlock()
		}
	}()

	for _, s := range removePipeline {
		log.Step(s.number, removeTotalSteps, s.name)
		if f := s.run(ctx, env); f != nil {
			log.Failure(services.FailureReport{
				Step:      s.number,
				Name:      s.name,
				Message:   f.Message,
				ExitCode:  f.ExitCode,
				ExitState: f.ExitState,
				Checks:    f.Checks,
			})
			return f.ExitCode
		}
	}
	return 0
}
