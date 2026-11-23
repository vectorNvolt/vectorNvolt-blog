// enter_agent: opens an interactive session (default: claude) in agent-<id>, creating or recreating the container first when
// it is absent or not running. Takes agentID and an optional `-- <cmd> [args...]`. Runs as root inside agent-host.
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
	fi, err := os.Stdin.Stat()
	code := sequence.RunEnter(ctx, sequence.EnterDeps{
		Config:      cfg,
		Runner:      services.ExecRunner{},
		FS:          services.OSFS{},
		Args:        os.Args[1:],
		IsRoot:      os.Geteuid() == 0,
		Interactive: err == nil && fi.Mode()&os.ModeCharDevice != 0,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
	})
	stop()
	os.Exit(code)
}
