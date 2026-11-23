// create_agent_net (USC-004): creates the shared bridge network agents-net (no arguments), or
// arms one container on it (`arm <container>`, called by create_agent_container), or reports the state of one
// container's port controls read-only (`check-gate <container>`), or deletes one container's port table and rules
// (`disarm <container>`). Runs as root inside agent-host.
package main

import (
	"context"
	"os"
	"os/signal"

	"agent-net/internal/config"
	"agent-net/internal/sequence"
	"agent-net/internal/services"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	cfg := config.Load(os.Getenv)
	code := sequence.Run(ctx, sequence.Deps{
		Config: cfg,
		Runner: services.ExecRunner{},
		Host:   services.OSHost{},
		Args:   os.Args[1:],
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	})
	stop()
	os.Exit(code)
}
