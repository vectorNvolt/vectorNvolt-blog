// Sequence assembly: runs the USC-002 steps in order and maps the first failure to an exit code.
// Cleanup is limited to the container this run created (done inside the failing step).
package sequence

import (
	"context"
	"io"
	"strings"

	"agent-sandbox/internal/config"
	"agent-sandbox/internal/services"
	"agent-sandbox/internal/steps"
)

// Deps are the injected collaborators; tests replace Runner and FS.
type Deps struct {
	Config  config.Config
	Runner  services.Runner
	FS      services.FS
	AgentID string
	Stdout  io.Writer
	Stderr  io.Writer
}

type stepDef struct {
	number int
	name   string
	run    func(context.Context, *steps.Env) *steps.Failure
}

const totalSteps = 14

var pipeline = []stepDef{
	{1, "validate agentID", steps.ValidateID},
	{2, "acquire agent lock", steps.AcquireLockCreate},
	{3, "wait for agent network ready", steps.WaitNetReady},
	{4, "check preconditions", steps.CheckPreconditions},
	{5, "prepare workspace folder", steps.PrepareWorkspace},
	{6, "prepare agent state and resolv.conf", steps.PrepareState},
	{7, "clear leftover network state", steps.CleanLeftovers},
	{8, "create and start container", steps.CreateAndStart},
	{9, "pause container", steps.PauseContainer},
	{10, "arm agent network", steps.ArmNetwork},
	{11, "release container", steps.ReleaseContainer},
	{12, "check resolv.conf", steps.CheckResolvConf},
	{13, "verify final state", steps.Verify},
	{14, "report success", steps.ReportSuccess},
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
	env := &steps.Env{
		Config:  d.Config,
		Docker:  services.NewDocker(runner),
		Arm:     &services.Arm{Runner: runner, Bin: d.Config.ArmBin},
		FS:      d.FS,
		Log:     log,
		AgentID: d.AgentID,
		Runner:  runner,
	}
	// The per-agent lock (step 2) is held for the whole run, including start, pause and unpause.
	defer func() {
		if env.Unlock != nil {
			env.Unlock()
		}
	}()

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
