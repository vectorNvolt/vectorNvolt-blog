// Gate set reconcile, shared by the guard and the create path: the set `released` of agents-net must hold only elements
// that are tagged with the name of an existing container. An untagged element is evicted by MAC; an element tagged with
// a container that does not exist is disarmed by name (which removes the rest of that name's state too).
package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"agent-sandbox/internal/config"
)

var agentIDRe = regexp.MustCompile(config.AgentIDPattern)

// AgentID returns the <id> of a container name agent-<id> made by the tools; ok is false for any other name.
func AgentID(name string) (id string, ok bool) {
	id, found := strings.CutPrefix(name, config.ContainerPrefix)
	if !found || !agentIDRe.MatchString(id) {
		return "", false
	}
	return id, true
}

// ReconcileReleased cleans the gate set as described above. An element whose container name has its agent lock held
// (a create or remove is running) is left for that run. All problems are collected into one error; progress lines go
// to log.
func ReconcileReleased(ctx context.Context, docker *Docker, arm *Arm, runDir string, log io.Writer) error {
	elems, err := arm.Released(ctx)
	if err != nil {
		return fmt.Errorf("cannot list the gate set: %w", err)
	}
	var failed []string
	for _, e := range elems {
		if e.Comment == "" {
			if err := arm.Evict(ctx, e.MAC); err != nil {
				failed = append(failed, fmt.Sprintf("evict %s: %v", e.MAC, err))
				continue
			}
			fmt.Fprintf(log, "agent_lifecycle: evicted the untagged element %s from the gate set\n", e.MAC)
			continue
		}
		var lock *Lock
		if id, ok := AgentID(e.Comment); ok {
			l, err := TryLock(runDir, id)
			if err != nil {
				if !errors.Is(err, ErrLockBusy) {
					failed = append(failed, fmt.Sprintf("lock of %s: %v", e.Comment, err))
				}
				continue
			}
			lock = l
		}
		exists, err := docker.ContainerExists(ctx, e.Comment)
		switch {
		case err != nil:
			failed = append(failed, fmt.Sprintf("check %s: %v", e.Comment, err))
		case exists:
		default:
			if err := arm.Disarm(ctx, e.Comment); err != nil {
				failed = append(failed, fmt.Sprintf("disarm %s (element %s): %v", e.Comment, e.MAC, err))
			} else {
				fmt.Fprintf(log, "agent_lifecycle: disarmed %s: its gate set element %s had no container\n", e.Comment, e.MAC)
			}
		}
		lock.Release()
	}
	if len(failed) > 0 {
		return errors.New(strings.Join(failed, "; "))
	}
	return nil
}
