package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-exec/internal/sequence"
	"agent-exec/internal/wsfs"
)

const toolProfile = `name: ngspice
kind: tool
image_id: sha256:` + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" + `
entry: fixed
argv: [ngspice, -b, -n, -r, "{out}/sim.raw", -o, "{out}/sim.log", "{in.netlist}", "{param.steps}"]
inputs:
  netlist: { path_under: [src/], glob: "*.cir" }
  include: { path_under: [src/], glob: "*.lib", multiple: true, optional: true }
input_policy: spice-safe
params:
  steps: { type: int, min: 1, max: 10, default: 2 }
limits: { mem: 512m, cpus: 1, pids: 128, timeout_s: 60, out_max: 64m }
`

const codeProfile = `name: python
kind: code
image_id: sha256:` + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" + `
entry: agent
inputs: { data: { path_under: [.input/], multiple: true, optional: true } }
limits: { mem: 512m, cpus: 1, pids: 128, timeout_s: 60, out_max: 64m }
`

// TestListJSONToolInputsAndParams: list --json reports, for kind tool only, the inputs (name, glob,
// multiple, optional) and the param schema; nothing is created (SFR-AGT-012, USC-009 alt-4).
func TestListJSONToolInputsAndParams(t *testing.T) {
	root := t.TempDir()
	for name, y := range map[string]string{"ngspice": toolProfile, "python": codeProfile} {
		if err := os.MkdirAll(filepath.Join(root, ".tools", name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, ".tools", name, "profile.yml"), []byte(y), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var so, se strings.Builder
	d := &sequence.Deps{WS: wsfs.New(root), Stdout: &so, Stderr: &se} // no engine: image_loaded is false
	if code := List(d, true); code != 0 {
		t.Fatalf("exit %d stderr %q", code, se.String())
	}
	var got []map[string]any
	if err := json.Unmarshal([]byte(so.String()), &got); err != nil {
		t.Fatalf("%v\n%s", err, so.String())
	}
	byName := map[string]map[string]any{}
	for _, e := range got {
		byName[e["name"].(string)] = e
	}
	tool := byName["ngspice"]
	if tool["kind"] != "tool" || tool["valid"] != true || tool["input_policy"] != "spice-safe" {
		t.Fatalf("tool entry %v", tool)
	}
	ins := tool["inputs"].([]any)
	if len(ins) != 2 {
		t.Fatalf("inputs %v", ins)
	}
	inc := ins[0].(map[string]any)
	net := ins[1].(map[string]any)
	if inc["name"] != "include" || inc["glob"] != "*.lib" || inc["multiple"] != true || inc["optional"] != true {
		t.Fatalf("include %v", inc)
	}
	if net["name"] != "netlist" || net["glob"] != "*.cir" || net["multiple"] != false || net["optional"] != false {
		t.Fatalf("netlist %v", net)
	}
	steps := tool["params"].(map[string]any)["steps"].(map[string]any)
	if steps["type"] != "int" || steps["min"] != 1.0 || steps["max"] != 10.0 || steps["default"] != "2" {
		t.Fatalf("param schema %v", steps)
	}
	code := byName["python"]
	if _, ok := code["inputs"]; ok {
		t.Fatalf("inputs are reported for kind tool only: %v", code)
	}
	if _, err := os.Stat(filepath.Join(root, ".output")); !os.IsNotExist(err) {
		t.Fatal("list must create nothing")
	}
}
