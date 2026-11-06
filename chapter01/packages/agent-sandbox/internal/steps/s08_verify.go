// SFR-AGC-009, SFR-NET-020 (USC-002 step 9, err-12): read the actual container state and compare; list every mismatch.
// Any mismatch or unreadable state contains the container (re-pause, verify paused) and never removes it.
package steps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"agent-sandbox/internal/config"
	"agent-sandbox/internal/services"
)

var (
	idUIDRe = regexp.MustCompile(`\buid=(\d+)`)
	idGIDRe = regexp.MustCompile(`\bgid=(\d+)`)
)

func Verify(ctx context.Context, env *Env) *Failure {
	info, err := env.Docker.Inspect(ctx, env.Container)
	if err != nil {
		return verifyFailure(ctx, env, []string{fmt.Sprintf("inspect %s unreadable: %v", env.Container, err)})
	}

	bad := inspectMismatches(env, info)
	bad = append(bad, workspaceMismatches(env)...)
	bad = append(bad, agentUserMismatches(ctx, env)...)
	bad = append(bad, resolvConfMismatches(ctx, env)...)

	if len(bad) > 0 {
		return verifyFailure(ctx, env, bad)
	}
	return nil
}

// verifyFailure contains the container and reports every mismatch (CONTAINED or UNSAFE, exit 2).
func verifyFailure(ctx context.Context, env *Env, bad []string) *Failure {
	state, note := containFailure(ctx, env)
	return &Failure{
		ExitCode:  ExitRuntime,
		ExitState: state,
		Message:   fmt.Sprintf("container %s does not match the requested configuration\n%s", env.Container, note),
		Checks:    bad,
	}
}

func inspectMismatches(env *Env, info services.ContainerInfo) []string {
	var bad []string
	if !info.State.Running {
		bad = append(bad, "State.Running is false, want true")
	}
	if info.State.Paused {
		bad = append(bad, "State.Paused is true, want false")
	}
	if info.HostConfig.Runtime != config.Runtime {
		bad = append(bad, fmt.Sprintf("HostConfig.Runtime is %q, want %q", info.HostConfig.Runtime, config.Runtime))
	}
	if u := info.Config.User; u != "" {
		bad = append(bad, fmt.Sprintf("Config.User is %q, want empty", u))
	}
	if !reflect.DeepEqual(info.HostConfig.CapDrop, []string{"ALL"}) {
		bad = append(bad, fmt.Sprintf("HostConfig.CapDrop is %v, want [ALL]", info.HostConfig.CapDrop))
	}
	if got, want := normCaps(info.HostConfig.CapAdd), normCaps(config.Capabilities); !reflect.DeepEqual(got, want) {
		bad = append(bad, fmt.Sprintf("HostConfig.CapAdd is %v, want exactly %v", got, want))
	}
	if !hasNoNewPrivileges(info.HostConfig.SecurityOpt) {
		bad = append(bad, fmt.Sprintf("HostConfig.SecurityOpt is %v, want no-new-privileges", info.HostConfig.SecurityOpt))
	}
	if m := info.HostConfig.Memory; m != config.MemoryBytes {
		bad = append(bad, fmt.Sprintf("HostConfig.Memory is %d, want %d", m, int64(config.MemoryBytes)))
	}
	if n := info.HostConfig.NanoCpus; n != config.NanoCPUs {
		bad = append(bad, fmt.Sprintf("HostConfig.NanoCpus is %d, want %d", n, int64(config.NanoCPUs)))
	}
	if p := info.HostConfig.PidsLimit; p != config.PidsLimitNumber {
		bad = append(bad, fmt.Sprintf("HostConfig.PidsLimit is %d, want %d", p, int64(config.PidsLimitNumber)))
	}
	if p := info.HostConfig.RestartPolicy.Name; p != "no" {
		bad = append(bad, fmt.Sprintf("HostConfig.RestartPolicy.Name is %q, want \"no\"", p))
	}

	wantTmpfs := map[string]string{
		config.TmpfsTmpPath:    config.TmpfsTmpOptions,
		config.TmpfsDockerPath: config.TmpfsDockerOptions,
	}
	if len(info.HostConfig.Tmpfs) != len(wantTmpfs) || !reflect.DeepEqual(info.HostConfig.Tmpfs, wantTmpfs) {
		bad = append(bad, fmt.Sprintf("HostConfig.Tmpfs is %v, want %v", info.HostConfig.Tmpfs, wantTmpfs))
	}

	// Exactly three bind mounts: the workspace, the per-agent Claude state (both rw) and the resolv.conf (ro).
	type wantMount struct {
		source string
		rw     bool
	}
	wantMounts := map[string]wantMount{
		config.WorkspaceMount: {env.Workspace, true},
		config.ClaudeMount:    {env.ClaudeDir, true},
		config.ResolvConfPath: {env.Config.HostResolv, false},
	}
	seen := map[string]bool{}
	for _, m := range info.Mounts {
		w, ok := wantMounts[m.Destination]
		if !ok {
			bad = append(bad, fmt.Sprintf("unexpected mount at %s (source %q)", m.Destination, m.Source))
			continue
		}
		seen[m.Destination] = true
		if m.Type != "bind" {
			bad = append(bad, fmt.Sprintf("mount %s has type %q, want \"bind\"", m.Destination, m.Type))
		}
		if m.Source != w.source {
			bad = append(bad, fmt.Sprintf("mount %s has source %q, want %q", m.Destination, m.Source, w.source))
		}
		if m.RW != w.rw {
			if w.rw {
				bad = append(bad, fmt.Sprintf("mount %s is not read-write", m.Destination))
			} else {
				bad = append(bad, fmt.Sprintf("mount %s is not read-only", m.Destination))
			}
		}
	}
	for _, dest := range []string{config.WorkspaceMount, config.ClaudeMount, config.ResolvConfPath} {
		if !seen[dest] {
			bad = append(bad, fmt.Sprintf("no mount at %s", dest))
		}
	}

	wantEnv := config.ClaudeEnvVar + "=" + config.ClaudeMount
	hasEnv := false
	for _, e := range info.Config.Env {
		if e == wantEnv {
			hasEnv = true
		}
	}
	if !hasEnv {
		bad = append(bad, fmt.Sprintf("Config.Env lacks %s", wantEnv))
	}

	var nets []string
	for n := range info.NetworkSettings.Networks {
		nets = append(nets, n)
	}
	sort.Strings(nets)
	if !reflect.DeepEqual(nets, []string{config.NetworkName}) {
		bad = append(bad, fmt.Sprintf("NetworkSettings.Networks is %v, want exactly [%s]", nets, config.NetworkName))
	}
	return bad
}

// normCaps strips an optional CAP_ prefix and sorts, so the sets compare by name.
func normCaps(in []string) []string {
	out := make([]string, 0, len(in))
	for _, c := range in {
		out = append(out, strings.TrimPrefix(strings.ToUpper(c), "CAP_"))
	}
	sort.Strings(out)
	return out
}

func hasNoNewPrivileges(opts []string) bool {
	for _, o := range opts {
		switch o {
		case "no-new-privileges", "no-new-privileges=true", "no-new-privileges:true":
			return true
		}
	}
	return false
}

// workspaceMismatches checks the workspace folder and its .output exist and are owned by agent-operator.
func workspaceMismatches(env *Env) []string {
	var bad []string
	for _, d := range []string{env.Workspace, filepath.Join(env.Workspace, config.OutputDir)} {
		fi, err := env.FS.Lstat(d)
		if os.IsNotExist(err) {
			bad = append(bad, fmt.Sprintf("folder %s does not exist", d))
			continue
		}
		if err != nil {
			bad = append(bad, fmt.Sprintf("folder %s cannot be inspected: %v", d, err))
			continue
		}
		if !fi.IsDir() {
			bad = append(bad, fmt.Sprintf("%s is not a directory", d))
		}
		uid, _, err := env.FS.Owner(d)
		if err != nil {
			bad = append(bad, fmt.Sprintf("owner of %s cannot be read: %v", d, err))
		} else if uid != config.WorkspaceUID {
			bad = append(bad, fmt.Sprintf("%s is owned by uid %d, want %d", d, uid, config.WorkspaceUID))
		}
	}
	return bad
}

// agentUserMismatches runs `id` and `id -nG` as uid 1001 (SSR-AGC-011).
func agentUserMismatches(ctx context.Context, env *Env) []string {
	var bad []string
	out, err := env.Docker.Exec(ctx, env.Container, config.AgentUser, "id")
	if err != nil {
		bad = append(bad, fmt.Sprintf("exec as %s: id failed: %v", config.AgentUser, err))
	} else {
		s := string(out)
		if m := idUIDRe.FindStringSubmatch(s); m == nil || m[1] != fmt.Sprint(config.AgentUID) {
			bad = append(bad, fmt.Sprintf("id as %s shows %q, want uid %d", config.AgentUser, strings.TrimSpace(s), config.AgentUID))
		}
		if m := idGIDRe.FindStringSubmatch(s); m == nil || m[1] != fmt.Sprint(config.AgentGID) {
			bad = append(bad, fmt.Sprintf("id as %s shows %q, want gid %d", config.AgentUser, strings.TrimSpace(s), config.AgentGID))
		}
	}
	out, err = env.Docker.Exec(ctx, env.Container, config.AgentUser, "id", "-nG")
	if err != nil {
		bad = append(bad, fmt.Sprintf("exec as %s: id -nG failed: %v", config.AgentUser, err))
	} else {
		found := false
		for _, g := range strings.Fields(string(out)) {
			if g == config.AgentGroup {
				found = true
			}
		}
		if !found {
			bad = append(bad, fmt.Sprintf("groups of %s are %q, want group %s among them", config.AgentUser, strings.TrimSpace(string(out)), config.AgentGroup))
		}
	}
	return bad
}
