// SFR-NET-020, SFR-AGC-009 (USC-002 step 9, err-12): read /etc/resolv.conf back with an exec as the agent user:
// exactly the two allowed resolvers, root:root, mode 644, and not writable by uid 1001. The mismatches are
// returned to the final verification (s08), which lists them and contains the container.
package steps

import (
	"context"
	"fmt"
	"strings"

	"agent-sandbox/internal/config"
)

func resolvConfMismatches(ctx context.Context, env *Env) []string {
	agent := config.AgentUser
	want := config.ResolvConfContent
	wantOwnerMode := config.ResolvConfOwner + " " + config.ResolvConfMode

	var bad []string
	if out, err := env.Docker.Exec(ctx, env.Container, agent, "cat", config.ResolvConfPath); err != nil {
		bad = append(bad, fmt.Sprintf("%s unreadable as %s: %v", config.ResolvConfPath, agent, err))
	} else if string(out) != want {
		bad = append(bad, fmt.Sprintf("%s content is %q, want %q", config.ResolvConfPath, string(out), want))
	}

	if out, err := env.Docker.Exec(ctx, env.Container, agent, "stat", "-c", "%U:%G %a", config.ResolvConfPath); err != nil {
		bad = append(bad, fmt.Sprintf("owner and mode of %s unreadable: %v", config.ResolvConfPath, err))
	} else if got := strings.TrimSpace(string(out)); got != wantOwnerMode {
		bad = append(bad, fmt.Sprintf("%s owner and mode are %q, want %q", config.ResolvConfPath, got, wantOwnerMode))
	}

	// An append of nothing succeeds only when the agent user can write the file.
	if writable, err := env.Docker.ExecSucceeds(ctx, env.Container, agent, "sh", "-c", ": >> "+config.ResolvConfPath); err != nil {
		bad = append(bad, fmt.Sprintf("write test of %s as %s could not run: %v", config.ResolvConfPath, agent, err))
	} else if writable {
		bad = append(bad, fmt.Sprintf("%s is writable by the agent user %s", config.ResolvConfPath, agent))
	}
	return bad
}
