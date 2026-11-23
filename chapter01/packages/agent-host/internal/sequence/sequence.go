// Sequence assembly: runs the USC-001 steps in order and maps the first failure to an exit code.
// No rollback is performed (open point in the design); the failure report states the exit_state.
package sequence

import (
	"context"
	"io"
	"strings"

	"agent-host/internal/config"
	"agent-host/internal/services"
	"agent-host/internal/steps"
)

// Deps are the injected collaborators; tests replace Runner and System.
type Deps struct {
	Config config.Config
	Runner services.HostRunner
	Stdout io.Writer
	Stderr io.Writer
	System *steps.System // nil means the real machine (steps.RealSystem)
}

type stepDef struct {
	number int
	name   string
	run    func(context.Context, *steps.Env) *steps.Failure
}

const totalSteps = 9

// The steps match USC-001 main_flow; step 1 is the preflight check of the foundation.
var pipeline = []stepDef{
	{1, "check foundation (root, Linux, " + config.FoundationMarkerPath + ")", steps.PreflightFoundation},
	{2, "read tool builds and compute reference SHA-256", steps.ObtainToolHashes},
	{3, "create default user", steps.CreateUser},
	{4, "set hostname", steps.SetHostname},
	{5, "install dependencies", steps.InstallDeps},
	{6, "register runsc runtime and restart docker", steps.RegisterRuntime},
	{7, "install agent tools", steps.InstallTools},
	{8, "verify acceptance criteria", steps.Verify},
	{9, "report success", steps.ReportSuccess},
}

// Run executes the pipeline and returns the process exit code.
func Run(ctx context.Context, d Deps) int {
	log := services.NewRenderer(d.Stdout, d.Stderr, d.Config.LogLevel)
	runner := d.Runner
	if log.IsDebug() {
		runner = services.TracingRunner{Inner: runner, Trace: func(args []string) {
			log.Debugf("%s", strings.Join(args, " "))
		}}
	}
	sys := steps.RealSystem()
	if d.System != nil {
		sys = *d.System
	}
	env := &steps.Env{
		Config: d.Config,
		Host:   services.NewHost(runner),
		Log:    log,
		System: sys,
	}

	for _, s := range pipeline {
		log.Step(s.number, totalSteps, s.name)
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
