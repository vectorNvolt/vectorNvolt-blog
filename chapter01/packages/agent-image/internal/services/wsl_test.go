package services

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

// TestHelperProcess is the child of TestExecRunnerKillsChildOnCancel: it only sleeps.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	time.Sleep(60 * time.Second)
	os.Exit(0)
}

func TestCRLFWriter(t *testing.T) {
	cases := []struct {
		name   string
		writes []string
		want   string
	}{
		{"bare LF", []string{"a\nb\n"}, "a\r\nb\r\n"},
		{"existing CRLF", []string{"a\r\nb"}, "a\r\nb"},
		{"CRLF split across writes", []string{"a\r", "\nb"}, "a\r\nb"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			w := &crlfWriter{w: &buf}
			for _, s := range tc.writes {
				n, err := w.Write([]byte(s))
				if err != nil || n != len(s) {
					t.Fatalf("Write(%q) = %d, %v; want %d, nil", s, n, err, len(s))
				}
			}
			if got := buf.String(); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// SFR-IMG-018: cancelling the context kills the child and Run returns promptly.
func TestExecRunnerKillsChildOnCancel(t *testing.T) {
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(100*time.Millisecond, cancel)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ExecRunner{Exe: os.Args[0]}.Run(ctx, nil, nil, "-test.run=^TestHelperProcess$")
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}

// ---- stub runner: records the last call ----

type stubRunner struct {
	stdout string
	stderr []byte
	code   int
	got    []string
	live   io.Writer
}

func (s *stubRunner) Run(_ context.Context, _ []byte, live io.Writer, args ...string) ([]byte, []byte, int, error) {
	s.got = args
	s.live = live
	return []byte(s.stdout), s.stderr, s.code, nil
}

func TestExecArgsAreRootInDistro(t *testing.T) {
	sr := &stubRunner{}
	if _, err := NewWSL(sr).Exec(context.Background(), "agent-host", "docker", "version"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(sr.got, " ") != "-d agent-host -u root -e docker version" {
		t.Errorf("args = %v", sr.got)
	}
}

func TestExecNonZeroCarriesStderr(t *testing.T) {
	sr := &stubRunner{code: 1, stderr: []byte("boom")}
	_, err := NewWSL(sr).Exec(context.Background(), "agent-host", "docker", "version")
	var we *WSLError
	if !errors.As(err, &we) || we.ExitCode != 1 || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v", err)
	}
}

func TestDecodeTextUTF16(t *testing.T) {
	u := utf16.Encode([]rune("hi"))
	b := []byte{0xFF, 0xFE}
	for _, c := range u {
		b = append(b, byte(c), byte(c>>8))
	}
	if got := decodeText(b); got != "hi" {
		t.Errorf("decodeText = %q", got)
	}
}
