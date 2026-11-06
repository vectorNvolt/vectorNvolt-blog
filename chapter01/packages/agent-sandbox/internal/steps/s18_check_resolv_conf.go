// SFR-AGC-024, SFR-NET-020 (USC-002 step 11, err-13): after the network arm and release, read the container's
// /etc/resolv.conf by a uid 0 docker exec. The file is the read-only bind mount of the host file (nothing is written in the
// container); both allowed resolvers must be present and Docker's embedded 127.0.0.11 must be absent. A failure
// contains the container (re-pause, verify paused); it is not removed. (File name kept from the former write step.)
package steps

import (
	"context"
	"fmt"
	"strings"

	"agent-sandbox/internal/config"
)

const embeddedResolver = "127.0.0.11"

func CheckResolvConf(ctx context.Context, env *Env) *Failure {
	out, err := env.Docker.Exec(ctx, env.Container, "0:0", "cat", config.ResolvConfPath)
	var problem string
	switch {
	case err != nil:
		problem = fmt.Sprintf("reading %s in %s failed: %v", config.ResolvConfPath, env.Container, err)
	default:
		got := string(out)
		var missing []string
		for _, ns := range []string{config.ResolverPrimary, config.ResolverSecondary} {
			if !strings.Contains(got, "nameserver "+ns) {
				missing = append(missing, ns)
			}
		}
		switch {
		case strings.Contains(got, embeddedResolver):
			problem = fmt.Sprintf("%s in %s still lists the embedded resolver %s: %q", config.ResolvConfPath, env.Container, embeddedResolver, got)
		case len(missing) > 0:
			problem = fmt.Sprintf("%s in %s lacks nameserver %s: %q", config.ResolvConfPath, env.Container, strings.Join(missing, ", "), got)
		}
	}
	if problem == "" {
		return nil
	}
	state, note := containFailure(ctx, env)
	return &Failure{
		ExitCode:  ExitRuntime,
		ExitState: state,
		Message:   problem + "\n" + note,
	}
}
