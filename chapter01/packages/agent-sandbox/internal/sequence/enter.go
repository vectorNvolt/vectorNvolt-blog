// Sequence for enter_agent: make sure agent-<id> exists, runs and passes the gate check (creating or recreating it when
// needed), then replace the process with `docker exec -it -u 1001 agent-<id> <cmd>`.
// A running container is never removed: a failing gate check is reported and the exit code of check-gate is returned.
package sequence

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"agent-sandbox/internal/config"
	"agent-sandbox/internal/services"
	"agent-sandbox/internal/steps"
)

// EnterUsage is printed for a missing or invalid argument.
const EnterUsage = "usage: enter_agent <agentID> [-- <cmd> [args...]]\n" +
	"  agentID must match " + config.AgentIDPattern + " (lowercase letters, digits and -; starts with a letter; 1 to 31 characters)\n" +
	"  default command: claude --continue when a previous conversation exists, else claude"

// StateNotEntered is the exit state of every enter_agent failure: the process was not replaced by docker exec.
const StateNotEntered = "NOT_ENTERED"

// EnterDeps are the injected collaborators; tests replace Runner, FS, LookPath and Exec.
type EnterDeps struct {
	Config      config.Config
	Runner      services.Runner
	FS          services.FS
	Args        []string // arguments after the program name: <agentID> [-- <cmd> [args...]]
	IsRoot      bool
	Interactive bool // stdin is a terminal: docker exec -it, else -i
	Stdout      io.Writer
	Stderr      io.Writer

	// LookPath finds the docker binary (exec.LookPath when nil); Exec replaces the process (syscall.Exec when nil).
	LookPath func(file string) (string, error)
	Exec     func(path string, argv []string, env []string) error
}

// RunEnter returns the process exit code; on success Exec normally does not return.
func RunEnter(ctx context.Context, d EnterDeps) int {
	failEnter := func(code int, format string, a ...any) int {
		fmt.Fprintf(d.Stderr, "enter_agent: "+format+"\n  exit_state: %s\n  exit_code: %d\n", append(a, StateNotEntered, code)...)
		return code
	}
	if !d.IsRoot {
		return failEnter(steps.ExitPrerequisite, "must run as root")
	}
	id, cmd, ok := parseEnterArgs(d.Args)
	if !ok {
		return failEnter(steps.ExitPrerequisite, "%s", strings.ReplaceAll(EnterUsage, "\n", "\n  "))
	}
	container := config.ContainerPrefix + id

	runner := d.Runner
	log := services.NewRenderer(d.Stdout, d.Stderr, d.Config.LogLevel)
	if log.IsDebug() {
		runner = services.TracingRunner{Inner: runner, Trace: func(args []string) {
			log.Debugf("%s", strings.Join(args, " "))
		}}
	}
	docker := services.NewDocker(runner)
	arm := &services.Arm{Runner: runner, Bin: d.Config.ArmBin}

	state, err := docker.State(ctx, container)
	if err != nil {
		return failEnter(steps.ExitRuntime, "cannot read the state of %s: %v", container, err)
	}
	switch state {
	case "running":
		if err := arm.CheckGate(ctx, container); err != nil {
			var ce *services.CmdError
			if !errors.As(err, &ce) || ce.Err != nil {
				return failEnter(steps.ExitRuntime, "cannot run the gate check of %s: %v", container, err)
			}
			fmt.Fprint(d.Stderr, ce.Stdout)
			if s := strings.TrimSpace(ce.Stderr); s != "" {
				fmt.Fprintln(d.Stderr, s)
			}
			return failEnter(ce.ExitCode, "the gate check of %s failed; run `remove_agent_container %s` and retry", container, id)
		}
	case "":
		if code := enterCreate(ctx, d, id); code != 0 {
			return code
		}
	default:
		fmt.Fprintf(d.Stderr, "enter_agent: %s is %s, recreating it\n", container, state)
		code := RunRemove(ctx, RemoveDeps{
			Config: d.Config, Runner: d.Runner, FS: d.FS, Args: []string{id}, Stdout: d.Stdout, Stderr: d.Stderr,
		})
		if code != 0 {
			return code
		}
		if code := enterCreate(ctx, d, id); code != 0 {
			return code
		}
	}

	if cmd == nil {
		cmd = []string{"claude"}
		if hasConversation(d.FS, d.Config, id) {
			cmd = append(cmd, "--continue")
		}
	}

	lookPath, execFn := d.LookPath, d.Exec
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	if execFn == nil {
		execFn = syscall.Exec
	}
	bin, err := lookPath(config.DockerExe)
	if err != nil {
		return failEnter(steps.ExitRuntime, "cannot find %s: %v", config.DockerExe, err)
	}
	flags := "-i"
	if d.Interactive {
		flags = "-it"
	}
	argv := append([]string{config.DockerExe, "exec", flags, "-u", config.AgentUser, container}, cmd...)
	fmt.Fprintf(d.Stderr, "entering %s (%s)\n", container, strings.Join(cmd, " "))
	if err := execFn(bin, argv, os.Environ()); err != nil {
		return failEnter(steps.ExitRuntime, "cannot exec %s: %v", config.DockerExe, err)
	}
	return 0
}

func enterCreate(ctx context.Context, d EnterDeps, id string) int {
	return Run(ctx, Deps{
		Config: d.Config, Runner: d.Runner, FS: d.FS, AgentID: id, Stdout: d.Stdout, Stderr: d.Stderr,
	})
}

// parseEnterArgs accepts <agentID> or <agentID> -- <cmd> [args...]; cmd is nil when not given.
func parseEnterArgs(args []string) (id string, cmd []string, ok bool) {
	if len(args) == 0 || !steps.ValidAgentID(args[0]) {
		return "", nil, false
	}
	if len(args) == 1 {
		return args[0], nil, true
	}
	if args[1] != "--" || len(args) < 3 {
		return "", nil, false
	}
	return args[0], args[2:], true
}

// hasConversation reports whether <state root>/<id>/claude/projects/*/*.jsonl has a match.
func hasConversation(fs services.FS, cfg config.Config, id string) bool {
	pattern := filepath.Join(cfg.StateRoot, id, config.StateClaudeDir, "projects", "*", "*.jsonl")
	matches, err := fs.Glob(pattern)
	return err == nil && len(matches) > 0
}
