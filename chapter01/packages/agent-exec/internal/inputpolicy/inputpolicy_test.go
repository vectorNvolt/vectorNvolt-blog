package inputpolicy

import (
	"errors"
	"os"
	"strings"
	"testing"
)

type memFS map[string]string

func (m memFS) ReadFile(rel string, max int64) ([]byte, error) {
	s, ok := m[rel]
	if !ok {
		return nil, os.ErrNotExist
	}
	if int64(len(s)) > max {
		return nil, errors.New("too big")
	}
	return []byte(s), nil
}

const lib = "* opamp model\n.subckt opamp inp inn out\n.ends\n"

// call builds the reference call: netlist src/tb_amp.cir (in place) and optionally the include
// src/models/opamp.lib (in place).
func call(netlist string, withInclude bool) (memFS, []File) {
	fs := memFS{"src/tb_amp.cir": netlist, "src/models/opamp.lib": lib}
	files := []File{{Name: "netlist", Rel: "src/tb_amp.cir", ContainerPath: "/workspace/src/tb_amp.cir"}}
	if withInclude {
		files = append(files, File{Name: "include", Rel: "src/models/opamp.lib", ContainerPath: "/workspace/src/models/opamp.lib"})
	}
	return fs, files
}

func TestSpiceSafeAccepts(t *testing.T) {
	cases := map[string]string{
		"relative include":      "tb amp\n.include models/opamp.lib\nV1 in 0 1\n.tran 1u 1m\n.end\n",
		"abbrev and case":       "tb amp\n.INC 'models/opamp.lib'\n.end\n",
		"absolute include":      "tb amp\n.include \"/workspace/src/models/opamp.lib\"\n.end\n",
		"dot slash":             "tb amp\n.include ./models/opamp.lib ; the model\n.end\n",
		"lib with section":      "tb amp\n.lib models/opamp.lib tt\n.end\n",
		"safe control":          "tb amp\n.include models/opamp.lib\n.control\nrun\nwrite $OUT/x.raw v(out)\nprint v(source_node)\n.endc\n.end\n",
		"comment mentions":      "tb amp\n* .control shell rm -rf /\n* .include /etc/passwd\n.include models/opamp.lib\n.end\n",
		"shell outside control": "tb amp\n.include models/opamp.lib\n.param shell=1\n.end\n",
	}
	for name, net := range cases {
		fs, files := call(net, true)
		if err := Check("spice-safe", fs, files); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	// The library file may hold a one-argument .lib section header.
	fs, files := call("tb\n.lib models/opamp.lib tt\n", true)
	fs["src/models/opamp.lib"] = ".lib tt\n.subckt opamp a b c\n.ends\n.endl tt\n"
	if err := Check("spice-safe", fs, files); err != nil {
		t.Errorf("section header: %v", err)
	}
	// No policy is a no-op.
	if err := Check("", memFS{}, []File{{Name: "x", Rel: "missing"}}); err != nil {
		t.Errorf("empty policy: %v", err)
	}
}

func TestSpiceSafeRejects(t *testing.T) {
	cases := map[string]struct {
		netlist     string
		withInclude bool
		want        string
	}{
		"shell in control":        {"tb\n.control\nshell rm -rf /workspace/src\n.endc\n", true, "shell"},
		"shell upper case":        {"tb\n.CONTROL\n  SHELL ls\n.ENDC\n", true, "shell"},
		"source in control":       {"tb\n.control\nsource /workspace/src/evil.sp\n.endc\n", true, "source"},
		"aliased shell":           {"tb\n.control\nalias x shell\nx ls\n.endc\n", true, "shell"},
		"quoted shell":            {"tb\n.control\nset a=1; \"shell\" ls\n.endc\n", true, "shell"},
		"codemodel":               {"tb\n.control\ncodemodel /workspace/out/x.cm\n.endc\n", true, "code model"},
		"unterminated control":    {"tb\n.control\nrun\nshell ls\n", true, "shell"},
		"ng_script file":          {"*ng_script\nshell ls\n", true, "shell"},
		"undeclared include":      {"tb\n.include models/opamp.lib\n.end\n", false, "not a declared include"},
		"other file":              {"tb\n.include models/other.lib\n.end\n", true, "not a declared include"},
		"absolute outside":        {"tb\n.include /etc/passwd\n.end\n", true, "not a declared include"},
		"dotdot":                  {"tb\n.include ../src/models/opamp.lib\n.end\n", true, ".."},
		"dotdot inside":           {"tb\n.include models/../models/opamp.lib\n.end\n", true, ".."},
		"tilde":                   {"tb\n.include ~/opamp.lib\n.end\n", true, "~"},
		"lib file form":           {"tb\n.lib /workspace/.tools/x/m.lib tt\n.end\n", true, "not a declared include"},
		"include without file":    {"tb\n.include\n.end\n", true, "without a file"},
		"continued include":       {"tb\n.include\n+ /etc/passwd\n.end\n", true, "not a declared include"},
		"incl abbreviation":       {"tb\n.incl /etc/passwd\n.end\n", true, "not a declared include"},
		"nul byte":                {"tb\x00\n", true, "NUL"},
		"include in netlist name": {"tb\n.include tb_amp.cir\n.end\n", true, "not a declared include"},
	}
	for name, c := range cases {
		fs, files := call(c.netlist, c.withInclude)
		err := Check("spice-safe", fs, files)
		var rej *Rejection
		if !errors.As(err, &rej) {
			t.Errorf("%s: err %v, want a rejection", name, err)
			continue
		}
		if !strings.Contains(err.Error(), c.want) || !strings.Contains(err.Error(), "netlist") {
			t.Errorf("%s: %v, want mentioning %q", name, err, c.want)
		}
	}
}

func TestSpiceSafeScansIncludeFiles(t *testing.T) {
	fs, files := call("tb\n.include models/opamp.lib\n", true)
	fs["src/models/opamp.lib"] = ".control\nshell id\n.endc\n"
	err := Check("spice-safe", fs, files)
	if err == nil || !strings.Contains(err.Error(), "input include") {
		t.Fatalf("a control block in an include file must be refused: %v", err)
	}
	fs["src/models/opamp.lib"] = ".include /etc/shadow\n"
	if err := Check("spice-safe", fs, files); err == nil {
		t.Fatal("a nested undeclared include must be refused")
	}
}

func TestSpiceSafeStagedPaths(t *testing.T) {
	// Netlist staged as in/netlist, include staged as in/include/opamp.lib (alt-8): a relative
	// include resolves against the netlist's directory as the command sees it.
	fs := memFS{".input/amp.cir": "tb\n.include include/opamp.lib\n", ".input/opamp.lib": lib}
	files := []File{
		{Name: "netlist", Rel: ".input/amp.cir", ContainerPath: "/workspace/.output/exec/e1/in/netlist"},
		{Name: "include", Rel: ".input/opamp.lib", ContainerPath: "/workspace/.output/exec/e1/in/include/opamp.lib"},
	}
	if err := Check("spice-safe", fs, files); err != nil {
		t.Fatal(err)
	}
	fs[".input/amp.cir"] = "tb\n.include opamp.lib\n"
	if err := Check("spice-safe", fs, files); err == nil {
		t.Fatal("a source-relative name must not match the staged include")
	}
}

func TestUnreadableAndUnknown(t *testing.T) {
	err := Check("spice-safe", memFS{}, []File{{Name: "netlist", Rel: "src/none.cir", ContainerPath: "/workspace/src/none.cir"}})
	var rej *Rejection
	if !errors.As(err, &rej) {
		t.Fatalf("unreadable: %v", err)
	}
	if err := Check("nope", memFS{}, nil); err == nil {
		t.Fatal("unknown policy accepted")
	}
}
