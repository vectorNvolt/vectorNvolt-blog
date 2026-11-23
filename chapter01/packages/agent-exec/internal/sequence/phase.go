package sequence

import (
	"context"
	"encoding/json"
	"io"
	"strconv"
	"time"

	"golang.org/x/sys/unix"

	"agent-exec/internal/engine"
	"agent-exec/internal/kind"
	"agent-exec/internal/profile"
)

type metaInput struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type metaSettings struct {
	User           string              `json:"user"`
	CapDrop        []string            `json:"cap_drop"`
	CapAdd         []string            `json:"cap_add"`
	SecurityOpt    []string            `json:"security_opt"`
	NetworkMode    string              `json:"network_mode"`
	ReadonlyRootfs bool                `json:"readonly_rootfs"`
	AutoRemove     bool                `json:"auto_remove"`
	Memory         int64               `json:"memory"`
	NanoCpus       int64               `json:"nano_cpus"`
	PidsLimit      int64               `json:"pids_limit"`
	Mounts         []engine.MountPoint `json:"mounts"`
	Tmpfs          map[string]string   `json:"tmpfs"`
	Labels         map[string]string   `json:"labels"`
}

type metaPhase struct {
	Phase     string       `json:"phase"`
	Container string       `json:"container"`
	Argv      []string     `json:"argv"`
	Settings  metaSettings `json:"settings"`
}

type metaRequest struct {
	Profile   string            `json:"profile"`
	Kind      string            `json:"kind"`
	ExecID    string            `json:"exec_id"`
	Args      []string          `json:"args"`
	Manifests []string          `json:"manifests"`
	Labels    map[string]string `json:"labels"`
	Params    map[string]string `json:"params,omitempty"` // kind tool: canonical values, defaults included
	TimeoutS  float64           `json:"timeout_s"`
}

type meta struct {
	Request metaRequest `json:"request"`
	Inputs  []metaInput `json:"inputs"`
	ImageID string      `json:"image_id"`
	Phases  []metaPhase `json:"phases"`
}

// writeMeta writes meta.json once, before start, with O_EXCL; it is never changed. SSR-AGE-010.
func (r *run) writeMeta() *failure {
	m := meta{
		Request: metaRequest{
			Profile: r.p.Name, Kind: r.p.Kind, ExecID: r.execID, Args: r.req.Args,
			Manifests: r.req.Manifests, Labels: r.req.Labels, Params: r.params, TimeoutS: r.timeout.Seconds(),
		},
		Inputs:  []metaInput{},
		ImageID: r.p.ImageID,
	}
	if m.Request.Args == nil {
		m.Request.Args = []string{}
	}
	if m.Request.Manifests == nil {
		m.Request.Manifests = []string{}
	}
	for _, in := range r.inputs {
		m.Inputs = append(m.Inputs, metaInput{Name: in.name, Source: in.source, Path: in.containerPath, SHA256: in.sha})
	}
	for _, c := range r.ctrs {
		hc := c.info.HostConfig
		m.Phases = append(m.Phases, metaPhase{
			Phase: c.phase, Container: c.plan.Name, Argv: c.plan.Params.Argv,
			Settings: metaSettings{
				User: c.info.Config.User, CapDrop: hc.CapDrop, CapAdd: hc.CapAdd, SecurityOpt: hc.SecurityOpt,
				NetworkMode: hc.NetworkMode, ReadonlyRootfs: hc.ReadonlyRootfs, AutoRemove: hc.AutoRemove,
				Memory: hc.Memory, NanoCpus: hc.NanoCpus, PidsLimit: hc.PidsLimit,
				Mounts: c.info.Mounts, Tmpfs: hc.Tmpfs, Labels: c.info.Config.Labels,
			},
		})
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fail(CauseSetup, "meta.json: %v", err)
	}
	if err := r.d.WS.WriteExcl(r.execRel+"/meta.json", data); err != nil {
		return fail(CauseSetup, "cannot write meta.json: %v", err)
	}
	return nil
}

// Result is result.json (SSR-AGE-012); phase is for install only.
type Result struct {
	Exit    int    `json:"exit"`
	Cause   string `json:"cause"`
	ExecDir string `json:"exec_dir"`
	ImageID string `json:"image_id"`
	Start   string `json:"start"`
	End     string `json:"end"`
	Phase   string `json:"phase,omitempty"`
}

// writeResult is written whenever the exec dir exists, once, after the containers are gone. SFR-AGE-014.
func (r *run) writeResult(o Outcome) {
	res := Result{
		Exit: o.Exit, Cause: o.Cause, ExecDir: r.d.WS.Abs(r.execRel), ImageID: r.p.ImageID,
		Start: r.start.UTC().Format(time.RFC3339Nano), End: r.d.Now().UTC().Format(time.RFC3339Nano),
	}
	if r.p.Kind == profile.KindInstall {
		res.Phase = r.phase
	}
	data, _ := json.MarshalIndent(res, "", "  ")
	if err := r.d.WS.WriteExcl(r.execRel+"/result.json", data); err != nil {
		r.diag("cannot write result.json: %v", err)
	}
}

// openLogs creates stdout.log and stderr.log (agent side, never mounted) and the tee writers. SFR-AGE-012.
func (r *run) openLogs() *failure {
	var err error
	flags := unix.O_WRONLY | unix.O_CREAT | unix.O_EXCL
	if r.logOut, err = r.d.WS.OpenFile(r.execRel+"/stdout.log", flags, 0o644); err != nil {
		return fail(CauseSetup, "cannot create stdout.log: %v", err)
	}
	if r.logErr, err = r.d.WS.OpenFile(r.execRel+"/stderr.log", flags, 0o644); err != nil {
		return fail(CauseSetup, "cannot create stderr.log: %v", err)
	}
	r.outW = io.MultiWriter(tolerant{r.d.Stdout}, tolerant{r.logOut})
	r.errW = io.MultiWriter(tolerant{r.d.Stderr}, tolerant{r.logErr})
	return nil
}

func (r *run) closeLogs() {
	if r.logOut != nil {
		r.logOut.Close()
	}
	if r.logErr != nil {
		r.logErr.Close()
	}
}

type phaseOutcome struct {
	exit  int
	cause string
	f     *failure
}

// runPhases is steps 8 and 9 for every container of the kind in order (install: fetch, then install).
// One wall-clock limit covers all phases. SFR-AGE-012, SFR-AGE-013, SFR-AGE-016, SFR-AGE-017.
func (r *run) runPhases() Outcome {
	if f := r.openLogs(); f != nil {
		return r.report(f)
	}
	r.baseline = r.watchSize()
	deadline := r.d.Now().Add(r.timeout)
	for i, c := range r.ctrs {
		if i > 0 && r.cancelled() {
			return r.cancelOutcome()
		}
		if r.p.Kind == profile.KindInstall {
			r.phase = c.phase
		}
		po := r.startAndWait(c, deadline)
		if po.f != nil {
			return r.report(po.f)
		}
		switch {
		case po.cause == CauseOK && i < len(r.ctrs)-1:
			continue
		case po.cause == CauseExit && c.phase == kind.PhaseFetch:
			r.diag("%s", FetchMessage)
			return Outcome{Exit: po.exit, Cause: CauseFetch}
		case po.cause == CauseTimeout:
			r.diag("timeout after %s", r.timeout)
		case po.cause == CauseCancel:
			r.diag("cancelled")
		case po.cause == CauseOutCap:
			r.diag("output above out_max (%d bytes)", r.p.OutMaxBytes)
		case po.cause == CauseOOM:
			r.diag("out of memory")
		}
		return Outcome{Exit: po.exit, Cause: po.cause}
	}
	return Outcome{Exit: 0, Cause: CauseOK}
}

func (r *run) watchSize() int64 {
	var sum int64
	for _, d := range kind.WatchDirs(r.p, r.execID) {
		sum += r.d.WS.DirSize(d)
	}
	return sum
}

func (r *run) kill(c *ctr) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = r.d.Engine.ContainerKill(ctx, c.id)
}

type waitRes struct {
	code int
	err  error
}

// startAndWait subscribes to the events (OOM, the container may be gone after exit), waits, attaches,
// starts, then watches wait, timeout, cancel and out_max. SFR-AGE-012, SFR-AGE-013.
func (r *run) startAndWait(c *ctr, deadline time.Time) phaseOutcome {
	eng := r.d.Engine
	sctx, stop := context.WithCancel(r.ctx)
	defer stop()

	events, err := eng.Events(sctx, c.id)
	if err != nil {
		return phaseOutcome{f: engineFail("subscribe to events", err)}
	}
	waitCh := make(chan waitRes, 1)
	go func() {
		code, err := eng.ContainerWait(sctx, c.id)
		waitCh <- waitRes{code, err}
	}()
	attachDone, err := eng.ContainerAttach(sctx, c.id, r.outW, r.errW)
	if err != nil {
		return phaseOutcome{f: engineFail("attach to "+c.plan.Name, err)}
	}
	if err := eng.ContainerStart(r.ctx, c.id); err != nil {
		return phaseOutcome{f: engineFail("start "+c.plan.Name, err)}
	}

	timer := time.NewTimer(deadline.Sub(r.d.Now()))
	defer timer.Stop()
	tick := time.NewTicker(r.d.Poll)
	defer tick.Stop()
	cancel := r.d.Cancel
	forced := ""
	oom := false
	dieCode := -1
	note := func(ev engine.Event) {
		switch ev.Action {
		case "oom":
			oom = true
		case "die":
			if n, err := strconv.Atoi(ev.Attributes["exitCode"]); err == nil {
				dieCode = n
			}
		}
	}

	var res waitRes
loop:
	for {
		select {
		case res = <-waitCh:
			break loop
		case ev, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			note(ev)
		case <-timer.C:
			if forced == "" {
				forced = CauseTimeout
				r.kill(c)
			}
		case <-cancel:
			cancel = nil
			if forced == "" {
				forced = CauseCancel
				r.kill(c)
			}
		case <-tick.C:
			if forced == "" && r.watchSize()-r.baseline > r.p.OutMaxBytes {
				forced = CauseOutCap
				r.kill(c)
			}
		}
	}

	// The oom event comes before die; give the event stream a moment to deliver them.
	grace := time.NewTimer(500 * time.Millisecond)
	for dieCode < 0 && events != nil {
		select {
		case ev, ok := <-events:
			if !ok {
				events = nil
			} else {
				note(ev)
			}
		case <-grace.C:
			events = nil
		}
	}
	grace.Stop()

	select {
	case err := <-attachDone:
		if err != nil {
			r.diag("output stream ended with an error: %v", err)
		}
	case <-time.After(5 * time.Second):
	}

	exit := res.code
	if res.err != nil {
		switch {
		case dieCode >= 0:
			exit = dieCode
		case forced != "":
			exit = ExitKilled
		default:
			return phaseOutcome{f: fail(CauseSetup, "wait for %s: %v", c.plan.Name, res.err)}
		}
	}
	switch {
	case forced == CauseTimeout:
		return phaseOutcome{exit: ExitTimeout, cause: CauseTimeout}
	case forced == CauseCancel:
		return phaseOutcome{exit: ExitCancel, cause: CauseCancel}
	case forced == CauseOutCap:
		return phaseOutcome{exit: ExitKilled, cause: CauseOutCap}
	case oom:
		return phaseOutcome{exit: ExitKilled, cause: CauseOOM}
	case exit == 0:
		return phaseOutcome{exit: 0, cause: CauseOK}
	}
	return phaseOutcome{exit: exit, cause: CauseExit}
}
