// SFR-NET-001 (USC-004 step 2, err-1): accept exactly `[]`, `["arm", <container>]`, `["disarm", <container>]`,
// `["check-gate", <container>]`, `["released"]` or `["evict", <mac>]`; anything else is a usage error. An invalid invocation whose first arg is `arm` /
// `disarm` / `check-gate` reports that mode's not-done state (GATED_UNARMED / NOT_DISARMED / NOT_CHECKED); any other
// invalid invocation reports NOT_CREATED.
package steps

import (
	"context"
	"net"
	"regexp"

	"agent-net/internal/config"
)

// Usage is printed for any invocation other than the four accepted forms.
const Usage = "usage: create_agent_net\n" +
	"       create_agent_net arm <container>\n" +
	"       create_agent_net disarm <container>\n" +
	"       create_agent_net check-gate <container>\n" +
	"       create_agent_net released\n" +
	"       create_agent_net evict <mac>\n" +
	"  <container> must match " + config.ContainerPattern + " (docker container name)"

var containerRe = regexp.MustCompile(config.ContainerPattern)

// validContainer is a docker container name whose port table name as_port_<id> is not empty. The name "gate" is
// refused: its rule tag agent-net:gate is the tag of the network gate rule.
func validContainer(name string) bool {
	return containerRe.MatchString(name) && config.PortID(name) != "" && config.CommentPrefix+name != config.GateComment
}

func ValidateInvocation(_ context.Context, env *Env) *Failure {
	if len(env.Args) == 0 {
		env.Mode = ModeCreate
		return nil
	}
	switch env.Args[0] {
	case "released":
		env.Mode = ModeReleased
		if len(env.Args) == 1 {
			return nil
		}
		return &Failure{ExitCode: ExitPrerequisite, ExitState: notDone(env), Message: Usage}
	case "evict":
		env.Mode = ModeEvict
		if len(env.Args) == 2 {
			if hw, err := net.ParseMAC(env.Args[1]); err == nil && len(hw) == 6 {
				env.EvictMAC = hw.String()
				return nil
			}
		}
		return &Failure{ExitCode: ExitPrerequisite, ExitState: notDone(env), Message: Usage}
	}
	modes := map[string]string{"arm": ModeArm, "disarm": ModeDisarm, "check-gate": ModeCheckGate}
	mode, known := modes[env.Args[0]]
	if known {
		// An invalid invocation of a known mode is reported with the not-done state of that mode, not NOT_CREATED.
		env.Mode = mode
		if len(env.Args) == 2 && validContainer(env.Args[1]) {
			env.Container = env.Args[1]
			return nil
		}
	}
	return &Failure{ExitCode: ExitPrerequisite, ExitState: notDone(env), Message: Usage}
}
