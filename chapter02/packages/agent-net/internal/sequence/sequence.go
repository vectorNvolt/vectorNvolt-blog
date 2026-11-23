// Sequence assembly: runs the USC-004 steps in order and maps the first failure to an exit code.
// Pipelines of create_agent_net: network creation (no arguments), arming (`arm <container>`, alt-1), disarming
// (`disarm <container>`, alt-2) and the read-only port check (`check-gate <container>`, alt-3). remove_agent_net (USC-007) has its own pipeline.
// There is no rollback: a failed run leaves what it created or has not yet removed in place.
package sequence

import (
	"context"
	"io"
	"strings"

	"agent-net/internal/config"
	"agent-net/internal/services"
	"agent-net/internal/steps"
)

// Deps are the injected collaborators; tests replace Runner and Host.
type Deps struct {
	Config config.Config
	Runner services.Runner
	Host   services.Host
	Args   []string  // os.Args[1:]
	Stdin  io.Reader // refresh_endpoints: the endpoints JSON
	Stdout io.Writer
	Stderr io.Writer
}

type stepDef struct {
	number int
	name   string
	run    func(context.Context, *steps.Env) *steps.Failure
}

// createPipeline is also selected for an invalid invocation; its step 1 then fails with the usage text.
var createPipeline = []stepDef{
	{1, "validate invocation", steps.ValidateInvocation},
	{2, "check privilege and engine", steps.CheckPrivilege},
	{3, "refuse existing network", steps.RefuseExisting},
	{4, "check address range clash", steps.CheckRangeClash},
	{5, "record engine state", steps.RecordEngineState},
	{6, "create network", steps.CreateNetwork},
	{7, "install network gate", steps.InstallGate},
	{8, "verify network", steps.VerifyNetwork},
	{9, "report success", steps.ReportCreated},
}

// armPipeline (USC-004 alt-1). The container is created on agents-net (attached at `docker create`, gated closed by the
// network gate) and paused by the caller; steps 4 and 5 verify the gate and the container before anything is
// changed. The port table is loaded closed right after the veth is identified (step 6, file s10); the accept rules
// and the MAC in the gate set come with the release, the last operation (step 9).
var armPipeline = []stepDef{
	{1, "validate invocation", steps.ValidateInvocation},
	{2, "check privilege and engine", steps.CheckPrivilege},
	{3, "record engine state", steps.RecordEngineState},
	{4, "require agents-net and its gate", steps.RequireNetwork},
	{5, "require paused container", steps.RequirePaused},
	{6, "clean slate (name-keyed disarm)", steps.CleanSlate},
	{7, "identify host veth", steps.IdentifyPort},
	{8, "close port (load port table)", steps.ClosePort},
	{9, "apply host-side controls", steps.ApplyControls},
	{10, "verify controls", steps.VerifyControls},
	{11, "release port", steps.ReleasePort},
	{12, "verify engine state", steps.VerifyEngineUnchanged},
	{13, "report armed", steps.ReportArmed},
}

// releasedPipeline lists the elements of the gate set `released` (read only); evictPipeline removes one by MAC.
var releasedPipeline = []stepDef{
	{1, "validate invocation", steps.ValidateInvocation},
	{2, "check privilege and engine", steps.CheckPrivilege},
	{3, "record engine state", steps.RecordEngineState},
	{4, "list released elements", steps.ListReleased},
	{5, "verify engine state", steps.VerifyEngineUnchanged},
}

var evictPipeline = []stepDef{
	{1, "validate invocation", steps.ValidateInvocation},
	{2, "check privilege and engine", steps.CheckPrivilege},
	{3, "record engine state", steps.RecordEngineState},
	{4, "evict element", steps.EvictElement},
	{5, "verify engine state", steps.VerifyEngineUnchanged},
}

// checkGatePipeline (USC-004 alt-3) changes nothing: it prints the state of each port control and the port state.
var checkGatePipeline = []stepDef{
	{1, "validate invocation", steps.ValidateInvocation},
	{2, "check privilege and engine", steps.CheckPrivilege},
	{3, "record engine state", steps.RecordEngineState},
	{4, "require agents-net", steps.RequireNetwork},
	{5, "inspect container", steps.CheckInspect},
	{6, "check port controls", steps.CheckPortControls},
	{7, "verify engine state", steps.VerifyEngineUnchanged},
}

// disarmPipeline takes the MAC out of the gate set, deletes the port table, then the iptables rules, then the static
// fdb entry of one container's port (USC-004 alt-2). The same step functions (19..23, 37) run per container inside
// removePipeline.
var disarmPipeline = []stepDef{
	{1, "validate invocation", steps.ValidateInvocation},
	{2, "check privilege and engine", steps.CheckPrivilege},
	{3, "record engine state", steps.RecordEngineState},
	{4, "require agents-net", steps.DisarmRequireNetwork},
	{5, "inspect container", steps.DisarmInspect},
	{6, "find ports", steps.DisarmFindPorts},
	{7, "remove MAC from gate", steps.DisarmCloseGate},
	{8, "delete port table", steps.DisarmDeleteTable},
	{9, "remove port rules", steps.DisarmRemoveRules},
	{10, "verify port state gone", steps.DisarmVerify},
	{11, "verify engine state", steps.VerifyEngineUnchanged},
	{12, "report disarmed", steps.ReportDisarmed},
}

// removePipeline is remove_agent_net (USC-007): containers first (created but never armed ones included, they are
// attached to agents-net at creation), then leftover port tables, leftover rules (the gate rule too), the gate table
// and the network. Steps 7-10 record a failure and let the later steps run; step 11 reports it.
var removePipeline = []stepDef{
	{1, "validate invocation", steps.ValidateRemoveInvocation},
	{2, "check privilege and engine", steps.CheckPrivilege},
	{3, "record engine state", steps.RecordEngineState},
	{4, "survey setup", steps.SurveyRemoval},
	{5, "refuse foreign containers", steps.RefuseForeign},
	{6, "remove agent containers", steps.RemoveAgents},
	{7, "remove leftover port tables", steps.RemovePortTables},
	{8, "remove leftover rules", steps.RemoveLeftoverRules},
	{9, "remove gate table", steps.RemoveGate},
	{10, "remove network", steps.RemoveNetwork},
	{11, "verify removal", steps.VerifyRemoved},
	{12, "verify engine state", steps.VerifyEngineUnchanged},
	{13, "report removed", steps.ReportRemoved},
}

func pipelineFor(args []string) []stepDef {
	if len(args) == 2 && args[0] == "arm" {
		return armPipeline
	}
	if len(args) == 2 && args[0] == "disarm" {
		return disarmPipeline
	}
	if len(args) == 2 && args[0] == "check-gate" {
		return checkGatePipeline
	}
	if len(args) == 1 && args[0] == "released" {
		return releasedPipeline
	}
	if len(args) == 2 && args[0] == "evict" {
		return evictPipeline
	}
	return createPipeline
}

// Run executes the create_agent_net pipeline selected by the arguments and returns the process exit code.
func Run(ctx context.Context, d Deps) int {
	return execute(ctx, d, "create_agent_net", pipelineFor(d.Args))
}

// RunRemove executes the remove_agent_net pipeline and returns the process exit code.
func RunRemove(ctx context.Context, d Deps) int {
	return execute(ctx, d, "remove_agent_net", removePipeline)
}

// refreshPipeline is refresh_endpoints: all lookups happen before the file is written and the first rule changes.
var refreshPipeline = []stepDef{
	{1, "validate invocation and read endpoints from stdin", steps.ValidateRefreshInvocation},
	{2, "check privilege", steps.RefreshCheckPrivilege},
	{3, "resolve endpoints", steps.ResolveEndpoints},
	{4, "install endpoints file", steps.InstallEndpointsFile},
	{5, "update endpoint rules", steps.UpdateEndpointRules},
}

// RunRefresh executes the refresh_endpoints pipeline and returns the process exit code.
func RunRefresh(ctx context.Context, d Deps) int {
	return execute(ctx, d, "refresh_endpoints", refreshPipeline)
}

func execute(ctx context.Context, d Deps, tool string, pipeline []stepDef) int {
	log := services.NewRenderer(d.Stdout, d.Stderr, d.Config.LogLevel)
	log.Tool = tool
	// check-gate is a report: stdout carries only the control lines and the summary.
	// `released` is a listing: stdout carries only its lines.
	log.ProgressToErr = tool == "create_agent_net" &&
		((len(d.Args) == 2 && d.Args[0] == "check-gate") || (len(d.Args) >= 1 && d.Args[0] == "released"))
	runner := d.Runner
	if log.IsDebug() {
		runner = services.TracingRunner{Inner: runner, Trace: func(args []string) {
			log.Debugf("%s", strings.Join(args, " "))
		}}
	}
	dk := services.NewDocker(runner)
	dk.Socket = d.Config.DockerSocket // the CLI uses the socket that step 2 checks
	env := &steps.Env{
		Config: d.Config,
		Docker: dk,
		Net:    services.NewNet(runner),
		Host:   d.Host,
		Log:    log,
		Args:   d.Args,
		Stdin:  d.Stdin,
	}

	for _, s := range pipeline {
		log.Step(s.number, len(pipeline), s.name)
		if f := s.run(ctx, env); f != nil {
			log.Failure(services.FailureReport{
				Step:      s.number,
				Name:      s.name,
				Message:   f.Message,
				Control:   f.Control,
				ExitCode:  f.ExitCode,
				ExitState: f.ExitState,
				Checks:    f.Checks,
			})
			return f.ExitCode
		}
		if env.Finished { // the step reported the final outcome itself (nothing to do)
			return 0
		}
	}
	return 0
}
