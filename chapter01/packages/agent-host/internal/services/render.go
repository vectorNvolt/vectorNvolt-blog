// Console rendering at IP-CLI-CREATE-AGENT-HOST: progress on stdout, failures on stderr.
package services

import (
	"fmt"
	"io"
	"strings"
)

type Renderer struct {
	Out, Err io.Writer
	Level    string
}

func NewRenderer(out, err io.Writer, level string) *Renderer {
	return &Renderer{Out: out, Err: err, Level: strings.ToUpper(level)}
}

func (r *Renderer) IsDebug() bool { return r.Level == "DEBUG" }

func (r *Renderer) showInfo() bool { return r.Level == "DEBUG" || r.Level == "INFO" || r.Level == "" }

func (r *Renderer) Step(n, total int, name string) {
	if r.showInfo() {
		fmt.Fprintf(r.Out, "[%d/%d] %s\n", n, total, name)
	}
}

func (r *Renderer) Debugf(format string, a ...any) {
	if r.IsDebug() {
		fmt.Fprintf(r.Out, "  debug: "+format+"\n", a...)
	}
}

// Warnf reports a non-fatal condition on stderr.
func (r *Renderer) Warnf(format string, a ...any) {
	fmt.Fprintf(r.Err, "warning: "+format+"\n", a...)
}

// FailureReport describes the failed step for the stderr report.
type FailureReport struct {
	Step      int
	Name      string
	Message   string
	ExitCode  int
	ExitState string
	Checks    []string
}

func (r *Renderer) Failure(f FailureReport) {
	fmt.Fprintf(r.Err, "create_agent_host: step %d (%s) failed\n", f.Step, f.Name)
	if f.Message != "" {
		fmt.Fprintf(r.Err, "  %s\n", strings.ReplaceAll(f.Message, "\n", "\n  "))
	}
	for _, c := range f.Checks {
		fmt.Fprintf(r.Err, "  FAILED CHECK: %s\n", strings.ReplaceAll(c, "\n", "\n    "))
	}
	fmt.Fprintf(r.Err, "  exit_state: %s\n", f.ExitState)
	if f.ExitState != "NOT_CREATED" {
		fmt.Fprintln(r.Err, "  no rollback was performed; remove the distro manually with `wsl.exe --unregister agent-host` before retrying")
	}
	fmt.Fprintf(r.Err, "  exit_code: %d\n", f.ExitCode)
}

func (r *Renderer) Success(lines ...string) {
	for _, l := range lines {
		fmt.Fprintln(r.Out, l)
	}
}
