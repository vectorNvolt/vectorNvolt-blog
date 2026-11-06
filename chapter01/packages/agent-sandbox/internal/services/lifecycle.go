// Lifecycle adapters shared by create/remove and agent_lifecycle: per-agent flock, ready marker, dockerd MainPID and the
// docker event stream. These use the real filesystem and processes (the marker and locks live on tmpfs in /run).
package services

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"agent-sandbox/internal/config"
)

// ErrLockBusy reports that another create/remove run holds the per-agent lock.
var ErrLockBusy = errors.New("agent lock is held")

// Lock is a held flock on <runDir>/<id>.lock; Release frees it.
type Lock struct{ f *os.File }

// Release frees the lock; it is safe on a nil Lock.
func (l *Lock) Release() {
	if l != nil && l.f != nil {
		_ = l.f.Close()
		l.f = nil
	}
}

func ensureRunDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.Chmod(dir, 0o755)
}

// TryLock takes the per-agent lock without blocking; ErrLockBusy when create/remove already holds it.
func TryLock(runDir, id string) (*Lock, error) {
	if err := ensureRunDir(runDir); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(runDir, id+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrLockBusy
		}
		return nil, err
	}
	return &Lock{f: f}, nil
}

// ReadyMarkerPath is the file agent_lifecycle restore writes once the gate is rebuilt.
func ReadyMarkerPath(runDir string) string { return filepath.Join(runDir, config.ReadyMarkerName) }

// DockerMainPID returns dockerd's MainPID from systemd.
func DockerMainPID(ctx context.Context, r Runner) (string, error) {
	out, err := runCmd(ctx, r, "systemctl", "show", "-p", "MainPID", "--value", config.DockerService)
	if err != nil {
		return "", err
	}
	pid := strings.TrimSpace(string(out))
	if pid == "" || pid == "0" {
		return "", fmt.Errorf("%s has no main pid (%q)", config.DockerService, pid)
	}
	return pid, nil
}

// RemoveReadyMarker deletes the marker; an absent marker is fine.
func RemoveReadyMarker(runDir string) error {
	if err := os.Remove(ReadyMarkerPath(runDir)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// WriteReadyMarker writes pid to the marker (dir 0755, file 0644) through a rename.
func WriteReadyMarker(runDir, pid string) error {
	if err := ensureRunDir(runDir); err != nil {
		return err
	}
	path := ReadyMarkerPath(runDir)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(pid+"\n"), 0o644); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// PollInterval derives a poll step from a timeout: timeout/20 within [10ms, 1s].
func PollInterval(timeout time.Duration) time.Duration {
	p := timeout / 20
	if p < 10*time.Millisecond {
		return 10 * time.Millisecond
	}
	if p > time.Second {
		return time.Second
	}
	return p
}

// Sleep waits d or until ctx ends.
func Sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// WaitReady waits until the marker exists and holds the current dockerd MainPID.
func WaitReady(ctx context.Context, r Runner, runDir string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	step := PollInterval(timeout)
	for {
		reason := ""
		pid, perr := DockerMainPID(ctx, r)
		data, rerr := os.ReadFile(ReadyMarkerPath(runDir))
		switch {
		case perr != nil:
			reason = fmt.Sprintf("dockerd main pid unreadable: %v", perr)
		case rerr != nil:
			reason = fmt.Sprintf("ready marker unreadable: %v", rerr)
		case strings.TrimSpace(string(data)) != pid:
			reason = fmt.Sprintf("ready marker holds pid %q, dockerd main pid is %q (stale marker)", strings.TrimSpace(string(data)), pid)
		default:
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New(reason)
		}
		if err := Sleep(ctx, step); err != nil {
			return err
		}
	}
}

// Guard event kinds: container start, unpause, die and update, and network connect and disconnect.
const (
	EventStart      = "start"
	EventUnpause    = "unpause"
	EventDie        = "die"
	EventUpdate     = "update"
	EventConnect    = "connect"
	EventDisconnect = "disconnect"
)

// GuardEvent is one docker event the guard follows. Container events carry the container name; network events carry
// the container ID (Actor.Attributes.container) and the network name.
type GuardEvent struct {
	Kind      string
	Name      string // container events: the container name
	Container string // network events: the container ID
	Network   string // network events: the network name
}

// IsNetwork reports a network connect/disconnect event.
func (e GuardEvent) IsNetwork() bool { return e.Kind == EventConnect || e.Kind == EventDisconnect }

// ParseGuardEvent reads one line of the guard's `docker events` format `{{.Type}}|{{.Action}}|{{.Actor.Attributes.name}}|
// {{.Actor.Attributes.container}}`; ok is false for a line that is not one of the followed events.
func ParseGuardEvent(line string) (GuardEvent, bool) {
	f := strings.SplitN(strings.TrimSpace(line), "|", 4)
	if len(f) != 4 {
		return GuardEvent{}, false
	}
	for i := range f {
		if f[i] == "<no value>" {
			f[i] = ""
		}
	}
	typ, action := f[0], f[1]
	switch {
	case typ == "container" && (action == EventStart || action == EventUnpause || action == EventDie || action == EventUpdate):
		if f[2] == "" {
			return GuardEvent{}, false
		}
		return GuardEvent{Kind: action, Name: f[2]}, true
	case typ == "network" && (action == EventConnect || action == EventDisconnect):
		if f[3] == "" {
			return GuardEvent{}, false
		}
		return GuardEvent{Kind: action, Container: f[3], Network: f[2]}, true
	}
	return GuardEvent{}, false
}

// EventSource follows the guard's docker events and calls handle with each one.
// It returns only on error or when ctx ends.
type EventSource interface {
	Follow(ctx context.Context, handle func(ev GuardEvent)) error
}

// DockerEvents is the real `docker events` stream.
type DockerEvents struct{ Stderr io.Writer }

func (e DockerEvents) Follow(ctx context.Context, handle func(ev GuardEvent)) error {
	cmd := exec.CommandContext(ctx, config.DockerExe, "events",
		"--filter", "type=container", "--filter", "type=network",
		"--filter", "event=start", "--filter", "event=unpause", "--filter", "event=die", "--filter", "event=update",
		"--filter", "event=connect", "--filter", "event=disconnect",
		"--format", "{{.Type}}|{{.Action}}|{{.Actor.Attributes.name}}|{{.Actor.Attributes.container}}")
	cmd.Stderr = e.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	sc := bufio.NewScanner(out)
	for sc.Scan() {
		if ev, ok := ParseGuardEvent(sc.Text()); ok {
			handle(ev)
		}
	}
	werr := cmd.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if werr != nil {
		return fmt.Errorf("docker events exited: %w", werr)
	}
	return errors.New("docker events ended")
}
