// create_agent_container (USC-002 step 1): creates one isolated gVisor agent container. Takes one argument, agentID.
// Runs as root inside agent-host.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"agent-sandbox/internal/config"
	"agent-sandbox/internal/sequence"
	"agent-sandbox/internal/services"
	"agent-sandbox/internal/steps"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, steps.Usage)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	cfg := config.Load(os.Getenv)
	code := sequence.Run(ctx, sequence.Deps{
		Config:  cfg,
		Runner:  services.ExecRunner{},
		FS:      services.OSFS{},
		AgentID: os.Args[1],
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	})
	stop()
	os.Exit(code)
}
