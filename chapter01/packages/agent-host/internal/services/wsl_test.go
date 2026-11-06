package services

import (
	"context"
	"strings"
	"testing"
	"unicode/utf16"
)

func utf16le(s string, bom bool) []byte {
	var b []byte
	if bom {
		b = append(b, 0xFF, 0xFE)
	}
	for _, u := range utf16.Encode([]rune(s)) {
		b = append(b, byte(u), byte(u>>8))
	}
	return b
}

const listing = "  NAME            STATE           VERSION\r\n* Ubuntu          Running         2\r\n  agent-host      Stopped         2\r\n  old-distro      Stopped         1\r\n"

func TestParseDistroListUTF16(t *testing.T) {
	got := parseDistroList(utf16le(listing, true))
	if got["Ubuntu"] != 2 || got["agent-host"] != 2 || got["old-distro"] != 1 || len(got) != 3 {
		t.Fatalf("unexpected result: %v", got)
	}
}

func TestParseDistroListUTF8Marker(t *testing.T) {
	got := parseDistroList([]byte(listing))
	if got["Ubuntu"] != 2 || len(got) != 3 {
		t.Fatalf("unexpected result: %v", got)
	}
}

func TestParseDistroListEmpty(t *testing.T) {
	got := parseDistroList([]byte("Windows Subsystem for Linux has no installed distributions.\r\n"))
	if len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}

func TestParseDistroListUnparseableVersionStillListed(t *testing.T) {
	got := parseDistroList([]byte("  NAME        STATE      VERSION\r\n  Agent-Host  Stopped    ?\r\n  Ubuntu      Running    2\r\n"))
	if v, ok := got["Agent-Host"]; !ok || v != UnknownWSLVersion || got["Ubuntu"] != 2 || len(got) != 2 {
		t.Fatalf("unexpected result: %v", got)
	}
}

func TestFindDistroCaseInsensitive(t *testing.T) {
	d := map[string]int{"Agent-Host": 1, "Ubuntu": 2}
	name, v, ok := FindDistro(d, "agent-host")
	if !ok || name != "Agent-Host" || v != 1 {
		t.Fatalf("got %q %d %v", name, v, ok)
	}
	if _, _, ok := FindDistro(d, "other"); ok {
		t.Fatal("unexpected match")
	}
}

func TestParseWSLVersion(t *testing.T) {
	for in, want := range map[string]string{
		"WSL version: 2.4.10.0\r\nKernel version: 6.6.87.2-1\r\n": "2.4.10.0",
		"WSL-Version: 2.6.1.0\r\n":                                "2.6.1.0",
		"\r\nWSL-Version: 2.3.26.0\r\n":                           "2.3.26.0",
	} {
		for _, raw := range [][]byte{[]byte(in), utf16le(in, true)} {
			if got, err := parseWSLVersion(raw); err != nil || got != want {
				t.Errorf("parseWSLVersion(%q) = %q, %v; want %q", in, got, err, want)
			}
		}
	}
	if _, err := parseWSLVersion([]byte("Invalid command line option: --version\r\n")); err == nil {
		t.Error("expected an error for usage text")
	}
}

func TestVersionAtLeast(t *testing.T) {
	for v, want := range map[string]bool{
		"2.4.10.0": true, "2.4.10": true, "2.6.1.0": true, "2.4.9.0": false, "2.3.26.0": false, "1.9.99": false, "3": true, "2.4": false,
	} {
		if got := VersionAtLeast(v, "2.4.10"); got != want {
			t.Errorf("VersionAtLeast(%q) = %v, want %v", v, got, want)
		}
	}
}

type recRunner struct{ calls [][]string }

func (r *recRunner) Run(_ context.Context, _ []byte, args ...string) ([]byte, []byte, int, error) {
	r.calls = append(r.calls, args)
	return nil, nil, 0, nil
}

func TestAdapterPinsDistro(t *testing.T) {
	r := &recRunner{}
	w := NewWSL(r)
	ctx := context.Background()
	_, _ = w.Exec(ctx, "root", nil, "true")
	_, _ = w.ExecDefaultUser(ctx, nil, "id", "-un")
	_ = w.Terminate(ctx)
	_ = w.Import(ctx, "C:\\x", "C:\\y.tar")
	want := []string{
		"-d agent-host -u root -e true",
		"-d agent-host -e id -un",
		"--terminate agent-host",
		"--import agent-host C:\\x C:\\y.tar --version 2",
	}
	for i, c := range r.calls {
		if got := strings.Join(c, " "); got != want[i] {
			t.Errorf("call %d = %q, want %q", i, got, want[i])
		}
	}
}
