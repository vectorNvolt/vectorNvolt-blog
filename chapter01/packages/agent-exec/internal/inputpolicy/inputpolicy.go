// Package inputpolicy holds the input filters a tool profile can name (`input_policy`). A policy
// runs on the source files of a tool run after validation and before the exec dir is created; a
// rejection is exit 125, cause `policy`, with nothing created. SFR-AGT-009, USC-009 step 5 / exc-9.
package inputpolicy

import (
	"fmt"
	"path"
	"strings"
)

// MaxCheckBytes bounds how much of one input file a policy reads; a larger file is refused.
const MaxCheckBytes int64 = 16 << 20

// IncludeInput is the input name whose files `.include`/`.lib` may resolve to under spice-safe.
const IncludeInput = "include"

// File is one input of the call.
type File struct {
	Name          string // declared input name
	Rel           string // workspace-relative source path, already resolved without symlinks
	ContainerPath string // absolute path the command sees (in place under src/ or staged under in/)
}

// Reader reads a workspace file (wsfs.Workspace satisfies it).
type Reader interface {
	ReadFile(rel string, max int64) ([]byte, error)
}

// Rejection is a refused input; Error is the reason written to stderr.
type Rejection struct {
	Input string
	Rel   string
	Line  int
	Msg   string
}

func (r *Rejection) Error() string {
	if r.Line > 0 {
		return fmt.Sprintf("input %s (%s) line %d: %s", r.Input, r.Rel, r.Line, r.Msg)
	}
	return fmt.Sprintf("input %s (%s): %s", r.Input, r.Rel, r.Msg)
}

// Check runs the named policy over every file of the call. "" is no policy. An unknown policy is
// an error (the profile validation already rejects it).
func Check(policy string, rd Reader, files []File) error {
	switch policy {
	case "":
		return nil
	case "spice-safe":
		return spiceSafe(rd, files)
	}
	return fmt.Errorf("unknown input policy %q", policy)
}

// spice-safe (USS-009): every input file is scanned (the netlist and the include files, since an
// included file is read by ngspice like the netlist).
//
//   - A `.control` ... `.endc` block (or a whole file starting with `*ng_script`) must not contain the
//     word `shell`, `source` or `codemodel` (start a shell, source a script, load a code model);
//     an unterminated `.control` runs to the end of the file. The check is on words, so
//     `alias x shell` and `echo source` are refused too (conservative).
//   - Every `.include`/`.inc*` and every file form of `.lib` (`.lib <file> <section>`) must resolve
//     to the container path of an input named `include` of this call. Resolution (open item of
//     USS-009, chosen here): an absolute path is taken as is; a relative path is joined to the
//     directory of the including file as the command sees it (in place `/workspace/src/...`, staged
//     `/workspace/.output/exec/<id>/in/...`), which is where ngspice looks first. A path with a `..`
//     component, a leading `~` or a NUL is refused. The include inputs themselves were resolved
//     without symlinks (SSR-AGE-013), so an exact match cannot leave the declared files.
//   - A one-argument `.lib <section>` is a section header inside a library file and is allowed.
func spiceSafe(rd Reader, files []File) error {
	allowed := map[string]bool{}
	for _, f := range files {
		if f.Name == IncludeInput {
			allowed[f.ContainerPath] = true
		}
	}
	for _, f := range files {
		data, err := rd.ReadFile(f.Rel, MaxCheckBytes)
		if err != nil {
			return &Rejection{Input: f.Name, Rel: f.Rel, Msg: fmt.Sprintf("cannot be checked: %v", err)}
		}
		if err := scanSpice(f, string(data), allowed); err != nil {
			return err
		}
	}
	return nil
}

var forbiddenControl = map[string]string{
	"shell":     "a .control block must not start a shell",
	"source":    "a .control block must not source a file",
	"codemodel": "a .control block must not load a code model",
}

type logical struct {
	line int // first physical line
	text string
}

// logicalLines joins `+` continuation lines to the line before them (SPICE syntax).
func logicalLines(data string) []logical {
	var out []logical
	for i, raw := range strings.Split(strings.ReplaceAll(data, "\r\n", "\n"), "\n") {
		t := strings.TrimSpace(raw)
		if strings.HasPrefix(t, "+") && len(out) > 0 {
			out[len(out)-1].text += " " + strings.TrimSpace(t[1:])
			continue
		}
		out = append(out, logical{line: i + 1, text: t})
	}
	return out
}

func scanSpice(f File, data string, allowed map[string]bool) error {
	if strings.ContainsRune(data, 0) {
		return &Rejection{Input: f.Name, Rel: f.Rel, Msg: "contains a NUL byte (not a netlist)"}
	}
	lines := logicalLines(data)
	inControl := false
	for i, l := range lines {
		lower := strings.ToLower(l.text)
		if i == 0 && strings.HasPrefix(lower, "*ng_script") {
			inControl = true // the whole file is a control script
			continue
		}
		if lower == "" || strings.HasPrefix(lower, "*") {
			continue
		}
		// The command word ends at the first blank or quote (`.include"x"` is `.include "x"`);
		// it is cut from the original text so lowercasing cannot shift the argument offset.
		cut := strings.IndexAny(l.text, " \t\"'")
		if cut < 0 {
			cut = len(l.text)
		}
		cmd, rest := strings.ToLower(l.text[:cut]), l.text[cut:]
		switch {
		case cmd == ".control":
			inControl = true
			continue
		case cmd == ".endc":
			inControl = false
			continue
		}
		if inControl {
			for _, w := range controlWords(lower) {
				if msg, bad := forbiddenControl[w]; bad {
					return &Rejection{Input: f.Name, Rel: f.Rel, Line: l.line, Msg: msg}
				}
			}
			continue
		}
		isInclude := strings.HasPrefix(cmd, ".inc")
		isLib := cmd == ".lib"
		if !isInclude && !isLib {
			continue
		}
		args := spiceArgs(rest)
		if isLib && len(args) < 2 {
			continue // `.lib <section>`: section header inside a library file
		}
		if len(args) == 0 {
			return &Rejection{Input: f.Name, Rel: f.Rel, Line: l.line, Msg: cmd + " without a file"}
		}
		if err := checkInclude(f, args[0], allowed); err != "" {
			return &Rejection{Input: f.Name, Rel: f.Rel, Line: l.line, Msg: fmt.Sprintf("%s %s: %s", cmd, args[0], err)}
		}
	}
	return nil
}

// controlWords splits a control line into words, dropping quotes and brackets around them.
func controlWords(s string) []string {
	var out []string
	for _, w := range strings.FieldsFunc(s, func(r rune) bool {
		return r == ' ' || r == '\t' || r == ';' || r == ','
	}) {
		w = strings.Trim(w, "\"'(){}[]`")
		if w != "" {
			out = append(out, w)
		}
	}
	return out
}

// spiceArgs splits the arguments of a dot command; a quoted argument keeps its spaces. An inline
// comment (`;`, `$ `, `//`) ends the list.
func spiceArgs(s string) []string {
	var out []string
	s = strings.TrimSpace(s)
	for s != "" {
		switch {
		case s[0] == '"' || s[0] == '\'':
			q := s[0]
			end := strings.IndexByte(s[1:], q)
			if end < 0 {
				out = append(out, s[1:])
				return out
			}
			out = append(out, s[1:1+end])
			s = strings.TrimSpace(s[2+end:])
			continue
		case s[0] == ';', strings.HasPrefix(s, "$ "), strings.HasPrefix(s, "//"):
			return out
		}
		end := strings.IndexAny(s, " \t")
		if end < 0 {
			out = append(out, s)
			return out
		}
		out = append(out, s[:end])
		s = strings.TrimSpace(s[end:])
	}
	return out
}

// checkInclude returns "" when p resolves to a declared include input, else the reason.
func checkInclude(f File, p string, allowed map[string]bool) string {
	switch {
	case p == "":
		return "empty path"
	case strings.ContainsRune(p, 0), strings.HasPrefix(p, "~"):
		return "path with ~ or NUL is not allowed"
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return "a .. component is not allowed"
		}
	}
	target := p
	if !path.IsAbs(p) {
		target = path.Join(path.Dir(f.ContainerPath), p)
	}
	target = path.Clean(target)
	if allowed[target] {
		return ""
	}
	return fmt.Sprintf("resolves to %s, which is not a declared %s input of this call (add --in %s=<path>)", target, IncludeInput, IncludeInput)
}
