// Console rendering at IP-CLI-CREATE-AGENT-IMAGE: progress on stdout, failures on stderr.
package services

import (
	"fmt"
	"io"
	"strings"
)

type Renderer struct {
	Out, Err io.Writer
}

func NewRenderer(out, err io.Writer) *Renderer {
	return &Renderer{Out: out, Err: err}
}

func (r *Renderer) Step(n, total int, name string) {
	fmt.Fprintf(r.Out, "[%d/%d] %s\n", n, total, name)
}

func (r *Renderer) Infof(format string, a ...any) {
	fmt.Fprintf(r.Out, format+"\n", a...)
}

// Prompt writes text without a newline (the overwrite question, SFR-IMG-006).
func (r *Renderer) Prompt(text string) { fmt.Fprint(r.Out, text) }

// FailureReport describes the failed step for the stderr report.
type FailureReport struct {
	Step      int
	Name      string
	Message   string
	ExitCode  int
	ExitState string
}

func (r *Renderer) Failure(f FailureReport) {
	fmt.Fprintf(r.Err, "create_agent_image: step %d (%s) failed\n", f.Step, f.Name)
	if f.Message != "" {
		fmt.Fprintf(r.Err, "  %s\n", strings.ReplaceAll(f.Message, "\n", "\n  "))
	}
	if f.ExitState != "" {
		fmt.Fprintf(r.Err, "  exit_state: %s\n", f.ExitState)
	}
	fmt.Fprintf(r.Err, "  exit_code: %d\n", f.ExitCode)
}

// Interrupted is the single stderr line of SFR-IMG-018.
func (r *Renderer) Interrupted(step int, name string) {
	fmt.Fprintf(r.Err, "create_agent_image: interrupted at step %d (%s); exit_state: INTERRUPTED, exit_code: 130\n", step, name)
}

func (r *Renderer) Success(lines ...string) {
	for _, l := range lines {
		fmt.Fprintln(r.Out, l)
	}
}
