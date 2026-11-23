// run_ephemeral_container runs, builds or installs application code in a fresh, throwaway container
// of the in-container Docker engine (USS-005, USC-005). Thin entry: parse, run, exit with the code of the execution.
package main

import (
	"os"

	"agent-exec/internal/app"
)

func main() {
	os.Exit(app.Main(os.Args[1:], os.Stdout, os.Stderr, os.Getenv))
}
