// Package cli is the agent_view command line: flag validation, listener, graceful shutdown.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"agent-view/internal/server"
	"agent-view/internal/ui"
	"agent-view/internal/wsread"
)

// DefaultListen is the loopback address used without --listen.
const DefaultListen = "127.0.0.1:8787"

// CheckLoopback fails unless addr (host:port) resolves only to loopback addresses.
func CheckLoopback(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" || host == "" {
		return errors.New("--listen must be loopback")
	}
	var ips []net.IP
	if ip := net.ParseIP(host); ip != nil {
		ips = []net.IP{ip}
	} else if host == "localhost" {
		ips, err = net.LookupIP(host)
		if err != nil || len(ips) == 0 {
			return errors.New("--listen must be loopback")
		}
	} else {
		return errors.New("--listen must be loopback")
	}
	for _, ip := range ips {
		if !ip.IsLoopback() {
			return errors.New("--listen must be loopback")
		}
	}
	return nil
}

func usage(stderr io.Writer, msg string) int {
	fmt.Fprintf(stderr, "agent_view: usage: %s\n", msg)
	return 2
}

// Run executes agent_view and returns the process exit code. It serves until ctx is cancelled.
func Run(ctx context.Context, args []string, stderr io.Writer) int {
	fl := flag.NewFlagSet("agent_view", flag.ContinueOnError)
	fl.SetOutput(io.Discard)
	ws := fl.String("ws", "", "workspace directory")
	listen := fl.String("listen", DefaultListen, "loopback listen address")
	if err := fl.Parse(args); err != nil {
		return usage(stderr, err.Error())
	}
	if fl.NArg() != 0 {
		return usage(stderr, "unexpected arguments")
	}
	if *ws == "" {
		return usage(stderr, "--ws is required")
	}
	if st, err := os.Stat(*ws); err != nil || !st.IsDir() {
		return usage(stderr, "--ws must be an existing directory")
	}
	if err := CheckLoopback(*listen); err != nil {
		return usage(stderr, err.Error())
	}
	reader, err := wsread.New(*ws)
	if err != nil {
		return usage(stderr, "--ws must be an existing directory")
	}
	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		fmt.Fprintf(stderr, "agent_view: listen: %v\n", err)
		return 1
	}
	srv := server.NewHTTPServer(ln.Addr().String(), server.New(server.Config{WS: reader, Static: ui.FS()}))
	fmt.Fprintf(stderr, "agent_view: serving %s on http://%s\n", *ws, ln.Addr())

	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	select {
	case err := <-errc:
		fmt.Fprintf(stderr, "agent_view: %v\n", err)
		return 1
	case <-ctx.Done():
	}
	sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		_ = srv.Close() // open SSE streams do not drain
	}
	return 0
}
