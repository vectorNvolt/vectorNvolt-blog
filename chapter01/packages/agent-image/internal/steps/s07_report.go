// SFR-IMG-011 (USC-003 step 7): report success on stdout; the sequence then returns exit 0.
package steps

import (
	"context"
	"fmt"
)

func ReportSuccess(_ context.Context, env *Env) *Failure {
	env.Log.Success(fmt.Sprintf("agent image %s built in agent-host", env.Image))
	return nil
}
