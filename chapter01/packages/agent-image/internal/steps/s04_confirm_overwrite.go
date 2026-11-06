// SFR-IMG-006 (USC-003 step 4, alt-2): replace an existing image only after y/Y, asked before the
// build; declining is exit 0. SFR-IMG-018: the prompt is interruptible.
package steps

import (
	"context"
	"fmt"
	"strings"
	"time"

	"agent-image/internal/config"
)

type answer struct {
	line string
	err  error
}

func ConfirmOverwrite(ctx context.Context, env *Env) *Failure {
	if !env.Exists {
		return nil
	}
	env.Log.Prompt(fmt.Sprintf("overwrite %s? [y/N] ", env.Image))
	ch := make(chan answer, 1) // buffered: an abandoned read never blocks its goroutine on send
	go func() {
		line, err := env.In.ReadString('\n')
		ch <- answer{line, err}
	}()
	select {
	case <-ctx.Done():
		return nil // the sequence turns this into exit 130
	case a := <-ch:
		if a.err != nil && a.line == "" { // EOF/error with no text: may be a Windows Ctrl-C ahead of the signal handler
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(config.InterruptGrace):
			}
		}
		if s := strings.TrimSpace(a.line); s == "y" || s == "Y" {
			return nil
		}
	}
	return &Failure{ExitCode: 0, Message: fmt.Sprintf("kept existing %s; nothing built", env.Image)}
}
