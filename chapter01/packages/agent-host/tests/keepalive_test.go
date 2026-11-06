package tests

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"

	"agent-host/internal/config"
	"agent-host/internal/steps"
)

const wantKeepAliveScript = "@echo off\r\n" +
	"rem agent-host keep-alive: holds one wsl.exe session so WSL does not idle-stop the agent-host distro.\r\n" +
	"rem Remove this file to disable. Written by create_agent_host.exe.\r\n" +
	"start \"\" /b conhost.exe --headless wsl.exe -d agent-host -e sleep infinity\r\n"

func startupFile(t *testing.T, appData string) string {
	t.Helper()
	p, err := steps.KeepAlivePath(appData)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(p, "agent-host-keepalive.cmd") || !strings.Contains(p, "Startup") {
		t.Fatalf("unexpected path %q", p)
	}
	return p
}

func TestToolNamesAreSeven(t *testing.T) {
	want := []string{"create_agent_container", "remove_agent_container", "create_agent_net", "remove_agent_net", "agent_lifecycle", "enter_agent", "refresh_endpoints"}
	if !reflect.DeepEqual(config.ToolNames, want) {
		t.Errorf("ToolNames = %v, want %v", config.ToolNames, want)
	}
}

func TestKeepAliveScriptWrittenWithExactContent(t *testing.T) {
	appData := t.TempDir()
	r := run(t, newRootfsServer(t), &fakeRunner{}, map[string]string{"APPDATA": appData})
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	got, err := os.ReadFile(startupFile(t, appData))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != wantKeepAliveScript {
		t.Errorf("script = %q, want %q", got, wantKeepAliveScript)
	}
	if steps.KeepAliveScript != wantKeepAliveScript {
		t.Error("steps.KeepAliveScript differs from the specified content")
	}
}

func TestKeepAliveStartedDetached(t *testing.T) {
	r := run(t, newRootfsServer(t), &fakeRunner{}, nil)
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	want := [][]string{{"conhost.exe", "--headless", "wsl.exe", "-d", "agent-host", "-e", "sleep", "infinity"}}
	if !reflect.DeepEqual(lastStarter.cmds, want) {
		t.Errorf("starts = %v, want %v", lastStarter.cmds, want)
	}
}

func TestKeepAliveNotRewrittenWhenIdentical(t *testing.T) {
	appData := t.TempDir()
	p := startupFile(t, appData)
	if err := os.MkdirAll(p[:strings.LastIndex(p, string(os.PathSeparator))], 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(wantKeepAliveScript), 0o444); err != nil { // read-only: a rewrite attempt would fail the run
		t.Fatal(err)
	}
	before, _ := os.Stat(p)
	r := run(t, newRootfsServer(t), &fakeRunner{}, map[string]string{"APPDATA": appData})
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	after, _ := os.Stat(p)
	if !after.ModTime().Equal(before.ModTime()) {
		t.Error("identical script was rewritten")
	}
	if len(lastStarter.cmds) != 1 {
		t.Errorf("keep-alive must still be started, got %v", lastStarter.cmds)
	}
}

func TestKeepAliveRewrittenWhenDifferent(t *testing.T) {
	appData := t.TempDir()
	p := startupFile(t, appData)
	if err := os.MkdirAll(p[:strings.LastIndex(p, string(os.PathSeparator))], 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("@echo off\r\nrem stale\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := run(t, newRootfsServer(t), &fakeRunner{}, map[string]string{"APPDATA": appData})
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	if got, _ := os.ReadFile(p); string(got) != wantKeepAliveScript {
		t.Errorf("script = %q, want %q", got, wantKeepAliveScript)
	}
}

func TestNoKeepAliveSkipsWriteStartAndVerify(t *testing.T) {
	appData := t.TempDir()
	r := runWith(t, newRootfsServer(t), &fakeRunner{}, map[string]string{"APPDATA": appData}, func(c *config.Config) { c.KeepAlive = false })
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
	if _, err := os.Stat(startupFile(t, appData)); !os.IsNotExist(err) {
		t.Errorf("Startup script must not be written, stat err = %v", err)
	}
	if len(lastStarter.cmds) != 0 {
		t.Errorf("keep-alive must not be started: %v", lastStarter.cmds)
	}
}

func TestNoKeepAliveDoesNotRequireAppData(t *testing.T) {
	r := runWith(t, newRootfsServer(t), &fakeRunner{}, map[string]string{"APPDATA": ""}, func(c *config.Config) { c.KeepAlive = false })
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.err)
	}
}

func TestMissingAppDataFailsBeforeAnyChange(t *testing.T) {
	rs, fr := newRootfsServer(t), &fakeRunner{}
	r := run(t, rs, fr, map[string]string{"APPDATA": ""})
	if r.code != 1 || !strings.Contains(r.err, "APPDATA") || !strings.Contains(r.err, "NOT_CREATED") {
		t.Fatalf("exit %d, stderr %q", r.code, r.err)
	}
	if fr.has("--import") {
		t.Error("import must not run without APPDATA")
	}
	if len(lastStarter.cmds) != 0 {
		t.Error("keep-alive must not be started")
	}
}

func TestKeepAliveStartFailureExits2(t *testing.T) {
	// Drive the step directly with a failing Starter. env.Log is unused on this path (KeepAlive is on).
	appData := t.TempDir()
	env := &steps.Env{Config: config.Config{KeepAlive: true, AppData: appData}, Starter: &fakeStarter{err: os.ErrPermission}}
	f := steps.InstallKeepAlive(context.Background(), env)
	if f == nil || f.ExitCode != 2 || !strings.Contains(f.Message, "starting the keep-alive process failed") {
		t.Fatalf("failure = %+v", f)
	}
}

func TestVerifyKeepAliveFile(t *testing.T) {
	appData := t.TempDir()
	if err := steps.VerifyKeepAliveFile(appData); err == nil {
		t.Error("missing script must fail verification")
	}
	p := startupFile(t, appData)
	if err := os.MkdirAll(p[:strings.LastIndex(p, string(os.PathSeparator))], 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("@echo off\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := steps.VerifyKeepAliveFile(appData); err == nil {
		t.Error("differing script must fail verification")
	}
	if err := os.WriteFile(p, []byte(wantKeepAliveScript), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := steps.VerifyKeepAliveFile(appData); err != nil {
		t.Errorf("expected script rejected: %v", err)
	}
	if err := steps.VerifyKeepAliveFile(""); err == nil {
		t.Error("empty APPDATA must fail")
	}
}

func TestParseArgs(t *testing.T) {
	if no, err := config.ParseArgs(nil); err != nil || no {
		t.Errorf("no args: %v %v", no, err)
	}
	if no, err := config.ParseArgs([]string{"--no-keepalive"}); err != nil || !no {
		t.Errorf("--no-keepalive: %v %v", no, err)
	}
	for _, bad := range [][]string{{"--bogus"}, {"--no-keepalive", "extra"}, {"-no-keepalive="}} {
		if _, err := config.ParseArgs(bad); err == nil {
			t.Errorf("%v accepted", bad)
		}
	}
	if !config.Load(func(string) string { return "" }).KeepAlive {
		t.Error("keep-alive must default to enabled")
	}
}
