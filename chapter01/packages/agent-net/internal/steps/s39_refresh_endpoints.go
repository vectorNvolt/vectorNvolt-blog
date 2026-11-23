// refresh_endpoints: reads the endpoints JSON from stdin (never from a path), validates it, resolves every name, installs
// it as the canonical endpoints file (skipped when the installed bytes are identical) and brings the endpoint accept
// rules of every armed container in DOCKER-USER up to date, without re-arming. Per container the missing rules are inserted FIRST, then the
// stale ones (destination not wanted any more) are deleted by exact spec, so the container never loses an address
// it still needs; a failed run converges when run again. Nothing else is touched: not the DNS rules, the default-deny,
// the INPUT rules or the gate rule (tag agent-net:gate).
package steps

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/netip"
	"sort"
	"strings"

	"agent-net/internal/config"
	"agent-net/internal/services"
)

// RefreshUsage is printed for any invocation other than the accepted forms.
const RefreshUsage = "usage: refresh_endpoints [--dry-run]\n" +
	"  reads the endpoints JSON from stdin (at most 64 KiB), validates it, resolves every name, installs it as " +
	config.DefaultEndpointsFile + "\n" +
	"  (skipped when the installed bytes are identical), then refreshes the rules of the armed containers"

// MaxEndpointsBytes caps the endpoints JSON read from stdin.
const MaxEndpointsBytes = 64 << 10

// ValidateRefreshInvocation parses the arguments and reads and validates the endpoints JSON from stdin (exit 1 on a bad
// invocation or content). Only the content rules apply; stdin bytes have no owner or mode.
func ValidateRefreshInvocation(_ context.Context, env *Env) *Failure {
	env.Mode = ModeRefresh
	for _, a := range env.Args {
		if a != "--dry-run" {
			return &Failure{ExitCode: ExitPrerequisite, ExitState: notDone(env), Message: RefreshUsage}
		}
		env.DryRun = true
	}

	var content []byte
	if env.Stdin != nil {
		b, err := io.ReadAll(io.LimitReader(env.Stdin, MaxEndpointsBytes+1))
		if err != nil {
			return fail(ExitPrerequisite, notDone(env), "endpoints could not be read from stdin", err)
		}
		content = b
	}
	if len(content) > MaxEndpointsBytes {
		return &Failure{ExitCode: ExitPrerequisite, ExitState: notDone(env),
			Message: fmt.Sprintf("endpoints on stdin exceed %d bytes", MaxEndpointsBytes)}
	}
	if len(bytes.TrimSpace(content)) == 0 {
		return &Failure{ExitCode: ExitPrerequisite, ExitState: notDone(env), Message: "no endpoints on stdin"}
	}
	list, err := config.ParseEndpoints(content)
	if err != nil {
		return fail(ExitPrerequisite, notDone(env), "endpoints on stdin unusable", err)
	}
	env.Config.ModelAPIs = list
	env.Config.EndpointsErr = nil
	env.EndpointsData = content
	return nil
}

// InstallEndpointsFile installs the validated stdin bytes as the canonical endpoints file, after the privilege check and
// the resolution and before the first rule change. An installed file with identical bytes is not rewritten. --dry-run
// never writes.
func InstallEndpointsFile(_ context.Context, env *Env) *Failure {
	target := env.Config.EndpointsFile
	if target == "" {
		target = config.DefaultEndpointsFile
	}
	if cur, err := env.Host.ReadFile(target); err == nil && bytes.Equal(cur, env.EndpointsData) {
		env.Log.Success(target + " already has these bytes; not written")
		return nil
	}
	if env.DryRun {
		env.Log.Success(fmt.Sprintf("would install stdin as %s (root:root, mode 0644):", target))
		env.Log.Success("  " + strings.TrimSpace(string(env.EndpointsData)))
		return nil
	}
	inst, ok := env.Host.(services.FileInstaller)
	if !ok {
		return &Failure{ExitCode: ExitRuntime, ExitState: StateNotRefreshed, Message: "this host cannot write the endpoints file"}
	}
	if err := inst.InstallFile(target, env.EndpointsData, 0o644); err != nil {
		return fail(ExitRuntime, StateNotRefreshed, "endpoints file "+target+" could not be written", err)
	}
	env.Log.Success(fmt.Sprintf("installed stdin as %s", target))
	return nil
}

// RefreshCheckPrivilege needs root and CAP_NET_ADMIN in the agent host's own network scope (iptables only; no docker).
func RefreshCheckPrivilege(_ context.Context, env *Env) *Failure {
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
	if len(missing) > 0 {
		return &Failure{ExitCode: ExitPrerequisite, ExitState: notDone(env), Message: "required privilege missing", Checks: missing}
	}
	return nil
}

// ResolveEndpoints resolves ALL endpoints before anything is changed; one failure stops the run (exit 2).
func ResolveEndpoints(ctx context.Context, env *Env) *Failure {
	if err := resolveModelAPIs(ctx, env); err != nil {
		return fail(ExitRuntime, StateNotRefreshed, "endpoint addresses could not be resolved", err)
	}
	return nil
}

// endpointGroup is the endpoint accept rules of one container and one source address.
type endpointGroup struct {
	container, src string
	have           []string // destination addresses of its rules, sorted
}

// endpointGroups lists the endpoint accept rules in DOCKER-USER per container and source. A rule counts only when it
// has exactly the shape of acceptSpec (tcp 443, /32 source and destination, ACCEPT, comment agent-net:<container>);
// the gate rule and every other rule are ignored.
func endpointGroups(ctx context.Context, env *Env) ([]*endpointGroup, error) {
	refs, err := env.Net.IptTagged(ctx, config.ChainDockerUser, func(c string) bool {
		return strings.HasPrefix(c, config.CommentPrefix) && c != config.GateComment
	})
	if err != nil {
		return nil, err
	}
	byKey := map[string]*endpointGroup{}
	for _, ref := range refs {
		f := strings.Fields(ref.Line)
		if len(f) < 3 {
			continue
		}
		rest := f[2:]
		dst := ""
		for i := 0; i+1 < len(rest); i++ {
			if rest[i] == "-d" {
				dst = rest[i+1]
				break
			}
		}
		p, perr := netip.ParsePrefix(dst)
		src := ref.Source()
		if perr != nil || !p.Addr().Is4() || p.Bits() != 32 || src == "" {
			continue
		}
		want := acceptSpec(ref.Comment, src+"/32", p.Addr().String(), "tcp", config.ModelAPIPort)
		if strings.Join(rest, " ") != strings.Join(want, " ") {
			continue
		}
		key := ref.Comment + " " + src
		g := byKey[key]
		if g == nil {
			g = &endpointGroup{container: strings.TrimPrefix(ref.Comment, config.CommentPrefix), src: src}
			byKey[key] = g
		}
		g.have = appendUnique(g.have, p.Addr().String())
	}
	var out []*endpointGroup
	for _, g := range byKey {
		sort.Strings(g.have)
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].container != out[j].container {
			return out[i].container < out[j].container
		}
		return out[i].src < out[j].src
	})
	return out, nil
}

// UpdateEndpointRules inserts the missing and then deletes the stale endpoint rules of every armed container.
func UpdateEndpointRules(ctx context.Context, env *Env) *Failure {
	wanted := append([]string(nil), env.Allowed.ModelIPs...)
	sort.Strings(wanted)

	groups, err := endpointGroups(ctx, env)
	if err != nil {
		return fail(ExitRuntime, StateNotRefreshed, "rules of "+config.ChainDockerUser+" unreadable", err)
	}
	if len(groups) == 0 {
		env.Log.Success("no armed container has endpoint rules; nothing to update")
		return nil
	}

	verbPlus, verbMinus := "added", "removed"
	if env.DryRun {
		verbPlus, verbMinus = "would add", "would remove"
	}
	for _, g := range groups {
		tag := config.CommentPrefix + g.container
		spec := func(ip string) []string { return acceptSpec(tag, g.src+"/32", ip, "tcp", config.ModelAPIPort) }
		var add, stale []string
		for _, ip := range wanted {
			if !contains(g.have, ip) {
				add = append(add, ip)
			}
		}
		for _, ip := range g.have {
			if !contains(wanted, ip) {
				stale = append(stale, ip)
			}
		}
		env.Log.Success(fmt.Sprintf("%s (%s): %s %d, %s %d, unchanged %d", g.container, g.src,
			verbPlus, len(add), verbMinus, len(stale), len(g.have)-len(stale)))
		for _, ip := range add {
			env.Log.Success("  + " + ip)
		}
		for _, ip := range stale {
			env.Log.Success("  - " + ip)
		}
		if env.DryRun {
			continue
		}
		for _, ip := range add { // insert first: the container keeps every address it still needs
			if err := env.Net.IptInsert(ctx, config.ChainDockerUser, spec(ip)...); err != nil {
				return fail(ExitRuntime, StateRefreshIncomplete, fmt.Sprintf("%s: could not add the rule for %s", g.container, ip), err)
			}
		}
		for _, ip := range stale {
			if err := env.Net.IptDelete(ctx, config.ChainDockerUser, spec(ip)...); err != nil {
				return fail(ExitRuntime, StateRefreshIncomplete, fmt.Sprintf("%s: could not delete the rule for %s", g.container, ip), err)
			}
		}
	}
	return nil
}
