// create_agent_host (SFR-HOST-001, USC-001 step 1): customizes the foundation VM into the agent-host.
// Built natively for Linux and run as root inside the VM (sudo create_agent_host). No flags.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"agent-host/internal/config"
	"agent-host/internal/sequence"
	"agent-host/internal/services"
)

func main() {
	if err := config.ParseArgs(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, "usage: create_agent_host")
		fmt.Fprintln(os.Stderr, "  run as root (sudo) inside the foundation VM; configure via AGENT_LOG_LEVEL and AGENT_HOST_* environment variables")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt) // canceled on interrupt; stop() releases resources
	code := sequence.Run(ctx, sequence.Deps{
		Config: config.Load(os.Getenv),
		Runner: services.ExecRunner{},
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	})
	stop()
	os.Exit(code)
}
