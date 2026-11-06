package tests

import (
	"encoding/json"
	"strings"
	"testing"

	"agent-host/internal/config"
	"agent-host/internal/steps"
)

const (
	opPrefix   = "-d agent-host -u agent-operator -e "
	rootPrefix = "-d agent-host -u root -e "
)

// daemonJSONWith renders a daemon.json whose runsc entry has the given path and args.
func daemonJSONWith(path string, args []string) string {
	doc := map[string]any{"runtimes": map[string]any{"runsc": map[string]any{"path": path, "runtimeArgs": args}}}
	b, _ := json.Marshal(doc)
	return string(b)
}

func assertVerifyFailure(t *testing.T, fr *fakeRunner, wants ...string) {
	t.Helper()
	r := run(t, newRootfsServer(t), fr, nil)
	if r.code != 2 {
		t.Fatalf("exit %d, want 2; stderr:\n%s", r.code, r.err)
	}
	for _, w := range append(wants, "UNVERIFIED") {
		if !strings.Contains(r.err, w) {
			t.Errorf("stderr lacks %q:\n%s", w, r.err)
		}
	}
	if strings.Contains(r.stdout, "agent-host created") {
		t.Error("success must not be reported")
	}
}

func swapped() []string {
	a := append([]string(nil), config.RunscRuntimeArgs...)
	a[0], a[1] = a[1], a[0]
	return a
}

func TestVerifyFailureCases(t *testing.T) {
	bad := strings.Repeat("ab", 32)
	cases := []struct {
		name string
		fr   *fakeRunner
		want string
	}{
		{"runsc missing from docker info", &fakeRunner{outputs: []outRule{{"docker info --format", `{"runc":{"path":"runc"}}`}}}, "docker reports runsc among its runtimes"},
		{"docker info fails", &fakeRunner{rules: []rule{{"docker info --format", "cannot connect", 1}}}, "docker reports runsc among its runtimes"},
		{"runsc args out of order", &fakeRunner{outputs: []outRule{{"cat /etc/docker/daemon.json", daemonJSONWith(config.RunscPath, swapped())}}}, "runsc runtimeArgs are the eight listed arguments in order"},
		{"runsc extra arg", &fakeRunner{outputs: []outRule{{"cat /etc/docker/daemon.json", daemonJSONWith(config.RunscPath, append(append([]string(nil), config.RunscRuntimeArgs...), "--debug"))}}}, "runsc runtimeArgs"},
		{"runsc missing arg", &fakeRunner{outputs: []outRule{{"cat /etc/docker/daemon.json", daemonJSONWith(config.RunscPath, config.RunscRuntimeArgs[:7])}}}, "runsc runtimeArgs"},
		{"runsc wrong path", &fakeRunner{outputs: []outRule{{"cat /etc/docker/daemon.json", daemonJSONWith("/usr/bin/runsc", config.RunscRuntimeArgs)}}}, "runsc runtime path is /usr/local/bin/runsc"},
		{"no runsc entry in daemon.json", &fakeRunner{outputs: []outRule{{"cat /etc/docker/daemon.json", `{"runtimes":{}}`}}}, "runsc runtime path"},
		{"dependency package docker.io missing", &fakeRunner{rules: []rule{{rootPrefix + "dpkg -s docker.io", "", 1}}}, "dependency package installed: docker.io"},
		{"dependency package runsc missing", &fakeRunner{rules: []rule{{rootPrefix + "dpkg -s runsc", "", 1}}}, "dependency package installed: runsc"},
		{"dependency package iproute2 missing", &fakeRunner{rules: []rule{{rootPrefix + "dpkg -s iproute2", "", 1}}}, "dependency package installed: iproute2"},
		{"dependency package docker-buildx missing", &fakeRunner{rules: []rule{{rootPrefix + "dpkg -s docker-buildx", "", 1}}}, "dependency package installed: docker-buildx"},
		{"docker buildx missing", &fakeRunner{rules: []rule{{rootPrefix + "/usr/bin/docker buildx version", "", 1}}}, "docker buildx version works"},
		{"nft missing", &fakeRunner{rules: []rule{{rootPrefix + "/usr/sbin/nft --version", "", 127}}}, "nft --version works"},
		{"iptables missing", &fakeRunner{rules: []rule{{rootPrefix + "/usr/sbin/iptables --version", "", 127}}}, "iptables --version works"},
		{"ip missing", &fakeRunner{rules: []rule{{rootPrefix + "ip -V", "", 127}}}, "ip -V works"},
		{"bridge missing", &fakeRunner{rules: []rule{{rootPrefix + "/usr/sbin/bridge -V", "", 127}}}, "bridge -V works"},
		{"ca bundle missing", &fakeRunner{rules: []rule{{"test -s /etc/ssl/certs/ca-certificates.crt", "", 1}}}, "ca-certificates available"},
		{"curl missing", &fakeRunner{rules: []rule{{rootPrefix + "curl --version", "", 127}}}, "curl available"},
		{"gpg missing", &fakeRunner{rules: []rule{{rootPrefix + "gpg --version", "", 127}}}, "gnupg available"},
		{"runsc binary missing", &fakeRunner{rules: []rule{{rootPrefix + config.RunscPath + " --version", "", 127}}}, "runsc available"},
		{"wsl.conf wrong owner", &fakeRunner{outputs: []outRule{{"stat -c %U:%G /etc/wsl.conf", "ubuntu:root\n"}}}, "/etc/wsl.conf owned root:root"},
		{"wsl.conf wrong group", &fakeRunner{outputs: []outRule{{"stat -c %U:%G /etc/wsl.conf", "root:agent-operator\n"}}}, "/etc/wsl.conf owned root:root"},
		{"wsl.conf writable", &fakeRunner{rules: []rule{{"test ! -w /etc/wsl.conf", "", 1}}}, "/etc/wsl.conf not writable by agent-operator"},
		{"daemon.json wrong owner", &fakeRunner{outputs: []outRule{{"stat -c %U:%G /etc/docker/daemon.json", "agent-operator:agent-operator\n"}}}, "/etc/docker/daemon.json owned root:root"},
		{"daemon.json writable", &fakeRunner{rules: []rule{{"test ! -w /etc/docker/daemon.json", "", 1}}}, "/etc/docker/daemon.json not writable by agent-operator"},
		{"sudo -n true succeeds", &fakeRunner{rules: []rule{{"sudo -n true", "", 1}}}, "no sudoers entry"},
		{"tool wrong owner", &fakeRunner{outputs: []outRule{{"stat -c %U:%G:%a /usr/local/bin/create_agent_net", "agent-operator:root:755\n"}}}, "/usr/local/bin/create_agent_net owned root:root with mode 755"},
		{"tool wrong group", &fakeRunner{outputs: []outRule{{"stat -c %U:%G:%a /usr/local/bin/create_agent_net", "root:agent-operator:755\n"}}}, "/usr/local/bin/create_agent_net owned root:root with mode 755"},
		{"tool wrong mode", &fakeRunner{outputs: []outRule{{"stat -c %U:%G:%a /usr/local/bin/remove_agent_container", "root:root:775\n"}}}, "/usr/local/bin/remove_agent_container owned root:root with mode 755"},
		{"tool wrong hash", &fakeRunner{outputs: []outRule{{opPrefix + "/usr/bin/sha256sum /usr/local/bin/remove_agent_net", bad + "  /usr/local/bin/remove_agent_net\n"}}}, "/usr/local/bin/remove_agent_net matches its reference SHA-256"},
		{"restore unit missing", &fakeRunner{rules: []rule{{opPrefix + "cat " + steps.AgentRestoreUnitPath, "No such file", 1}}}, steps.AgentRestoreUnitPath + " has the expected content"},
		{"restore unit content differs", &fakeRunner{outputs: []outRule{{opPrefix + "cat " + steps.AgentRestoreUnitPath, "[Unit]\n"}}}, steps.AgentRestoreUnitPath + " has the expected content"},
		{"guard unit content differs", &fakeRunner{outputs: []outRule{{opPrefix + "cat " + steps.AgentGuardUnitPath, "[Unit]\n"}}}, steps.AgentGuardUnitPath + " has the expected content"},
		{"guard unit wrong owner", &fakeRunner{outputs: []outRule{{"stat -c %U:%G:%a " + steps.AgentGuardUnitPath, "agent-operator:root:644\n"}}}, steps.AgentGuardUnitPath + " owned root:root with mode 644"},
		{"restore unit wrong mode", &fakeRunner{outputs: []outRule{{"stat -c %U:%G:%a " + steps.AgentRestoreUnitPath, "root:root:664\n"}}}, steps.AgentRestoreUnitPath + " owned root:root with mode 644"},
		{"docker.service not enabled", &fakeRunner{outputs: []outRule{{"is-enabled docker.service", "disabled\n"}}}, "docker.service is enabled"},
		{"agent-restore not enabled", &fakeRunner{rules: []rule{{"is-enabled agent-restore.service", "", 1}}}, "agent-restore.service is enabled"},
		{"agent-guard not enabled", &fakeRunner{outputs: []outRule{{"is-enabled agent-guard.service", "disabled\n"}}}, "agent-guard.service is enabled"},
		{"agent-guard not active", &fakeRunner{outputs: []outRule{{"is-active agent-guard.service", "failed\n"}}}, "agent-guard.service is active"},
		{"agent-restore not exited", &fakeRunner{outputs: []outRule{{"show agent-restore.service", "ActiveState=failed\nSubState=failed\n"}}}, "agent-restore.service is active (exited)"},
		{"/usr/local/bin wrong owner", &fakeRunner{outputs: []outRule{{"stat -c %U /usr/local/bin", "agent-operator\n"}}}, "/usr/local/bin owned by root"},
		{"/usr/local/bin writable", &fakeRunner{rules: []rule{{"test ! -w /usr/local/bin", "", 1}}}, "/usr/local/bin not writable by agent-operator"},
		{"/mnt/c present", &fakeRunner{rules: []rule{{"test ! -e /mnt/c", "", 1}}}, "no host drive: /mnt/c absent"},
		{"drvfs mount", &fakeRunner{rules: []rule{{"awk ", "", 1}}}, "no host drive mount"},
		{"powershell runnable", &fakeRunner{rules: []rule{{"command -v powershell.exe", "", 1}}}, "no host program: powershell.exe"},
		{"interop registered", &fakeRunner{rules: []rule{{"binfmt_misc/WSLInterop", "", 1}}}, "no interop"},
		{"PATH has /mnt entry", &fakeRunner{outputs: []outRule{{"-e printenv PATH", "/usr/bin:/mnt/c/Windows\n"}}}, "no host path: PATH has no /mnt/ entries"},
		{"systemd degraded by another unit", &fakeRunner{outputs: []outRule{{"systemctl is-system-running", "degraded\n"}, {"systemctl list-units", "docker.service loaded failed failed Docker\n"}}}, "systemd reports running"},
		{"systemd degraded, list-units fails", &fakeRunner{outputs: []outRule{{"systemctl is-system-running", "degraded\n"}}, rules: []rule{{"systemctl list-units", "boom", 1}}}, "systemd reports running"},
		{"wrong default user", &fakeRunner{outputs: []outRule{{"-e id -un", "root\n"}}}, "default user is agent-operator"},
		{"wrong UID", &fakeRunner{outputs: []outRule{{opPrefix + "id -u", "1000\n"}}}, "has UID and GID 1001"},
		{"resolv.conf is a symlink", &fakeRunner{rules: []rule{{"test -f /etc/resolv.conf", "", 1}}}, "/etc/resolv.conf is a regular file, not a symlink"},
		{"resolv.conf wrong content", &fakeRunner{outputs: []outRule{{opPrefix + "cat /etc/resolv.conf", "nameserver 10.255.255.254\n"}}}, "/etc/resolv.conf has the expected content"},
		{"resolv.conf wrong owner", &fakeRunner{outputs: []outRule{{"stat -c %U:%G /etc/resolv.conf", "agent-operator:root\n"}}}, "/etc/resolv.conf owned root:root"},
		{"resolv.conf writable", &fakeRunner{rules: []rule{{"test ! -w /etc/resolv.conf", "", 1}}}, "/etc/resolv.conf not writable by agent-operator"},
		{"hosts wrong content", &fakeRunner{outputs: []outRule{{opPrefix + "cat /etc/hosts", "127.0.1.1 DESKTOP-WIN\n"}}}, "/etc/hosts has the expected content"},
		{"wsl.conf content differs", &fakeRunner{outputs: []outRule{{opPrefix + "cat /etc/wsl.conf", "[automount]\nenabled=false\n"}}}, "/etc/wsl.conf has the expected content"},
		{"hostname wrong", &fakeRunner{outputs: []outRule{{opPrefix + "hostname", "DESKTOP-WIN\n"}}}, "hostname is agent-host"},
		{"/mnt/wslg still mounted", &fakeRunner{rules: []rule{{"mountpoint -q /mnt/wslg", "", 1}}}, "/mnt/wslg is not a mountpoint"},
		{"/tmp/.X11-unix still mounted", &fakeRunner{rules: []rule{{"mountpoint -q /tmp/.X11-unix", "", 1}}}, "/tmp/.X11-unix is not a mountpoint"},
		{"/mnt/wsl still mounted", &fakeRunner{rules: []rule{{"mountpoint -q /mnt/wsl", "", 1}}}, "/mnt/wsl is not a mountpoint"},
		{"/usr/lib/wsl still mounted", &fakeRunner{rules: []rule{{"mountpoint -q /usr/lib/wsl", "", 1}}}, "/usr/lib/wsl is not a mountpoint"},
		{"/dev/dxg present and not masked", &fakeRunner{rules: []rule{{"test ! -e /dev/dxg", "", 1}}}, "no GPU device access: /dev/dxg absent or masked by /dev/null"},
		{"GPU libraries present", &fakeRunner{rules: []rule{{"find /usr/lib/wsl/lib /usr/lib/wsl/drivers", "", 1}}}, "no GPU libraries: /usr/lib/wsl/lib and /usr/lib/wsl/drivers empty"},
		{"isolate script content differs", &fakeRunner{outputs: []outRule{{opPrefix + "cat " + steps.IsolateScriptPath, "#!/bin/sh\n"}}}, steps.IsolateScriptPath + " has the expected content"},
		{"isolate script wrong mode", &fakeRunner{outputs: []outRule{{"stat -c %U:%G:%a " + steps.IsolateScriptPath, "root:root:644\n"}}}, steps.IsolateScriptPath + " owned root:root with mode 755"},
		{"isolate unit wrong owner", &fakeRunner{outputs: []outRule{{"stat -c %U:%G:%a " + steps.IsolateUnitPath, "agent-operator:root:644\n"}}}, steps.IsolateUnitPath + " owned root:root with mode 644"},
		{"isolate drop-in content differs", &fakeRunner{outputs: []outRule{{opPrefix + "cat " + steps.IsolateDropInPath, "[Unit]\n"}}}, steps.IsolateDropInPath + " has the expected content"},
		{"isolate service not enabled", &fakeRunner{outputs: []outRule{{"is-enabled agent-host-isolate.service", "disabled\n"}}}, "agent-host-isolate.service is enabled"},
		{"isolate service inactive", &fakeRunner{outputs: []outRule{{"show agent-host-isolate.service", "ActiveState=failed\nSubState=failed\n"}}}, "agent-host-isolate.service is active (exited)"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) { assertVerifyFailure(t, c.fr, c.want) })
	}
}

func TestVerifySystemdRunningPasses(t *testing.T) {
	r := run(t, newRootfsServer(t), &fakeRunner{outputs: []outRule{{"systemctl is-system-running", "running\n"}}}, nil)
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	if strings.Contains(r.err, "warning:") {
		t.Errorf("unexpected warning:\n%s", r.err)
	}
}

func TestVerifySystemdDegradedOnlyByUser0Passes(t *testing.T) {
	fr := &fakeRunner{outputs: []outRule{
		{"systemctl is-system-running", "degraded\n"},
		{"systemctl list-units", "user@0.service loaded failed failed User Manager for UID 0\n"},
	}}
	r := run(t, newRootfsServer(t), fr, nil)
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	if !strings.Contains(r.err, "warning: systemd degraded only by user@0.service (known WSL issue); accepted") {
		t.Errorf("stderr lacks the degraded warning:\n%s", r.err)
	}
	if !fr.has(opPrefix + "/usr/bin/systemctl list-units --state=failed --no-legend --plain") {
		t.Error("failed-unit listing not issued")
	}
}

func TestVerifySystemdDegradedByUser0AndOtherFailsNamingBoth(t *testing.T) {
	fr := &fakeRunner{outputs: []outRule{
		{"systemctl is-system-running", "degraded\n"},
		{"systemctl list-units", "user@0.service loaded failed failed User Manager for UID 0\ndocker.service loaded failed failed Docker\n"},
	}}
	assertVerifyFailure(t, fr, "systemd reports running", "user@0.service", "docker.service")
}

func TestVerifyForbiddenGroups(t *testing.T) {
	for _, g := range []string{"sudo", "admin", "wheel", "adm", "disk", "lxd", "libvirt", "docker"} {
		g := g
		t.Run(g, func(t *testing.T) {
			fr := &fakeRunner{outputs: []outRule{{"-e id -nG", "agent-operator " + g + "\n"}}}
			assertVerifyFailure(t, fr, "no root-equivalent group membership", "member of group(s) "+g)
		})
	}
}

func TestVerifyToolHashMismatchNamesBothHashes(t *testing.T) {
	bad := strings.Repeat("cd", 32)
	fr := &fakeRunner{outputs: []outRule{{opPrefix + "/usr/bin/sha256sum /usr/local/bin/create_agent_container", bad + "  x\n"}}}
	assertVerifyFailure(t, fr, "create_agent_container", bad, sha("ELF fake build of create_agent_container"))
}

func TestVerifyAllIssuedCommands(t *testing.T) {
	fr := &fakeRunner{}
	if r := run(t, newRootfsServer(t), fr, nil); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	want := []string{
		rootPrefix + "docker info --format {{json .Runtimes}}",
		rootPrefix + "/usr/bin/docker buildx version",
		rootPrefix + "/usr/sbin/nft --version",
		rootPrefix + "/usr/sbin/iptables --version",
		rootPrefix + "ip -V",
		rootPrefix + "/usr/sbin/bridge -V",
		rootPrefix + "curl --version",
		rootPrefix + "gpg --version",
		rootPrefix + "test -s /etc/ssl/certs/ca-certificates.crt",
		rootPrefix + config.RunscPath + " --version",
		rootPrefix + "docker run --rm --runtime=runsc hello-world",
		opPrefix + "cat /etc/docker/daemon.json",
		opPrefix + "stat -c %U:%G /etc/wsl.conf",
		opPrefix + "stat -c %U:%G /etc/docker/daemon.json",
		opPrefix + "/bin/sh -c test ! -w /etc/wsl.conf",
		opPrefix + "/bin/sh -c test ! -w /etc/docker/daemon.json",
		opPrefix + `/bin/sh -c test ! -e /dev/dxg || test "$(stat -c %t:%T /dev/dxg)" = "1:3"`,
		opPrefix + `/bin/sh -c test -z "$(find /usr/lib/wsl/lib /usr/lib/wsl/drivers -mindepth 1 -print -quit 2>/dev/null)"`,
		opPrefix + "id -nG",
		opPrefix + "/bin/sh -c ! sudo -n true",
		opPrefix + "stat -c %U /usr/local/bin",
		opPrefix + "/bin/sh -c test ! -w /usr/local/bin",
		opPrefix + "/usr/bin/systemctl is-system-running",
		opPrefix + "printenv PATH",
		opPrefix + "id -u",
		opPrefix + "id -g",
		"-d agent-host -e id -un",
	}
	for _, pkg := range steps.DependencyPackages {
		want = append(want, rootPrefix+"dpkg -s "+pkg)
	}
	if len(steps.DependencyPackages) != 13 {
		t.Errorf("%d dependency packages, want 13", len(steps.DependencyPackages))
	}
	for _, name := range config.ToolNames {
		want = append(want,
			opPrefix+"stat -c %U:%G:%a /usr/local/bin/"+name,
			opPrefix+"/usr/bin/sha256sum /usr/local/bin/"+name)
	}
	for _, u := range steps.LifecycleUnits {
		want = append(want, opPrefix+"cat "+u.Path, opPrefix+"stat -c %U:%G:%a "+u.Path)
	}
	for _, unit := range steps.LifecycleEnabledUnits {
		want = append(want, opPrefix+"/usr/bin/systemctl is-enabled "+unit)
	}
	want = append(want,
		opPrefix+"/usr/bin/systemctl is-active agent-guard.service",
		opPrefix+"/usr/bin/systemctl show agent-restore.service -p ActiveState -p SubState")
	for _, w := range want {
		if !fr.has(w) {
			t.Errorf("verify command not issued: %q", w)
		}
	}
}

func TestVerifyCollectsFailuresAcrossAllCheckKinds(t *testing.T) {
	bad := strings.Repeat("ef", 32)
	fr := &fakeRunner{
		rules: []rule{
			{rootPrefix + "/usr/sbin/nft --version", "", 127},
			{"test ! -w /etc/wsl.conf", "", 1},
			{"sudo -n true", "", 1},
			{"test ! -w /usr/local/bin", "", 1},
		},
		outputs: []outRule{
			{"docker info --format", `{"runc":{}}`},
			{"cat /etc/docker/daemon.json", daemonJSONWith("/x", swapped())},
			{"-e id -nG", "agent-operator docker lxd\n"},
			{"stat -c %U:%G:%a /usr/local/bin/create_agent_net", "root:root:777\n"},
			{opPrefix + "/usr/bin/sha256sum /usr/local/bin/remove_agent_net", bad + "  x\n"},
			{"-e printenv PATH", "/mnt/c/bin\n"},
			{"systemctl is-system-running", "degraded\n"},
			{"-e id -un", "root\n"},
		},
	}
	r := run(t, newRootfsServer(t), fr, nil)
	if r.code != 2 {
		t.Fatalf("exit %d, want 2", r.code)
	}
	for _, w := range []string{
		"docker reports runsc", "nft --version works", "runsc runtime path", "runsc runtimeArgs",
		"/etc/wsl.conf not writable", "member of group(s) docker, lxd", "no sudoers entry",
		"create_agent_net owned root:root with mode 755", "remove_agent_net matches its reference SHA-256", bad,
		"/usr/local/bin not writable", "no host path", "systemd reports running", "default user is agent-operator", "UNVERIFIED",
	} {
		if !strings.Contains(r.err, w) {
			t.Errorf("stderr lacks %q:\n%s", w, r.err)
		}
	}
	// checks that were not broken must not be reported
	for _, w := range []string{"iptables --version works", "curl available", "/etc/docker/daemon.json owned root:root", "create_agent_container owned"} {
		if strings.Contains(r.err, w) {
			t.Errorf("stderr reports passing check %q:\n%s", w, r.err)
		}
	}
	if strings.Contains(r.stdout, "agent-host created") {
		t.Error("success must not be reported")
	}
}
