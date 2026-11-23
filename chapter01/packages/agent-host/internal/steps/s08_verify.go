// SFR-HOST-009, SSR-HOST-001..007 (USC-001 step 8): verify every acceptance criterion; collect all failures.
package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"reflect"
	"strconv"
	"strings"

	"agent-host/internal/config"
)

// DependencyPackages are the thirteen packages installed by install_deps.sh (SFR-HOST-008, SSR-HOST-004).
var DependencyPackages = []string{
	"ca-certificates", "curl", "gnupg", "docker.io", "docker-buildx", "runsc", "python3",
	"gnome-keyring", "git", "tar", "nftables", "iptables", "iproute2",
}

type check struct {
	name string
	run  func(ctx context.Context, env *Env) error
}

// as runs cmd as agent-operator and returns trimmed stdout.
func as(ctx context.Context, env *Env, cmd ...string) (string, error) {
	out, err := env.Host.ExecAs(ctx, config.User, nil, cmd...)
	return strings.TrimSpace(string(out)), err
}

func sh(ctx context.Context, env *Env, script string) error {
	_, err := as(ctx, env, "/bin/sh", "-c", script)
	return err
}

func checks() []check {
	var list []check
	for _, tool := range []string{"docker", "python3", "gnome-keyring-daemon", "git", "tar"} {
		tool := tool
		list = append(list, check{"tool available: " + tool, func(ctx context.Context, env *Env) error {
			return sh(ctx, env, "command -v "+tool)
		}})
	}
	for _, pkg := range DependencyPackages {
		list = append(list, rootCheck("dependency package installed: "+pkg, "dpkg", "-s", pkg))
	}
	list = append(list, rootChecks()...)
	list = append(list, configChecks()...)
	list = append(list, toolChecks()...)
	list = append(list, unitChecks()...)
	list = append(list, guardChecks()...)
	list = append(list, hostnameChecks()...)
	list = append(list, upgradeChecks()...)
	list = append(list, bridgeChecks()...)
	list = append(list, mountChecks()...)
	return append(list,
		check{"docker runs a gVisor (runsc) container", func(ctx context.Context, env *Env) error {
			// Runs as root: agent-operator is not in the docker group.
			_, err := env.Host.Exec(ctx, nil, "docker", "run", "--rm", "--runtime=runsc", "hello-world")
			return err
		}},
		check{"systemd reports running", func(ctx context.Context, env *Env) error {
			// is-system-running exits non-zero for "degraded", so the state is read before the error.
			out, err := as(ctx, env, "/usr/bin/systemctl", "is-system-running")
			if err != nil {
				return fmt.Errorf("state %q: %w", out, err)
			}
			if out != "running" {
				return fmt.Errorf("state %q, want \"running\"", out)
			}
			return nil
		}},
		check{config.User + " has UID and GID 1001", func(ctx context.Context, env *Env) error {
			want := strconv.Itoa(config.UserID)
			for _, flag := range []string{"-u", "-g"} {
				out, err := as(ctx, env, "id", flag)
				if err != nil {
					return err
				}
				if out != want {
					return fmt.Errorf("id %s is %q, want %q", flag, out, want)
				}
			}
			return nil
		}},
		check{config.User + " has no root-equivalent group membership", func(ctx context.Context, env *Env) error {
			out, err := as(ctx, env, "id", "-nG")
			if err != nil {
				return err
			}
			var bad []string
			for _, g := range strings.Fields(out) {
				for _, f := range forbiddenGroups {
					if g == f {
						bad = append(bad, g)
					}
				}
			}
			if len(bad) > 0 {
				return fmt.Errorf("member of group(s) %s", strings.Join(bad, ", "))
			}
			return nil
		}},
		check{config.User + " has no sudoers entry: sudo -n true fails", func(ctx context.Context, env *Env) error {
			// A missing sudo binary also passes: without sudo there is no sudoers entry to use.
			if err := sh(ctx, env, "! sudo -n true"); err != nil {
				return fmt.Errorf("sudo -n true succeeded or could not be checked: %w", err)
			}
			return nil
		}},
	)
}

var forbiddenGroups = []string{"sudo", "admin", "wheel", "adm", "disk", "lxd", "libvirt", "docker"}

// rootExec runs cmd as root and returns trimmed stdout.
func rootExec(ctx context.Context, env *Env, cmd ...string) (string, error) {
	out, err := env.Host.Exec(ctx, nil, cmd...)
	return strings.TrimSpace(string(out)), err
}

func rootCheck(name string, cmd ...string) check {
	return check{name, func(ctx context.Context, env *Env) error {
		_, err := rootExec(ctx, env, cmd...)
		return err
	}}
}

// rootChecks: SFR-HOST-009 / SSR-HOST-004 / SSR-HOST-006 checks run as root (only root reaches the engine socket).
func rootChecks() []check {
	return []check{
		{"docker reports runsc among its runtimes (as root)", func(ctx context.Context, env *Env) error {
			out, err := rootExec(ctx, env, "docker", "info", "--format", "{{json .Runtimes}}")
			if err != nil {
				return err
			}
			var rts map[string]json.RawMessage
			if err := json.Unmarshal([]byte(out), &rts); err != nil {
				return fmt.Errorf("cannot parse runtimes %q: %w", out, err)
			}
			if _, ok := rts["runsc"]; !ok {
				return fmt.Errorf("runsc not among runtimes: %s", out)
			}
			return nil
		}},
		rootCheck("docker buildx version works (as root)", "/usr/bin/docker", "buildx", "version"),
		rootCheck("nft --version works (as root)", "/usr/sbin/nft", "--version"),
		rootCheck("iptables --version works (as root)", "/usr/sbin/iptables", "--version"),
		rootCheck("ip -V works (iproute2, as root)", "ip", "-V"),
		rootCheck("bridge -V works (iproute2, as root)", "/usr/sbin/bridge", "-V"),
		rootCheck("ca-certificates available: CA bundle present (as root)", "test", "-s", "/etc/ssl/certs/ca-certificates.crt"),
		rootCheck("curl available (as root)", "curl", "--version"),
		rootCheck("gnupg available: gpg --version (as root)", "gpg", "--version"),
		rootCheck("runsc available: "+config.RunscPath+" --version (as root)", config.RunscPath, "--version"),
	}
}

// runscEntry reads the runsc runtime entry from daemon.json as agent-operator.
func runscEntry(ctx context.Context, env *Env) (path string, args []string, err error) {
	out, err := as(ctx, env, "cat", config.DaemonJSONPath)
	if err != nil {
		return "", nil, err
	}
	var doc struct {
		Runtimes map[string]struct {
			Path        string   `json:"path"`
			RuntimeArgs []string `json:"runtimeArgs"`
		} `json:"runtimes"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		return "", nil, fmt.Errorf("%s is not valid JSON: %w", config.DaemonJSONPath, err)
	}
	rt, ok := doc.Runtimes["runsc"]
	if !ok {
		return "", nil, fmt.Errorf("%s has no runtime entry runsc", config.DaemonJSONPath)
	}
	return rt.Path, rt.RuntimeArgs, nil
}

// configChecks: runsc definition and ownership of daemon.json, as agent-operator.
func configChecks() []check {
	list := []check{
		{"runsc runtime path is " + config.RunscPath, func(ctx context.Context, env *Env) error {
			p, _, err := runscEntry(ctx, env)
			if err != nil {
				return err
			}
			if p != config.RunscPath {
				return fmt.Errorf("runsc path is %q, want %q", p, config.RunscPath)
			}
			return nil
		}},
		{"runsc runtimeArgs are the eight listed arguments in order", func(ctx context.Context, env *Env) error {
			_, args, err := runscEntry(ctx, env)
			if err != nil {
				return err
			}
			if !reflect.DeepEqual(args, config.RunscRuntimeArgs) {
				return fmt.Errorf("runtimeArgs are %q, want %q", args, config.RunscRuntimeArgs)
			}
			return nil
		}},
	}
	for _, p := range []string{config.DaemonJSONPath} {
		p := p
		list = append(list,
			check{p + " owned root:root", func(ctx context.Context, env *Env) error {
				out, err := as(ctx, env, "stat", "-c", "%U:%G", p)
				if err != nil {
					return err
				}
				if out != "root:root" {
					return fmt.Errorf("owner is %q, want \"root:root\"", out)
				}
				return nil
			}},
			check{p + " not writable by " + config.User, func(ctx context.Context, env *Env) error {
				return sh(ctx, env, "test ! -w "+p)
			}},
		)
	}
	return list
}

// toolChecks: SSR-HOST-007, all as agent-operator.
func toolChecks() []check {
	var list []check
	for _, name := range config.ToolNames {
		name := name
		p := config.ToolInstallDir + "/" + name
		list = append(list,
			check{p + " owned root:root with mode 755", func(ctx context.Context, env *Env) error {
				out, err := as(ctx, env, "stat", "-c", "%U:%G:%a", p)
				if err != nil {
					return err
				}
				if out != "root:root:755" {
					return fmt.Errorf("owner:group:mode is %q, want \"root:root:755\"", out)
				}
				return nil
			}},
			check{p + " matches its reference SHA-256", func(ctx context.Context, env *Env) error {
				want, ok := env.ToolHashes[name]
				if !ok {
					return fmt.Errorf("no reference hash for %s", name)
				}
				out, err := as(ctx, env, "/usr/bin/sha256sum", p)
				if err != nil {
					return err
				}
				got := ""
				if f := strings.Fields(out); len(f) > 0 {
					got = strings.ToLower(f[0])
				}
				if got != strings.ToLower(want) {
					return fmt.Errorf("%s: installed sha256 %s, reference sha256 %s", name, got, want)
				}
				return nil
			}},
		)
	}
	return append(list,
		check{config.ToolInstallDir + " owned by root", func(ctx context.Context, env *Env) error {
			out, err := as(ctx, env, "stat", "-c", "%U", config.ToolInstallDir)
			if err != nil {
				return err
			}
			if out != "root" {
				return fmt.Errorf("owner is %q, want \"root\"", out)
			}
			return nil
		}},
		check{config.ToolInstallDir + " not writable by " + config.User, func(ctx context.Context, env *Env) error {
			return sh(ctx, env, "test ! -w "+config.ToolInstallDir)
		}},
	)
}

// unitChecks: agent-restore / agent-guard unit files and service states, all as agent-operator.
func unitChecks() []check {
	var list []check
	for _, u := range LifecycleUnits {
		u := u
		list = append(list,
			check{u.Path + " has the expected content", func(ctx context.Context, env *Env) error {
				out, err := as(ctx, env, "cat", u.Path)
				if err != nil {
					return err
				}
				if out != strings.TrimSpace(u.Content) {
					return fmt.Errorf("content differs from the expected unit")
				}
				return nil
			}},
			check{u.Path + " owned root:root with mode 644", func(ctx context.Context, env *Env) error {
				out, err := as(ctx, env, "stat", "-c", "%U:%G:%a", u.Path)
				if err != nil {
					return err
				}
				if out != "root:root:644" {
					return fmt.Errorf("owner:group:mode is %q, want \"root:root:644\"", out)
				}
				return nil
			}},
		)
	}
	for _, unit := range LifecycleEnabledUnits {
		unit := unit
		list = append(list, check{unit + " is enabled", func(ctx context.Context, env *Env) error {
			out, err := as(ctx, env, systemctl, "is-enabled", unit)
			if err != nil {
				return fmt.Errorf("state %q: %w", out, err)
			}
			if out != "enabled" {
				return fmt.Errorf("state %q, want \"enabled\"", out)
			}
			return nil
		}})
	}
	return append(list,
		check{"agent-guard.service is active", func(ctx context.Context, env *Env) error {
			out, err := as(ctx, env, systemctl, "is-active", "agent-guard.service")
			if err != nil {
				return fmt.Errorf("state %q: %w", out, err)
			}
			if out != "active" {
				return fmt.Errorf("state %q, want \"active\"", out)
			}
			return nil
		}},
		check{"agent-restore.service is active (exited)", func(ctx context.Context, env *Env) error {
			out, err := as(ctx, env, systemctl, "show", "agent-restore.service", "-p", "ActiveState", "-p", "SubState")
			if err != nil {
				return err
			}
			state := map[string]string{}
			for _, line := range strings.Split(out, "\n") {
				if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
					state[k] = v
				}
			}
			if state["ActiveState"] != "active" || state["SubState"] != "exited" {
				return fmt.Errorf("ActiveState=%q SubState=%q, want active/exited", state["ActiveState"], state["SubState"])
			}
			return nil
		}},
	)
}

// guardReadyPath is the marker written by `agent_lifecycle restore` (dir 0755, file 0644: readable by agent-operator).
const guardReadyPath = "/run/agent-sandbox/ready"

// guardShow reads the agent-guard.service properties as agent-operator into a key -> value map.
func guardShow(ctx context.Context, env *Env) (map[string]string, error) {
	out, err := as(ctx, env, systemctl, "show", "-p", "NRestarts", "-p", "Restart", "-p", "MainPID", "-p", "ExecStart", "agent-guard.service")
	if err != nil {
		return nil, err
	}
	props := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			props[k] = v
		}
	}
	return props, nil
}

// isLifecycleGuard reports whether argv is `<...>/agent_lifecycle guard`.
func isLifecycleGuard(argv []string) bool {
	return len(argv) >= 2 && path.Base(argv[0]) == "agent_lifecycle" && argv[1] == "guard"
}

// guardChecks: agent-guard.service ran without restart, its unit restarts always and runs agent_lifecycle guard,
// and the ready marker exists, all as agent-operator.
func guardChecks() []check {
	return []check{
		{"agent-guard.service has run without restart (NRestarts=0)", func(ctx context.Context, env *Env) error {
			props, err := guardShow(ctx, env)
			if err != nil {
				return err
			}
			if props["NRestarts"] != "0" {
				return fmt.Errorf("NRestarts=%q, want \"0\"", props["NRestarts"])
			}
			return nil
		}},
		{"agent-guard.service main process is agent_lifecycle guard", func(ctx context.Context, env *Env) error {
			props, err := guardShow(ctx, env)
			if err != nil {
				return err
			}
			pid := props["MainPID"]
			if n, convErr := strconv.Atoi(pid); convErr != nil || n <= 0 {
				return fmt.Errorf("MainPID=%q, want a running process", pid)
			}
			out, err := as(ctx, env, "cat", "/proc/"+pid+"/cmdline")
			if err != nil {
				return err
			}
			argv := strings.Split(strings.Trim(out, "\x00"), "\x00")
			if !isLifecycleGuard(argv) {
				return fmt.Errorf("process %s runs %q, want agent_lifecycle guard", pid, strings.Join(argv, " "))
			}
			return nil
		}},
		{"agent-guard.service unit has Restart=always", func(ctx context.Context, env *Env) error {
			props, err := guardShow(ctx, env)
			if err != nil {
				return err
			}
			if props["Restart"] != "always" {
				return fmt.Errorf("Restart=%q, want \"always\"", props["Restart"])
			}
			return nil
		}},
		{"agent-guard.service unit ExecStart runs agent_lifecycle guard", func(ctx context.Context, env *Env) error {
			props, err := guardShow(ctx, env)
			if err != nil {
				return err
			}
			// systemd renders: { path=... ; argv[]=/usr/local/bin/agent_lifecycle guard ; ignore_errors=no ; ... }
			v := props["ExecStart"]
			_, rest, found := strings.Cut(v, "argv[]=")
			if !found {
				return fmt.Errorf("ExecStart=%q has no argv", v)
			}
			argvText, _, _ := strings.Cut(rest, ";")
			if argv := strings.Fields(argvText); !isLifecycleGuard(argv) {
				return fmt.Errorf("ExecStart argv is %q, want agent_lifecycle guard", strings.Join(argv, " "))
			}
			return nil
		}},
		{guardReadyPath + " exists (written by agent_lifecycle restore)", func(ctx context.Context, env *Env) error {
			_, err := as(ctx, env, "test", "-f", guardReadyPath)
			return err
		}},
	}
}

// hostnameChecks: the hostname and the cloud-init drop-in that keeps it, as agent-operator.
func hostnameChecks() []check {
	return []check{
		{"hostname is " + config.HostName, func(ctx context.Context, env *Env) error {
			out, err := as(ctx, env, "hostname")
			if err != nil {
				return err
			}
			if out != config.HostName {
				return fmt.Errorf("hostname is %q, want %q", out, config.HostName)
			}
			return nil
		}},
		{config.PreserveHostnameCfgPath + " has the expected content", func(ctx context.Context, env *Env) error {
			out, err := as(ctx, env, "cat", config.PreserveHostnameCfgPath)
			if err != nil {
				return err
			}
			if out != strings.TrimSpace(PreserveHostnameCfg) {
				return fmt.Errorf("content differs from the expected content")
			}
			return nil
		}},
	}
}

// upgradeChecks: the unattended-upgrades blacklist for Docker and its runtime, as agent-operator.
func upgradeChecks() []check {
	p := config.NoDockerUpgradeCfgPath
	return []check{
		{p + " has the expected content", func(ctx context.Context, env *Env) error {
			out, err := as(ctx, env, "cat", p)
			if err != nil {
				return err
			}
			if out != strings.TrimSpace(NoDockerUpgradeCfg) {
				return fmt.Errorf("content differs from the expected content")
			}
			return nil
		}},
		{p + " owned root:root with mode 644", func(ctx context.Context, env *Env) error {
			out, err := as(ctx, env, "stat", "-c", "%U:%G:%a", p)
			if err != nil {
				return err
			}
			if out != "root:root:644" {
				return fmt.Errorf("owner:group:mode is %q, want \"root:root:644\"", out)
			}
			return nil
		}},
	}
}

// bridgeChecks: br_netfilter persisted (modules-load + sysctl drop-ins), loaded, and bridge-nf-call-iptables is 1, as agent-operator.
func bridgeChecks() []check {
	var list []check
	for _, f := range []struct{ path, content string }{
		{config.BrNetfilterModulesPath, config.BrNetfilterModulesContent},
		{config.BridgeSysctlPath, config.BridgeSysctlContent},
	} {
		f := f
		list = append(list,
			check{f.path + " has the expected content", func(ctx context.Context, env *Env) error {
				out, err := as(ctx, env, "cat", f.path)
				if err != nil {
					return err
				}
				if out != strings.TrimSpace(f.content) {
					return fmt.Errorf("content differs from the expected content")
				}
				return nil
			}},
			check{f.path + " owned root:root with mode 644", func(ctx context.Context, env *Env) error {
				out, err := as(ctx, env, "stat", "-c", "%U:%G:%a", f.path)
				if err != nil {
					return err
				}
				if out != "root:root:644" {
					return fmt.Errorf("owner:group:mode is %q, want \"root:root:644\"", out)
				}
				return nil
			}},
		)
	}
	return append(list,
		check{"br_netfilter kernel module is loaded", func(ctx context.Context, env *Env) error {
			_, err := as(ctx, env, "test", "-d", config.BrNetfilterSysModuleDir)
			return err
		}},
		check{config.BridgeNfCallIptablesProc + " is 1", func(ctx context.Context, env *Env) error {
			out, err := as(ctx, env, "cat", config.BridgeNfCallIptablesProc)
			if err != nil {
				return err
			}
			if out != "1" {
				return fmt.Errorf("value is %q, want \"1\"", out)
			}
			return nil
		}},
	)
}

// mountChecks: the VM has no host mounts (read in-process as root).
func mountChecks() []check {
	return []check{
		{"no host mounts (virtiofs, 9p, sshfs)", func(ctx context.Context, env *Env) error {
			mounts, err := HostMounts(env.System.MountsPath)
			if err != nil {
				return err
			}
			if len(mounts) > 0 {
				return fmt.Errorf("host mount(s) present: %s", strings.Join(mounts, ", "))
			}
			return nil
		}},
	}
}

// HomeWriteScript prints every path under the home directory $1 that agent-operator can write (at most 20 lines).
// The home is a positional argument: /bin/sh -c <HomeWriteScript> sh <home>.
const HomeWriteScript = `{ for p in "$1" "$1/.bashrc" "$1/.profile" "$1/.ssh" "$1/.ssh/authorized_keys"; do ` +
	`if [ -e "$p" ] && [ -w "$p" ]; then echo "$p"; fi; done; ` +
	`find "$1" -writable -print 2>/dev/null; } | awk '!seen[$0]++' | head -n 20`

// homeChecks: one check per login user (UID 1000..65533, not agent-operator); the passwd file is read in-process.
func homeChecks(env *Env) []check {
	users, err := loginUsers(env.System.PasswdPath, config.User)
	if err != nil {
		return []check{{"login users read from " + env.System.PasswdPath, func(context.Context, *Env) error { return err }}}
	}
	var list []check
	for _, u := range users {
		u := u
		list = append(list, check{config.User + " cannot write under " + u.Name + "'s home " + u.Home, func(ctx context.Context, env *Env) error {
			out, err := as(ctx, env, "/bin/sh", "-c", HomeWriteScript, "sh", u.Home)
			if out != "" {
				return fmt.Errorf("writable: %s", strings.Join(strings.Split(out, "\n"), ", "))
			}
			return err
		}})
	}
	return list
}

func Verify(ctx context.Context, env *Env) *Failure {
	var failed []string
	for _, c := range append(checks(), homeChecks(env)...) {
		env.Log.Debugf("check: %s", c.name)
		if err := c.run(ctx, env); err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", c.name, err))
		}
	}
	if len(failed) > 0 {
		return &Failure{
			ExitCode:  ExitRuntime,
			ExitState: StateUnverified,
			Message:   fmt.Sprintf("%d acceptance check(s) failed", len(failed)),
			Checks:    failed,
		}
	}
	return nil
}
