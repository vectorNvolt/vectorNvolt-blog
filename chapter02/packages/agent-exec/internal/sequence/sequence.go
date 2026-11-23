// Package sequence runs one execution: validate (USC-005 step 2), prepare (3), stage inputs (4),
// ensure the image (5), create (6), verify (7), run and stream (8-9), finish (10).
// The exit code and cause of every outcome follow SSR-AGE-012; result.json records them.
// Kind tool (USC-009 steps 4-6) adds param validation and the input policy to step 2.
package sequence

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"agent-exec/internal/cli"
	"agent-exec/internal/engine"
	"agent-exec/internal/inputpolicy"
	"agent-exec/internal/kind"
	"agent-exec/internal/limits"
	"agent-exec/internal/profile"
	"agent-exec/internal/wsfs"
)

// Prog prefixes every diagnostic (USC-005 step 8 / "Exec dir layout").
const Prog = "run_ephemeral_container"

// Causes of SSR-AGE-012.
const (
	CauseOK      = "ok"
	CauseExit    = "exit"
	CauseUsage   = "usage"
	CauseSetup   = "setup"
	CauseDenied  = "denied"
	CausePolicy  = "policy" // kind tool: the profile's input_policy refused an input (SFR-AGT-009)
	CauseVerify  = "verify"
	CauseBusy    = "busy"
	CauseFetch   = "fetch"
	CauseTimeout = "timeout"
	CauseCancel  = "cancel"
	CauseOutCap  = "out_cap"
	CauseOOM     = "oom"
)

// Exit codes of SSR-AGE-012.
const (
	ExitTool    = 125
	ExitTimeout = 124
	ExitCancel  = 130
	ExitKilled  = 137
)

// FetchMessage is the exact stderr text of exc-11 (after the prefix). SFR-AGE-017.
const FetchMessage = "fetch failed: packages without a prebuilt wheel must be built or downloaded as a wheel on agent-host and copied by the operator to Workspace-Agents/<agentID>/.packages/"

// Outcome is the exit code and cause of one call.
type Outcome struct {
	Exit  int
	Cause string
}

// Deps are the collaborators of a call; tests replace the engine and the timing.
type Deps struct {
	WS     *wsfs.Workspace
	Engine engine.Engine
	Stdout io.Writer
	Stderr io.Writer
	// Cancel is closed on SIGTERM or SIGINT (SFR-AGE-013).
	Cancel <-chan struct{}
	Pid    int
	// PidAlive tells whether a tool PID of a container label still exists (SFR-AGE-006).
	PidAlive func(pid int) bool
	// Poll is the interval of the out_max watch.
	Poll time.Duration
	// LockWait bounds the wait for a folder lock before `busy` (SFR-AGE-005).
	LockWait time.Duration
	// TimeoutOverride replaces the computed wall-clock limit (tests).
	TimeoutOverride time.Duration
	Now             func() time.Time
	NewExecID       func() string
}

func (d *Deps) defaults() {
	if d.Stdout == nil {
		d.Stdout = io.Discard
	}
	if d.Stderr == nil {
		d.Stderr = io.Discard
	}
	if d.Pid == 0 {
		d.Pid = os.Getpid()
	}
	if d.PidAlive == nil {
		d.PidAlive = pidAlive
	}
	if d.Poll <= 0 {
		d.Poll = time.Second
	}
	if d.LockWait <= 0 {
		d.LockWait = time.Duration(limits.LockWaitMillis) * time.Millisecond
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.NewExecID == nil {
		d.NewExecID = generateExecID
	}
}

func pidAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// generateExecID returns e<base36 seconds>-<8 hex>, matching cli.ExecIDRe.
func generateExecID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "e" + strconv.FormatInt(time.Now().Unix(), 36) + "-" + hex.EncodeToString(b)
}

type failure struct {
	exit  int
	cause string
	msg   string
}

func fail(cause, format string, a ...any) *failure {
	return &failure{exit: ExitTool, cause: cause, msg: fmt.Sprintf(format, a...)}
}

// engineFail maps an engine error: a refusal by the plugin is `denied`, anything else `setup`. SFR-AGE-010.
func engineFail(what string, err error) *failure {
	if engine.IsDenied(err) {
		return fail(CauseDenied, "%s: %v", what, err)
	}
	return fail(CauseSetup, "%s: %v", what, err)
}

type input struct {
	name, source, rel, base string
	inPlace, multiple       bool
	st                      wsfs.Info
	stagedRel               string
	containerPath           string
	sha                     string
}

type ctr struct {
	phase string
	plan  *kind.Plan
	id    string
	info  *engine.Inspect
}

type run struct {
	d   *Deps
	req *cli.Request
	ctx context.Context

	p       *profile.Profile
	execID  string
	execRel string
	timeout time.Duration

	inputs      []*input
	params      map[string]string // kind tool: canonical values, defaults included
	manifestRel []string
	argv        map[string][]string
	locks       []*wsfs.Lock
	ctrs        []*ctr
	imageEnv    []string

	phase    string
	start    time.Time
	baseline int64

	logOut, logErr *os.File
	outW, errW     io.Writer
}

// tolerant never reports a write error, so a closed pipe cannot stop the log files.
type tolerant struct{ w io.Writer }

func (t tolerant) Write(p []byte) (int, error) {
	_, _ = t.w.Write(p)
	return len(p), nil
}

func (r *run) diag(format string, a ...any) {
	fmt.Fprintf(r.d.Stderr, Prog+": "+format+"\n", a...)
}

func (r *run) report(f *failure) Outcome {
	r.diag("%s: %s", f.cause, f.msg)
	return Outcome{Exit: f.exit, Cause: f.cause}
}

func (r *run) cancelled() bool {
	select {
	case <-r.d.Cancel:
		return true
	default:
		return false
	}
}

func (r *run) cancelOutcome() Outcome {
	r.diag("cancelled")
	return Outcome{Exit: ExitCancel, Cause: CauseCancel}
}

// Execute runs one call of run, build, install or tool and returns its exit code and cause.
func Execute(d *Deps, req *cli.Request) Outcome {
	d.defaults()
	r := &run{d: d, req: req, ctx: context.Background()}
	if f := r.validate(); f != nil {
		return r.report(f) // nothing is created (exc-1, exc-3)
	}
	if f := r.prepare(); f != nil {
		r.releaseLocks()
		return r.report(f) // no exec dir, no result.json (exc-2, exc-4, exc-6, exc-10)
	}
	out := r.execute()
	r.cleanup()
	r.closeLogs()
	r.releaseLocks()
	r.writeResult(out)
	return out
}

// validate is USC-005 step 2: arguments against the profile, paths resolved without symlink escape.
// Nothing is written. SFR-AGE-001, SFR-AGE-002, SFR-AGE-007, SSR-AGE-005, SSR-AGE-013.
func (r *run) validate() *failure {
	list, err := profile.LoadAll(r.d.WS)
	if err != nil {
		return fail(CauseSetup, "cannot read %s: %v", profile.ToolsDir, err)
	}
	l := profile.Find(list, r.req.Profile)
	if l == nil {
		return fail(CauseUsage, "unknown profile %q", r.req.Profile)
	}
	if l.Err != nil {
		return fail(CauseSetup, "profile %q is invalid: %v", r.req.Profile, l.Err)
	}
	r.p = l.Profile
	want := map[string]string{"run": profile.KindCode, "build": profile.KindBuild, "install": profile.KindInstall, "tool": profile.KindTool}[r.req.Sub]
	if r.p.Kind != want {
		return fail(CauseUsage, "subcommand %s does not match the kind %s of profile %q", r.req.Sub, r.p.Kind, r.p.Name)
	}

	ts := r.p.TimeoutS
	if r.req.TimeoutS > 0 {
		if r.req.TimeoutS > r.p.TimeoutS {
			return fail(CauseUsage, "--timeout %d is above the profile's timeout_s %d", r.req.TimeoutS, r.p.TimeoutS)
		}
		ts = r.req.TimeoutS
	}
	r.timeout = time.Duration(ts) * time.Second
	if r.d.TimeoutOverride > 0 {
		r.timeout = r.d.TimeoutOverride
	}

	if r.req.ExecID != "" {
		r.execID = r.req.ExecID
		if r.d.WS.Exists(wsfs.ExecRel(r.execID)) {
			return fail(CauseUsage, "exec dir for --exec-id %s already exists", r.execID)
		}
	} else {
		for i := 0; ; i++ {
			r.execID = r.d.NewExecID()
			if !r.d.WS.Exists(wsfs.ExecRel(r.execID)) {
				break
			}
			if i > 10 {
				return fail(CauseSetup, "cannot find a free exec ID")
			}
		}
	}
	r.execRel = wsfs.ExecRel(r.execID)

	if f := r.validateInputs(); f != nil {
		return f
	}
	if f := r.validateParams(); f != nil {
		return f
	}
	if f := r.buildArgv(); f != nil {
		return f
	}
	return r.applyInputPolicy()
}

// validateParams checks every --param against the typed params of a tool profile and fills in the
// defaults (SFR-AGT-008, exc-8). Other kinds take no --param (the parser already refuses it).
func (r *run) validateParams() *failure {
	if r.p.Kind != profile.KindTool {
		if len(r.req.Params) > 0 {
			return fail(CauseUsage, "--param is for kind tool only")
		}
		return nil
	}
	r.params = map[string]string{}
	for n, v := range r.req.Params {
		decl, ok := r.p.Params[n]
		if !ok {
			return fail(CauseUsage, "--param %s: the profile declares no param of that name", n)
		}
		c, err := decl.Check(v)
		if err != nil {
			return fail(CauseUsage, "--param %s: %v", n, err)
		}
		r.params[n] = c
	}
	for n, decl := range r.p.Params {
		if _, ok := r.params[n]; ok {
			continue
		}
		if decl.Default == nil {
			return fail(CauseUsage, "param %s is required by the profile (--param %s=<%s>)", n, n, decl.Type)
		}
		c, err := decl.Check(*decl.Default)
		if err != nil {
			return fail(CauseSetup, "params.%s default: %v", n, err)
		}
		r.params[n] = c
	}
	return nil
}

// applyInputPolicy is USC-009 step 5: the profile's input_policy on the source files, after
// validation and before anything is created. A refusal is exit 125, cause policy (SFR-AGT-009, exc-9).
func (r *run) applyInputPolicy() *failure {
	if r.p.Kind != profile.KindTool || r.p.InputPolicy == "" {
		return nil
	}
	var files []inputpolicy.File
	for _, in := range r.inputs {
		files = append(files, inputpolicy.File{Name: in.name, Rel: in.rel, ContainerPath: in.containerPath})
	}
	if err := inputpolicy.Check(r.p.InputPolicy, r.d.WS, files); err != nil {
		return fail(CausePolicy, "%s: %v", r.p.InputPolicy, err)
	}
	return nil
}

func under(rel, prefix string) bool {
	return rel == strings.TrimSuffix(prefix, "/") || strings.HasPrefix(rel, strings.TrimSuffix(prefix, "/")+"/")
}

// validateInputs checks every --in against the profile (SFR-AGE-001, SFR-AGE-007, SSR-AGE-013).
func (r *run) validateInputs() *failure {
	byName := map[string][]*input{}
	for _, in := range r.req.Inputs {
		decl, ok := r.p.Inputs[in.Name]
		if !ok {
			return fail(CauseUsage, "--in %s: the profile declares no input of that name", in.Name)
		}
		rel, err := r.d.WS.Rel(in.Path)
		if err != nil {
			return fail(CauseUsage, "--in %s: %v", in.Name, err)
		}
		allowed := false
		for _, pu := range decl.PathUnder {
			if under(rel, path.Clean(pu)) {
				allowed = true
			}
		}
		if !allowed {
			return fail(CauseUsage, "--in %s: %s is not under an allowed folder of the profile", in.Name, in.Path)
		}
		if decl.Glob != "" {
			if m, _ := path.Match(decl.Glob, path.Base(rel)); !m {
				return fail(CauseUsage, "--in %s: %s does not match glob %s", in.Name, in.Path, decl.Glob)
			}
		}
		st, err := r.d.WS.Stat(rel)
		if err != nil {
			return fail(CauseUsage, "--in %s: %s is not readable: %v", in.Name, in.Path, err)
		}
		ip := &input{name: in.Name, source: in.Path, rel: rel, base: path.Base(rel), multiple: decl.Multiple, st: st}
		ip.inPlace = under(rel, "src") || (r.mountsBuild() && under(rel, "build"))
		if (!ip.inPlace || r.p.Kind == profile.KindTool) && !st.IsRegular() {
			return fail(CauseUsage, "--in %s: %s must be a regular file", in.Name, in.Path)
		}
		if st.IsRegular() {
			f, err := r.d.WS.OpenRegular(rel)
			if err != nil {
				return fail(CauseUsage, "--in %s: %s is not readable: %v", in.Name, in.Path, err)
			}
			f.Close()
		}
		for _, prev := range byName[in.Name] {
			if !decl.Multiple {
				return fail(CauseUsage, "--in %s given twice but the profile does not allow multiple", in.Name)
			}
			if prev.base == ip.base {
				return fail(CauseUsage, "--in %s: duplicate basename %s", in.Name, ip.base)
			}
		}
		switch {
		case ip.inPlace:
			ip.containerPath = kind.ContainerRoot + "/" + rel
		case decl.Multiple:
			ip.stagedRel = r.execRel + "/in/" + in.Name + "/" + ip.base
			ip.containerPath = kind.ContainerRoot + "/" + ip.stagedRel
		default:
			ip.stagedRel = r.execRel + "/in/" + in.Name
			ip.containerPath = kind.ContainerRoot + "/" + ip.stagedRel
		}
		byName[in.Name] = append(byName[in.Name], ip)
		r.inputs = append(r.inputs, ip)
	}
	for n, decl := range r.p.Inputs {
		if !decl.Optional && len(byName[n]) == 0 {
			return fail(CauseUsage, "input %s is required by the profile (--in %s=<path>)", n, n)
		}
	}
	return nil
}

func (r *run) mountsBuild() bool {
	return r.p.Kind == profile.KindBuild || ((r.p.Kind == profile.KindCode || r.p.Kind == profile.KindTool) && r.p.BuildDir)
}

// ExpandIn expands {in.<name>} in command arguments: an argument that is exactly the placeholder
// becomes the staged (or in-place) paths, one argument each; inside a longer argument only a name
// with a single path is replaced.
func ExpandIn(args []string, paths map[string][]string) []string {
	var out []string
	for _, a := range args {
		if strings.HasPrefix(a, "{in.") && strings.HasSuffix(a, "}") {
			if ps, ok := paths[a[4:len(a)-1]]; ok {
				out = append(out, ps...)
				continue
			}
		}
		for n, ps := range paths {
			if len(ps) == 1 {
				a = strings.ReplaceAll(a, "{in."+n+"}", ps[0])
			}
		}
		out = append(out, a)
	}
	return out
}

// buildArgv fixes the command line of every container (SSR-AGE-005: only the profile and data).
func (r *run) buildArgv() *failure {
	r.argv = map[string][]string{}
	paths := map[string][]string{}
	for _, in := range r.inputs {
		paths[in.name] = append(paths[in.name], in.containerPath)
	}
	switch r.p.Kind {
	case profile.KindCode:
		r.argv[kind.PhaseRun] = ExpandIn(r.req.Args, paths)
	case profile.KindBuild:
		r.argv[kind.PhaseRun] = append(profile.Substitute(r.p.Argv, r.execID), ExpandIn(r.req.Args, paths)...)
	case profile.KindTool:
		// The profile's argv with only the placeholders expanded; nothing appended (SFR-AGT-010).
		out := kind.ContainerRoot + "/" + r.execRel + "/out"
		r.argv[kind.PhaseRun] = profile.ExpandToolArgv(r.p.Argv, out, paths, r.params)
	case profile.KindInstall:
		var cpaths []string
		for _, m := range r.req.Manifests {
			rel, err := r.d.WS.Rel(m)
			if err != nil || !under(rel, "src") || rel == "src" {
				return fail(CauseUsage, "--manifest %s is not a path under src/", m)
			}
			st, err := r.d.WS.Stat(rel)
			if err != nil || !st.IsRegular() {
				return fail(CauseUsage, "--manifest %s does not exist or is not a file", m)
			}
			r.manifestRel = append(r.manifestRel, rel)
			cpaths = append(cpaths, kind.ContainerRoot+"/"+rel)
		}
		margs, err := kind.ManifestArgs(r.p, cpaths)
		if err != nil {
			return fail(CauseSetup, "%v", err)
		}
		r.argv[kind.PhaseFetch] = append(profile.Substitute(r.p.FetchArgv, r.execID), margs...)
		r.argv[kind.PhaseInstall] = append(profile.Substitute(r.p.Argv, r.execID), margs...)
	}
	return nil
}

// prepare is USC-005 step 3: engine reachable, stale containers removed, workspace folders present,
// locks and slot taken, and as the last action the exec dir. Failures before the exec dir leave nothing.
func (r *run) prepare() *failure {
	if err := r.d.Engine.Ping(r.ctx); err != nil {
		return engineFail("engine unreachable", err) // SFR-AGE-003
	}
	alive, f := r.removeStale()
	if f != nil {
		return f
	}
	if f := r.checkFolders(); f != nil {
		return f
	}
	if f := r.takeLocks(); f != nil {
		return f
	}
	if f := r.checkSlot(alive); f != nil {
		return f
	}
	err := r.d.WS.CreateExecDir(r.execID, r.p.Kind != profile.KindInstall, r.p.Kind == profile.KindInstall)
	if errors.Is(err, os.ErrExist) {
		return fail(CauseUsage, "exec dir for %s already exists", r.execID)
	}
	if err != nil {
		return fail(CauseSetup, "cannot create exec dir: %v", err)
	}
	r.start = r.d.Now()
	fmt.Fprintf(r.d.Stderr, "%s: exec_dir=%s\n", Prog, r.d.WS.Abs(r.execRel)) // first stderr line, SFR-AGE-004
	return nil
}

// removeStale removes every ephemeral.exec container whose ephemeral.pid is gone (SFR-AGE-006)
// and returns the others. It prints nothing: exec_dir must be the first stderr line.
func (r *run) removeStale() ([]engine.ContainerSummary, *failure) {
	list, err := r.d.Engine.ContainerList(r.ctx, []string{kind.LabelExec}, true)
	if err != nil {
		return nil, engineFail("list containers", err)
	}
	var alive []engine.ContainerSummary
	for _, c := range list {
		pid, err := strconv.Atoi(c.Labels[kind.LabelPid])
		if err == nil && pid > 0 && r.d.PidAlive(pid) {
			alive = append(alive, c)
			continue
		}
		_ = r.d.Engine.ContainerRemove(r.ctx, c.ID)
	}
	return alive, nil
}

// checkFolders: the folders agent-host root creates must exist; the tool never creates them (SFR-AGE-004, exc-6).
func (r *run) checkFolders() *failure {
	need := []string{"src"}
	if r.mountsBuild() {
		need = append(need, "build")
	}
	need = append(need, r.p.SortedEnvDirs()...)
	if r.p.Kind == profile.KindInstall {
		need = append(need, ".packages")
	}
	for _, f := range need {
		st, err := r.d.WS.Stat(f)
		if err != nil || !st.IsDir() {
			return fail(CauseSetup, "workspace folder %s is missing (agent-host root creates it; the tool never does)", f)
		}
	}
	if r.p.RequiresBuild {
		names, err := r.d.WS.ReadDir("build")
		if err != nil || len(names) == 0 {
			return fail(CauseSetup, "build/ is missing or empty but the profile has requires_build (run build first)")
		}
	}
	return nil
}

// takeLocks: exclusive on a rw folder, shared on a ro one, bounded wait then `busy`. SFR-AGE-005, SSR-AGE-011.
func (r *run) takeLocks() *failure {
	type spec struct {
		rel string
		ex  bool
	}
	var specs []spec
	switch r.p.Kind {
	case profile.KindCode, profile.KindTool:
		if r.p.BuildDir {
			specs = append(specs, spec{"build", false})
		}
	case profile.KindBuild:
		specs = append(specs, spec{"build", true})
	}
	for _, d := range r.p.SortedEnvDirs() {
		specs = append(specs, spec{d, r.p.EnvDirs[d] == "rw"})
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].rel < specs[j].rel })
	for _, s := range specs {
		l, err := r.d.WS.Lock(s.rel, s.ex, r.d.LockWait)
		if errors.Is(err, wsfs.ErrBusy) {
			return fail(CauseBusy, "%s is locked by another execution", s.rel)
		}
		if err != nil {
			return fail(CauseSetup, "cannot lock %s: %v", s.rel, err)
		}
		r.locks = append(r.locks, l)
	}
	return nil
}

func (r *run) releaseLocks() {
	for _, l := range r.locks {
		l.Release()
	}
	r.locks = nil
}

// checkSlot sums the limits of the executions already present (the larger container of an
// install pair counts once) plus this one against the agent container's bounds (SSR-AGC-012).
// How concurrent tool processes share the count is TBD (no central process): a race is possible.
func (r *run) checkSlot(list []engine.ContainerSummary) *failure {
	type res struct{ mem, cpu, pids int64 }
	groups := map[string]*res{}
	for _, c := range list {
		if c.State != "running" && c.State != "created" {
			continue
		}
		info, err := r.d.Engine.ContainerInspect(r.ctx, c.ID)
		if err != nil {
			continue
		}
		g := groups[c.Labels[kind.LabelExec]]
		if g == nil {
			g = &res{}
			groups[c.Labels[kind.LabelExec]] = g
		}
		g.mem = max64(g.mem, info.HostConfig.Memory)
		g.cpu = max64(g.cpu, info.HostConfig.NanoCpus)
		g.pids = max64(g.pids, info.HostConfig.PidsLimit)
	}
	sum := res{r.p.MemBytes, r.p.NanoCPUs, r.p.Pids}
	for _, g := range groups {
		sum.mem += g.mem
		sum.cpu += g.cpu
		sum.pids += g.pids
	}
	if sum.mem > limits.AgentMemoryBytes || sum.cpu > limits.AgentNanoCPUs || sum.pids > limits.AgentPids {
		return fail(CauseBusy, "no slot: the summed limits of the running executions would exceed the agent container's bounds")
	}
	return nil
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// execute runs steps 4 to 9 once the exec dir exists.
func (r *run) execute() Outcome {
	steps := []func() *failure{r.stage, r.ensureImage, r.createAll, r.verifyAll, r.writeMeta}
	for _, s := range steps {
		if r.cancelled() {
			return r.cancelOutcome()
		}
		if f := s(); f != nil {
			return r.report(f)
		}
	}
	if r.cancelled() {
		return r.cancelOutcome()
	}
	return r.runPhases()
}

// stage is USC-005 step 4: files under src/ (or a mounted build/) are used in place, others are copied
// into in/ with sha256 and a size cap (exc-5). SFR-AGE-007.
func (r *run) stage() *failure {
	budget := limits.MaxStageBytes
	for _, in := range r.inputs {
		if in.inPlace {
			if in.st.IsRegular() {
				res, err := r.d.WS.HashFile(in.rel)
				if err != nil {
					return fail(CauseSetup, "cannot read input %s: %v", in.name, err)
				}
				in.sha = res.SHA256
			}
			continue
		}
		if in.multiple {
			err := r.d.WS.Mkdir(r.execRel+"/in/"+in.name, 0o755)
			if err != nil && !errors.Is(err, os.ErrExist) {
				return fail(CauseSetup, "cannot stage input %s: %v", in.name, err)
			}
		}
		res, err := r.d.WS.CopyFile(in.rel, in.stagedRel, budget)
		if err != nil {
			return fail(CauseSetup, "cannot stage input %s: %v", in.name, err)
		}
		budget -= res.Size
		in.sha = res.SHA256
	}
	return nil
}

// ensureImage is USC-005 step 5: lazy load of image.tar when the image is absent; never pull or build.
// SFR-AGE-008, SSR-AGE-006.
func (r *run) ensureImage() *failure {
	eng := r.d.Engine
	img, err := eng.ImageInspect(r.ctx, r.p.ImageID)
	if err != nil && !errors.Is(err, engine.ErrNotFound) {
		return engineFail("inspect image", err)
	}
	if err != nil {
		tarRel := profile.ToolsDir + "/" + r.p.Name + "/image.tar"
		f, oerr := r.d.WS.OpenRegular(tarRel)
		if oerr != nil {
			return fail(CauseSetup, "image %s is not loaded and image.tar is not readable: %v", r.p.ImageID, oerr)
		}
		defer f.Close()
		ids, terr := profile.TarImageIDs(f)
		if terr != nil {
			return fail(CauseSetup, "image.tar: %v", terr)
		}
		found := false
		for _, id := range ids {
			if id == r.p.ImageID {
				found = true
			}
		}
		if !found {
			return fail(CauseSetup, "image.tar holds %s but the profile pins %s", strings.Join(ids, " or "), r.p.ImageID)
		}
		if _, serr := f.Seek(0, io.SeekStart); serr != nil {
			return fail(CauseSetup, "image.tar: %v", serr)
		}
		if lerr := eng.ImageLoad(r.ctx, f); lerr != nil {
			return engineFail("load image.tar", lerr)
		}
		img, err = eng.ImageInspect(r.ctx, r.p.ImageID)
		if err != nil {
			return engineFail("image not available after load", err)
		}
	}
	if img.ID != r.p.ImageID {
		return fail(CauseSetup, "engine image ID %s differs from the profile's %s", img.ID, r.p.ImageID)
	}
	r.imageEnv = img.Config.Env
	return nil
}

// createAll is USC-005 step 6: create (not start) every container of the kind. SFR-AGE-009, SSR-AGE-009.
func (r *run) createAll() *failure {
	for _, ph := range kind.Phases(r.p.Kind) {
		pl := kind.NewPlan(kind.Params{
			Profile: r.p, ExecID: r.execID, Phase: ph, WorkspaceRoot: r.d.WS.Root,
			Argv: r.argv[ph], Labels: r.req.Labels, Pid: r.d.Pid,
		})
		id, err := r.d.Engine.ContainerCreate(r.ctx, pl.Name, pl.Body)
		if err != nil {
			return engineFail("create "+pl.Name, err)
		}
		r.ctrs = append(r.ctrs, &ctr{phase: ph, plan: pl, id: id})
	}
	return nil
}

// verifyAll is step 7: inspect and verify; a mismatch ends the call with `verify` (cleanup removes the containers).
func (r *run) verifyAll() *failure {
	for _, c := range r.ctrs {
		info, err := r.d.Engine.ContainerInspect(r.ctx, c.id)
		if err != nil {
			return fail(CauseVerify, "cannot inspect %s: %v", c.plan.Name, err)
		}
		if bad := kind.Verify(c.plan, info, r.imageEnv); len(bad) > 0 {
			return fail(CauseVerify, "%s does not match the fixed settings: %s", c.plan.Name, strings.Join(bad, "; "))
		}
		c.info = info
	}
	return nil
}

// cleanup is step 10: make sure every container is gone (AutoRemove, and here explicitly).
func (r *run) cleanup() {
	for _, c := range r.ctrs {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		if err := r.d.Engine.ContainerRemove(ctx, c.id); err != nil {
			r.diag("cannot remove container %s: %v", c.plan.Name, err)
		}
		cancel()
	}
}

// Sweep removes only the containers that carry the label (kill if running, then remove). SFR-AGE-019.
func Sweep(d *Deps, key, val string) Outcome {
	d.defaults()
	r := &run{d: d, ctx: context.Background()}
	if err := d.Engine.Ping(r.ctx); err != nil {
		return r.report(engineFail("engine unreachable", err))
	}
	list, err := d.Engine.ContainerList(r.ctx, []string{key + "=" + val}, true)
	if err != nil {
		return r.report(engineFail("list containers", err))
	}
	for _, c := range list {
		if c.Labels[key] != val {
			continue // never touch a container without the exact label
		}
		if err := d.Engine.ContainerRemove(r.ctx, c.ID); err != nil {
			return r.report(engineFail("remove container", err))
		}
		fmt.Fprintf(d.Stderr, "%s: swept %s\n", Prog, strings.TrimPrefix(strings.Join(c.Names, ","), "/"))
	}
	return Outcome{Exit: 0, Cause: CauseOK}
}
