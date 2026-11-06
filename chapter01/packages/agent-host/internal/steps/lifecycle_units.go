// Part of USC-001 step 11: write the agent-restore and agent-guard systemd units (root:root 644),
// enable them together with docker.service and start them. Idempotent: a unit is rewritten only if its content differs.
package steps

import "context"

const (
	AgentRestoreUnitPath = "/etc/systemd/system/agent-restore.service"
	AgentGuardUnitPath   = "/etc/systemd/system/agent-guard.service"
)

const AgentRestoreUnit = `[Unit]
Description=Rebuild agents-net gate and clear dead agent containers after dockerd start
Requires=docker.service
After=docker.service
PartOf=docker.service

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/usr/local/bin/agent_lifecycle restore
ExecStopPost=/bin/rm -f /run/agent-sandbox/ready

[Install]
WantedBy=docker.service multi-user.target
`

const AgentGuardUnit = `[Unit]
Description=Remove agent containers started outside the agent tools
Requires=docker.service
After=docker.service
PartOf=docker.service

[Service]
Type=simple
ExecStart=/usr/local/bin/agent_lifecycle guard
Restart=always
RestartSec=2

[Install]
WantedBy=docker.service multi-user.target
`

// LifecycleUnits maps each unit file path to its expected content.
var LifecycleUnits = []struct{ Path, Content string }{
	{AgentRestoreUnitPath, AgentRestoreUnit},
	{AgentGuardUnitPath, AgentGuardUnit},
}

// LifecycleEnabledUnits are enabled in this order.
var LifecycleEnabledUnits = []string{"docker.service", "agent-restore.service", "agent-guard.service"}

const systemctl = "/usr/bin/systemctl"

func InstallLifecycleUnits(ctx context.Context, env *Env) *Failure {
	for _, u := range LifecycleUnits {
		// A missing or unreadable file counts as "differs".
		if cur, err := env.WSL.Exec(ctx, "root", nil, "/bin/cat", u.Path); err == nil && string(cur) == u.Content {
			continue
		}
		if _, err := env.WSL.Exec(ctx, "root", []byte(u.Content), WriteFileCmd(u.Path, "644")...); err != nil {
			return fail(ExitRuntime, StatePartiallyConfigured, "writing "+u.Path+" failed", err)
		}
	}
	if _, err := env.WSL.Exec(ctx, "root", nil, systemctl, "daemon-reload"); err != nil {
		return fail(ExitRuntime, StatePartiallyConfigured, "systemctl daemon-reload failed", err)
	}
	enable := append([]string{systemctl, "enable"}, LifecycleEnabledUnits...)
	if _, err := env.WSL.Exec(ctx, "root", nil, enable...); err != nil {
		return fail(ExitRuntime, StatePartiallyConfigured, "enabling docker, agent-restore and agent-guard units failed", err)
	}
	if _, err := env.WSL.Exec(ctx, "root", nil, systemctl, "start", "agent-guard.service", "agent-restore.service"); err != nil {
		return fail(ExitRuntime, StatePartiallyConfigured, "starting agent-guard and agent-restore units failed", err)
	}
	return nil
}
