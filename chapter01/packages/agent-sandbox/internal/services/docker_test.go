package services

import (
	"context"
	"strings"
	"testing"
)

func TestCreateArgsShape(t *testing.T) {
	got := strings.Join(CreateArgs("agent-dev", "/ws/dev", "/var/lib/agent-sandbox/dev/claude", "/etc/agent-sandbox/resolv.conf"), " ")
	want := "create --name agent-dev --runtime=runsc --restart no --network agents-net --cap-drop ALL" +
		" --cap-add SYS_ADMIN --cap-add SYS_CHROOT --cap-add MKNOD" +
		" --cap-add CHOWN --cap-add DAC_OVERRIDE --cap-add FOWNER --cap-add FSETID" +
		" --cap-add SETUID --cap-add SETGID --cap-add SETPCAP --cap-add SETFCAP" +
		" --cap-add KILL --cap-add AUDIT_WRITE --cap-add NET_BIND_SERVICE --cap-add SYS_RESOURCE" +
		" --security-opt no-new-privileges --memory 4g --cpus 2" +
		" --pids-limit 512 --tmpfs /tmp:rw,noexec,nosuid,size=512m --tmpfs /var/lib/docker:rw,nosuid,size=2g" +
		" -v /ws/dev:/workspace:rw -v /var/lib/agent-sandbox/dev/claude:/home/agent/.claude:rw" +
		" -v /etc/agent-sandbox/resolv.conf:/etc/resolv.conf:ro -e CLAUDE_CONFIG_DIR=/home/agent/.claude agent-image:latest"
	if got != want {
		t.Errorf("create args =\n%s\nwant\n%s", got, want)
	}
	for _, bad := range []string{"--restart always", "--restart on-failure", "--restart unless-stopped", "--init", "--privileged"} {
		if strings.Contains(got, bad) {
			t.Errorf("create args must not contain %s", bad)
		}
	}
}

type stubRunner struct {
	stdout string
	stderr string
	code   int
	got    []string
}

func (s *stubRunner) Run(_ context.Context, _ []byte, args ...string) ([]byte, []byte, int, error) {
	s.got = args
	return []byte(s.stdout), []byte(s.stderr), s.code, nil
}

func TestExistenceDistinguishesAbsentFromEngineError(t *testing.T) {
	ctx := context.Background()
	for _, stderr := range []string{"Error: No such container: agent-x", "Error: No such object: agent-x"} {
		sr := &stubRunner{code: 1, stderr: stderr}
		if ok, err := NewDocker(sr).ContainerExists(ctx, "agent-x"); ok || err != nil {
			t.Errorf("stderr %q: ok=%v err=%v, want absent without error", stderr, ok, err)
		}
	}
	sr := &stubRunner{code: 1, stderr: "Cannot connect to the Docker daemon"}
	if ok, err := NewDocker(sr).ContainerExists(ctx, "agent-x"); ok || err == nil {
		t.Errorf("engine error: ok=%v err=%v, want an error", ok, err)
	}
	sr = &stubRunner{code: 0}
	if ok, err := NewDocker(sr).ContainerExists(ctx, "agent-x"); !ok || err != nil {
		t.Errorf("exit 0: ok=%v err=%v, want present", ok, err)
	}
}

// pausedRunner models a runsc container: `rm -f` fails with the zombie error while it is paused.
type pausedRunner struct {
	paused      bool
	unpauseFail bool
	calls       []string
}

func (p *pausedRunner) Run(_ context.Context, _ []byte, args ...string) ([]byte, []byte, int, error) {
	j := strings.Join(args, " ")
	p.calls = append(p.calls, j)
	switch {
	case strings.HasPrefix(j, "docker inspect -f {{.State.Paused}} "):
		if p.paused {
			return []byte("true\n"), nil, 0, nil
		}
		return []byte("false\n"), nil, 0, nil
	case strings.HasPrefix(j, "docker unpause "):
		if p.unpauseFail {
			return nil, []byte("cannot unpause"), 1, nil
		}
		p.paused = false
		return nil, nil, 0, nil
	case strings.HasPrefix(j, "docker rm -f "):
		if p.paused {
			return nil, []byte("could not kill container: container x PID 1 is zombie and can not be killed"), 1, nil
		}
		return nil, nil, 0, nil
	}
	return nil, []byte("unexpected: " + j), 99, nil
}

func TestRemovePausedContainerUnpausesThenRemoves(t *testing.T) {
	pr := &pausedRunner{paused: true}
	if err := NewDocker(pr).Remove(context.Background(), "agent-x"); err != nil {
		t.Fatal(err)
	}
	want := "docker inspect -f {{.State.Paused}} agent-x|docker unpause agent-x|docker rm -f agent-x"
	if got := strings.Join(pr.calls, "|"); got != want {
		t.Errorf("calls = %s, want %s", got, want)
	}
}

func TestRemoveRunningContainerDoesNotUnpause(t *testing.T) {
	pr := &pausedRunner{}
	if err := NewDocker(pr).Remove(context.Background(), "agent-x"); err != nil {
		t.Fatal(err)
	}
	for _, c := range pr.calls {
		if strings.HasPrefix(c, "docker unpause") {
			t.Errorf("unexpected unpause: %v", pr.calls)
		}
	}
}

func TestRemoveUnpauseFailureIsWrapped(t *testing.T) {
	pr := &pausedRunner{paused: true, unpauseFail: true}
	err := NewDocker(pr).Remove(context.Background(), "agent-x")
	if err == nil || !strings.Contains(err.Error(), "unpause before remove") || !strings.Contains(err.Error(), "cannot unpause") {
		t.Fatalf("err = %v", err)
	}
	for _, c := range pr.calls {
		if strings.HasPrefix(c, "docker rm") {
			t.Errorf("no rm after a failed unpause: %v", pr.calls)
		}
	}
}

func TestRuntimeRegistered(t *testing.T) {
	sr := &stubRunner{stdout: `{"runc":{"path":"runc"},"runsc":{"path":"/usr/local/bin/runsc"}}`}
	ok, err := NewDocker(sr).RuntimeRegistered(context.Background(), "runsc")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if strings.Join(sr.got, " ") != "docker info --format {{json .Runtimes}}" {
		t.Errorf("args = %v", sr.got)
	}
	sr.stdout = `{"runc":{}}`
	if ok, _ := NewDocker(sr).RuntimeRegistered(context.Background(), "runsc"); ok {
		t.Error("runsc must not be reported when absent")
	}
}
