// agent_view: read-only loopback HTTP viewer for ngspice exec results in a workspace.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"agent-view/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.Run(ctx, os.Args[1:], os.Stderr)
	stop()
	os.Exit(code)
}
