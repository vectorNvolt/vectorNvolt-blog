// SFR-HOST-006 (USC-001 step 7): write /etc/wsl.conf (isolation, default user, systemd) via stdin as root:root 644,
// then /etc/hosts as a regular file (root:root 644; wsl.conf stops WSL generating it), then masks
// systemd-resolved (service, two sockets, tmpfiles rule) so it cannot turn resolv.conf into a symlink at boot, then the
// isolation script, service and docker.service drop-in (InstallIsolation). All take effect at the next start (step 8 terminates).
// /etc/resolv.conf is not written here: a file written before the step 8 restart has been found gone afterwards, so step 9
// (InstallDeps) writes it after the restart.
package steps

import (
	"context"

	"agent-host/internal/config"
)

const WSLConf = `# /etc/wsl.conf for agent-host.
[boot]
systemd=true

# No /mnt/c, /mnt/d: Windows drives are not mounted. fstab cannot re-add drvfs mounts.
[automount]
enabled=false
mountFsTab=false

# No Windows executables from Linux, no Windows directories in PATH.
[interop]
enabled=false
appendWindowsPath=false

# /etc/hosts and /etc/resolv.conf are not generated from Windows. Provisioning writes both.
[network]
hostname=agent-host
generateHosts=false
generateResolvConf=false

# Requests no GPU passthrough. /dev/dxg stays (shared kernel device); the isolate service blocks it and unmounts /usr/lib/wsl.
[gpu]
enabled=false

[time]
useWindowsTimezone=false

[user]
default=agent-operator
`

// ResolvConf lists the same fixed resolvers agent-net uses (config.ResolverPrimary / ResolverSecondary there).
const ResolvConf = "nameserver 1.1.1.1\nnameserver 9.9.9.9\n"

// Hosts replaces the file WSL generated at first boot, which carries the Windows hostname.
const Hosts = "127.0.0.1 localhost\n127.0.1.1 agent-host\n::1 localhost ip6-localhost ip6-loopback\n"

func WriteWSLConf(ctx context.Context, env *Env) *Failure {
	if _, err := env.WSL.Exec(ctx, "root", []byte(WSLConf), WriteFileCmd(config.WSLConfPath, "644")...); err != nil {
		return fail(ExitRuntime, StatePartiallyConfigured, "writing /etc/wsl.conf failed", err)
	}
	if _, err := env.WSL.Exec(ctx, "root", []byte(Hosts), WriteFileCmd(config.HostsPath, "644")...); err != nil {
		return fail(ExitRuntime, StatePartiallyConfigured, "writing "+config.HostsPath+" failed", err)
	}
	// Without these masks systemd-resolved is enabled in the 26.04 image and its tmpfiles rule turns /etc/resolv.conf
	// into a symlink to the stub at the first boot after this step. File-based, so no running manager is needed.
	if _, err := env.WSL.Exec(ctx, "root", nil, "/bin/mkdir", "-p", config.TmpfilesDir, config.SystemUnitDir); err != nil {
		return fail(ExitRuntime, StatePartiallyConfigured, "creating "+config.TmpfilesDir+" and "+config.SystemUnitDir+" failed", err)
	}
	for _, m := range config.ResolvedMasks {
		if _, err := env.WSL.Exec(ctx, "root", nil, "/bin/ln", "-sfn", "/dev/null", m); err != nil {
			return fail(ExitRuntime, StatePartiallyConfigured, "masking "+m+" failed", err)
		}
	}
	return InstallIsolation(ctx, env)
}
