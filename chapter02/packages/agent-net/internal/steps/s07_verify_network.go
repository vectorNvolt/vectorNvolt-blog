// SFR-NET-006 + SSR-NET-001 (USC-004 step 5, err-8): re-inspect the network (exists, driver bridge,
// Internal false, subnet 172.30.0.0/24), read back the gate (nft table agents_gate, its DOCKER-USER rule,
// bridge-nf-call-iptables == 1) and compare daemon.json hash and engine start identity with the
// recorded state. A mismatch is reported by check name; the network is left in place.
package steps

import (
	"context"
	"fmt"

	"agent-net/internal/config"
)

func VerifyNetwork(ctx context.Context, env *Env) *Failure {
	var bad []string

	info, found, err := env.Docker.InspectNetwork(ctx, config.NetworkName)
	switch {
	case err != nil:
		bad = append(bad, fmt.Sprintf("network %s unreadable: %v", config.NetworkName, err))
	case !found:
		bad = append(bad, fmt.Sprintf("network %s is missing", config.NetworkName))
	default:
		if info.Driver != config.NetworkDriver {
			bad = append(bad, fmt.Sprintf("driver is %q, want %q", info.Driver, config.NetworkDriver))
		}
		if info.Internal {
			bad = append(bad, "network is internal (Internal=true), want Internal=false")
		}
		if subnets := info.Subnets(); !contains(subnets, config.Subnet) {
			bad = append(bad, fmt.Sprintf("subnet is %v, want %s", subnets, config.Subnet))
		}
		// The gate installed by InstallGate: nft table, DOCKER-USER rule, bridge-nf-call-iptables.
		for _, g := range gateProblems(ctx, env, info.BridgeName()) {
			bad = append(bad, "gate: "+g)
		}
	}

	after, err := snapshotEngine(env)
	if err != nil {
		bad = append(bad, fmt.Sprintf("engine state unreadable after creation: %v", err))
	} else {
		if after.DaemonHash != env.Before.DaemonHash {
			bad = append(bad, fmt.Sprintf("daemon.json changed (SHA-256 %s -> %s)", env.Before.DaemonHash, after.DaemonHash))
		}
		if after.Identity != env.Before.Identity {
			bad = append(bad, fmt.Sprintf("docker engine was restarted (%s -> %s)", env.Before.Identity, after.Identity))
		}
	}

	if len(bad) > 0 {
		return &Failure{
			ExitCode:  ExitRuntime,
			ExitState: StateCreatedUnverified,
			Message:   fmt.Sprintf("network %s does not match the requested state; left in place", config.NetworkName),
			Checks:    bad,
		}
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
