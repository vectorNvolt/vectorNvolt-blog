// Sequence assembly for `agent_lifecycle restore` (the guard of `agent_lifecycle guard` is in guard.go).
// restore removes every agent-* container through the remove path (never purging state), rebuilds agents-net and writes the
// ready marker last; any failure exits non-zero and leaves no marker.
package sequence

import (
	"bytes"
	"context"
	"io"
	"strings"

	"agent-sandbox/internal/config"
	"agent-sandbox/internal/services"
	"agent-sandbox/internal/steps"
)

// RestoreDeps are the injected collaborators; tests replace Runner and FS.
type RestoreDeps struct {
	Config config.Config
	Runner services.Runner
	FS     services.FS
	IsRoot bool
	Stdout io.Writer
	Stderr io.Writer
}

const restoreTotalSteps = 7

var restorePipeline = []stepDef{
	{1, "check root", steps.RestoreCheckRoot},
	{2, "remove ready marker", steps.RestoreRemoveMarker},
	{3, "wait for docker daemon", steps.RestoreWaitDocker},
	{4, "remove agent containers", steps.RestoreRemoveAgents},
	{5, "rebuild agent network", steps.RestoreRebuildNet},
	{6, "verify agent network", steps.RestoreVerifyGate},
	{7, "write ready marker", steps.RestoreWriteMarker},
}

// RunRestore executes the restore pipeline and returns the process exit code.
func RunRestore(ctx context.Context, d RestoreDeps) int {
	log := services.NewRenderer(d.Stdout, d.Stderr, d.Config.LogLevel)
	log.Prog = "agent_lifecycle restore"
	runner := d.Runner
	if log.IsDebug() {
		runner = services.TracingRunner{Inner: runner, Trace: func(args []string) {
			log.Debugf("%s", strings.Join(args, " "))
		}}
	}
	env := &steps.Env{
		Config: d.Config,
		Docker: services.NewDocker(runner),
		Arm:    &services.Arm{Runner: runner, Bin: d.Config.ArmBin, RemoveBin: d.Config.RemoveNetBin},
		FS:     d.FS,
		Log:    log,
		Runner: runner,
		IsRoot: d.IsRoot,
		RemoveAgent: func(ctx context.Context, id string) (int, string) {
			var out, errb bytes.Buffer
			code := RunRemove(ctx, RemoveDeps{
				Config: d.Config, Runner: d.Runner, FS: d.FS, Args: []string{id},
				Stdout: &out, Stderr: &errb, TolerantDisarm: true,
			})
			return code, errb.String()
		},
	}

	for _, s := range restorePipeline {
		log.Step(s.number, restoreTotalSteps, s.name)
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

