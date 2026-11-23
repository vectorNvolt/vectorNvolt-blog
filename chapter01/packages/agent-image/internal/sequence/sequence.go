// Sequence assembly: runs the USC-003 steps in order and maps the first failure to an exit code.
// No rollback by design.
package sequence

import (
	"bufio"
	"context"
	"io"

	"agent-image/internal/config"
	"agent-image/internal/services"
	"agent-image/internal/steps"
)

// Deps are the injected collaborators; tests replace Runner.
type Deps struct {
	Config      config.Config
	Runner      services.Runner
	Tag         string
	RebuildBase bool // --rebuild-base: always build the base image
	Stdin       io.Reader
	Stdout      io.Writer
	Stderr      io.Writer
}

type stepDef struct {
	number int
	name   string
	run    func(context.Context, *steps.Env) *steps.Failure
}

// The apt packages (snapshot.ubuntu.com) are built into the base image of step 5, only when it is missing.
const totalSteps = 8

var pipeline = []stepDef{
	{1, "validate tag and build files", steps.ValidateTag},
	{2, "check agent-host docker engine", steps.CheckEngine},
	{3, "check for existing image", steps.CheckExisting},
	{4, "confirm overwrite", steps.ConfirmOverwrite},
	{5, "ensure base image in agent-host", steps.EnsureBase},
	{6, "build image in agent-host", steps.BuildImage},
	{7, "verify image in agent-host", steps.VerifyImage},
	{8, "report success", steps.ReportSuccess},
}

// Run executes the pipeline and returns the process exit code.
func Run(ctx context.Context, d Deps) int {
	log := services.NewRenderer(d.Stdout, d.Stderr)
	host := services.NewDocker(services.NewHost(d.Runner))
	host.Live = log.Err // build output goes to stderr; stdout stays clean
	env := &steps.Env{
		Config:      d.Config,
		Host:        host,
		Log:         log,
		In:          bufio.NewReader(d.Stdin),
		Tag:         d.Tag,
		RebuildBase: d.RebuildBase,
	}

	for _, s := range pipeline {
		if ctx.Err() != nil { // SFR-IMG-018: no step starts after an interrupt
			log.Interrupted(s.number, s.name)
			return steps.ExitInterrupted
		}
		log.Step(s.number, totalSteps, s.name)
		f := s.run(ctx, env)
		if ctx.Err() != nil { // SFR-IMG-018: the interrupt overrides the step's own result
			log.Interrupted(s.number, s.name)
			return steps.ExitInterrupted
		}
		if f != nil {
			if f.ExitCode == 0 { // declined overwrite: normal outcome, not an error
				log.Success(f.Message)
				return 0
			}
			log.Failure(services.FailureReport{
				Step:      s.number,
				Name:      s.name,
				Message:   f.Message,
				ExitCode:  f.ExitCode,
				ExitState: f.ExitState,
			})
			return f.ExitCode
		}
	}
	return 0
}
