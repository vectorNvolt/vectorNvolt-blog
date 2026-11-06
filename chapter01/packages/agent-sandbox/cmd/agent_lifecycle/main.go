// agent_lifecycle: keeps the agent sandbox consistent across docker restarts. Runs as root inside agent-host.
//
//	agent_lifecycle restore   after each docker start: remove agent-* containers, rebuild agents-net, write the ready marker
//	agent_lifecycle guard     long-running: remove agent containers started outside the tools (docker start/restart)
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"agent-sandbox/internal/config"
	"agent-sandbox/internal/sequence"
	"agent-sandbox/internal/services"
)

const usage = "usage: agent_lifecycle restore|guard"

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cfg := config.Load(os.Getenv)

	switch os.Args[1] {
	case "restore":
		code := sequence.RunRestore(ctx, sequence.RestoreDeps{
			Config: cfg,
			Runner: services.ExecRunner{},
			FS:     services.OSFS{},
			IsRoot: os.Geteuid() == 0,
			Stdout: os.Stdout,
			Stderr: os.Stderr,
		})
		stop()
		os.Exit(code)
	case "guard":
		if os.Geteuid() != 0 {
			fmt.Fprintln(os.Stderr, "agent_lifecycle guard must run as root")
			os.Exit(1)
		}
		err := sequence.RunGuard(ctx, sequence.GuardDeps{
			Config: cfg,
			Runner: services.ExecRunner{},
			FS:     services.OSFS{},
			Events: services.DockerEvents{Stderr: os.Stderr},
			Stderr: os.Stderr,
		})
		if ctx.Err() != nil {
			return // stopped by a signal: clean exit
		}
		fmt.Fprintf(os.Stderr, "agent_lifecycle guard: %v\n", err)
		stop()
		os.Exit(1)
	default:
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(1)
	}
}
