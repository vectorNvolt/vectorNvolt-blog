// Sequence assembly: runs USC-001 steps 2..13 in order and maps the first failure to an exit code.
// No rollback is performed (open point in the design); the failure report states the exit_state.
package sequence

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/ProtonMail/go-crypto/openpgp"

	"agent-host/internal/config"
	"agent-host/internal/services"
	"agent-host/internal/steps"
)

// Deps are the injected collaborators; tests replace Runner and HTTPClient.
type Deps struct {
	Config     config.Config
	Runner     services.Runner
	HTTPClient *http.Client
	Stdout     io.Writer
	Stderr     io.Writer
	Keyring    openpgp.EntityList // tests only: replaces the pinned Ubuntu signing key
	Starter    services.Starter   // detached process start; nil means services.DetachedStarter
}

type stepDef struct {
	number int
	name   string
	run    func(context.Context, *steps.Env) *steps.Failure
}

const totalSteps = 13

// Step numbers match USC-001 main_flow (step 1 is the CLI invocation itself).
var pipeline = []stepDef{
	{2, "confirm no agent-host distro exists", steps.ConfirmNoDistro},
	{3, "read tool builds and compute reference SHA-256", steps.ObtainToolHashes},
	{4, "obtain and verify rootfs", steps.ObtainRootfs},
	{5, "register distro", steps.RegisterDistro},
	{6, "create default user", steps.CreateUser},
	{7, "write /etc/wsl.conf", steps.WriteWSLConf},
	{8, "terminate distro to apply configuration", steps.Terminate},
	{9, "install dependencies", steps.InstallDeps},
	{10, "register runsc runtime and restart docker", steps.RegisterRuntime},
	{11, "install agent tools", steps.InstallTools},
	{12, "verify acceptance criteria", steps.Verify},
	{13, "report success", steps.ReportSuccess},
}

// Run executes the pipeline and returns the process exit code.
func Run(ctx context.Context, d Deps) int {
	log := services.NewRenderer(d.Stdout, d.Stderr, d.Config.LogLevel)
	runner := d.Runner
	if log.IsDebug() {
		runner = services.TracingRunner{Inner: runner, Trace: func(args []string) {
			log.Debugf("%s %s", config.WSLExe, strings.Join(args, " "))
		}}
	}
	starter := d.Starter
	if starter == nil {
		starter = services.DetachedStarter{}
	}
	env := &steps.Env{
		Config: d.Config,
		WSL:    services.NewWSL(runner),
		Rootfs: &services.RootfsProvider{
			Client:    d.HTTPClient,
			UserAgent: config.UserAgent,
			Timeout:   d.Config.DownloadTimeout,
			Attempts:  d.Config.DownloadAttempt,
			Backoff:   d.Config.Backoff,
			Keyring:   d.Keyring,
		},
		Log: log,
	}
	env.Starter = starter
	defer func() {
		if env.RootfsPath != "" {
			os.Remove(env.RootfsPath)
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
