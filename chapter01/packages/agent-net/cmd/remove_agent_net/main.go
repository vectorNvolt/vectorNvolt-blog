// remove_agent_net (USC-007): removes every agent container on agents-net (their workspaces are kept), the
// leftover as_port_* nft tables, the leftover agent-net: rules and the network itself (no arguments). Runs as root
// inside agent-host.
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
	code := sequence.RunRemove(ctx, sequence.Deps{
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
