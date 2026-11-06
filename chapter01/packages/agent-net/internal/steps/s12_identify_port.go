// SFR-NET-008 + SSR-NET-002 (USC-004 alt-1, alt-3): identify the host-side veth from host state only. docker inspect gives the PID and
// the IP/MAC docker assigned on agents-net; the interface in the container's netns with that MAC and IP is found
// by running the host's `ip` through nsenter (no container code runs); its peer index (link_index) is mapped to
// the host interface, whose master must be the agents-net bridge. Under gVisor (runsc) the sandbox moves the
// interface IP into its own netstack and removes it from the netns, so an interface with the MAC and no IPv4
// address also matches; one with the MAC and a different IPv4 address never does.
package steps

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"

	"agent-net/internal/config"
	"agent-net/internal/services"
)

func IdentifyPort(ctx context.Context, env *Env) *Failure {
	const control = "port identification"
	info, err := env.Docker.InspectContainer(ctx, env.Container)
	if err != nil {
		return failControl(control, "container unreadable", err)
	}
	env.Inspected = info
	ep, ok := info.NetworkSettings.Networks[config.NetworkName]
	if !ok || ep.IPAddress == "" || ep.MacAddress == "" {
		return failControl(control, "docker reports no IP/MAC for "+config.NetworkName, nil)
	}
	if info.State.Pid <= 0 {
		return failControl(control, "container has no running process (is it started?)", nil)
	}
	env.Port = Port{PID: info.State.Pid, IP: ep.IPAddress, MAC: strings.ToLower(ep.MacAddress)}

	inside, err := env.Net.NetnsIfaces(ctx, env.Port.PID)
	if err != nil {
		return failControl(control, "network namespace unreadable", err)
	}
	peer, matches := 0, 0
	otherIP := "" // IPv4 of a MAC match that carries a different address
	for _, i := range inside {
		if !strings.EqualFold(i.Address, env.Port.MAC) {
			continue
		}
		hasIP, hasV4 := false, false
		for _, a := range i.AddrInfo {
			if a.Local == env.Port.IP {
				hasIP = true
			}
			if addr, perr := netip.ParseAddr(a.Local); perr == nil && addr.Is4() {
				hasV4 = true
				if otherIP == "" && a.Local != env.Port.IP {
					otherIP = a.Local
				}
			}
		}
		if hasIP || !hasV4 { // one interface counts once
			peer = i.LinkIndex
			matches++
		}
	}
	if matches > 1 { // ambiguous: fail closed instead of picking one
		return failControl(control, fmt.Sprintf("%d interfaces with MAC %s (and IP %s or no IPv4 address) in the container namespace, the port is ambiguous", matches, env.Port.MAC, env.Port.IP), nil)
	}
	if peer <= 0 {
		if otherIP != "" {
			return failControl(control, fmt.Sprintf("interface with MAC %s carries IPv4 %s, want %s or none (runsc)", env.Port.MAC, otherIP, env.Port.IP), nil)
		}
		return failControl(control, fmt.Sprintf("no interface with MAC %s (and IP %s or no IPv4 address) in the container namespace", env.Port.MAC, env.Port.IP), nil)
	}

	links, err := env.Net.HostLinks(ctx)
	if err != nil {
		return failControl(control, "host links unreadable", err)
	}
	var host []services.Iface
	for _, l := range links {
		if l.IfIndex == peer {
			host = append(host, l)
		}
	}
	switch {
	case len(host) > 1:
		return failControl(control, fmt.Sprintf("%d host interfaces with index %d, the port is ambiguous", len(host), peer), nil)
	case len(host) == 0:
		return failControl(control, fmt.Sprintf("no host interface with index %d", peer), nil)
	case host[0].Master != env.Bridge:
		return failControl(control, fmt.Sprintf("host interface %s has master %q, want %s", host[0].IfName, host[0].Master, env.Bridge), errors.New("not attached to agents-net"))
	}
	env.Port.Veth = host[0].IfName
	return nil
}
