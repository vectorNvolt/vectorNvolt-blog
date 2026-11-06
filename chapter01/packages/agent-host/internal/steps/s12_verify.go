// SFR-HOST-009, SSR-HOST-001..007 (USC-001 step 12): verify every acceptance criterion; collect all failures.
package steps

import (
	"context"
	"encoding/json"
	"fmt"
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

// as runs cmd inside agent-host as agent-operator and returns trimmed stdout.
func as(ctx context.Context, env *Env, cmd ...string) (string, error) {
	out, err := env.WSL.Exec(ctx, config.User, nil, cmd...)
	return strings.TrimSpace(string(out)), err
}

func sh(ctx context.Context, env *Env, script string) error {
	_, err := as(ctx, env, "/bin/sh", "-c", script)
	return err
}

func checks() []check {
	list := []check{
		{"distro registered as WSL version 2", func(ctx context.Context, env *Env) error {
			distros, err := env.WSL.List(ctx)
			if err != nil {
				return err
			}
			if v, ok := distros[config.DistroName]; !ok || v != 2 {
				return fmt.Errorf("%s listed with version %d (found=%v), want 2", config.DistroName, v, ok)
			}
			return nil
		}},
	}
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
	list = append(list, isolationChecks()...)
	return append(list,
		check{"docker runs a gVisor (runsc) container", func(ctx context.Context, env *Env) error {
			// Runs as root: agent-operator is not in the docker group.
			_, err := env.WSL.Exec(ctx, "root", nil, "docker", "run", "--rm", "--runtime=runsc", "hello-world")
			return err
		}},
		check{"no host drive: /mnt/c absent", func(ctx context.Context, env *Env) error {
			return sh(ctx, env, "test ! -e /mnt/c")
		}},
		check{"no host drive mount (drvfs/9p) in /proc/mounts", func(ctx context.Context, env *Env) error {
			return sh(ctx, env, `awk '$3 ~ /^drvfs$/ || ($3 ~ /^9p$/ && ($2 ~ /^\/mnt\// || $1 ~ /^[A-Za-z]:/)) {f=1} END{exit f}' /proc/mounts`)
		}},
		check{"no host program: powershell.exe not runnable", func(ctx context.Context, env *Env) error {
			return sh(ctx, env, "! command -v powershell.exe")
		}},
		check{"no interop: WSLInterop binfmt not registered", func(ctx context.Context, env *Env) error {
			return sh(ctx, env, "test ! -e /proc/sys/fs/binfmt_misc/WSLInterop && test ! -e /proc/sys/fs/binfmt_misc/WSLInterop-late")
		}},
		check{"no host path: PATH has no /mnt/ entries", func(ctx context.Context, env *Env) error {
			path, err := as(ctx, env, "printenv", "PATH")
			if err != nil {
				return err
			}
			for _, p := range strings.Split(path, ":") {
				if strings.HasPrefix(p, "/mnt/") {
					return fmt.Errorf("PATH contains %q", p)
				}
			}
			return nil
		}},
		check{"systemd reports running", func(ctx context.Context, env *Env) error {
			// is-system-running exits non-zero for "degraded", so the state is read before the error.
			out, err := as(ctx, env, "/usr/bin/systemctl", "is-system-running")
			if out == "degraded" {
				return acceptKnownDegraded(ctx, env)
			}
			if err != nil {
				return fmt.Errorf("state %q: %w", out, err)
			}
			if out != "running" {
				return fmt.Errorf("state %q, want \"running\"", out)
			}
			return nil
		}},
		check{"default user is " + config.User, func(ctx context.Context, env *Env) error {
			out, err := env.WSL.ExecDefaultUser(ctx, nil, "id", "-un")
			if err != nil {
				return err
			}
			if got := strings.TrimSpace(string(out)); got != config.User {
				return fmt.Errorf("default user is %q", got)
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

// knownWSLFailedUnit is root's user manager, which WSL fails to start ("Device or resource busy"); tolerated as the only failed unit.
const knownWSLFailedUnit = "user@0.service"

// acceptKnownDegraded passes a degraded systemd only when the failed units are exactly {knownWSLFailedUnit}.
func acceptKnownDegraded(ctx context.Context, env *Env) error {
	out, err := as(ctx, env, "/usr/bin/systemctl", "list-units", "--state=failed", "--no-legend", "--plain")
	if err != nil {
		return fmt.Errorf("state \"degraded\"; listing failed units: %w", err)
	}
	seen := map[string]bool{}
	var units []string
	for _, line := range strings.Split(out, "\n") {
		if f := strings.Fields(line); len(f) > 0 && !seen[f[0]] {
			seen[f[0]] = true
			units = append(units, f[0])
		}
	}
	if len(units) == 1 && units[0] == knownWSLFailedUnit {
		env.Log.Warnf("systemd degraded only by %s (known WSL issue); accepted", knownWSLFailedUnit)
		return nil
	}
	return fmt.Errorf("state \"degraded\", failed units: %s", strings.Join(units, ", "))
}

var forbiddenGroups = []string{"sudo", "admin", "wheel", "adm", "disk", "lxd", "libvirt", "docker"}

// rootExec runs cmd inside agent-host as root and returns trimmed stdout.
func rootExec(ctx context.Context, env *Env, cmd ...string) (string, error) {
	out, err := env.WSL.Exec(ctx, "root", nil, cmd...)
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

// configChecks: runsc definition and ownership of the two configuration files, as agent-operator.
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
	for _, p := range []string{config.WSLConfPath, config.DaemonJSONPath, config.ResolvConfPath, config.HostsPath} {
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

// contentCheck compares the trimmed content of path, read as agent-operator, with the trimmed expected content.
func contentCheck(name, path, want string) check {
	return check{name, func(ctx context.Context, env *Env) error {
		out, err := as(ctx, env, "cat", path)
		if err != nil {
			return err
		}
		if out != strings.TrimSpace(want) {
			return fmt.Errorf("content differs from the expected content")
		}
		return nil
	}}
}

// isolationChecks: wsl.conf content, resolv.conf and hosts, hostname, WSL-injected mounts, GPU device and the
// isolate script, service and drop-in, all as agent-operator.
func isolationChecks() []check {
	list := []check{
		contentCheck(config.WSLConfPath+" has the expected content", config.WSLConfPath, WSLConf),
		{config.ResolvConfPath + " is a regular file, not a symlink", func(ctx context.Context, env *Env) error {
			return sh(ctx, env, "test -f "+config.ResolvConfPath+" && test ! -L "+config.ResolvConfPath)
		}},
		contentCheck(config.ResolvConfPath+" has the expected content", config.ResolvConfPath, ResolvConf),
		contentCheck(config.HostsPath+" has the expected content", config.HostsPath, Hosts),
		{"hostname is " + config.DistroName, func(ctx context.Context, env *Env) error {
			out, err := as(ctx, env, "hostname")
			if err != nil {
				return err
			}
			if out != config.DistroName {
				return fmt.Errorf("hostname is %q, want %q", out, config.DistroName)
			}
			return nil
		}},
		{"no GPU device access: /dev/dxg absent or masked by /dev/null", func(ctx context.Context, env *Env) error {
			return sh(ctx, env, `test ! -e /dev/dxg || test "$(stat -c %t:%T /dev/dxg)" = "1:3"`)
		}},
		{"no GPU libraries: /usr/lib/wsl/lib and /usr/lib/wsl/drivers empty", func(ctx context.Context, env *Env) error {
			return sh(ctx, env, `test -z "$(find /usr/lib/wsl/lib /usr/lib/wsl/drivers -mindepth 1 -print -quit 2>/dev/null)"`)
		}},
	}
	for _, m := range IsolatedMounts {
		m := m
		list = append(list, check{m + " is not a mountpoint", func(ctx context.Context, env *Env) error {
			return sh(ctx, env, "! /usr/bin/mountpoint -q "+m)
		}})
	}
	for _, f := range IsolationFiles {
		f := f
		list = append(list,
			contentCheck(f.Path+" has the expected content", f.Path, f.Content),
			check{f.Path + " owned root:root with mode " + f.Mode, func(ctx context.Context, env *Env) error {
				out, err := as(ctx, env, "stat", "-c", "%U:%G:%a", f.Path)
				if err != nil {
					return err
				}
				if want := "root:root:" + f.Mode; out != want {
					return fmt.Errorf("owner:group:mode is %q, want %q", out, want)
				}
				return nil
			}},
		)
	}
	return append(list,
		check{IsolateUnitName + " is enabled", func(ctx context.Context, env *Env) error {
			out, err := as(ctx, env, systemctl, "is-enabled", IsolateUnitName)
			if err != nil {
				return fmt.Errorf("state %q: %w", out, err)
			}
			if out != "enabled" {
				return fmt.Errorf("state %q, want \"enabled\"", out)
			}
			return nil
		}},
		check{IsolateUnitName + " is active (exited)", func(ctx context.Context, env *Env) error {
			out, err := as(ctx, env, systemctl, "show", IsolateUnitName, "-p", "ActiveState", "-p", "SubState")
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

func Verify(ctx context.Context, env *Env) *Failure {
	var failed []string
	list := checks()
	if env.Config.KeepAlive {
		list = append(list, check{"keep-alive Startup script has the expected content", func(_ context.Context, env *Env) error {
			return VerifyKeepAliveFile(env.Config.AppData)
		}})
	}
	for _, c := range list {
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
