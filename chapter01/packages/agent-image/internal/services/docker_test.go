package services

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestDockerCommandShapes(t *testing.T) {
	ctx := context.Background()
	sr := &stubRunner{}
	d := NewDocker(NewWSL(sr), "agent-host")
	cases := []struct {
		name string
		do   func() error
		want string
	}{
		{"version", func() error { return d.Version(ctx) }, "-d agent-host -u root -e docker version"},
		{"build", func() error { return d.Build(ctx, "Dockerfile.agent", "agent-image:t", []byte("tar")) },
			"-d agent-host -u root -e docker build --progress=plain -f Dockerfile.agent -t agent-image:t -"},
		{"inspect", func() error { return d.Inspect(ctx, "agent-image:t") },
			"-d agent-host -u root -e docker image inspect agent-image:t"},
	}
	for _, c := range cases {
		if err := c.do(); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := strings.Join(sr.got, " "); got != c.want {
			t.Errorf("%s args = %s, want %s", c.name, got, c.want)
		}
	}
}

func TestOnlyBuildGetsTheLiveWriter(t *testing.T) {
	ctx := context.Background()
	sr := &stubRunner{}
	var live bytes.Buffer
	d := NewDocker(NewWSL(sr), "agent-host")
	d.Live = &live
	d.Version(ctx)
	d.Inspect(ctx, "agent-image:t")
	if sr.live != nil {
		t.Error("version and inspect must stay quiet")
	}
	if err := d.Build(ctx, "Dockerfile.agent", "agent-image:t", []byte("tar")); err != nil {
		t.Fatal(err)
	}
	if sr.live != io.Writer(&live) {
		t.Error("build must receive the live writer")
	}
}

func TestImageExistsAbsentOnlyOnNoSuchImage(t *testing.T) {
	ctx := context.Background()
	sr := &stubRunner{code: 1, stderr: []byte("Error response from daemon: No such image: agent-image:latest")}
	ok, err := NewDocker(NewWSL(sr), "agent-host").ImageExists(ctx, "agent-image:latest")
	if err != nil || ok {
		t.Fatalf("no such image: ok=%v err=%v", ok, err)
	}
	sr.code, sr.stderr = 0, nil
	if ok, err := NewDocker(NewWSL(sr), "agent-host").ImageExists(ctx, "agent-image:latest"); err != nil || !ok {
		t.Fatalf("present: ok=%v err=%v", ok, err)
	}
}

type failingRunner struct{}

func (failingRunner) Run(context.Context, []byte, io.Writer, ...string) ([]byte, []byte, int, error) {
	return nil, nil, -1, errors.New("executable file not found")
}

func TestImageExistsOtherFailuresAreErrors(t *testing.T) {
	ctx := context.Background()
	sr := &stubRunner{code: 1, stderr: []byte("Cannot connect to the Docker daemon")}
	if ok, err := NewDocker(NewWSL(sr), "agent-host").ImageExists(ctx, "agent-image:latest"); err == nil || ok {
		t.Errorf("other non-zero exit: ok=%v err=%v", ok, err)
	}
	if ok, err := NewDocker(NewWSL(failingRunner{}), "agent-host").ImageExists(ctx, "agent-image:latest"); err == nil || ok {
		t.Errorf("wsl.exe failure: ok=%v err=%v", ok, err)
	}
}

func TestHistoryAndInspectFormatShapes(t *testing.T) {
	ctx := context.Background()
	sr := &stubRunner{stdout: "[\"A=1\"]"}
	d := NewDocker(NewWSL(sr), "agent-host")
	env, err := d.ImageEnv(ctx, "agent-image:t")
	if err != nil || len(env) != 1 || env[0] != "A=1" {
		t.Fatalf("env=%v err=%v", env, err)
	}
	if got := strings.Join(sr.got, " "); got != "-d agent-host -u root -e docker image inspect --format {{json .Config.Env}} agent-image:t" {
		t.Errorf("env args = %s", got)
	}
	sr.stdout = "null"
	if labels, err := d.ImageLabels(ctx, "agent-image:t"); err != nil || len(labels) != 0 {
		t.Fatalf("labels=%v err=%v", labels, err)
	}
	sr.stdout = "RUN a\nRUN b\n"
	h, err := d.History(ctx, "agent-image:t")
	if err != nil || len(h) != 2 {
		t.Fatalf("history=%v err=%v", h, err)
	}
	if got := strings.Join(sr.got, " "); got != "-d agent-host -u root -e docker history --no-trunc --format {{.CreatedBy}} agent-image:t" {
		t.Errorf("history args = %s", got)
	}
}
