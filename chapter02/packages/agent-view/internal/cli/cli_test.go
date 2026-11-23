package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestCheckLoopback(t *testing.T) {
	ok := []string{"127.0.0.1:8787", "127.5.5.5:1", "[::1]:8787", "localhost:8787"}
	bad := []string{"0.0.0.0:8787", "192.168.1.5:8787", "[::]:8787", ":8787", "example.com:80", "8787", "127.0.0.1", "10.0.0.1:1", ""}
	for _, a := range ok {
		if err := CheckLoopback(a); err != nil {
			t.Errorf("%q rejected: %v", a, err)
		}
	}
	for _, a := range bad {
		if err := CheckLoopback(a); err == nil {
			t.Errorf("%q accepted", a)
		}
	}
}

func TestRunUsageErrors(t *testing.T) {
	ws := t.TempDir()
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"non-loopback", []string{"--ws", ws, "--listen", "0.0.0.0:0"}, "agent_view: usage: --listen must be loopback"},
		{"lan", []string{"--ws", ws, "--listen", "192.168.1.5:8787"}, "agent_view: usage: --listen must be loopback"},
		{"no ws", nil, "agent_view: usage: --ws is required"},
		{"missing ws", []string{"--ws", ws + "/nope"}, "agent_view: usage: --ws must be an existing directory"},
		{"unknown flag", []string{"--ws", ws, "--bogus"}, "agent_view: usage:"},
		{"extra arg", []string{"--ws", ws, "extra"}, "agent_view: usage:"},
	}
	for _, c := range cases {
		var buf bytes.Buffer
		if code := Run(context.Background(), c.args, &buf); code != 2 {
			t.Errorf("%s: exit %d; want 2", c.name, code)
		}
		if !strings.Contains(buf.String(), c.want) {
			t.Errorf("%s: stderr %q; want %q", c.name, buf.String(), c.want)
		}
	}
}

func TestRunServesAndStops(t *testing.T) {
	ws := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(300 * time.Millisecond)
		cancel()
	}()
	var buf bytes.Buffer
	if code := Run(ctx, []string{"--ws", ws, "--listen", "127.0.0.1:0"}, &buf); code != 0 {
		t.Fatalf("exit %d; stderr %q", code, buf.String())
	}
	want := "agent_view: serving " + ws + " on http://127.0.0.1:"
	if !strings.HasPrefix(buf.String(), want) {
		t.Errorf("stderr %q; want prefix %q", buf.String(), want)
	}
}
