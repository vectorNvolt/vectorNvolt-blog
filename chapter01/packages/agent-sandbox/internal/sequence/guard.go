// The guard of `agent_lifecycle guard`: follows docker events and a periodic reconcile, and removes (or disarms) an agent
// container that was started or changed outside the tools.
//
// Events: container start, unpause, die, update; network connect, disconnect (a network event carries the container ID
// in Actor.Attributes.container; any connect or disconnect of an agent-* container counts, whatever the network).
// An event for an agent whose lock is held belongs to a running create/remove and is ignored. Otherwise the guard holds
// the agent lock from the check to the end of the removal or disarm:
//
//   - die: `create_agent_net disarm <name>` (the container is stopped; its port state goes, the container stays)
//   - other events: `check-gate`; a clean gate failure (exit 2) removes the container through the remove path (which
//     disarms first)
//   - an error (the existence check or check-gate could not run, or exit 1): fail closed, disarm first and remove only
//     when the disarm exited 0; when it did not, the container is left in place (paused if possible)
//
// Reconcile (at start and every GuardReconcile): every agent container whose lock is free gets the same check; then the
// gate set `released` is cleaned (untagged element: evict; element of a container that does not exist: disarm).
package sequence

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"agent-sandbox/internal/config"
	"agent-sandbox/internal/services"
)

// GuardDeps are the injected collaborators of the guard; tests replace Runner, FS and Events.
type GuardDeps struct {
	Config config.Config
	Runner services.Runner
	FS     services.FS
	Events services.EventSource
	Stderr io.Writer // journal
}

// RunGuard follows the guard events and reconciles at start and periodically. It returns only when the event stream ends
// (the caller exits non-zero so systemd restarts it) or ctx is cancelled.
func RunGuard(ctx context.Context, d GuardDeps) error {
	var mu sync.Mutex // events and reconcile passes run one at a time
	locked := func(f func()) {
		mu.Lock()
		defer mu.Unlock()
		f()
	}
	locked(func() { reconcile(ctx, d) })

	if period := d.Config.GuardReconcile; period > 0 {
		rctx, stop := context.WithCancel(ctx)
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			t := time.NewTicker(period)
			defer t.Stop()
			for {
				select {
				case <-rctx.Done():
					return
				case <-t.C:
					locked(func() { reconcile(rctx, d) })
				}
			}
		}()
		defer func() {
			stop()
			wg.Wait()
		}()
	}
	return d.Events.Follow(ctx, func(ev services.GuardEvent) { locked(func() { guardEvent(ctx, d, ev) }) })
}

func guardEvent(ctx context.Context, d GuardDeps, ev services.GuardEvent) {
	docker := services.NewDocker(d.Runner)
	name := ev.Name
	if ev.IsNetwork() {
		n, found, err := docker.ContainerName(ctx, ev.Container)
		if err != nil {
			fmt.Fprintf(d.Stderr, "agent_lifecycle guard: network %s of container %s: cannot resolve its name: %v\n", ev.Kind, ev.Container, err)
			return
		}
		if !found {
			return // already gone (late event of a removal)
		}
		name = n
	}
	id, ok := services.AgentID(name)
	if !ok {
		return
	}
	lock, err := services.TryLock(d.Config.RuntimeDir, id)
	if err != nil {
		if !errors.Is(err, services.ErrLockBusy) {
			fmt.Fprintf(d.Stderr, "agent_lifecycle guard: cannot check the lock of %s: %v\n", name, err)
		}
		return // create/remove in progress: a tool-driven start, unpause or network change
	}
	defer lock.Release() // held until the check, the removal and the disarm are done

	if ev.Kind == services.EventDie {
		arm := &services.Arm{Runner: d.Runner, Bin: d.Config.ArmBin}
		if err := arm.Disarm(context.WithoutCancel(ctx), name); err != nil {
			fmt.Fprintf(d.Stderr, "agent_lifecycle guard: container %s died and its disarm failed: %v\n", name, err)
		}
		return
	}
	guardCheck(ctx, d, name, id, ev.Kind)
}

// guardCheck gate-checks one agent container whose lock the caller holds, and removes it when the check fails.
func guardCheck(ctx context.Context, d GuardDeps, name, id, why string) {
	docker := services.NewDocker(d.Runner)
	exists, err := docker.ContainerExists(ctx, name)
	if err != nil {
		failClosed(ctx, d, name, why, "cannot check whether the container exists: "+err.Error())
		return
	}
	if !exists {
		return // already gone (late event of a removal)
	}
	// A container the tools created and armed passes the gate check; a late event of a finished create lands here.
	arm := &services.Arm{Runner: d.Runner, Bin: d.Config.ArmBin}
	err = arm.CheckGate(ctx, name)
	if err == nil {
		return
	}
	var ce *services.CmdError
	if !errors.As(err, &ce) || ce.Err != nil || ce.ExitCode != 2 {
		failClosed(ctx, d, name, why, "check-gate could not be evaluated: "+err.Error())
		return
	}

	var out, errb bytes.Buffer
	code := RunRemove(context.WithoutCancel(ctx), RemoveDeps{
		Config: d.Config, Runner: d.Runner, FS: d.FS, Args: []string{id}, Stdout: &out, Stderr: &errb, LockHeld: true,
	})
	switch {
	case code == 0 && (why == services.EventStart || why == services.EventUnpause):
		fmt.Fprintf(d.Stderr, "agent_lifecycle guard: container %s was started outside the tools (docker start/restart/unpause) and was removed; use create_agent_container\n", name)
	case code == 0:
		fmt.Fprintf(d.Stderr, "agent_lifecycle guard: container %s was changed outside the tools (%s) and failed the gate check; it was removed; use create_agent_container\n", name, why)
	default:
		fmt.Fprintf(d.Stderr, "agent_lifecycle guard: container %s failed the gate check (%s) but could not be removed (exit %d)\n%s", name, why, code, errb.String())
	}
}

// failClosed handles an error (not a clean gate failure): disarm first, then remove the container only when the disarm
// exited 0. When it did not, the container stays in place, paused if possible. Both outcomes go to the journal.
func failClosed(ctx context.Context, d GuardDeps, name, why, problem string) {
	ctx = context.WithoutCancel(ctx)
	docker := services.NewDocker(d.Runner)
	arm := &services.Arm{Runner: d.Runner, Bin: d.Config.ArmBin}
	fmt.Fprintf(d.Stderr, "agent_lifecycle guard: container %s (%s): %s; failing closed\n", name, why, problem)
	if err := arm.Disarm(ctx, name); err != nil {
		fmt.Fprintf(d.Stderr, "agent_lifecycle guard: disarm of %s failed: %v; the container is left in place\n", name, err)
		if perr := docker.Pause(ctx, name); perr != nil {
			fmt.Fprintf(d.Stderr, "agent_lifecycle guard: %s could not be paused: %v\n", name, perr)
		} else {
			fmt.Fprintf(d.Stderr, "agent_lifecycle guard: %s paused\n", name)
		}
		return
	}
	fmt.Fprintf(d.Stderr, "agent_lifecycle guard: disarm of %s succeeded\n", name)
	if err := docker.Remove(ctx, name); err != nil {
		fmt.Fprintf(d.Stderr, "agent_lifecycle guard: removal of %s failed: %v\n", name, err)
		return
	}
	fmt.Fprintf(d.Stderr, "agent_lifecycle guard: container %s was removed\n", name)
}

// reconcile gate-checks every agent container whose lock is free, then cleans the gate set.
func reconcile(ctx context.Context, d GuardDeps) {
	docker := services.NewDocker(d.Runner)
	names, err := docker.ListAgentContainers(ctx)
	if err != nil {
		fmt.Fprintf(d.Stderr, "agent_lifecycle guard: reconcile cannot list the agent containers: %v\n", err)
	}
	for _, name := range names {
		id, ok := services.AgentID(name)
		if !ok {
			continue
		}
		lock, err := services.TryLock(d.Config.RuntimeDir, id)
		if err != nil {
			if !errors.Is(err, services.ErrLockBusy) {
				fmt.Fprintf(d.Stderr, "agent_lifecycle guard: cannot check the lock of %s: %v\n", name, err)
			}
			continue
		}
		guardCheck(ctx, d, name, id, "reconcile")
		lock.Release()
	}
	arm := &services.Arm{Runner: d.Runner, Bin: d.Config.ArmBin}
	if err := services.ReconcileReleased(ctx, docker, arm, d.Config.RuntimeDir, d.Stderr); err != nil {
		fmt.Fprintf(d.Stderr, "agent_lifecycle guard: reconcile of the gate set: %v\n", err)
	}
}
