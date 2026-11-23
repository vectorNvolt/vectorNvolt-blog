// Command ephemeral-authz is the Docker legacy authorization plugin of the agent
// container (SSR-AGE-007). The policy is compiled in; nothing is read from /workspace.
//
// The listening socket is passed by agent-init (root) as inherited fd 3; --socket
// binds a socket itself (tests and development only).
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"agent-exec/internal/authz"
)

func listen(path string) (net.Listener, error) {
	if path != "" {
		_ = os.Remove(path)
		l, err := net.Listen("unix", path)
		if err != nil {
			return nil, err
		}
		_ = os.Chmod(path, 0o600)
		return l, nil
	}
	f := os.NewFile(3, "authz-socket")
	if f == nil {
		return nil, fmt.Errorf("no inherited socket on fd 3")
	}
	defer f.Close()
	return net.FileListener(f)
}

func main() {
	socket := flag.String("socket", "", "bind this unix socket path (default: use the inherited listening socket on fd 3)")
	flag.Parse()

	l, err := listen(*socket)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ephemeral-authz: listen:", err)
		os.Exit(1)
	}
	srv := &http.Server{
		Handler:           authz.NewHandler(authz.Default),
		ReadHeaderTimeout: 10 * time.Second,
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sig
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	if err := srv.Serve(l); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, "ephemeral-authz: serve:", err)
		os.Exit(1)
	}
}
