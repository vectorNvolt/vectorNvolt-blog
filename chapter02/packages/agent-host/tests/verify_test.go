package tests

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"agent-host/internal/config"
	"agent-host/internal/steps"
)

// daemonJSONWith renders a daemon.json whose runsc entry has the given path and args.
func daemonJSONWith(path string, args []string) string {
	doc := map[string]any{"runtimes": map[string]any{"runsc": map[string]any{"path": path, "runtimeArgs": args}}}
	b, _ := json.Marshal(doc)
	return string(b)
}

func assertVerifyFailure(t *testing.T, fr *fakeRunner, wants ...string) {
	t.Helper()
	r := run(t, fr, nil)
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
		{"docker run runsc fails", &fakeRunner{rules: []rule{{"docker run --rm --runtime=runsc", "no such runtime", 125}}}, "docker runs a gVisor (runsc) container"},
		{"runsc args out of order", &fakeRunner{outputs: []outRule{{"cat /etc/docker/daemon.json", daemonJSONWith(config.RunscPath, swapped())}}}, "runsc runtimeArgs are the eight listed arguments in order"},
		{"runsc extra arg", &fakeRunner{outputs: []outRule{{"cat /etc/docker/daemon.json", daemonJSONWith(config.RunscPath, append(append([]string(nil), config.RunscRuntimeArgs...), "--debug"))}}}, "runsc runtimeArgs"},
		{"runsc missing arg", &fakeRunner{outputs: []outRule{{"cat /etc/docker/daemon.json", daemonJSONWith(config.RunscPath, config.RunscRuntimeArgs[:7])}}}, "runsc runtimeArgs"},
		{"runsc wrong path", &fakeRunner{outputs: []outRule{{"cat /etc/docker/daemon.json", daemonJSONWith("/usr/bin/runsc", config.RunscRuntimeArgs)}}}, "runsc runtime path is /usr/local/bin/runsc"},
		{"no runsc entry in daemon.json", &fakeRunner{outputs: []outRule{{"cat /etc/docker/daemon.json", `{"runtimes":{}}`}}}, "runsc runtime path"},
		{"dependency package docker.io missing", &fakeRunner{rules: []rule{{"^dpkg -s docker.io", "", 1}}}, "dependency package installed: docker.io"},
		{"dependency package runsc missing", &fakeRunner{rules: []rule{{"^dpkg -s runsc", "", 1}}}, "dependency package installed: runsc"},
		{"dependency package iproute2 missing", &fakeRunner{rules: []rule{{"^dpkg -s iproute2", "", 1}}}, "dependency package installed: iproute2"},
		{"dependency package docker-buildx missing", &fakeRunner{rules: []rule{{"^dpkg -s docker-buildx", "", 1}}}, "dependency package installed: docker-buildx"},
		{"docker buildx missing", &fakeRunner{rules: []rule{{"^/usr/bin/docker buildx version", "", 1}}}, "docker buildx version works"},
		{"nft missing", &fakeRunner{rules: []rule{{"^/usr/sbin/nft --version", "", 127}}}, "nft --version works"},
		{"iptables missing", &fakeRunner{rules: []rule{{"^/usr/sbin/iptables --version", "", 127}}}, "iptables --version works"},
		{"ip missing", &fakeRunner{rules: []rule{{"^ip -V", "", 127}}}, "ip -V works"},
		{"bridge missing", &fakeRunner{rules: []rule{{"^/usr/sbin/bridge -V", "", 127}}}, "bridge -V works"},
		{"ca bundle missing", &fakeRunner{rules: []rule{{"test -s /etc/ssl/certs/ca-certificates.crt", "", 1}}}, "ca-certificates available"},
		{"curl missing", &fakeRunner{rules: []rule{{"^curl --version", "", 127}}}, "curl available"},
		{"gpg missing", &fakeRunner{rules: []rule{{"^gpg --version", "", 127}}}, "gnupg available"},
		{"runsc binary missing", &fakeRunner{rules: []rule{{"^" + config.RunscPath + " --version", "", 127}}}, "runsc available"},
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
		{"agent-guard restarted once", &fakeRunner{outputs: []outRule{{"show -p NRestarts", guardShowOut("1", "always")}}}, "agent-guard.service has run without restart"},
		{"agent-guard Restart=on-failure", &fakeRunner{outputs: []outRule{{"show -p NRestarts", guardShowOut("0", "on-failure")}}}, "agent-guard.service unit has Restart=always"},
		{"agent-guard ExecStart wrong", &fakeRunner{outputs: []outRule{{"show -p NRestarts", strings.Replace(guardShowOut("0", "always"), "agent_lifecycle guard", "agent_lifecycle restore", 1)}}}, "agent-guard.service unit ExecStart runs agent_lifecycle guard"},
		{"agent-guard wrong main process", &fakeRunner{outputs: []outRule{{"cat /proc/" + guardPID + "/cmdline", "/usr/bin/sleep\x00100\x00"}}}, "agent-guard.service main process is agent_lifecycle guard"},
		{"agent-guard no main process", &fakeRunner{outputs: []outRule{{"show -p NRestarts", strings.Replace(guardShowOut("0", "always"), "MainPID="+guardPID, "MainPID=0", 1)}}}, "agent-guard.service main process is agent_lifecycle guard"},
		{"ready file missing", &fakeRunner{rules: []rule{{"test -f /run/agent-sandbox/ready", "", 1}}}, "/run/agent-sandbox/ready exists"},
		{"agent-restore not exited", &fakeRunner{outputs: []outRule{{"show agent-restore.service", "ActiveState=failed\nSubState=failed\n"}}}, "agent-restore.service is active (exited)"},
		{"/usr/local/bin wrong owner", &fakeRunner{outputs: []outRule{{"stat -c %U /usr/local/bin", "agent-operator\n"}}}, "/usr/local/bin owned by root"},
		{"/usr/local/bin writable", &fakeRunner{rules: []rule{{"test ! -w /usr/local/bin", "", 1}}}, "/usr/local/bin not writable by agent-operator"},
		{"systemd degraded", &fakeRunner{outputs: []outRule{{"systemctl is-system-running", "degraded\n"}}}, "systemd reports running"},
		{"systemd state unreadable", &fakeRunner{rules: []rule{{"^" + opPrefix + "/usr/bin/systemctl is-system-running", "boom", 1}}}, "systemd reports running"},
		{"wrong UID", &fakeRunner{outputs: []outRule{{opPrefix + "id -u", "1000\n"}}}, "has UID and GID 1001"},
		{"hostname wrong", &fakeRunner{outputs: []outRule{{opPrefix + "hostname", "ubuntu\n"}}}, "hostname is agent-host"},
		{"hostname drop-in differs", &fakeRunner{outputs: []outRule{{opPrefix + "cat " + config.PreserveHostnameCfgPath, "preserve_hostname: false\n"}}}, config.PreserveHostnameCfgPath + " has the expected content"},
		{"no-docker-upgrade file missing", &fakeRunner{rules: []rule{{opPrefix + "cat " + config.NoDockerUpgradeCfgPath, "No such file", 1}}}, config.NoDockerUpgradeCfgPath + " has the expected content"},
		{"no-docker-upgrade content differs", &fakeRunner{outputs: []outRule{{opPrefix + "cat " + config.NoDockerUpgradeCfgPath, "Unattended-Upgrade::Package-Blacklist {\n};\n"}}}, config.NoDockerUpgradeCfgPath + " has the expected content"},
		{"no-docker-upgrade wrong owner", &fakeRunner{outputs: []outRule{{"stat -c %U:%G:%a " + config.NoDockerUpgradeCfgPath, "agent-operator:root:644\n"}}}, config.NoDockerUpgradeCfgPath + " owned root:root with mode 644"},
		{"no-docker-upgrade wrong mode", &fakeRunner{outputs: []outRule{{"stat -c %U:%G:%a " + config.NoDockerUpgradeCfgPath, "root:root:664\n"}}}, config.NoDockerUpgradeCfgPath + " owned root:root with mode 644"},
		{"modules-load content differs", &fakeRunner{outputs: []outRule{{opPrefix + "cat " + config.BrNetfilterModulesPath, "other\n"}}}, config.BrNetfilterModulesPath + " has the expected content"},
		{"modules-load wrong mode", &fakeRunner{outputs: []outRule{{"stat -c %U:%G:%a " + config.BrNetfilterModulesPath, "root:root:664\n"}}}, config.BrNetfilterModulesPath + " owned root:root with mode 644"},
		{"sysctl drop-in content differs", &fakeRunner{outputs: []outRule{{opPrefix + "cat " + config.BridgeSysctlPath, "net.bridge.bridge-nf-call-iptables = 0\n"}}}, config.BridgeSysctlPath + " has the expected content"},
		{"sysctl drop-in wrong owner", &fakeRunner{outputs: []outRule{{"stat -c %U:%G:%a " + config.BridgeSysctlPath, "agent-operator:root:644\n"}}}, config.BridgeSysctlPath + " owned root:root with mode 644"},
		{"br_netfilter not loaded", &fakeRunner{rules: []rule{{"test -d " + config.BrNetfilterSysModuleDir, "", 1}}}, "br_netfilter kernel module is loaded"},
		{"bridge-nf-call-iptables is 0", &fakeRunner{outputs: []outRule{{"cat " + config.BridgeNfCallIptablesProc, "0\n"}}}, config.BridgeNfCallIptablesProc + " is 1"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) { assertVerifyFailure(t, c.fr, c.want) })
	}
}

func TestVerifyHostMounts(t *testing.T) {
	for _, fstype := range []string{"virtiofs", "9p", "sshfs", "fuse.sshfs", "fuse.reverse-sshfs"} {
		fstype := fstype
		t.Run(fstype, func(t *testing.T) {
			sys := foundationSystem(t, goodMarker)
			// Preflight sees a clean VM; the mounts appear once commands run (a mount added mid-run), so step 8 must catch them.
			fr := &fakeRunner{onCall: func(string) {
				if err := os.WriteFile(sys.MountsPath, []byte(cleanMounts+"share /mnt/host "+fstype+" rw 0 0\nshare2 /mnt/two "+fstype+" rw 0 0\n"), 0o644); err != nil {
					t.Error(err)
				}
			}}
			r := runOn(t, fr, sys, nil)
			if r.code != 2 {
				t.Fatalf("exit %d, want 2; stderr:\n%s", r.code, r.err)
			}
			for _, w := range []string{"no host mounts", "/mnt/host", "/mnt/two", "UNVERIFIED"} {
				if !strings.Contains(r.err, w) {
					t.Errorf("stderr lacks %q:\n%s", w, r.err)
				}
			}
		})
	}
}

func TestVerifySystemdRunningPasses(t *testing.T) {
	r := run(t, &fakeRunner{outputs: []outRule{{"systemctl is-system-running", "running\n"}}}, nil)
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	if strings.Contains(r.err, "warning:") {
		t.Errorf("unexpected warning:\n%s", r.err)
	}
}

func TestVerifyForbiddenGroups(t *testing.T) {
	for _, g := range []string{"sudo", "admin", "wheel", "adm", "disk", "lxd", "libvirt", "docker"} {
		g := g
		t.Run(g, func(t *testing.T) {
			fr := &fakeRunner{outputs: []outRule{{"id -nG", "agent-operator " + g + "\n"}}}
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
	if r := run(t, fr, nil); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	want := []string{
		"docker info --format {{json .Runtimes}}",
		"/usr/bin/docker buildx version",
		"/usr/sbin/nft --version",
		"/usr/sbin/iptables --version",
		"ip -V",
		"/usr/sbin/bridge -V",
		"curl --version",
		"gpg --version",
		"test -s /etc/ssl/certs/ca-certificates.crt",
		config.RunscPath + " --version",
		"docker run --rm --runtime=runsc hello-world",
		opPrefix + "cat /etc/docker/daemon.json",
		opPrefix + "stat -c %U:%G /etc/docker/daemon.json",
		opPrefix + "/bin/sh -c test ! -w /etc/docker/daemon.json",
		opPrefix + "id -nG",
		opPrefix + "/bin/sh -c ! sudo -n true",
		opPrefix + "/bin/sh -c " + steps.HomeWriteScript + " sh /home/ubuntu",
		opPrefix + "stat -c %U /usr/local/bin",
		opPrefix + "/bin/sh -c test ! -w /usr/local/bin",
		opPrefix + "/usr/bin/systemctl is-system-running",
		opPrefix + "id -u",
		opPrefix + "id -g",
		opPrefix + "hostname",
		opPrefix + "cat " + config.PreserveHostnameCfgPath,
		opPrefix + "cat " + config.NoDockerUpgradeCfgPath,
		opPrefix + "stat -c %U:%G:%a " + config.NoDockerUpgradeCfgPath,
		opPrefix + "cat " + config.BrNetfilterModulesPath,
		opPrefix + "stat -c %U:%G:%a " + config.BrNetfilterModulesPath,
		opPrefix + "cat " + config.BridgeSysctlPath,
		opPrefix + "stat -c %U:%G:%a " + config.BridgeSysctlPath,
		opPrefix + "test -d " + config.BrNetfilterSysModuleDir,
		opPrefix + "cat " + config.BridgeNfCallIptablesProc,
	}
	for _, pkg := range steps.DependencyPackages {
		want = append(want, "dpkg -s "+pkg)
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
		opPrefix+"/usr/bin/systemctl show agent-restore.service -p ActiveState -p SubState",
		opPrefix+guardShowCmd,
		opPrefix+"cat /proc/"+guardPID+"/cmdline",
		opPrefix+"test -f /run/agent-sandbox/ready")
	for _, w := range want {
		if !fr.has(w) {
			t.Errorf("verify command not issued: %q", w)
		}
	}
}

func TestVerifyLoginHomeWritable(t *testing.T) {
	fr := &fakeRunner{outputs: []outRule{{steps.HomeWriteScript, "/home/ubuntu/.bashrc\n"}}}
	assertVerifyFailure(t, fr, "agent-operator cannot write under ubuntu's home /home/ubuntu", "/home/ubuntu/.bashrc")
}

func TestVerifyNoLoginUserIssuesNoHomeCheck(t *testing.T) {
	sys := foundationSystem(t, goodMarker)
	if err := os.WriteFile(sys.PasswdPath, []byte("root:x:0:0:root:/root:/bin/bash\nnobody:x:65534:65534::/nonexistent:/usr/sbin/nologin\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fr := &fakeRunner{}
	if r := runOn(t, fr, sys, nil); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	if fr.has(opPrefix + "/bin/sh -c " + steps.HomeWriteScript) {
		t.Error("home write check issued although there is no login user")
	}
}

func TestVerifyCollectsFailuresAcrossAllCheckKinds(t *testing.T) {
	bad := strings.Repeat("ef", 32)
	fr := &fakeRunner{
		rules: []rule{
			{"^/usr/sbin/nft --version", "", 127},
			{"sudo -n true", "", 1},
			{"test ! -w /usr/local/bin", "", 1},
		},
		outputs: []outRule{
			{"docker info --format", `{"runc":{}}`},
			{"cat /etc/docker/daemon.json", daemonJSONWith("/x", swapped())},
			{"id -nG", "agent-operator docker lxd\n"},
			{"stat -c %U:%G:%a /usr/local/bin/create_agent_net", "root:root:777\n"},
			{opPrefix + "/usr/bin/sha256sum /usr/local/bin/remove_agent_net", bad + "  x\n"},
			{"systemctl is-system-running", "degraded\n"},
			{opPrefix + "hostname", "ubuntu\n"},
		},
	}
	r := run(t, fr, nil)
	if r.code != 2 {
		t.Fatalf("exit %d, want 2", r.code)
	}
	for _, w := range []string{
		"docker reports runsc", "nft --version works", "runsc runtime path", "runsc runtimeArgs",
		"member of group(s) docker, lxd", "no sudoers entry",
		"create_agent_net owned root:root with mode 755", "remove_agent_net matches its reference SHA-256", bad,
		"/usr/local/bin not writable", "systemd reports running", "hostname is agent-host", "UNVERIFIED",
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
