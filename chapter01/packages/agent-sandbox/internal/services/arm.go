// IP-AGENT-NET adapter: calls the arming unit `create_agent_net arm <container>` (USS-004).
package services

import (
	"context"
	"strings"
)

// Arm runs the arming unit. Its exit codes are undefined in the design, so any
// non-zero exit or failure to run counts as an arming failure.
type Arm struct {
	Runner    Runner
	Bin       string
	RemoveBin string // remove_agent_net, used by agent_lifecycle restore only
}

// CreateNet runs `create_agent_net` without arguments (creates agents-net and its gate).
func (a *Arm) CreateNet(ctx context.Context) error {
	_, err := runCmd(ctx, a.Runner, a.Bin)
	return err
}

// RemoveNet runs `remove_agent_net` (no arguments).
func (a *Arm) RemoveNet(ctx context.Context) error {
	_, err := runCmd(ctx, a.Runner, a.RemoveBin)
	return err
}

func (a *Arm) Apply(ctx context.Context, container string) error {
	_, err := runCmd(ctx, a.Runner, a.Bin, "arm", container)
	return err
}

// CheckGate runs `create_agent_net check-gate <container>` (read-only read-back of the controls, SFR-AGC-007):
// exit 0 = all controls present and verified; 1 (prerequisite) or 2 (control missing or differing) is a
// CmdError with Err nil whose Stdout holds the per-control report lines.
func (a *Arm) CheckGate(ctx context.Context, container string) error {
	_, err := runCmd(ctx, a.Runner, a.Bin, "check-gate", container)
	return err
}

// Disarm runs `create_agent_net disarm <container>` (USS-004 contract: exit 0 = rules removed
// or nothing to disarm, 1 = prerequisite, 2 = runtime). A CmdError with Err set means the
// binary could not run; a CmdError with Err nil carries the tool's non-zero exit and stderr.
func (a *Arm) Disarm(ctx context.Context, container string) error {
	_, err := runCmd(ctx, a.Runner, a.Bin, "disarm", container)
	return err
}

// ReleasedElem is one line of `create_agent_net released`: a MAC in the gate set and the container name it is tagged with.
type ReleasedElem struct {
	MAC     string
	Comment string // "" for an untagged element
}

// Released runs `create_agent_net released` (read only): one `<mac> <comment-or-empty>` line per element of the set.
func (a *Arm) Released(ctx context.Context) ([]ReleasedElem, error) {
	out, err := runCmd(ctx, a.Runner, a.Bin, "released")
	if err != nil {
		return nil, err
	}
	var elems []ReleasedElem
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		e := ReleasedElem{MAC: f[0]}
		if len(f) > 1 {
			e.Comment = strings.Join(f[1:], " ")
		}
		elems = append(elems, e)
	}
	return elems, nil
}

// Evict runs `create_agent_net evict <mac>`: removes one element of the gate set (untagged or orphaned).
func (a *Arm) Evict(ctx context.Context, mac string) error {
	_, err := runCmd(ctx, a.Runner, a.Bin, "evict", mac)
	return err
}
