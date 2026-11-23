package kind

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"agent-exec/internal/engine"
	"agent-exec/internal/limits"
	"agent-exec/internal/profile"
)

var imageID = "sha256:" + strings.Repeat("a", 64)

const lim = "limits: { mem: 512m, cpus: 1, pids: 128, timeout_s: 60, out_max: 64m }\n"

func parse(t *testing.T, name, y string) *profile.Profile {
	t.Helper()
	p, err := profile.Parse([]byte(strings.ReplaceAll(y, "IMAGEID", imageID)+lim), name)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func code(t *testing.T, extra string) *profile.Profile {
	return parse(t, "python", "name: python\nkind: code\nimage_id: IMAGEID\nentry: agent\nenv_dirs: { .venv: ro }\nenv: { PATH: /workspace/.venv/bin:/usr/bin }\n"+extra)
}

func build(t *testing.T) *profile.Profile {
	return parse(t, "c-make", "name: c-make\nkind: build\nimage_id: IMAGEID\nentry: fixed-args\nargv: [make]\nbuild_dir: true\nenv_dirs: { .npm-env: ro }\ntmp_exec: true\n")
}

func install(t *testing.T) *profile.Profile {
	return parse(t, "install-python", "name: install-python\nkind: install\nimage_id: IMAGEID\nentry: fixed-args\nfetch_argv: [pip, download]\nargv: [pip, install]\nenv_dirs: { .venv: rw, .npm-env: ro }\n")
}

func tool(t *testing.T, extra string) *profile.Profile {
	return parse(t, "ngspice", "name: ngspice\nkind: tool\nimage_id: IMAGEID\nentry: fixed\n"+
		"argv: [ngspice, -b, -n, -r, \"{out}/sim.raw\", \"{in.netlist}\"]\n"+
		"inputs: { netlist: { path_under: [src/], glob: '*.cir' } }\ninput_policy: spice-safe\n"+extra)
}

func mountModes(pl *Plan) map[string]string {
	out := map[string]string{}
	for _, m := range pl.Body.HostConfig.Mounts {
		mode := "rw"
		if m.ReadOnly {
			mode = "ro"
		}
		out[m.Target] = mode
	}
	return out
}

func TestMountModelPerKindAndPhase(t *testing.T) {
	cases := []struct {
		name string
		p    *profile.Profile
		ph   string
		want map[string]string
	}{
		{"code", code(t, ""), PhaseRun, map[string]string{
			"/workspace/src": "ro", "/workspace/.venv": "ro",
			"/workspace/.output/exec/e1/in": "ro", "/workspace/.output/exec/e1/out": "rw"}},
		{"code with build", code(t, "build_dir: true\n"), PhaseRun, map[string]string{
			"/workspace/src": "ro", "/workspace/build": "ro", "/workspace/.venv": "ro",
			"/workspace/.output/exec/e1/in": "ro", "/workspace/.output/exec/e1/out": "rw"}},
		{"tool", tool(t, ""), PhaseRun, map[string]string{
			"/workspace/src": "ro", "/workspace/.output/exec/e1/in": "ro", "/workspace/.output/exec/e1/out": "rw"}},
		{"tool with build", tool(t, "build_dir: true\n"), PhaseRun, map[string]string{
			"/workspace/src": "ro", "/workspace/build": "ro",
			"/workspace/.output/exec/e1/in": "ro", "/workspace/.output/exec/e1/out": "rw"}},
		{"build", build(t), PhaseRun, map[string]string{
			"/workspace/src": "ro", "/workspace/build": "rw", "/workspace/.npm-env": "ro",
			"/workspace/.output/exec/e1/in": "ro", "/workspace/.output/exec/e1/out": "rw"}},
		{"install fetch", install(t), PhaseFetch, map[string]string{
			"/workspace/src": "ro", "/workspace/.packages": "ro",
			"/workspace/.output/exec/e1/dl": "rw", "/workspace/.output/exec/e1/out": "rw"}},
		{"install install", install(t), PhaseInstall, map[string]string{
			"/workspace/src": "ro", "/workspace/.packages": "ro", "/workspace/.output/exec/e1/dl": "ro",
			"/workspace/.venv": "rw", "/workspace/.npm-env": "ro", "/workspace/.output/exec/e1/out": "rw"}},
	}
	for _, c := range cases {
		pl := NewPlan(Params{Profile: c.p, ExecID: "e1", Phase: c.ph, WorkspaceRoot: "/workspace", Argv: []string{"x"}, Pid: 7})
		got := mountModes(pl)
		if fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("%s: mounts %v, want %v", c.name, got, c.want)
		}
		for _, m := range pl.Body.HostConfig.Mounts {
			if m.Type != "bind" {
				t.Errorf("%s: mount type %s", c.name, m.Type)
			}
			for _, banned := range []string{".input", ".process", ".tools", ".agent", ".git", "docker.sock"} {
				if strings.Contains(m.Source, banned) || strings.Contains(m.Target, banned) {
					t.Errorf("%s: %s must never be mounted", c.name, banned)
				}
			}
		}
	}
}

func TestNetworkPerKind(t *testing.T) {
	if Network(profile.KindCode, PhaseRun) != "none" || Network(profile.KindBuild, PhaseRun) != "none" ||
		Network(profile.KindInstall, PhaseInstall) != "none" || Network(profile.KindInstall, PhaseFetch) != "host" {
		t.Fatal("network model")
	}
	if got := Phases(profile.KindInstall); len(got) != 2 || got[0] != PhaseFetch {
		t.Fatalf("phases %v", got)
	}
	if ContainerName("e1", PhaseFetch) != "ephemeral-e1-fetch" || ContainerName("e1", PhaseInstall) != "ephemeral-e1" || ContainerName("e1", PhaseRun) != "ephemeral-e1" {
		t.Fatal("container names")
	}
}

func TestCreateBodyFixedSettings(t *testing.T) {
	p := code(t, "")
	pl := NewPlan(Params{
		Profile: p, ExecID: "e1", Phase: PhaseRun, WorkspaceRoot: "/workspace", Argv: []string{"python", "-V"}, Pid: 4242,
		Labels: map[string]string{"agent-graph.run": "r1", "ephemeral.exec": "evil", "ephemeral.pid": "1"},
	})
	b, hc := pl.Body, pl.Body.HostConfig
	if b.Image != imageID || b.User != "1001:1001" || b.WorkingDir != "/work" || b.Tty || b.OpenStdin || b.AttachStdin {
		t.Errorf("config %+v", b)
	}
	if b.Entrypoint == nil || len(b.Entrypoint) != 0 {
		t.Errorf("entrypoint must be an empty non-nil list: %#v", b.Entrypoint)
	}
	if len(hc.CapDrop) != 1 || hc.CapDrop[0] != "ALL" || len(hc.CapAdd) != 0 || hc.Privileged {
		t.Errorf("caps %+v", hc)
	}
	if len(hc.SecurityOpt) != 1 || hc.SecurityOpt[0] != "no-new-privileges" || !hc.ReadonlyRootfs || !hc.AutoRemove || hc.OomKillDisable {
		t.Errorf("security %+v", hc)
	}
	if hc.NetworkMode != "none" || hc.PidMode != "" || hc.IpcMode != "private" || hc.CgroupnsMode != "host" || hc.UsernsMode != "" || hc.UTSMode != "" {
		t.Errorf("namespaces %+v", hc)
	}
	if hc.Memory != 512<<20 || hc.NanoCpus != 1_000_000_000 || hc.PidsLimit != 128 {
		t.Errorf("limits %+v", hc)
	}
	if hc.Memory > limits.MaxMemoryBytes || hc.NanoCpus > limits.MaxNanoCPUs || hc.PidsLimit > limits.MaxPids {
		t.Errorf("above the ceilings")
	}
	if b.Labels["ephemeral.exec"] != "e1" || b.Labels["ephemeral.pid"] != "4242" || b.Labels["agent-graph.run"] != "r1" || len(b.Labels) != 3 {
		t.Errorf("labels %v (caller labels must not override ephemeral.*)", b.Labels)
	}
	var tm []string
	for k := range hc.Tmpfs {
		tm = append(tm, k)
	}
	sort.Strings(tm)
	if strings.Join(tm, ",") != "/home/run,/tmp,/work" {
		t.Errorf("tmpfs %v", tm)
	}
	for k, v := range hc.Tmpfs {
		if !strings.Contains(v, "size=") {
			t.Errorf("tmpfs %s without size: %s", k, v)
		}
	}
	if !strings.Contains(hc.Tmpfs["/tmp"], "noexec") {
		t.Errorf("/tmp must be noexec unless tmp_exec: %s", hc.Tmpfs["/tmp"])
	}
	if hc.Tmpfs["/tmp"] != "rw,noexec,nosuid,nodev,size=536870912,mode=1777" {
		t.Errorf("tmp options %s", hc.Tmpfs["/tmp"])
	}
	if tmpl := NewPlan(Params{Profile: build(t), ExecID: "e1", Phase: PhaseRun, WorkspaceRoot: "/workspace"}); strings.Contains(tmpl.Body.HostConfig.Tmpfs["/tmp"], "noexec") {
		t.Errorf("tmp_exec profile: %s", tmpl.Body.HostConfig.Tmpfs["/tmp"])
	}
}

func TestEnvironment(t *testing.T) {
	env := strings.Join(ToolEnv(code(t, ""), "e1"), "\n")
	for _, want := range []string{"HOME=/home/run", "LANG=C.UTF-8", "TZ=UTC", "OUT=/workspace/.output/exec/e1/out", "IN=/workspace/.output/exec/e1/in", "PATH=/workspace/.venv/bin:/usr/bin"} {
		if !strings.Contains(env, want) {
			t.Errorf("env lacks %s: %s", want, env)
		}
	}
	if strings.Contains(strings.Join(ToolEnv(install(t), "e1"), "\n"), "IN=") {
		t.Error("install has no IN")
	}
}

func TestManifestArgs(t *testing.T) {
	got, err := ManifestArgs(install(t), []string{"/workspace/src/a.txt", "/workspace/src/b.txt"})
	if err != nil || strings.Join(got, " ") != "-r /workspace/src/a.txt -r /workspace/src/b.txt" {
		t.Fatalf("%v %v", got, err)
	}
	npm := parse(t, "install-npm", "name: install-npm\nkind: install\nimage_id: IMAGEID\nentry: fixed-args\nfetch_argv: [npm]\nargv: [npm]\nenv_dirs: { .npm-env: rw }\n")
	if _, err := ManifestArgs(npm, []string{"/workspace/src/package.json"}); err == nil {
		t.Fatal("the npm manifest form is TBD")
	}
}

// inspectOf is what the engine would report for the created container.
func inspectOf(pl *Plan) *engine.Inspect {
	i := &engine.Inspect{Name: "/" + pl.Name, Image: pl.Body.Image}
	i.Config.User = pl.Body.User
	i.Config.Env = append([]string{}, pl.Body.Env...)
	i.Config.Cmd = pl.Body.Cmd
	i.Config.Entrypoint = pl.Body.Entrypoint
	i.Config.WorkingDir = pl.Body.WorkingDir
	i.Config.Labels = pl.Body.Labels
	i.HostConfig = pl.Body.HostConfig
	i.HostConfig.CapDrop = []string{"ALL"}
	for _, m := range pl.Body.HostConfig.Mounts {
		i.Mounts = append(i.Mounts, engine.MountPoint{Type: "bind", Source: m.Source, Destination: m.Target, RW: !m.ReadOnly})
	}
	return i
}

func TestVerify(t *testing.T) {
	newPlan := func() *Plan {
		return NewPlan(Params{Profile: code(t, "build_dir: true\n"), ExecID: "e1", Phase: PhaseRun, WorkspaceRoot: "/workspace", Argv: []string{"python", "-V"}, Pid: 9})
	}
	pl := newPlan()
	imageEnv := []string{"LANG=en_US.UTF-8", "PYTHON_VERSION=3.12"}
	good := inspectOf(pl)
	good.Config.Env = append(good.Config.Env, "PYTHON_VERSION=3.12")
	if bad := Verify(pl, good, imageEnv); len(bad) != 0 {
		t.Fatalf("a matching container must verify: %v", bad)
	}
	// The engine may add its default PATH when neither image nor tool set one.
	p2 := NewPlan(Params{Profile: parse(t, "python", "name: python\nkind: code\nimage_id: IMAGEID\nentry: agent\n"), ExecID: "e1", Phase: PhaseRun, WorkspaceRoot: "/workspace", Argv: []string{"x"}})
	i2 := inspectOf(p2)
	i2.Config.Env = append(i2.Config.Env, "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")
	if bad := Verify(p2, i2, nil); len(bad) != 0 {
		t.Fatalf("default PATH: %v", bad)
	}

	mutations := map[string]struct {
		mut  func(*engine.Inspect)
		want string
	}{
		"cap added":        {func(i *engine.Inspect) { i.HostConfig.CapAdd = []string{"SYS_ADMIN"} }, "CapAdd"},
		"cap drop missing": {func(i *engine.Inspect) { i.HostConfig.CapDrop = nil }, "CapDrop"},
		"privileged":       {func(i *engine.Inspect) { i.HostConfig.Privileged = true }, "Privileged"},
		"nnp missing":      {func(i *engine.Inspect) { i.HostConfig.SecurityOpt = nil }, "SecurityOpt"},
		"seccomp off": {func(i *engine.Inspect) {
			i.HostConfig.SecurityOpt = []string{"no-new-privileges", "seccomp=unconfined"}
		}, "SecurityOpt"},
		"writable rootfs":   {func(i *engine.Inspect) { i.HostConfig.ReadonlyRootfs = false }, "ReadonlyRootfs"},
		"root user":         {func(i *engine.Inspect) { i.Config.User = "0:0" }, "user"},
		"empty user":        {func(i *engine.Inspect) { i.Config.User = "" }, "user"},
		"network host":      {func(i *engine.Inspect) { i.HostConfig.NetworkMode = "host" }, "NetworkMode"},
		"network bridge":    {func(i *engine.Inspect) { i.HostConfig.NetworkMode = "bridge" }, "NetworkMode"},
		"no autoremove":     {func(i *engine.Inspect) { i.HostConfig.AutoRemove = false }, "AutoRemove"},
		"other image":       {func(i *engine.Inspect) { i.Image = "sha256:" + strings.Repeat("b", 64) }, "image"},
		"memory":            {func(i *engine.Inspect) { i.HostConfig.Memory = 0 }, "Memory"},
		"cpus":              {func(i *engine.Inspect) { i.HostConfig.NanoCpus = 8_000_000_000 }, "NanoCpus"},
		"pids":              {func(i *engine.Inspect) { i.HostConfig.PidsLimit = 0 }, "PidsLimit"},
		"oom kill disabled": {func(i *engine.Inspect) { i.HostConfig.OomKillDisable = true }, "OomKillDisable"},
		"host pid":          {func(i *engine.Inspect) { i.HostConfig.PidMode = "host" }, "PidMode"},
		"host ipc":          {func(i *engine.Inspect) { i.HostConfig.IpcMode = "container:abc" }, "IpcMode"},
		"cgroupns private":  {func(i *engine.Inspect) { i.HostConfig.CgroupnsMode = "private" }, `CgroupnsMode "private", want "host"`},
		"cgroupns empty":    {func(i *engine.Inspect) { i.HostConfig.CgroupnsMode = "" }, `CgroupnsMode "", want "host"`},
		"cgroup parent":     {func(i *engine.Inspect) { i.HostConfig.CgroupParent = "/x" }, "CgroupParent"},
		"extra mount": {func(i *engine.Inspect) {
			i.Mounts = append(i.Mounts, engine.MountPoint{Type: "bind", Source: "/", Destination: "/host", RW: true})
		}, "unexpected mount /host"},
		"volume mount": {func(i *engine.Inspect) {
			i.Mounts = append(i.Mounts, engine.MountPoint{Type: "volume", Destination: "/data", RW: true})
		}, "unexpected mount /data"},
		"missing mount": {func(i *engine.Inspect) { i.Mounts = i.Mounts[1:] }, "missing"},
		"src writable": {func(i *engine.Inspect) {
			for k := range i.Mounts {
				if i.Mounts[k].Destination == "/workspace/src" {
					i.Mounts[k].RW = true
				}
			}
		}, "/workspace/src rw=true"},
		"build writable": {func(i *engine.Inspect) {
			for k := range i.Mounts {
				if i.Mounts[k].Destination == "/workspace/build" {
					i.Mounts[k].RW = true
				}
			}
		}, "/workspace/build rw=true"},
		"other source": {func(i *engine.Inspect) {
			for k := range i.Mounts {
				if i.Mounts[k].Destination == "/workspace/src" {
					i.Mounts[k].Source = "/workspace/.tools"
				}
			}
		}, "source"},
		"extra tmpfs": {func(i *engine.Inspect) {
			i.HostConfig.Tmpfs = map[string]string{"/work": "x", "/tmp": "x", "/home/run": "x", "/etc": "x"}
		}, "tmpfs"},
		"tmp exec": {func(i *engine.Inspect) {
			i.HostConfig.Tmpfs["/tmp"] = strings.Replace(i.HostConfig.Tmpfs["/tmp"], "noexec", "exec", 1)
		}, "/tmp"},
		"tmpfs size":     {func(i *engine.Inspect) { i.HostConfig.Tmpfs["/work"] = "rw,noexec,size=1099511627776" }, "/work"},
		"env injected":   {func(i *engine.Inspect) { i.Config.Env = append(i.Config.Env, "AWS_SECRET=x") }, "AWS_SECRET"},
		"env overridden": {func(i *engine.Inspect) { i.Config.Env = append(i.Config.Env, "OUT=/etc") }, "OUT"},
		"env missing":    {func(i *engine.Inspect) { i.Config.Env = i.Config.Env[1:] }, "missing"},
		"entrypoint":     {func(i *engine.Inspect) { i.Config.Entrypoint = []string{"/bin/sh"} }, "Entrypoint"},
		"cmd":            {func(i *engine.Inspect) { i.Config.Cmd = []string{"sh"} }, "Cmd"},
		"binds":          {func(i *engine.Inspect) { i.HostConfig.Binds = []string{"/:/host"} }, "forbidden"},
		"devices":        {func(i *engine.Inspect) { i.HostConfig.Devices = []any{"x"} }, "forbidden"},
		"group add":      {func(i *engine.Inspect) { i.HostConfig.GroupAdd = []string{"docker"} }, "forbidden"},
		"stdin open":     {func(i *engine.Inspect) { i.Config.OpenStdin = true }, "stdin"},
		"runtime":        {func(i *engine.Inspect) { i.HostConfig.Runtime = "nvidia" }, "Runtime"},
		"wrong name":     {func(i *engine.Inspect) { i.Name = "/other" }, "name"},
		"wrong label":    {func(i *engine.Inspect) { i.Config.Labels = map[string]string{"ephemeral.exec": "e2"} }, "label"},
	}
	for name, m := range mutations {
		pl := newPlan()
		i := inspectOf(pl)
		i.HostConfig.Tmpfs = map[string]string{}
		for k, v := range pl.Body.HostConfig.Tmpfs {
			i.HostConfig.Tmpfs[k] = v
		}
		m.mut(i)
		bad := Verify(pl, i, nil)
		if len(bad) == 0 {
			t.Errorf("%s: verify passed a mutated container", name)
			continue
		}
		if !strings.Contains(strings.Join(bad, "\n"), m.want) {
			t.Errorf("%s: mismatches %v do not mention %q", name, bad, m.want)
		}
	}
}

func TestToolKindModel(t *testing.T) {
	p := tool(t, "")
	if got := Phases(p.Kind); len(got) != 1 || got[0] != PhaseRun {
		t.Fatalf("phases %v", got)
	}
	if Network(p.Kind, PhaseRun) != "none" {
		t.Fatal("tool network must be none")
	}
	pl := NewPlan(Params{Profile: p, ExecID: "e7", Phase: PhaseRun, WorkspaceRoot: "/workspace",
		Argv: []string{"ngspice", "-b"}, Pid: 9})
	hc := pl.Body.HostConfig
	if hc.NetworkMode != "none" || !hc.ReadonlyRootfs || pl.Body.WorkingDir != "/work" || pl.Body.User != "1001:1001" {
		t.Fatalf("settings %+v", pl.Body)
	}
	for target, opts := range hc.Tmpfs {
		if !strings.Contains(opts, "noexec") {
			t.Errorf("tmpfs %s must be noexec without tmp_exec: %s", target, opts)
		}
	}
	if len(hc.Tmpfs) != 3 {
		t.Errorf("tmpfs %v", hc.Tmpfs)
	}
	if strings.Join(WatchDirs(p, "e7"), ",") != ".output/exec/e7/out" {
		t.Errorf("watch dirs %v", WatchDirs(p, "e7"))
	}
	if bad := Verify(pl, inspectOf(pl), nil); len(bad) != 0 {
		t.Fatalf("a matching tool container must verify: %v", bad)
	}
	// tmp_exec opens /tmp only.
	pe := NewPlan(Params{Profile: tool(t, "tmp_exec: true\n"), ExecID: "e7", Phase: PhaseRun, WorkspaceRoot: "/workspace"})
	if strings.Contains(pe.Body.HostConfig.Tmpfs["/tmp"], "noexec") || !strings.Contains(pe.Body.HostConfig.Tmpfs["/work"], "noexec") {
		t.Errorf("tmp_exec: %v", pe.Body.HostConfig.Tmpfs)
	}
}
