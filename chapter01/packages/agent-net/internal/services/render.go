// Console rendering at IP-CLI-CREATE-AGENT-NET: progress on stdout, failures on stderr.
// For `arm` the stderr text is what create_agent_container reports (IP-AGENT-NET, err-7).
package services

import (
	"fmt"
	"io"
	"strings"
)

type Renderer struct {
	Out, Err io.Writer
	Level    string
	Tool     string // program name in failure reports; empty is create_agent_net

	// ProgressToErr sends the `[n/m]` step lines and the debug lines to stderr, so that stdout carries only the
	// result (check-gate: the control lines and the summary).
	ProgressToErr bool
}

func (r *Renderer) progressOut() io.Writer {
	if r.ProgressToErr {
		return r.Err
	}
	return r.Out
}

func (r *Renderer) tool() string {
	if r.Tool == "" {
		return "create_agent_net"
	}
	return r.Tool
}

func NewRenderer(out, err io.Writer, level string) *Renderer {
	return &Renderer{Out: out, Err: err, Level: strings.ToUpper(level)}
}

func (r *Renderer) IsDebug() bool { return r.Level == "DEBUG" }

func (r *Renderer) showInfo() bool { return r.Level == "DEBUG" || r.Level == "INFO" || r.Level == "" }

func (r *Renderer) Step(n, total int, name string) {
	if r.showInfo() {
		fmt.Fprintf(r.progressOut(), "[%d/%d] %s\n", n, total, name)
	}
}

func (r *Renderer) Debugf(format string, a ...any) {
	if r.IsDebug() {
		fmt.Fprintf(r.progressOut(), "  debug: "+format+"\n", a...)
	}
}

// FailureReport describes the failed step for the stderr report.
// Control is set by the arming steps: the first stderr line is then "failed control: <name>: <detail>".
type FailureReport struct {
	Step      int
	Name      string
	Message   string
	Control   string
	ExitCode  int
	ExitState string
	Checks    []string
}

func (r *Renderer) Failure(f FailureReport) {
	if f.Control != "" {
		fmt.Fprintf(r.Err, "failed control: %s: %s\n", f.Control, strings.ReplaceAll(f.Message, "\n", "\n  "))
		fmt.Fprintf(r.Err, "  %s: step %d (%s) failed\n", r.tool(), f.Step, f.Name)
	} else {
		fmt.Fprintf(r.Err, "%s: step %d (%s) failed\n", r.tool(), f.Step, f.Name)
		if f.Message != "" {
			fmt.Fprintf(r.Err, "  %s\n", strings.ReplaceAll(f.Message, "\n", "\n  "))
		}
	}
	for _, c := range f.Checks {
		fmt.Fprintf(r.Err, "  FAILED CHECK: %s\n", strings.ReplaceAll(c, "\n", "\n    "))
	}
	if f.ExitState != "" {
		fmt.Fprintf(r.Err, "  exit_state: %s\n", f.ExitState)
	}
	fmt.Fprintf(r.Err, "  exit_code: %d\n", f.ExitCode)
}

func (r *Renderer) Success(lines ...string) {
	for _, l := range lines {
		fmt.Fprintln(r.Out, l)
	}
}
