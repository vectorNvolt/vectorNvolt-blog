package tests

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"agent-exec/internal/sequence"
)

// USC-009 steps 4-6: kind tool against the fake engine.

const ngspiceProfile = `
name: ngspice
kind: tool
image_id: IMAGEID
ref: exec-ngspice:42
entry: fixed
argv: [ngspice, -b, -n, -r, "{out}/sim.raw", -o, "{out}/sim.log", "{in.netlist}"]
inputs:
  netlist: { path_under: [src/, .input/, .output/exec/], glob: "*.cir" }
  include: { path_under: [src/, .input/], glob: "*.lib", multiple: true, optional: true }
input_policy: spice-safe
limits: { mem: 512m, cpus: 1, pids: 128, timeout_s: 60, out_max: 1m }
`

const paramProfile = `
name: sim
kind: tool
image_id: IMAGEID
entry: fixed
argv: [sim, "--steps={param.steps}", "{param.mode}", "{in.model}"]
inputs: { model: { path_under: [src/] } }
params:
  steps: { type: int, min: 1, max: 100 }
  mode: { type: string, values: [fast, exact], default: fast }
limits: { mem: 512m, cpus: 1, pids: 128, timeout_s: 60, out_max: 1m }
`

const tbAmp = "tb amp\n.include models/opamp.lib\nX1 in 0 out opamp\n.tran 1u 1m\n.end\n"

func toolWorld(t *testing.T) *world {
	w := newWorld(t)
	w.write(".tools/ngspice/profile.yml", ngspiceProfile)
	w.write(".tools/sim/profile.yml", paramProfile)
	w.write("src/tb_amp.cir", tbAmp)
	w.write("src/models/opamp.lib", ".subckt opamp a b c\n.ends\n")
	w.write("src/model.txt", "m\n")
	return w
}

func TestToolRunReference(t *testing.T) {
	w := toolWorld(t)
	w.eng.behave = func(string) behavior {
		return behavior{stdout: "Circuit: tb amp\n", before: func() {
			w.write(".output/exec/e7/out/sim.log", "done\n")
		}}
	}
	out, so, se := w.run("tool", "ngspice", "--exec-id", "e7",
		"--in", "netlist="+w.path("src/tb_amp.cir"), "--in", "include="+w.path("src/models/opamp.lib"))
	if out != (sequence.Outcome{Exit: 0, Cause: "ok"}) {
		t.Fatalf("outcome %+v stderr %q", out, se)
	}
	if first := strings.SplitN(se, "\n", 2)[0]; first != "run_ephemeral_container: exec_dir="+w.path(".output/exec/e7") {
		t.Fatalf("first stderr line %q", first)
	}
	if so != "Circuit: tb amp\n" || w.read(".output/exec/e7/stdout.log") != so {
		t.Fatalf("stdout %q", so)
	}
	body := w.eng.bodies["ephemeral-e7"]
	want := []string{"ngspice", "-b", "-n",
		"-r", "/workspace/.output/exec/e7/out/sim.raw",
		"-o", "/workspace/.output/exec/e7/out/sim.log",
		"/workspace/src/tb_amp.cir"}
	if strings.Join(body.Cmd, "|") != strings.Join(want, "|") {
		t.Fatalf("cmd %v, want the profile argv with placeholders expanded and nothing appended", body.Cmd)
	}
	if body.HostConfig.NetworkMode != "none" {
		t.Fatalf("network %s", body.HostConfig.NetworkMode)
	}
	modes := map[string]bool{}
	for _, m := range body.HostConfig.Mounts {
		modes[strings.TrimPrefix(m.Target, "/workspace/")] = !m.ReadOnly
	}
	wantModes := map[string]bool{"src": false, ".output/exec/e7/in": false, ".output/exec/e7/out": true}
	if len(modes) != len(wantModes) {
		t.Fatalf("mounts %v", modes)
	}
	for k, rw := range wantModes {
		if got, ok := modes[k]; !ok || got != rw {
			t.Fatalf("mounts %v, want %v", modes, wantModes)
		}
	}
	// Inputs under src/ are used in place: in/ exists and is empty.
	if names, err := os.ReadDir(w.path(".output/exec/e7/in")); err != nil || len(names) != 0 {
		t.Fatalf("in/ must be empty: %v %v", names, err)
	}
	if r := w.result("e7"); r.Exit != 0 || r.Cause != "ok" {
		t.Fatalf("result %+v", r)
	}
	if w.read(".output/exec/e7/out/sim.log") != "done\n" || !w.exists(".output/exec/e7/meta.json") || !w.exists(".output/exec/e7/stderr.log") {
		t.Fatal("exec dir layout")
	}
	var m struct {
		Request struct {
			Kind string   `json:"kind"`
			Args []string `json:"args"`
		} `json:"request"`
		Inputs []struct{ Name, Path string } `json:"inputs"`
		Phases []struct{ Argv []string }     `json:"phases"`
	}
	if err := json.Unmarshal([]byte(w.read(".output/exec/e7/meta.json")), &m); err != nil {
		t.Fatal(err)
	}
	if m.Request.Kind != "tool" || len(m.Request.Args) != 0 || len(m.Inputs) != 2 || m.Inputs[1].Path != "/workspace/src/models/opamp.lib" ||
		len(m.Phases) != 1 || strings.Join(m.Phases[0].Argv, "|") != strings.Join(want, "|") {
		t.Fatalf("meta %+v", m)
	}
	w.noContainers()
}

func TestToolRunNonZeroExitPassedThrough(t *testing.T) {
	w := toolWorld(t)
	w.eng.behave = func(string) behavior { return behavior{exit: 1, stderr: "Error: no circuit\n"} }
	out, _, se := w.run("tool", "ngspice", "--exec-id", "e7", "--in", "netlist="+w.path("src/tb_amp.cir"),
		"--in", "include="+w.path("src/models/opamp.lib"))
	if out != (sequence.Outcome{Exit: 1, Cause: "exit"}) || !strings.Contains(se, "no circuit") {
		t.Fatalf("outcome %+v stderr %q", out, se)
	}
	if r := w.result("e7"); r.Exit != 1 || r.Cause != "exit" {
		t.Fatalf("result %+v", r)
	}
}

func TestToolRunTimeoutPassedThrough(t *testing.T) {
	w := toolWorld(t)
	w.eng.behave = func(string) behavior { return behavior{hang: true} }
	out, _, _ := w.runWith(func(d *sequence.Deps) { d.TimeoutOverride = 50 * time.Millisecond }, "tool", "ngspice", "--exec-id", "e7",
		"--in", "netlist="+w.path("src/tb_amp.cir"), "--in", "include="+w.path("src/models/opamp.lib"))
	if out != (sequence.Outcome{Exit: 124, Cause: "timeout"}) {
		t.Fatalf("outcome %+v", out)
	}
	w.noContainers()
}

// alt-8: an input outside src/ is copied to in/<name>, a repeated multiple input to in/<name>/<basename>;
// relative includes then resolve against the staged netlist.
func TestToolStagesInputsOutsideSrc(t *testing.T) {
	w := toolWorld(t)
	w.write(".input/amp.cir", "tb\n.include include/a.lib\n.include include/b.lib\n.end\n")
	w.write(".input/libs/a.lib", "* a\n")
	w.write(".input/libs/b.lib", "* b\n")
	out, _, se := w.run("tool", "ngspice", "--exec-id", "s1", "--in", "netlist="+w.path(".input/amp.cir"),
		"--in", "include="+w.path(".input/libs/a.lib"), "--in", "include="+w.path(".input/libs/b.lib"))
	if out.Cause != "ok" {
		t.Fatalf("outcome %+v stderr %q", out, se)
	}
	if w.read(".output/exec/s1/in/netlist") != "tb\n.include include/a.lib\n.include include/b.lib\n.end\n" ||
		w.read(".output/exec/s1/in/include/a.lib") != "* a\n" || w.read(".output/exec/s1/in/include/b.lib") != "* b\n" {
		t.Fatal("inputs not staged as in/<name> and in/<name>/<basename>")
	}
	cmd := w.eng.bodies["ephemeral-s1"].Cmd
	if cmd[len(cmd)-1] != "/workspace/.output/exec/s1/in/netlist" {
		t.Fatalf("cmd %v", cmd)
	}
}

func TestToolParams(t *testing.T) {
	w := toolWorld(t)
	out, _, se := w.run("tool", "sim", "--exec-id", "p1", "--in", "model="+w.path("src/model.txt"), "--param", "steps=007")
	if out.Cause != "ok" {
		t.Fatalf("outcome %+v stderr %q", out, se)
	}
	if got := strings.Join(w.eng.bodies["ephemeral-p1"].Cmd, " "); got != "sim --steps=7 fast /workspace/src/model.txt" {
		t.Fatalf("cmd %q", got)
	}
	var m struct {
		Request struct {
			Params map[string]string `json:"params"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(w.read(".output/exec/p1/meta.json")), &m); err != nil {
		t.Fatal(err)
	}
	if m.Request.Params["steps"] != "7" || m.Request.Params["mode"] != "fast" {
		t.Fatalf("meta params %v", m.Request.Params)
	}
}

// exc-8: a call that does not match the profile is exit 125, cause usage, nothing created.
func TestToolUsageErrorsCreateNothing(t *testing.T) {
	w := toolWorld(t)
	w.write("src/other.txt", "x")
	w.write("src/b/opamp.lib", "x")
	if err := os.Symlink(w.path(".process"), w.path("src/link.cir")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(w.path("src/dir.cir"), 0o755); err != nil {
		t.Fatal(err)
	}
	net := "netlist=" + w.path("src/tb_amp.cir")
	inc := "include=" + w.path("src/models/opamp.lib")
	cases := map[string][]string{
		"code profile via tool":     {"tool", "python", "--in", net},
		"tool profile via run":      {"run", "ngspice", "--", "ngspice"},
		"unknown profile":           {"tool", "nope", "--in", net},
		"missing netlist":           {"tool", "ngspice", "--in", inc},
		"undeclared input":          {"tool", "ngspice", "--in", net, "--in", "model=" + w.path("src/model.txt")},
		"glob mismatch":             {"tool", "ngspice", "--in", "netlist=" + w.path("src/other.txt")},
		"repeated single":           {"tool", "ngspice", "--in", net, "--in", net},
		"duplicate include base":    {"tool", "ngspice", "--in", net, "--in", inc, "--in", "include=" + w.path("src/b/opamp.lib")},
		"outside path_under":        {"tool", "ngspice", "--in", "netlist=" + w.path(".agent/x.cir")},
		"outside workspace":         {"tool", "ngspice", "--in", "netlist=/etc/x.cir"},
		"symlink input":             {"tool", "ngspice", "--in", "netlist=" + w.path("src/link.cir")},
		"directory input":           {"tool", "ngspice", "--in", "netlist=" + w.path("src/dir.cir")},
		"unknown param":             {"tool", "ngspice", "--in", net, "--param", "tstop=1"},
		"missing required param":    {"tool", "sim", "--in", "model=" + w.path("src/model.txt")},
		"mistyped param":            {"tool", "sim", "--in", "model=" + w.path("src/model.txt"), "--param", "steps=many"},
		"param out of range":        {"tool", "sim", "--in", "model=" + w.path("src/model.txt"), "--param", "steps=101"},
		"param not in values":       {"tool", "sim", "--in", "model=" + w.path("src/model.txt"), "--param", "steps=1", "--param", "mode=slow"},
		"timeout above the profile": {"tool", "ngspice", "--in", net, "--in", inc, "--timeout", "61"},
	}
	for name, args := range cases {
		out, _, se := w.run(args...)
		if out != (sequence.Outcome{Exit: 125, Cause: "usage"}) {
			t.Errorf("%s: outcome %+v stderr %q", name, out, se)
		}
		if strings.Contains(se, "exec_dir=") {
			t.Errorf("%s: exec dir announced", name)
		}
	}
	if w.exists(".output/exec") {
		t.Fatal("a usage error must create nothing")
	}
	if len(w.eng.startedNames()) != 0 || len(w.eng.bodies) != 0 {
		t.Fatal("no container may be created")
	}
}

// exc-9: the input policy refuses before anything exists: exit 125, cause policy, reason on stderr.
func TestToolInputPolicyRefusesBeforeExecDir(t *testing.T) {
	w := toolWorld(t)
	w.write("src/shell.cir", "tb\n.control\nshell cat /workspace/.agent/x\n.endc\n.end\n")
	cases := map[string]struct {
		args []string
		want string
	}{
		"shell control block": {[]string{"tool", "ngspice", "--exec-id", "x1", "--in", "netlist=" + w.path("src/shell.cir")}, "shell"},
		// Reference step 4: the call without --in include=... and a netlist that includes the model.
		"undeclared include": {[]string{"tool", "ngspice", "--exec-id", "x2", "--in", "netlist=" + w.path("src/tb_amp.cir")}, "not a declared include"},
	}
	for name, c := range cases {
		out, _, se := w.run(c.args...)
		if out != (sequence.Outcome{Exit: 125, Cause: "policy"}) {
			t.Errorf("%s: outcome %+v stderr %q", name, out, se)
		}
		if !strings.HasPrefix(se, "run_ephemeral_container: policy: spice-safe: ") || !strings.Contains(se, c.want) {
			t.Errorf("%s: stderr %q", name, se)
		}
	}
	if w.exists(".output/exec") || len(w.eng.bodies) != 0 {
		t.Fatal("a policy refusal must create no exec dir, no result.json and no container")
	}
}
