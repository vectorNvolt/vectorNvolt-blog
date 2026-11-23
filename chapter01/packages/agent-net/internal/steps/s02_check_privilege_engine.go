// SFR-NET-002 (USC-004 step 3, err-2/err-3; also the arm precondition): root, CAP_NET_ADMIN,
// access to the docker socket and a reachable engine; name what is missing, create nothing.
package steps

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"agent-net/internal/config"
	"agent-net/internal/services"
)

func CheckPrivilege(ctx context.Context, env *Env) *Failure {
	state := notDone(env)
	var missing []string
	if env.Host.Geteuid() != 0 {
		missing = append(missing, "not running as root (euid is not 0)")
	}
	if ok, err := hasCapNetAdmin(env.Host); err != nil {
		missing = append(missing, fmt.Sprintf("CAP_NET_ADMIN could not be checked: %v", err))
	} else if !ok {
		missing = append(missing, "CAP_NET_ADMIN missing from the effective capability set")
	}
	if own, detail, err := inOwnNetScope(env.Host); err != nil {
		missing = append(missing, fmt.Sprintf("the agent host's own network scope could not be checked: %v", err))
	} else if !own {
		missing = append(missing, fmt.Sprintf("not in the agent host's own network scope (%s)", detail))
	}
	if err := env.Host.SocketAccessible(env.Config.DockerSocket); err != nil {
		missing = append(missing, fmt.Sprintf("no access to the docker socket %s: %v", env.Config.DockerSocket, err))
	}
	if len(missing) > 0 {
		return &Failure{ExitCode: ExitPrerequisite, ExitState: state, Message: "required privilege missing", Checks: missing}
	}
	if err := env.Docker.Version(ctx); err != nil {
		return fail(ExitPrerequisite, state, "docker engine unreachable", err)
	}
	return nil
}

// inOwnNetScope compares the network namespace of this process with that of pid 1 of the agent host. CAP_NET_ADMIN
// only counts in the namespace the host network state (bridge, nftables, iptables) lives in.
func inOwnNetScope(h services.Host) (own bool, detail string, err error) {
	self, err := h.Readlink(config.ProcSelfNetNS)
	if err != nil {
		return false, "", fmt.Errorf("%s: %w", config.ProcSelfNetNS, err)
	}
	initNS, err := h.Readlink(config.ProcInitNetNS)
	if err != nil {
		return false, "", fmt.Errorf("%s: %w", config.ProcInitNetNS, err)
	}
	return self == initNS, fmt.Sprintf("this process %s, pid 1 %s", self, initNS), nil
}

// hasCapNetAdmin parses CapEff of /proc/self/status (hex mask, bit 12).
func hasCapNetAdmin(h services.Host) (bool, error) {
	data, err := h.ReadFile(config.ProcStatus)
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		v, ok := strings.CutPrefix(line, "CapEff:")
		if !ok {
			continue
		}
		mask, err := strconv.ParseUint(strings.TrimSpace(v), 16, 64)
		if err != nil {
			return false, fmt.Errorf("cannot parse CapEff %q: %w", strings.TrimSpace(v), err)
		}
		return mask&(1<<config.CapNetAdmin) != 0, nil
	}
	return false, fmt.Errorf("no CapEff line in %s", config.ProcStatus)
}
