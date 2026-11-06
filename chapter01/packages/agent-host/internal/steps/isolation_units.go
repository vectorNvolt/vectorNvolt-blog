// Part of USC-001 step 7: install the agent-host-isolate script and service (root:root), the docker.service drop-in
// that requires it, and enable it with an explicit symlink (systemd may not be running yet at step 7).
package steps

import (
	"context"
	_ "embed"
)

const (
	IsolateScriptPath     = "/usr/local/libexec/agent-host-isolate"
	IsolateUnitPath       = "/etc/systemd/system/agent-host-isolate.service"
	IsolateDropInPath     = "/etc/systemd/system/docker.service.d/agent-host-isolate.conf"
	IsolateEnableLinkPath = "/etc/systemd/system/multi-user.target.wants/agent-host-isolate.service"
	IsolateUnitName       = "agent-host-isolate.service"
)

// IsolateScript unmounts the mounts WSL injects into every distro and unregisters the interop binfmt entries.
//
//go:embed scripts/agent_host_isolate.sh
var IsolateScript string

const IsolateUnit = `[Unit]
Description=Remove Windows-facing mounts and interop from agent-host
# Early boot: tmp.mount puts a tmpfs on /tmp over WSL's /tmp/.X11-unix mount, which umount cannot reach by path afterwards.
DefaultDependencies=no
# The binfmt_misc mount must be up, or the interop entries would look absent.
Wants=proc-sys-fs-binfmt_misc.mount
After=proc-sys-fs-binfmt_misc.mount
Before=tmp.mount docker.service

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/usr/local/libexec/agent-host-isolate

[Install]
WantedBy=multi-user.target
`

const IsolateDropIn = `# If isolation fails, docker.service does not start and no agent can run.
[Unit]
Requires=agent-host-isolate.service
After=agent-host-isolate.service
`

// IsolatedMounts are injected by WSL regardless of wsl.conf; the isolate script unmounts them and verify checks they are gone.
var IsolatedMounts = []string{"/mnt/wslg", "/tmp/.X11-unix", "/mnt/wsl", "/usr/lib/wsl"}

// IsolationFiles maps each isolation file to its expected content and mode.
var IsolationFiles = []struct{ Path, Content, Mode string }{
	{IsolateScriptPath, IsolateScript, "755"},
	{IsolateUnitPath, IsolateUnit, "644"},
	{IsolateDropInPath, IsolateDropIn, "644"},
}

// isolationDirs are created before the files: WriteFileCmd does not create parent directories.
var isolationDirs = []string{
	"/usr/local/libexec",
	"/etc/systemd/system/docker.service.d",
	"/etc/systemd/system/multi-user.target.wants",
}

func InstallIsolation(ctx context.Context, env *Env) *Failure {
	for _, d := range isolationDirs {
		if _, err := env.WSL.Exec(ctx, "root", nil, "/bin/mkdir", "-p", d); err != nil {
			return fail(ExitRuntime, StatePartiallyConfigured, "creating "+d+" failed", err)
		}
	}
	for _, f := range IsolationFiles {
		if _, err := env.WSL.Exec(ctx, "root", []byte(f.Content), WriteFileCmd(f.Path, f.Mode)...); err != nil {
			return fail(ExitRuntime, StatePartiallyConfigured, "writing "+f.Path+" failed", err)
		}
	}
	if _, err := env.WSL.Exec(ctx, "root", nil, "/bin/ln", "-sf", "../"+IsolateUnitName, IsolateEnableLinkPath); err != nil {
		return fail(ExitRuntime, StatePartiallyConfigured, "enabling "+IsolateEnableLinkPath+" failed", err)
	}
	return nil
}
