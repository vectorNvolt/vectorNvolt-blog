package tests

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"strings"
	"testing"

	"agent-net/internal/config"
	"agent-net/internal/sequence"
)

const (
	epFile   = "/etc/agent-net/endpoints.json"
	epJSON   = `{"endpoints": ["api.anthropic.com", "platform.claude.com"]}`
	ipAPI    = "160.79.104.10"
	ipPlat   = "160.79.104.20"
	ipNew    = "160.79.104.99"
	dnsRule  = "-s 172.30.0.2/32 -d 1.1.1.1/32 -p udp -m udp --dport 53 -m comment --comment agent-net:web-1 -j ACCEPT"
	denyRule = "-s 172.30.0.2/32 ! -o " + bridgeName + " -m comment --comment agent-net:web-1 -j DROP"
)

func epRule(ctr, src, ip string) string {
	return fmt.Sprintf("-s %s/32 -d %s/32 -p tcp -m tcp --dport 443 -m comment --comment agent-net:%s -j ACCEPT", src, ip, ctr)
}

// writeHost records the endpoints file installs (the optional services.FileInstaller), noting how many rule changes
// had happened by then.
type writeHost struct {
	*fakeHost
	fr      *fakeRunner
	err     error
	writes  []string
	data    map[string]string
	mutsAtW []int
}

func (h *writeHost) InstallFile(p string, data []byte, perm fs.FileMode) error {
	if h.err != nil {
		return h.err
	}
	if perm != 0o644 {
		return fmt.Errorf("mode %04o, want 0644", perm)
	}
	h.writes = append(h.writes, p)
	h.data[p] = string(data)
	h.mutsAtW = append(h.mutsAtW, mutations(h.fr))
	return nil
}

const newJSON = `{"endpoints": ["api.anthropic.com", "new.example.com"]}`

// writeSetup is refreshSetup with a host that records installs; new.example.com resolves to ipNew.
func writeSetup() (*fakeRunner, *writeHost) {
	fr, h := refreshSetup()
	h.lookup["new.example.com"] = []string{ipNew}
	return fr, &writeHost{fakeHost: h, fr: fr, data: map[string]string{}}
}

// refreshSetup is a host with the endpoints file and two armed containers plus the gate rule.
func refreshSetup() (*fakeRunner, *fakeHost) {
	fr := newRunner()
	fr.netExists = true
	fr.ipt["DOCKER-USER"] = []string{
		epRule("web-1", "172.30.0.2", ipAPI),
		epRule("web-1", "172.30.0.2", ipPlat),
		dnsRule,
		denyRule,
		epRule("web-2", "172.30.0.3", ipAPI),
		epRule("web-2", "172.30.0.3", ipPlat),
		epRule("gate", "172.30.0.9", "1.2.3.4"), // shaped like an endpoint rule, but the gate tag
	}
	h := newHost()
	h.files[epFile] = epJSON
	return fr, h
}

// runRefresh feeds epJSON on stdin.
func runRefresh(fr *fakeRunner, h any, args ...string) result {
	return runRefreshStdin(fr, h, epJSON, args...)
}

func runRefreshStdin(fr *fakeRunner, h any, stdin string, args ...string) result {
	var out, errb bytes.Buffer
	d := sequence.Deps{Config: config.Config{LogLevel: "INFO", EndpointsFile: epFile}, Runner: fr, Args: args,
		Stdin: strings.NewReader(stdin), Stdout: &out, Stderr: &errb}
	switch v := h.(type) {
	case *fakeHost:
		d.Host = v
	case *writeHost:
		d.Host = v
	}
	code := sequence.RunRefresh(context.Background(), d)
	return result{code, out.String(), errb.String()}
}

func mutations(fr *fakeRunner) int { return fr.count("iptables -I") + fr.count("iptables -D") }

func hasRule(fr *fakeRunner, rule string) bool {
	for _, l := range fr.ipt["DOCKER-USER"] {
		if l == rule {
			return true
		}
	}
	return false
}

func TestRefreshUnchangedMakesNoChanges(t *testing.T) {
	fr, h := writeSetup()
	h.files[epFile] = epJSON
	r := runRefresh(fr, h)
	if r.code != 0 || mutations(fr) != 0 {
		t.Fatalf("exit %d, mutations %d, stderr %q", r.code, mutations(fr), r.err)
	}
	if !strings.Contains(r.stdout, "web-1 (172.30.0.2): added 0, removed 0, unchanged 2") {
		t.Errorf("summary missing: %q", r.stdout)
	}
}

func TestRefreshRotatedIPInsertsBeforeDeleting(t *testing.T) {
	fr, h := refreshSetup()
	h.lookup["api.anthropic.com"] = []string{ipNew}
	r := runRefresh(fr, h)
	if r.code != 0 {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	for _, c := range []struct{ ctr, src string }{{"web-1", "172.30.0.2"}, {"web-2", "172.30.0.3"}} {
		ins := "iptables -I DOCKER-USER 1 " + epRule(c.ctr, c.src, ipNew)
		del := "iptables -D DOCKER-USER " + epRule(c.ctr, c.src, ipAPI)
		if !fr.has(ins) || !fr.has(del) {
			t.Fatalf("%s: calls %v", c.ctr, fr.calls)
		}
		if fr.index(ins) > fr.index(del) {
			t.Errorf("%s: the new rule must be inserted before the stale one is deleted", c.ctr)
		}
		if !hasRule(fr, epRule(c.ctr, c.src, ipNew)) || hasRule(fr, epRule(c.ctr, c.src, ipAPI)) || !hasRule(fr, epRule(c.ctr, c.src, ipPlat)) {
			t.Errorf("%s: wrong rules left: %v", c.ctr, fr.ipt["DOCKER-USER"])
		}
	}
	if !hasRule(fr, dnsRule) || !hasRule(fr, denyRule) {
		t.Errorf("DNS and default-deny rules must stay: %v", fr.ipt["DOCKER-USER"])
	}
	if fr.count("iptables -I") != 2 || fr.count("iptables -D") != 2 {
		t.Errorf("expected 2 inserts and 2 deletes, calls %v", fr.calls)
	}
	// a second run converges: nothing to change
	if r2 := runRefresh(fr, h); r2.code != 0 || mutations(fr) != 4 {
		t.Errorf("second run must change nothing: exit %d, mutations %d", r2.code, mutations(fr))
	}
}

func TestRefreshLookupFailureChangesNothing(t *testing.T) {
	fr, h := refreshSetup()
	h.lookupErr = fmt.Errorf("servfail")
	r := runRefresh(fr, h)
	if r.code != 2 || mutations(fr) != 0 {
		t.Errorf("exit %d, mutations %d, stderr %q", r.code, mutations(fr), r.err)
	}
}

func TestRefreshIPv6OnlyNameFails(t *testing.T) {
	fr, h := refreshSetup()
	h.lookup["platform.claude.com"] = nil // only AAAA records
	r := runRefresh(fr, h)
	if r.code != 2 || mutations(fr) != 0 || !strings.Contains(r.err, "no IPv4 address") {
		t.Errorf("exit %d, mutations %d, stderr %q", r.code, mutations(fr), r.err)
	}
}

func TestRefreshBadStdinExits1(t *testing.T) {
	for name, content := range map[string]string{
		"empty":            ``,
		"blank":            " \n\t\n",
		"empty list":       `{"endpoints": []}`,
		"missing key":      `{}`,
		"duplicate":        `{"endpoints": ["a.example.com", "A.example.com"]}`,
		"bad hostname":     `{"endpoints": ["bad host"]}`,
		"bad ip":           `{"endpoints": ["999.1.1.1"]}`,
		"not json":         `api.anthropic.com`,
		"unknown key":      `{"endpoints": ["a.example.com"], "x": 1}`,
		"trailing garbage": `{"endpoints":["a.example"]} x`,
		"two objects":      `{"endpoints":["a.example"]}{"endpoints":["b.example"]}`,
		"oversize":         `{"endpoints": ["a.example.com"]}` + strings.Repeat(" ", 64<<10),
	} {
		fr, h := writeSetup()
		r := runRefreshStdin(fr, h, content)
		if r.code != 1 || len(fr.calls) != 0 || len(h.writes) != 0 {
			t.Errorf("%s: exit %d, calls %v, writes %v, stderr %q", name, r.code, fr.calls, h.writes, r.err)
		}
	}
	// exactly 64 KiB is accepted
	fr, h := writeSetup()
	h.files[epFile] = epJSON
	exact := epJSON + strings.Repeat(" ", 64<<10-len(epJSON))
	if r := runRefreshStdin(fr, h, exact); r.code != 0 {
		t.Errorf("64 KiB: exit %d, stderr %q", r.code, r.err)
	}
}

func TestRefreshBadInvocationExits1(t *testing.T) {
	for _, args := range [][]string{{"--bogus"}, {"--config"}, {"--config", "/x/endpoints.json"}, {"extra"}, {"--dry-run", "extra"}} {
		fr, h := writeSetup()
		if r := runRefresh(fr, h, args...); r.code != 1 || !strings.Contains(r.err, "usage: refresh_endpoints [--dry-run]") || len(fr.calls) != 0 || len(h.writes) != 0 {
			t.Errorf("args %q: exit %d, stderr %q", args, r.code, r.err)
		}
	}
}

func TestRefreshGateTagExcluded(t *testing.T) {
	fr, h := refreshSetup()
	h.lookup = map[string][]string{"api.anthropic.com": {ipNew}, "platform.claude.com": {ipNew}}
	r := runRefresh(fr, h)
	if r.code != 0 {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	gate := epRule("gate", "172.30.0.9", "1.2.3.4")
	if !hasRule(fr, gate) {
		t.Errorf("the gate-tagged rule must stay: %v", fr.ipt["DOCKER-USER"])
	}
	for _, c := range fr.calls {
		if strings.Contains(c.joined(), "agent-net:gate") && (strings.HasPrefix(c.joined(), "iptables -D") || strings.HasPrefix(c.joined(), "iptables -I")) {
			t.Errorf("gate rule touched: %s", c.joined())
		}
	}
	if strings.Contains(r.stdout, "gate") {
		t.Errorf("gate must not be listed: %q", r.stdout)
	}
}

func TestRefreshDryRunChangesNothing(t *testing.T) {
	fr, h := refreshSetup()
	h.lookup["api.anthropic.com"] = []string{ipNew}
	r := runRefresh(fr, h, "--dry-run")
	if r.code != 0 || mutations(fr) != 0 {
		t.Fatalf("exit %d, mutations %d, stderr %q", r.code, mutations(fr), r.err)
	}
	if !strings.Contains(r.stdout, "would add 1, would remove 1, unchanged 1") || !strings.Contains(r.stdout, "+ "+ipNew) || !strings.Contains(r.stdout, "- "+ipAPI) {
		t.Errorf("plan missing: %q", r.stdout)
	}
}

func TestRefreshFailedInsertStopsBeforeDelete(t *testing.T) {
	fr, h := refreshSetup()
	h.lookup["api.anthropic.com"] = []string{ipNew}
	fr.rules = []rule{{contains: "iptables -I DOCKER-USER", stderr: "boom", code: 1}}
	r := runRefresh(fr, h)
	if r.code != 2 || fr.count("iptables -D") != 0 || !strings.Contains(r.err, "REFRESH_INCOMPLETE") {
		t.Errorf("exit %d, deletes %d, stderr %q", r.code, fr.count("iptables -D"), r.err)
	}
}

func TestRefreshStdinInstallsThenUpdatesRules(t *testing.T) {
	fr, h := writeSetup()
	r := runRefreshStdin(fr, h, newJSON)
	if r.code != 0 {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if len(h.writes) != 1 || h.writes[0] != epFile || h.data[epFile] != newJSON {
		t.Fatalf("writes %v, data %v", h.writes, h.data)
	}
	if h.mutsAtW[0] != 0 {
		t.Errorf("the file must be written before the first rule change, %d changes had happened", h.mutsAtW[0])
	}
	// the rules follow the installed list: the new address is added, the platform.claude.com one is stale
	for _, c := range []struct{ ctr, src string }{{"web-1", "172.30.0.2"}, {"web-2", "172.30.0.3"}} {
		if !hasRule(fr, epRule(c.ctr, c.src, ipNew)) || hasRule(fr, epRule(c.ctr, c.src, ipPlat)) || !hasRule(fr, epRule(c.ctr, c.src, ipAPI)) {
			t.Errorf("%s: wrong rules: %v", c.ctr, fr.ipt["DOCKER-USER"])
		}
	}
}

func TestRefreshStdinIdenticalBytesWritesNothing(t *testing.T) {
	fr, h := writeSetup() // the installed file holds epJSON
	r := runRefresh(fr, h)
	if r.code != 0 || len(h.writes) != 0 {
		t.Fatalf("exit %d, writes %v, stderr %q", r.code, h.writes, r.err)
	}
	if !strings.Contains(r.stdout, "not written") {
		t.Errorf("the skipped write must be logged: %q", r.stdout)
	}
	// different bytes, same endpoints: still written (byte comparison)
	fr, h = writeSetup()
	if r := runRefreshStdin(fr, h, "{\"endpoints\": [\"api.anthropic.com\", \"platform.claude.com\"]}\n"); r.code != 0 || len(h.writes) != 1 {
		t.Errorf("differing bytes: exit %d, writes %v", r.code, h.writes)
	}
}

func TestRefreshStdinNoInstalledFileInstalls(t *testing.T) {
	fr, h := writeSetup()
	delete(h.files, epFile)
	if r := runRefresh(fr, h); r.code != 0 || len(h.writes) != 1 || h.data[epFile] != epJSON {
		t.Errorf("exit %d, writes %v, data %v, stderr %q", r.code, h.writes, h.data, r.err)
	}
}

func TestRefreshStdinNoArmedContainersInstallsOnly(t *testing.T) {
	fr, h := writeSetup()
	fr.ipt["DOCKER-USER"] = nil
	r := runRefreshStdin(fr, h, newJSON)
	if r.code != 0 || len(h.writes) != 1 || mutations(fr) != 0 {
		t.Errorf("exit %d, writes %v, mutations %d, stderr %q", r.code, h.writes, mutations(fr), r.err)
	}
}

func TestRefreshStdinResolveFailureWritesNothing(t *testing.T) {
	fr, h := writeSetup()
	h.lookupErr = fmt.Errorf("servfail")
	r := runRefreshStdin(fr, h, newJSON)
	if r.code != 2 || len(h.writes) != 0 || mutations(fr) != 0 {
		t.Errorf("exit %d, writes %v, mutations %d, stderr %q", r.code, h.writes, mutations(fr), r.err)
	}
	fr, h = writeSetup()
	h.lookup["new.example.com"] = nil // only AAAA records
	if r := runRefreshStdin(fr, h, newJSON); r.code != 2 || len(h.writes) != 0 || mutations(fr) != 0 {
		t.Errorf("IPv6-only: exit %d, writes %v, mutations %d", r.code, h.writes, mutations(fr))
	}
}

func TestRefreshStdinWriteFailureChangesNoRule(t *testing.T) {
	fr, h := writeSetup()
	h.err = fmt.Errorf("read-only file system")
	r := runRefreshStdin(fr, h, newJSON)
	if r.code != 2 || mutations(fr) != 0 || !strings.Contains(r.err, "could not be written") {
		t.Errorf("exit %d, mutations %d, stderr %q", r.code, mutations(fr), r.err)
	}
}

func TestRefreshStdinDryRunWritesNothing(t *testing.T) {
	fr, h := writeSetup()
	r := runRefreshStdin(fr, h, newJSON, "--dry-run")
	if r.code != 0 || len(h.writes) != 0 || mutations(fr) != 0 {
		t.Fatalf("exit %d, writes %v, mutations %d, stderr %q", r.code, h.writes, mutations(fr), r.err)
	}
	if !strings.Contains(r.stdout, "would install stdin as "+epFile) || !strings.Contains(r.stdout, "+ "+ipNew) {
		t.Errorf("plan missing: %q", r.stdout)
	}
}

func TestRefreshStdinNeedsPrivilege(t *testing.T) {
	fr, h := writeSetup()
	h.euid = 1000
	if r := runRefresh(fr, h); r.code != 1 || len(h.writes) != 0 || mutations(fr) != 0 {
		t.Errorf("exit %d, writes %v, mutations %d, stderr %q", r.code, h.writes, mutations(fr), r.err)
	}
}

func TestEndpointsFileReplacesDefaultsInConfig(t *testing.T) {
	env := func(k string) string { return "" }
	read := func(string) ([]byte, error) { return []byte(`{"endpoints": ["a.example.com"]}`), nil }
	stat := func(string) (int, fs.FileMode, error) { return 0, 0o644, nil }
	if c := config.LoadWith(env, read, stat); c.EndpointsErr != nil || len(c.ModelAPIs) != 1 || c.ModelAPIs[0] != "a.example.com" {
		t.Errorf("file not used: %+v", c)
	}
	missing := func(p string) ([]byte, error) { return nil, &fs.PathError{Op: "open", Path: p, Err: fs.ErrNotExist} }
	missingStat := func(p string) (int, fs.FileMode, error) {
		return 0, 0, &fs.PathError{Op: "stat", Path: p, Err: fs.ErrNotExist}
	}
	if c := config.LoadWith(env, missing, missingStat); c.EndpointsErr != nil || len(c.ModelAPIs) != 2 {
		t.Errorf("defaults expected without a file: %+v", c)
	}
	named := func(k string) string {
		if k == config.EnvEndpointsFile {
			return "/x/endpoints.json"
		}
		return ""
	}
	if c := config.LoadWith(named, missing, missingStat); c.EndpointsErr == nil {
		t.Error("a named but missing file must be an error")
	}
	writable := func(string) (int, fs.FileMode, error) { return 0, 0o666, nil }
	if c := config.LoadWith(env, read, writable); c.EndpointsErr == nil {
		t.Error("a writable file must be refused")
	}
}
