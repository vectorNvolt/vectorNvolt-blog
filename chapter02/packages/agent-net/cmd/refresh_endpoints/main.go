// refresh_endpoints [--dry-run]: reads the endpoints JSON from stdin, installs it as the endpoints file, re-resolves it and updates the endpoint accept rules of the armed
// containers in DOCKER-USER, without re-arming them. Runs as root inside agent-host.
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
	code := sequence.RunRefresh(ctx, sequence.Deps{
		Config: cfg,
		Runner: services.ExecRunner{},
		Host:   services.OSHost{},
		Args:   os.Args[1:],
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	})
	stop()
	os.Exit(code)
}
