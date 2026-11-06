// refresh_endpoints.exe [--dry-run]: Windows side of refresh_endpoints. Reads endpoints.json from the current working
// directory, validates it, and sends the bytes on stdin to refresh_endpoints in the agent-host distro via wsl.exe.
// Build: GOOS=windows GOARCH=amd64 go build -o ../../dist/refresh_endpoints.exe ./cmd/refresh_endpoints_win
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"agent-net/internal/winrefresh"
)

func main() {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot determine the current directory: %v\n", err)
		os.Exit(winrefresh.ExitPrerequisite)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := winrefresh.Run(ctx, winrefresh.Deps{
		Args:   os.Args[1:],
		Dir:    dir,
		WSL:    winrefresh.ExecWSL{Exe: winrefresh.WSLExe},
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	})
	stop()
	os.Exit(code)
}
