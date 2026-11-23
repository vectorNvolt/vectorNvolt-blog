// Package cli parses the command line of run_ephemeral_container. The tool has no image, mount or
// flag argument that changes the container: arguments only supply data (the command, driver args,
// manifests, input files, timeout, labels, exec ID, tool params). SFR-AGE-001 (reject invalid invocation),
// SSR-AGE-005 (caller supplies data only), SFR-AGT-008 (tool: profile, inputs and params only). Every error here is exit 125, cause `usage`, nothing created.
package cli

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ExecIDRe is the execution ID pattern (USS-005 "Invocation").
var ExecIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

var (
	labelKeyRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	inNameRe   = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
)

// UsageError is a bad invocation.
type UsageError struct{ Msg string }

func (e *UsageError) Error() string { return e.Msg }

func usage(format string, a ...any) error { return &UsageError{Msg: fmt.Sprintf(format, a...)} }

type Input struct{ Name, Path string }

// Request is a parsed invocation.
type Request struct {
	Sub       string // run, build, install, tool, list, sweep
	Profile   string
	ExecID    string
	Inputs    []Input
	Params    map[string]string // tool only: --param <name>=<value>
	Manifests []string
	Labels    map[string]string
	TimeoutS  int // 0: the profile's timeout_s
	Args      []string
	JSON      bool
	SweepKey  string
	SweepVal  string
}

// ParseLabel splits k=v. SFR-AGE-001.
func ParseLabel(s string) (string, string, error) {
	k, v, ok := strings.Cut(s, "=")
	if !ok || !labelKeyRe.MatchString(k) {
		return "", "", usage("--label %q is not <k>=<v>", s)
	}
	if len(v) > 256 || strings.ContainsAny(v, "\x00\n") {
		return "", "", usage("--label %q has an invalid value", s)
	}
	return k, v, nil
}

// Parse parses the arguments after the program name.
func Parse(args []string) (*Request, error) {
	if len(args) == 0 {
		return nil, usage("missing subcommand (run, build, install, tool, list, sweep)")
	}
	req := &Request{Sub: args[0], Labels: map[string]string{}}
	rest := args[1:]
	switch req.Sub {
	case "run", "build", "install", "tool":
		if len(rest) == 0 || strings.HasPrefix(rest[0], "-") {
			return nil, usage("%s: missing profile name", req.Sub)
		}
		req.Profile, rest = rest[0], rest[1:]
	case "list", "sweep":
	default:
		return nil, usage("unknown subcommand %q", req.Sub)
	}

	flags, dashes := rest, false
	for i, a := range rest {
		if a == "--" {
			flags, req.Args, dashes = rest[:i], rest[i+1:], true
			break
		}
	}

	for i := 0; i < len(flags); i++ {
		a := flags[i]
		if !strings.HasPrefix(a, "--") {
			return nil, usage("unexpected argument %q (the command goes after --)", a)
		}
		name, val, hasVal := a, "", false
		if k := strings.Index(a, "="); k > 0 {
			name, val, hasVal = a[:k], a[k+1:], true
		}
		if name == "--json" {
			if req.Sub != "list" || hasVal {
				return nil, usage("--json is for list only")
			}
			req.JSON = true
			continue
		}
		if !hasVal {
			if i+1 >= len(flags) {
				return nil, usage("%s needs a value", name)
			}
			i++
			val = flags[i]
		}
		if err := req.setFlag(name, val); err != nil {
			return nil, err
		}
	}
	return req, req.check(dashes)
}

func (r *Request) setFlag(name, val string) error {
	install := r.Sub == "install"
	switch {
	case r.Sub == "list" || (r.Sub == "sweep" && name != "--label"):
		return usage("%s: unknown flag %s", r.Sub, name)
	case name == "--label":
		k, v, err := ParseLabel(val)
		if err != nil {
			return err
		}
		if strings.HasPrefix(k, "ephemeral.") {
			return usage("--label %q: the ephemeral.* labels are set by the tool", k)
		}
		if r.Sub == "sweep" {
			if r.SweepKey != "" {
				return usage("sweep takes exactly one --label")
			}
			r.SweepKey, r.SweepVal = k, v
			return nil
		}
		r.Labels[k] = v
	case name == "--exec-id":
		if !ExecIDRe.MatchString(val) {
			return usage("--exec-id %q does not match %s", val, ExecIDRe)
		}
		r.ExecID = val
	case name == "--timeout":
		n, err := strconv.Atoi(val)
		if err != nil || n <= 0 {
			return usage("--timeout %q is not a positive number of seconds", val)
		}
		r.TimeoutS = n
	case name == "--in":
		if install {
			return usage("install takes no --in")
		}
		n, p, ok := strings.Cut(val, "=")
		if !ok || !inNameRe.MatchString(n) || p == "" {
			return usage("--in %q is not <name>=<path>", val)
		}
		r.Inputs = append(r.Inputs, Input{Name: n, Path: p})
	case name == "--param":
		if r.Sub != "tool" {
			return usage("--param is for tool only")
		}
		n, v, ok := strings.Cut(val, "=")
		if !ok || !inNameRe.MatchString(n) {
			return usage("--param %q is not <name>=<value>", val)
		}
		if r.Params == nil {
			r.Params = map[string]string{}
		}
		if _, dup := r.Params[n]; dup {
			return usage("--param %s given twice", n)
		}
		r.Params[n] = v
	case name == "--manifest":
		if !install {
			return usage("--manifest is for install only")
		}
		if val == "" {
			return usage("--manifest needs a path")
		}
		r.Manifests = append(r.Manifests, val)
	default:
		return usage("unknown flag %s", name)
	}
	return nil
}

func (r *Request) check(dashes bool) error {
	switch r.Sub {
	case "run":
		if !dashes || len(r.Args) == 0 {
			return usage("run: missing command after --")
		}
	case "tool":
		if dashes {
			return usage("tool takes no -- arguments: the profile fixes the command line")
		}
	case "install":
		if dashes {
			return usage("install takes no free manager args")
		}
		if len(r.Manifests) == 0 {
			return usage("install: at least one --manifest is required")
		}
	case "list", "sweep":
		if dashes {
			return usage("%s takes no -- arguments", r.Sub)
		}
		if r.Sub == "sweep" && r.SweepKey == "" {
			return usage("sweep needs --label <k>=<v>")
		}
	}
	return nil
}
