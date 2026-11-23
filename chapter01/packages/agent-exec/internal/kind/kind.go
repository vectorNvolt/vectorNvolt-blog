// Package kind is the compiled-in kind model: for each kind and phase the mounts, their modes,
// the network and the writable areas, and the create body built from it. A profile only selects
// within it. SSR-AGE-014 (kind model fixed), SSR-AGE-001 (fixed unprivileged settings),
// SSR-AGE-002 (no network), SSR-AGE-003 (mount allowlist), SSR-AGE-004 (bounded writable areas),
// SFR-AGE-009 (create with fixed settings), SFR-AGE-011 (verify the created container),
// SSR-AGT-006 (tool-kind mount model).
package kind

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"agent-exec/internal/engine"
	"agent-exec/internal/limits"
	"agent-exec/internal/profile"
	"agent-exec/internal/wsfs"
)

const (
	PhaseRun     = "run"
	PhaseFetch   = "fetch"
	PhaseInstall = "install"

	ContainerRoot = "/workspace"
	LabelExec     = "ephemeral.exec"
	LabelPid      = "ephemeral.pid"
	LabelPrefix   = "ephemeral."
)

// Mount is one bind mount; Rel is relative to the workspace, the target is /workspace/<Rel>.
type Mount struct {
	Rel string
	RW  bool
}

func (m Mount) Target() string { return ContainerRoot + "/" + m.Rel }

// Phases lists the containers of a kind in start order.
func Phases(kindName string) []string {
	if kindName == profile.KindInstall {
		return []string{PhaseFetch, PhaseInstall}
	}
	return []string{PhaseRun}
}

// ContainerName is ephemeral-<id>, and ephemeral-<id>-fetch for the fetch phase. SSR-AGE-009.
func ContainerName(execID, phase string) string {
	if phase == PhaseFetch {
		return "ephemeral-" + execID + "-fetch"
	}
	return "ephemeral-" + execID
}

// Network is fixed per kind and phase: only the fetch phase of install is `host`. SSR-AGE-002.
func Network(kindName, phase string) string {
	if kindName == profile.KindInstall && phase == PhaseFetch {
		return "host"
	}
	return "none"
}

// Mounts is the exact bind allowlist of a kind and phase (USS-005 "Mounts"). SSR-AGE-003/014.
// Kind tool (USS-009, SSR-AGT-006): src/ ro, build/ ro only if declared, own in/ ro, own out/ rw;
// no env dir, no other folder.
func Mounts(p *profile.Profile, execID, phase string) []Mount {
	exec := wsfs.ExecRel(execID)
	m := []Mount{{Rel: "src"}}
	switch p.Kind {
	case profile.KindCode:
		if p.BuildDir {
			m = append(m, Mount{Rel: "build"})
		}
		for _, d := range p.SortedEnvDirs() {
			m = append(m, Mount{Rel: d})
		}
		m = append(m, Mount{Rel: exec + "/in"})
	case profile.KindTool:
		if p.BuildDir {
			m = append(m, Mount{Rel: "build"})
		}
		m = append(m, Mount{Rel: exec + "/in"})
	case profile.KindBuild:
		m = append(m, Mount{Rel: "build", RW: true})
		for _, d := range p.SortedEnvDirs() {
			m = append(m, Mount{Rel: d})
		}
		m = append(m, Mount{Rel: exec + "/in"})
	case profile.KindInstall:
		m = append(m, Mount{Rel: ".packages"})
		if phase == PhaseFetch {
			m = append(m, Mount{Rel: exec + "/dl", RW: true})
		} else {
			m = append(m, Mount{Rel: exec + "/dl"})
			for _, d := range p.SortedEnvDirs() {
				m = append(m, Mount{Rel: d, RW: p.EnvDirs[d] == "rw"})
			}
		}
	}
	return append(m, Mount{Rel: exec + "/out", RW: true})
}

// Tmpfs are the three size-limited scratch areas; /tmp is noexec unless the profile says tmp_exec. SSR-AGE-004.
func Tmpfs(p *profile.Profile) map[string]string {
	return map[string]string{
		"/work":     tmpfsOpts(limits.WorkTmpfsBytes, false),
		"/tmp":      tmpfsOpts(limits.TmpTmpfsBytes, p.TmpExec),
		"/home/run": tmpfsOpts(limits.HomeTmpfsBytes, false),
	}
}

func tmpfsOpts(size int64, exec bool) string {
	e := "noexec"
	if exec {
		e = "exec"
	}
	return fmt.Sprintf("rw,%s,nosuid,nodev,size=%d,mode=1777", e, size)
}

// WatchDirs are the workspace folders whose growth out_max limits (out/, build/, dl/, target env dir).
func WatchDirs(p *profile.Profile, execID string) []string {
	exec := wsfs.ExecRel(execID)
	d := []string{exec + "/out"}
	switch p.Kind {
	case profile.KindBuild:
		d = append(d, "build")
	case profile.KindInstall:
		d = append(d, exec+"/dl", p.TargetEnvDir())
	}
	return d
}

// ToolEnv is the environment the tool sets: HOME, LANG, TZ, OUT, IN (not install) and the profile env.
func ToolEnv(p *profile.Profile, execID string) []string {
	exec := ContainerRoot + "/" + wsfs.ExecRel(execID)
	env := map[string]string{
		"HOME": "/home/run",
		"LANG": "C.UTF-8",
		"TZ":   "UTC",
		"OUT":  exec + "/out",
	}
	if p.Kind != profile.KindInstall {
		env["IN"] = exec + "/in"
	}
	for k, v := range p.Env {
		env[k] = v
	}
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}

// ManifestArgs builds the manager arguments from the manifests (container paths). pip: -r <manifest>;
// the form for npm and conda is TBD (USS-005 open item), so those targets are refused.
func ManifestArgs(p *profile.Profile, manifests []string) ([]string, error) {
	if t := p.TargetEnvDir(); t != ".venv" {
		return nil, fmt.Errorf("the --manifest form for target %s is TBD", t)
	}
	var out []string
	for _, m := range manifests {
		out = append(out, "-r", m)
	}
	return out, nil
}

// Params is everything one container needs.
type Params struct {
	Profile       *profile.Profile
	ExecID        string
	Phase         string
	WorkspaceRoot string // host-side source root of the binds (default /workspace)
	Argv          []string
	Labels        map[string]string // caller labels
	Pid           int
}

// Plan is a container to create and what to expect when inspecting it.
type Plan struct {
	Params Params
	Name   string
	Body   engine.CreateBody
	Mounts []Mount
	Tmpfs  map[string]string
	Env    []string
}

// NewPlan builds the create body with the fixed settings (SFR-AGE-009). Caller labels cannot override ephemeral.*.
func NewPlan(par Params) *Plan {
	p := par.Profile
	pl := &Plan{
		Params: par,
		Name:   ContainerName(par.ExecID, par.Phase),
		Mounts: Mounts(p, par.ExecID, par.Phase),
		Tmpfs:  Tmpfs(p),
		Env:    ToolEnv(p, par.ExecID),
	}
	labels := map[string]string{}
	for k, v := range par.Labels {
		if !strings.HasPrefix(k, LabelPrefix) {
			labels[k] = v
		}
	}
	labels[LabelExec] = par.ExecID
	labels[LabelPid] = fmt.Sprint(par.Pid)

	var mounts []engine.Mount
	for _, m := range pl.Mounts {
		mounts = append(mounts, engine.Mount{
			Type: "bind", Source: filepath.Join(par.WorkspaceRoot, m.Rel), Target: m.Target(), ReadOnly: !m.RW,
		})
	}
	pl.Body = engine.CreateBody{
		Image:        p.ImageID,
		User:         fmt.Sprintf("%d:%d", limits.AgentUID, limits.AgentGID),
		Env:          pl.Env,
		Cmd:          par.Argv,
		Entrypoint:   []string{},
		WorkingDir:   "/work",
		Labels:       labels,
		AttachStdout: true,
		AttachStderr: true,
		HostConfig: engine.HostConfig{
			Mounts:         mounts,
			Tmpfs:          pl.Tmpfs,
			NetworkMode:    Network(p.Kind, par.Phase),
			CapDrop:        []string{"ALL"},
			SecurityOpt:    []string{"no-new-privileges"},
			ReadonlyRootfs: true,
			AutoRemove:     true,
			Memory:         p.MemBytes,
			NanoCpus:       p.NanoCPUs,
			PidsLimit:      p.Pids,
			IpcMode:        "private",
			// gVisor has no cgroup namespaces; "host" is the agent container's own view, not agent-host's.
			CgroupnsMode: "host",
		},
	}
	return pl
}

// defaultPath is what the engine adds when neither the image nor the tool sets PATH.
const defaultPath = "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

// Verify compares the inspected container with the fixed settings and returns every mismatch
// (empty: ok). It checks the inspected container, not the tool's own request. SFR-AGE-011, exc-9.
func Verify(pl *Plan, info *engine.Inspect, imageEnv []string) []string {
	var bad []string
	add := func(format string, a ...any) { bad = append(bad, fmt.Sprintf(format, a...)) }
	p := pl.Params.Profile
	hc := info.HostConfig

	if strings.TrimPrefix(info.Name, "/") != pl.Name {
		add("name %q, want %q", info.Name, pl.Name)
	}
	if info.Image != p.ImageID {
		add("image %q, want %q", info.Image, p.ImageID)
	}
	if want := fmt.Sprintf("%d:%d", limits.AgentUID, limits.AgentGID); info.Config.User != want {
		add("user %q, want %s", info.Config.User, want)
	}
	if len(hc.CapDrop) != 1 || hc.CapDrop[0] != "ALL" {
		add("CapDrop %v, want [ALL]", hc.CapDrop)
	}
	if len(hc.CapAdd) != 0 {
		add("CapAdd %v, want none", hc.CapAdd)
	}
	if hc.Privileged {
		add("Privileged is true")
	}
	if !nnp(hc.SecurityOpt) {
		add("SecurityOpt %v, want only no-new-privileges", hc.SecurityOpt)
	}
	if !hc.ReadonlyRootfs {
		add("ReadonlyRootfs is false")
	}
	if want := Network(p.Kind, pl.Params.Phase); hc.NetworkMode != want {
		add("NetworkMode %q, want %q", hc.NetworkMode, want)
	}
	if !hc.AutoRemove {
		add("AutoRemove is false")
	}
	if hc.OomKillDisable {
		add("OomKillDisable is true")
	}
	if hc.Memory != p.MemBytes {
		add("Memory %d, want %d", hc.Memory, p.MemBytes)
	}
	if hc.NanoCpus != p.NanoCPUs {
		add("NanoCpus %d, want %d", hc.NanoCpus, p.NanoCPUs)
	}
	if hc.PidsLimit != p.Pids {
		add("PidsLimit %d, want %d", hc.PidsLimit, p.Pids)
	}
	for name, v := range map[string]string{"PidMode": hc.PidMode, "IpcMode": hc.IpcMode, "UTSMode": hc.UTSMode,
		"UsernsMode": hc.UsernsMode} {
		if v == "host" || strings.HasPrefix(v, "container:") {
			add("%s %q is not private", name, v)
		}
	}
	// Only the cgroup namespace is host (gVisor has no cgroup namespaces; host = the agent container's view).
	if hc.CgroupnsMode != "host" {
		add("CgroupnsMode %q, want \"host\"", hc.CgroupnsMode)
	}
	if hc.CgroupParent != "" {
		add("CgroupParent %q, want none", hc.CgroupParent)
	}
	if hc.Runtime != "" && hc.Runtime != "runc" {
		add("Runtime %q, want the default", hc.Runtime)
	}
	if len(hc.GroupAdd)+len(hc.Binds)+len(hc.Links)+len(hc.VolumesFrom)+len(hc.Devices)+len(hc.Sysctls)+len(hc.PortBindings) != 0 {
		add("forbidden HostConfig field set (GroupAdd, Binds, Links, VolumesFrom, Devices, Sysctls or PortBindings)")
	}
	if info.Config.Tty || info.Config.OpenStdin {
		add("stdin or tty is open")
	}
	if info.Config.WorkingDir != "/work" {
		add("WorkingDir %q, want /work", info.Config.WorkingDir)
	}
	if len(info.Config.Entrypoint) != 0 {
		add("Entrypoint %v, want none", info.Config.Entrypoint)
	}
	if strings.Join(info.Config.Cmd, "\x00") != strings.Join(pl.Params.Argv, "\x00") {
		add("Cmd differs from the requested command")
	}
	if info.Config.Labels[LabelExec] != pl.Params.ExecID {
		add("label %s is %q, want %q", LabelExec, info.Config.Labels[LabelExec], pl.Params.ExecID)
	}
	bad = append(bad, mountMismatches(pl, info)...)
	bad = append(bad, tmpfsMismatches(pl, info)...)
	bad = append(bad, envMismatches(pl.Env, info.Config.Env, imageEnv)...)
	return bad
}

func nnp(opts []string) bool {
	if len(opts) == 0 {
		return false
	}
	for _, o := range opts {
		if o != "no-new-privileges" && o != "no-new-privileges=true" && o != "no-new-privileges:true" {
			return false
		}
	}
	return true
}

// mountMismatches checks the exact bind allowlist; tmpfs entries of the top-level list are checked in tmpfsMismatches.
func mountMismatches(pl *Plan, info *engine.Inspect) []string {
	var bad []string
	want := map[string]Mount{}
	for _, m := range pl.Mounts {
		want[m.Target()] = m
	}
	seen := map[string]bool{}
	for _, mp := range info.Mounts {
		if mp.Type == "tmpfs" {
			if _, ok := pl.Tmpfs[mp.Destination]; !ok {
				bad = append(bad, fmt.Sprintf("unexpected tmpfs mount %s", mp.Destination))
			}
			continue
		}
		w, ok := want[mp.Destination]
		switch {
		case !ok:
			bad = append(bad, fmt.Sprintf("unexpected mount %s (%s)", mp.Destination, mp.Type))
		case mp.Type != "bind":
			bad = append(bad, fmt.Sprintf("mount %s has type %s, want bind", mp.Destination, mp.Type))
		case filepath.Clean(mp.Source) != filepath.Join(pl.Params.WorkspaceRoot, w.Rel):
			bad = append(bad, fmt.Sprintf("mount %s has source %s, want %s", mp.Destination, mp.Source, filepath.Join(pl.Params.WorkspaceRoot, w.Rel)))
		case mp.RW != w.RW:
			bad = append(bad, fmt.Sprintf("mount %s rw=%v, want %v", mp.Destination, mp.RW, w.RW))
		}
		seen[mp.Destination] = true
	}
	for t := range want {
		if !seen[t] {
			bad = append(bad, fmt.Sprintf("mount %s is missing", t))
		}
	}
	return bad
}

func tmpfsMismatches(pl *Plan, info *engine.Inspect) []string {
	var bad []string
	got := info.HostConfig.Tmpfs
	for target, want := range pl.Tmpfs {
		g, ok := got[target]
		if !ok {
			bad = append(bad, fmt.Sprintf("tmpfs %s is missing", target))
			continue
		}
		if tmpfsKey(g) != tmpfsKey(want) {
			bad = append(bad, fmt.Sprintf("tmpfs %s options %q, want %q", target, g, want))
		}
	}
	for target := range got {
		if _, ok := pl.Tmpfs[target]; !ok {
			bad = append(bad, fmt.Sprintf("unexpected tmpfs %s", target))
		}
	}
	return bad
}

// tmpfsKey reduces tmpfs options to the parts that matter: size and exec/noexec.
func tmpfsKey(opts string) string {
	size, exec := "", "exec"
	for _, o := range strings.Split(opts, ",") {
		switch {
		case strings.HasPrefix(o, "size="):
			size = o
		case o == "noexec":
			exec = "noexec"
		}
	}
	return size + "," + exec
}

// envMismatches: every variable the tool sets is present with its value, and every other
// variable comes from the image (or is the engine's default PATH). Values are not echoed.
func envMismatches(tool, got, image []string) []string {
	var bad []string
	toolKV := map[string]string{}
	for _, e := range tool {
		k, v, _ := strings.Cut(e, "=")
		toolKV[k] = v
	}
	imageSet := map[string]bool{defaultPath: true}
	for _, e := range image {
		imageSet[e] = true
	}
	gotKV := map[string]string{}
	for _, e := range got {
		k, v, _ := strings.Cut(e, "=")
		gotKV[k] = v
		if tv, ok := toolKV[k]; ok {
			if tv != v {
				bad = append(bad, fmt.Sprintf("environment variable %s differs from the tool's value", k))
			}
			continue
		}
		if !imageSet[e] {
			bad = append(bad, fmt.Sprintf("environment variable %s is neither the tool's nor the image's", k))
		}
	}
	for k := range toolKV {
		if _, ok := gotKV[k]; !ok {
			bad = append(bad, fmt.Sprintf("environment variable %s is missing", k))
		}
	}
	sort.Strings(bad)
	return bad
}
