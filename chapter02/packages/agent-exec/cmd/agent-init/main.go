//go:build linux

// Command agent-init is the init supervisor of the agent container (USS-005). tini is PID 1 and starts
// it as root: it starts the authorization plugin ephemeral-authz as user authz, then dockerd, restarts the
// plugin if it exits and exits with dockerd's exit code.
package main

import (
	"os"
	"os/signal"
	"syscall"

	"agent-exec/internal/agentinit"
)

func main() {
	sigs := make(chan os.Signal, 4)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	i := &agentinit.Init{
		SocketPath: agentinit.SocketPath,
		PluginPath: agentinit.PluginPath,
		PluginUser: agentinit.PluginUser,
		Starter:    agentinit.ExecStarter{},
	}
	os.Exit(i.Run(os.Args[1:], sigs))
}
