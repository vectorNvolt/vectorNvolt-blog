package cli

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParseValid(t *testing.T) {
	r, err := Parse([]string{"run", "python", "--exec-id", "e3", "--in", "data=/workspace/.input/m.csv", "--in=more=/workspace/src/a",
		"--label", "agent-graph.run=r1", "--label=node=n1", "--timeout", "30", "--", "python", "-c", "print(1)", "--not-a-flag"})
	if err != nil {
		t.Fatal(err)
	}
	want := &Request{Sub: "run", Profile: "python", ExecID: "e3", TimeoutS: 30,
		Inputs: []Input{{"data", "/workspace/.input/m.csv"}, {"more", "/workspace/src/a"}},
		Labels: map[string]string{"agent-graph.run": "r1", "node": "n1"},
		Args:   []string{"python", "-c", "print(1)", "--not-a-flag"},
	}
	if !reflect.DeepEqual(r, want) {
		t.Fatalf("got %+v", r)
	}

	if r, err := Parse([]string{"build", "c-make"}); err != nil || len(r.Args) != 0 {
		t.Fatalf("build without driver args: %+v %v", r, err)
	}
	if r, err := Parse([]string{"build", "c-make", "--", "-C", "/workspace/src"}); err != nil || len(r.Args) != 2 {
		t.Fatalf("build with driver args: %+v %v", r, err)
	}
	r, err = Parse([]string{"install", "install-python", "--manifest", "/workspace/src/r.txt", "--manifest", "/workspace/src/s.txt"})
	if err != nil || len(r.Manifests) != 2 {
		t.Fatalf("install: %+v %v", r, err)
	}
	r, err = Parse([]string{"tool", "ngspice", "--exec-id", "e7", "--in", "netlist=/workspace/src/tb.cir",
		"--in", "include=/workspace/src/m.lib", "--param", "tstop=1e-3", "--param=mode=fast", "--label", "agent-graph.run=r1", "--timeout", "20"})
	if err != nil {
		t.Fatal(err)
	}
	wantTool := &Request{Sub: "tool", Profile: "ngspice", ExecID: "e7", TimeoutS: 20,
		Inputs: []Input{{"netlist", "/workspace/src/tb.cir"}, {"include", "/workspace/src/m.lib"}},
		Params: map[string]string{"tstop": "1e-3", "mode": "fast"},
		Labels: map[string]string{"agent-graph.run": "r1"},
	}
	if !reflect.DeepEqual(r, wantTool) {
		t.Fatalf("tool: got %+v", r)
	}
	if r, err := Parse([]string{"tool", "ngspice", "--param", "s="}); err != nil || r.Params["s"] != "" {
		t.Fatalf("tool with empty param value (the schema decides): %+v %v", r, err)
	}
	if r, err := Parse([]string{"list", "--json"}); err != nil || !r.JSON {
		t.Fatalf("list: %+v %v", r, err)
	}
	if r, err := Parse([]string{"list"}); err != nil || r.JSON {
		t.Fatalf("list: %+v %v", r, err)
	}
	if r, err := Parse([]string{"sweep", "--label", "agent-graph.run=r1"}); err != nil || r.SweepKey != "agent-graph.run" || r.SweepVal != "r1" {
		t.Fatalf("sweep: %+v %v", r, err)
	}
}

func TestParseUsageErrors(t *testing.T) {
	cases := map[string][]string{
		"no args":                  {},
		"unknown subcommand":       {"exec", "python"},
		"tool with dashes":         {"tool", "x", "--", "ngspice"},
		"tool without profile":     {"tool"},
		"tool manifest":            {"tool", "x", "--manifest", "/workspace/src/r.txt"},
		"tool bad param":           {"tool", "x", "--param", "Tstop=1"},
		"tool param without =":     {"tool", "x", "--param", "tstop"},
		"tool param twice":         {"tool", "x", "--param", "a=1", "--param", "a=2"},
		"param on run":             {"run", "python", "--param", "a=1", "--", "x"},
		"param on build":           {"build", "c-make", "--param", "a=1"},
		"missing profile":          {"run"},
		"flag instead of profile":  {"run", "--exec-id", "e1", "--", "x"},
		"run without command":      {"run", "python"},
		"run empty command":        {"run", "python", "--"},
		"stray argument":           {"run", "python", "x", "--", "y"},
		"bad exec id uppercase":    {"run", "python", "--exec-id", "E1", "--", "x"},
		"bad exec id leading -":    {"run", "python", "--exec-id", "-e1", "--", "x"},
		"exec id too long":         {"run", "python", "--exec-id", strings.Repeat("a", 65), "--", "x"},
		"exec id with slash":       {"run", "python", "--exec-id", "a/b", "--", "x"},
		"timeout zero":             {"run", "python", "--timeout", "0", "--", "x"},
		"timeout text":             {"run", "python", "--timeout", "soon", "--", "x"},
		"in without name":          {"run", "python", "--in", "=/x", "--", "x"},
		"in without path":          {"run", "python", "--in", "data=", "--", "x"},
		"in without equals":        {"run", "python", "--in", "data", "--", "x"},
		"label without value":      {"run", "python", "--label", "k", "--", "x"},
		"label ephemeral prefix":   {"run", "python", "--label", "ephemeral.exec=x", "--", "x"},
		"flag without value":       {"run", "python", "--timeout"},
		"unknown flag":             {"run", "python", "--image", "evil", "--", "x"},
		"mount flag":               {"run", "python", "--mount", "/:/host", "--", "x"},
		"manifest on run":          {"run", "python", "--manifest", "/workspace/src/r.txt", "--", "x"},
		"manifest on build":        {"build", "c-make", "--manifest", "/workspace/src/r.txt"},
		"install without manifest": {"install", "install-python"},
		"install with in":          {"install", "install-python", "--manifest", "/workspace/src/r", "--in", "d=/workspace/x"},
		"install free args":        {"install", "install-python", "--manifest", "/workspace/src/r", "--", "--no-deps"},
		"list with args":           {"list", "--", "x"},
		"list unknown flag":        {"list", "--all"},
		"json on run":              {"run", "python", "--json", "--", "x"},
		"sweep without label":      {"sweep"},
		"sweep two labels":         {"sweep", "--label", "a=b", "--label", "c=d"},
		"sweep other flag":         {"sweep", "--label", "a=b", "--exec-id", "e1"},
	}
	for name, args := range cases {
		_, err := Parse(args)
		var ue *UsageError
		if !errors.As(err, &ue) {
			t.Errorf("%s: err %v, want a usage error", name, err)
		}
	}
}

func TestParseLabel(t *testing.T) {
	if k, v, err := ParseLabel("a.b-c=x=y"); err != nil || k != "a.b-c" || v != "x=y" {
		t.Fatalf("%q %q %v", k, v, err)
	}
	for _, s := range []string{"", "=v", "k", "k v=x", "k=" + strings.Repeat("x", 257), "k=a\nb"} {
		if _, _, err := ParseLabel(s); err == nil {
			t.Errorf("%q accepted", s)
		}
	}
}
