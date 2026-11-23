// remove_agent_container (USC-006 step 1): disarms and removes one agent container. Takes agentID and the optional
// --purge-workspace and --purge (agent state). Runs as root inside agent-host.
package main

import (
	"context"
	"os"
	"os/signal"

	"agent-sandbox/internal/config"
	"agent-sandbox/internal/sequence"
	"agent-sandbox/internal/services"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	cfg := config.Load(os.Getenv)
	code := sequence.RunRemove(ctx, sequence.RemoveDeps{
		Config: cfg,
		Runner: services.ExecRunner{},
		FS:     services.OSFS{},
		Args:   os.Args[1:],
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	})
	stop()
	os.Exit(code)
}
