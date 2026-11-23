// create_agent_image (USC-003 step 1): builds agent-image directly in the agent-host docker engine.
// Takes an optional --tag <tag> (default latest). Runs as root inside the VM; docker is run directly.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"agent-image/internal/config"
	"agent-image/internal/sequence"
	"agent-image/internal/services"
	"agent-image/internal/steps"
)

func main() {
	stdout := os.Stdout
	stderr := os.Stderr
	tag, rebuildBase, err := steps.ParseOptions(os.Args[1:])
	if err != nil {
		fmt.Fprintln(stderr, steps.Usage)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	cfg := config.Load(os.Getenv)
	code := sequence.Run(ctx, sequence.Deps{
		Config:      cfg,
		Runner:      services.ExecRunner{},
		Tag:         tag,
		RebuildBase: rebuildBase,
		Stdin:       os.Stdin,
		Stdout:      stdout,
		Stderr:      stderr,
	})
	stop()
	os.Exit(code)
}
