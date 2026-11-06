// create_agent_host (SFR-HOST-001, USC-001 step 1): creates the isolated agent-host WSL2 distro. Only flag: --no-keepalive.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"

	"agent-host/internal/config"
	"agent-host/internal/sequence"
	"agent-host/internal/services"
)

func main() {
	// this block parses command-line arguments and sets up the context for handling interrupts
	noKeepAlive, err := config.ParseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, "usage: create_agent_host [--no-keepalive]")
		fmt.Fprintln(os.Stderr, "  --no-keepalive  do not write the Startup keep-alive script and do not start the keep-alive wsl.exe")
		fmt.Fprintln(os.Stderr, "  otherwise configure via AGENT_LOG_LEVEL and AGENT_HOST_* environment variables")
		os.Exit(1)
	}
	// this block loads the configuration and sets the KeepAlive flag based on the command-line argument
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt) // create a context that is canceled on interrupt signal, it is necessary to call stop() to release resources when done
	cfg := config.Load(os.Getenv)                                         // load configuration from environment variables
	cfg.KeepAlive = !noKeepAlive
	code := sequence.Run(ctx, sequence.Deps{ // run the sequence with the provided dependencies and context
		Config:     cfg,
		Runner:     services.ExecRunner{Exe: config.WSLExe},
		HTTPClient: &http.Client{},
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
	})
	stop()
	os.Exit(code)
}
