package profile

// Kind tool (USS-009 "Tool runs", USC-009 steps 4-6): the operator's profile fixes the whole
// command line (`entry: fixed`, `argv` with placeholders), the inputs, the input policy and the
// typed parameters. The caller supplies only the profile name, inputs by name and param values.
// SFR-AGT-008 (validate the call), SFR-AGT-010 (fixed command line), SSR-AGT-006.

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Placeholders of a tool argv. Nothing else is expanded and nothing is appended.
//
//	{out}           the execution's own out/ folder in the container
//	{in.<name>}     the path(s) of input <name>; as a whole argument one argument per path
//	                (none for an absent optional input), inside a longer argument only for a
//	                required single input
//	{param.<name>}  the validated (canonical) value of parameter <name>
var placeholderRe = regexp.MustCompile(`\{([^{}]*)\}`)

// InputPolicySpiceSafe is the netlist filter of USS-009 (SFR-AGT-009). The policy itself is in internal/inputpolicy.
const InputPolicySpiceSafe = "spice-safe"

// InputPolicies are the input policies the tool knows; a profile naming another is invalid.
var InputPolicies = []string{InputPolicySpiceSafe}

// Param types.
const (
	ParamInt    = "int"
	ParamNumber = "number"
	ParamString = "string"
	ParamBool   = "bool"
)

const maxParamValue = 256

var paramNameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// Param is the schema of one typed parameter. A parameter with a default is optional.
// A string parameter must be restricted by pattern (whole-value match) or values.
type Param struct {
	Type    string   `yaml:"type" json:"type"`
	Min     *float64 `yaml:"min" json:"min,omitempty"`
	Max     *float64 `yaml:"max" json:"max,omitempty"`
	Pattern string   `yaml:"pattern" json:"pattern,omitempty"`
	Values  []string `yaml:"values" json:"values,omitempty"`
	Default *string  `yaml:"default" json:"default,omitempty"`
}

// validateTool checks the fields that only kind tool has, and rejects them on other kinds.
func (p *Profile) validateTool() error {
	if p.Kind != KindTool {
		if p.InputPolicy != "" {
			return errors.New("input_policy is for kind tool only")
		}
		if len(p.Params) > 0 {
			return errors.New("params is for kind tool only")
		}
		return nil
	}
	if p.InputPolicy != "" && !contains(InputPolicies, p.InputPolicy) {
		return fmt.Errorf("input_policy %q is not one of %s", p.InputPolicy, strings.Join(InputPolicies, ", "))
	}
	for n, prm := range p.Params {
		if !paramNameRe.MatchString(n) {
			return fmt.Errorf("params: name %q is not valid", n)
		}
		if err := prm.validate(); err != nil {
			return fmt.Errorf("params.%s: %v", n, err)
		}
	}
	return p.validateToolArgv()
}

func (prm Param) validate() error {
	switch prm.Type {
	case ParamInt, ParamNumber:
		if prm.Pattern != "" || len(prm.Values) > 0 {
			return errors.New("pattern and values are for type string only")
		}
		for _, b := range []*float64{prm.Min, prm.Max} {
			if b != nil && (math.IsNaN(*b) || math.IsInf(*b, 0)) {
				return errors.New("min and max must be finite")
			}
		}
		if prm.Min != nil && prm.Max != nil && *prm.Min > *prm.Max {
			return errors.New("min is above max")
		}
	case ParamString:
		if prm.Min != nil || prm.Max != nil {
			return errors.New("min and max are for int and number only")
		}
		if prm.Pattern == "" && len(prm.Values) == 0 {
			return errors.New("a string parameter needs pattern or values")
		}
		if prm.Pattern != "" {
			if _, err := regexp.Compile(prm.Pattern); err != nil {
				return fmt.Errorf("pattern: %v", err)
			}
		}
	case ParamBool:
		if prm.Min != nil || prm.Max != nil || prm.Pattern != "" || len(prm.Values) > 0 {
			return errors.New("a bool parameter has no min, max, pattern or values")
		}
	default:
		return fmt.Errorf("type %q is not int, number, string or bool", prm.Type)
	}
	if prm.Default != nil {
		if _, err := prm.Check(*prm.Default); err != nil {
			return fmt.Errorf("default: %v", err)
		}
	}
	return nil
}

// Check validates a caller value against the schema and returns its canonical form
// (what {param.<name>} expands to). SFR-AGT-008: a mistyped value is a usage error.
func (prm Param) Check(v string) (string, error) {
	if len(v) > maxParamValue || strings.ContainsAny(v, "\x00\n\r") {
		return "", fmt.Errorf("value is longer than %d bytes or has a control character", maxParamValue)
	}
	inRange := func(f float64) error {
		if prm.Min != nil && f < *prm.Min {
			return fmt.Errorf("%s is below min %v", v, *prm.Min)
		}
		if prm.Max != nil && f > *prm.Max {
			return fmt.Errorf("%s is above max %v", v, *prm.Max)
		}
		return nil
	}
	switch prm.Type {
	case ParamInt:
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return "", fmt.Errorf("%q is not an int", v)
		}
		if err := inRange(float64(n)); err != nil {
			return "", err
		}
		return strconv.FormatInt(n, 10), nil
	case ParamNumber:
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return "", fmt.Errorf("%q is not a finite number", v)
		}
		if err := inRange(f); err != nil {
			return "", err
		}
		return strconv.FormatFloat(f, 'g', -1, 64), nil
	case ParamBool:
		switch v {
		case "true", "false":
			return v, nil
		}
		return "", fmt.Errorf("%q is not true or false", v)
	case ParamString:
		if len(prm.Values) > 0 {
			if !contains(prm.Values, v) {
				return "", fmt.Errorf("%q is not one of %s", v, strings.Join(prm.Values, ", "))
			}
			return v, nil
		}
		re, err := regexp.Compile(`^(?:` + prm.Pattern + `)$`)
		if err != nil || !re.MatchString(v) {
			return "", fmt.Errorf("%q does not match pattern %s", v, prm.Pattern)
		}
		return v, nil
	}
	return "", fmt.Errorf("unknown type %q", prm.Type)
}

// validateToolArgv: argv[0] is literal, every {...} is a known placeholder of a declared input or
// param, a placeholder inside a longer argument names a required single input, and every param is used.
func (p *Profile) validateToolArgv() error {
	used := map[string]bool{}
	for i, a := range p.Argv {
		matches := placeholderRe.FindAllStringSubmatchIndex(a, -1)
		if i == 0 && len(matches) > 0 {
			return errors.New("argv[0] must be a literal program, not a placeholder")
		}
		rest := placeholderRe.ReplaceAllString(a, "")
		if strings.ContainsAny(rest, "{}") {
			return fmt.Errorf("argv %q has an unbalanced { or }", a)
		}
		whole := len(matches) == 1 && matches[0][0] == 0 && matches[0][1] == len(a)
		for _, m := range matches {
			key := a[m[2]:m[3]]
			switch {
			case key == "out":
			case strings.HasPrefix(key, "in."):
				n := strings.TrimPrefix(key, "in.")
				in, ok := p.Inputs[n]
				if !ok {
					return fmt.Errorf("argv placeholder {%s}: no input %q is declared", key, n)
				}
				if !whole && (in.Multiple || in.Optional) {
					return fmt.Errorf("argv placeholder {%s} inside %q: an optional or multiple input must be a whole argument", key, a)
				}
			case strings.HasPrefix(key, "param."):
				n := strings.TrimPrefix(key, "param.")
				if _, ok := p.Params[n]; !ok {
					return fmt.Errorf("argv placeholder {%s}: no param %q is declared", key, n)
				}
				used[n] = true
			default:
				return fmt.Errorf("argv placeholder {%s} is not {out}, {in.<name>} or {param.<name>}", key)
			}
		}
	}
	for n := range p.Params {
		if !used[n] {
			return fmt.Errorf("params.%s is not used in argv", n)
		}
	}
	return nil
}

// ExpandToolArgv expands the placeholders of a tool argv in one pass (an expanded value is never
// expanded again) and appends nothing. in holds the container paths per input name, params the
// canonical values (defaults included). Validation guarantees every referenced name is present.
func ExpandToolArgv(argv []string, out string, in map[string][]string, params map[string]string) []string {
	res := []string{}
	for _, a := range argv {
		if m := placeholderRe.FindStringSubmatch(a); m != nil && m[0] == a && strings.HasPrefix(m[1], "in.") {
			res = append(res, in[strings.TrimPrefix(m[1], "in.")]...)
			continue
		}
		res = append(res, placeholderRe.ReplaceAllStringFunc(a, func(ph string) string {
			key := ph[1 : len(ph)-1]
			switch {
			case key == "out":
				return out
			case strings.HasPrefix(key, "in."):
				if ps := in[strings.TrimPrefix(key, "in.")]; len(ps) == 1 {
					return ps[0]
				}
			case strings.HasPrefix(key, "param."):
				if v, ok := params[strings.TrimPrefix(key, "param.")]; ok {
					return v
				}
			}
			return ph
		}))
	}
	return res
}

// SortedInputNames lists the declared inputs in a fixed order.
func (p *Profile) SortedInputNames() []string {
	var out []string
	for n := range p.Inputs {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
