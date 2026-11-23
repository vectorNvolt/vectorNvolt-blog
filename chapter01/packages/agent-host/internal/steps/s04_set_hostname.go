// SFR-HOST-015 (USC-001 step 4): set the hostname agent-host. Idempotent: hostnamectl is a set, the cloud-init
// drop-in is rewritten atomically and the /etc/hosts entry is appended only if absent.
// The drop-in (preserve_hostname) keeps cloud-init from resetting the name at the next boot.
package steps

import (
	"context"

	"agent-host/internal/config"
)

const PreserveHostnameCfg = "# Written by create_agent_host: keep the hostname agent-host across reboots.\npreserve_hostname: true\n"

// EnsureHostsEntryCmd maps the hostname to 127.0.1.1 unless /etc/hosts already names it (avoids sudo resolver warnings).
var EnsureHostsEntryCmd = []string{"/bin/sh", "-c",
	`grep -qE '(^|[[:space:]])` + config.HostName + `([[:space:]]|$)' ` + config.HostsPath +
		` || echo '127.0.1.1 ` + config.HostName + `' >> ` + config.HostsPath}

func SetHostname(ctx context.Context, env *Env) *Failure {
	if _, err := env.Host.Exec(ctx, nil, "/usr/bin/hostnamectl", "set-hostname", config.HostName); err != nil {
		return fail(ExitRuntime, StatePartiallyConfigured, "setting the hostname failed", err)
	}
	if _, err := env.Host.Exec(ctx, nil, "/bin/mkdir", "-p", config.CloudCfgDir); err != nil {
		return fail(ExitRuntime, StatePartiallyConfigured, "creating "+config.CloudCfgDir+" failed", err)
	}
	if _, err := env.Host.Exec(ctx, []byte(PreserveHostnameCfg), WriteFileCmd(config.PreserveHostnameCfgPath, "644")...); err != nil {
		return fail(ExitRuntime, StatePartiallyConfigured, "writing "+config.PreserveHostnameCfgPath+" failed", err)
	}
	if _, err := env.Host.Exec(ctx, nil, EnsureHostsEntryCmd...); err != nil {
		return fail(ExitRuntime, StatePartiallyConfigured, "adding the hostname to "+config.HostsPath+" failed", err)
	}
	return nil
}
