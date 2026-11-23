package profile

import (
	"strings"
	"testing"
)

// ngspiceProfile is the USS-009 reference profile.
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
`

func toolYAML(body string) string {
	return strings.ReplaceAll(body, "IMAGEID", img) + limitsOK
}

func TestParseToolReference(t *testing.T) {
	p, err := Parse([]byte(toolYAML(ngspiceProfile)), "ngspice")
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != KindTool || p.Entry != EntryFixed || p.InputPolicy != InputPolicySpiceSafe || len(p.Argv) != 8 {
		t.Fatalf("%+v", p)
	}
	if in := p.Inputs["include"]; !in.Multiple || !in.Optional || in.Glob != "*.lib" {
		t.Fatalf("include %+v", in)
	}
	if got := p.SortedInputNames(); strings.Join(got, ",") != "include,netlist" {
		t.Fatalf("names %v", got)
	}
}

func TestParseToolWithParams(t *testing.T) {
	y := `
name: sim
kind: tool
image_id: IMAGEID
entry: fixed
argv: [sim, "--steps={param.steps}", "--tol", "{param.tol}", "{param.mode}", "--verbose={param.verbose}", "{in.model}"]
inputs: { model: { path_under: [src/] } }
params:
  steps: { type: int, min: 1, max: 1000 }
  tol: { type: number, min: 0, default: 1e-3 }
  mode: { type: string, values: [fast, exact], default: fast }
  verbose: { type: bool, default: false }
build_dir: true
`
	p, err := Parse([]byte(toolYAML(y)), "sim")
	if err != nil {
		t.Fatal(err)
	}
	if *p.Params["tol"].Default != "1e-3" || p.Params["steps"].Type != ParamInt || !p.BuildDir {
		t.Fatalf("%+v", p.Params)
	}
}

func TestParseToolInvalid(t *testing.T) {
	head := "name: t\nkind: tool\nimage_id: " + img + "\n"
	in := "inputs: { a: { path_under: [src/] }, m: { path_under: [src/], multiple: true }, o: { path_under: [src/], optional: true } }\n"
	cases := map[string]struct{ yaml, want string }{
		"no entry":             {head + "argv: [x]\n", "entry: fixed"},
		"entry fixed-args":     {head + "entry: fixed-args\nargv: [x]\n", "entry: fixed"},
		"no argv":              {head + "entry: fixed\n", "argv"},
		"fetch_argv":           {head + "entry: fixed\nargv: [x]\nfetch_argv: [y]\n", "fetch_argv"},
		"env dir":              {head + "entry: fixed\nargv: [x]\nenv_dirs: { .venv: ro }\n", "env_dirs"},
		"mount field":          {head + "entry: fixed\nargv: [x]\nmounts: [/]\n", "mounts"},
		"network field":        {head + "entry: fixed\nargv: [x]\nnetwork: host\n", "network"},
		"unknown policy":       {head + "entry: fixed\nargv: [x]\ninput_policy: none-safe\n", "input_policy"},
		"argv0 placeholder":    {head + in + "entry: fixed\nargv: [\"{in.a}\"]\n", "argv[0]"},
		"unknown placeholder":  {head + "entry: fixed\nargv: [x, \"{home}\"]\n", "placeholder"},
		"undeclared input":     {head + "entry: fixed\nargv: [x, \"{in.z}\"]\n", "no input"},
		"undeclared param":     {head + "entry: fixed\nargv: [x, \"{param.z}\"]\n", "no param"},
		"unbalanced brace":     {head + "entry: fixed\nargv: [x, \"{out\"]\n", "unbalanced"},
		"embedded multiple":    {head + in + "entry: fixed\nargv: [x, \"--m={in.m}\"]\n", "whole argument"},
		"embedded optional":    {head + in + "entry: fixed\nargv: [x, \"--o={in.o}\"]\n", "whole argument"},
		"unused param":         {head + "entry: fixed\nargv: [x]\nparams: { p: { type: int } }\n", "not used"},
		"bad param type":       {head + "entry: fixed\nargv: [x, \"{param.p}\"]\nparams: { p: { type: float } }\n", "type"},
		"bad param name":       {head + "entry: fixed\nargv: [x, \"{param.P}\"]\nparams: { P: { type: int } }\n", "name"},
		"string unrestricted":  {head + "entry: fixed\nargv: [x, \"{param.p}\"]\nparams: { p: { type: string } }\n", "pattern or values"},
		"bad pattern":          {head + "entry: fixed\nargv: [x, \"{param.p}\"]\nparams: { p: { type: string, pattern: \"(\" } }\n", "pattern"},
		"min above max":        {head + "entry: fixed\nargv: [x, \"{param.p}\"]\nparams: { p: { type: int, min: 5, max: 1 } }\n", "min"},
		"bad default":          {head + "entry: fixed\nargv: [x, \"{param.p}\"]\nparams: { p: { type: int, default: abc } }\n", "default"},
		"default out of range": {head + "entry: fixed\nargv: [x, \"{param.p}\"]\nparams: { p: { type: int, max: 3, default: 4 } }\n", "default"},
		"unknown param field":  {head + "entry: fixed\nargv: [x, \"{param.p}\"]\nparams: { p: { type: int, argv: [y] } }\n", "argv"},
		"params on code":       {"name: t\nkind: code\nentry: agent\nimage_id: " + img + "\nparams: { p: { type: int } }\n", "kind tool only"},
		"policy on code":       {"name: t\nkind: code\nentry: agent\nimage_id: " + img + "\ninput_policy: spice-safe\n", "kind tool only"},
	}
	for name, c := range cases {
		_, err := Parse([]byte(c.yaml+limitsOK), "t")
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err %v, want containing %q", name, err, c.want)
		}
	}
}

func f64(v float64) *float64 { return &v }

func TestParamCheck(t *testing.T) {
	good := []struct {
		p       Param
		in, out string
	}{
		{Param{Type: ParamInt, Min: f64(1), Max: f64(10)}, "10", "10"},
		{Param{Type: ParamInt}, "-3", "-3"},
		{Param{Type: ParamNumber}, "1e-3", "0.001"},
		{Param{Type: ParamNumber, Min: f64(0)}, "2.50", "2.5"},
		{Param{Type: ParamBool}, "true", "true"},
		{Param{Type: ParamString, Values: []string{"tran", "ac"}}, "ac", "ac"},
		{Param{Type: ParamString, Pattern: `[a-z]+`}, "abc", "abc"},
	}
	for _, c := range good {
		if got, err := c.p.Check(c.in); err != nil || got != c.out {
			t.Errorf("%+v %q: %q %v, want %q", c.p, c.in, got, err, c.out)
		}
	}
	bad := []struct {
		p  Param
		in string
	}{
		{Param{Type: ParamInt}, "1.5"},
		{Param{Type: ParamInt}, "abc"},
		{Param{Type: ParamInt, Max: f64(10)}, "11"},
		{Param{Type: ParamInt, Min: f64(1)}, "0"},
		{Param{Type: ParamNumber}, "NaN"},
		{Param{Type: ParamNumber}, "Inf"},
		{Param{Type: ParamNumber}, "1e999"},
		{Param{Type: ParamNumber}, "1m"},
		{Param{Type: ParamBool}, "yes"},
		{Param{Type: ParamBool}, "1"},
		{Param{Type: ParamString, Values: []string{"tran"}}, "dc"},
		{Param{Type: ParamString, Pattern: `[a-z]+`}, "abc; rm"},
		{Param{Type: ParamString, Pattern: `[a-z]+`}, "-o/etc"},
		{Param{Type: ParamString, Pattern: `.*`}, "a\nb"},
		{Param{Type: ParamString, Pattern: `.*`}, strings.Repeat("a", 257)},
	}
	for _, c := range bad {
		if _, err := c.p.Check(c.in); err == nil {
			t.Errorf("%+v %q accepted", c.p, c.in)
		}
	}
}

func TestExpandToolArgv(t *testing.T) {
	out := "/workspace/.output/exec/e7/out"
	in := map[string][]string{
		"netlist": {"/workspace/src/tb_amp.cir"},
		"include": {"/workspace/src/models/a.lib", "/workspace/.output/exec/e7/in/include/b.lib"},
	}
	argv := []string{"ngspice", "-b", "-n", "-r", "{out}/sim.raw", "-o", "{out}/sim.log", "{in.netlist}"}
	got := ExpandToolArgv(argv, out, in, nil)
	want := "ngspice -b -n -r /workspace/.output/exec/e7/out/sim.raw -o /workspace/.output/exec/e7/out/sim.log /workspace/src/tb_amp.cir"
	if strings.Join(got, " ") != want {
		t.Fatalf("got %v", got)
	}
	// Whole-argument placeholders of a multiple input give one argument per path; an absent
	// optional input gives none; params expand once (a value is never expanded again).
	got = ExpandToolArgv([]string{"x", "{in.include}", "{in.absent}", "--p={param.p}", "{param.q}"}, out, in,
		map[string]string{"p": "{out}", "q": "2"})
	if strings.Join(got, "|") != "x|/workspace/src/models/a.lib|/workspace/.output/exec/e7/in/include/b.lib|--p={out}|2" {
		t.Fatalf("got %v", got)
	}
	if len(argv) != 8 || argv[4] != "{out}/sim.raw" {
		t.Fatal("the profile argv must not be modified")
	}
}
