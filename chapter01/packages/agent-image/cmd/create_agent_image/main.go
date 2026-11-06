// create_agent_image (USC-003 step 1): builds agent-image directly in the agent-host docker engine.
// Takes an optional --tag <tag> (default latest). Runs on Windows only; every command goes through wsl.exe.
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
	tag, err := steps.ParseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, steps.Usage)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	cfg := config.Load(os.Getenv)
	code := sequence.Run(ctx, sequence.Deps{
		Config: cfg,
		Runner: services.ExecRunner{Exe: config.WSLExe},
		Tag:    tag,
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	})
	stop()
	os.Exit(code)
}
