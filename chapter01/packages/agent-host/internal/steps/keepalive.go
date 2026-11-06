// Keep-alive (part of USC-001 step 11): WSL stops agent-host when no wsl.exe session is attached, which kills
// dockerd and every agent container. A hidden per-user wsl.exe holds one session open; a Startup-folder
// script starts it at Windows logon and this tool starts it once now. Skipped with --no-keepalive.
package steps

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agent-host/internal/config"
)

// KeepAliveFileName is the script written to the user's Startup folder.
const KeepAliveFileName = "agent-host-keepalive.cmd"

// KeepAliveScript is the exact content of the Startup script (CRLF line endings).
var KeepAliveScript = strings.Join([]string{
	"@echo off",
	"rem agent-host keep-alive: holds one wsl.exe session so WSL does not idle-stop the agent-host distro.",
	"rem Remove this file to disable. Written by create_agent_host.exe.",
	`start "" /b conhost.exe --headless wsl.exe -d agent-host -e sleep infinity`,
	"",
}, "\r\n")

// KeepAliveCommand is the process started now: a headless console host running a sleeping wsl.exe session.
var KeepAliveCommand = []string{"conhost.exe", "--headless", config.WSLExe, "-d", config.DistroName, "-e", "sleep", "infinity"}

// KeepAlivePath returns the Startup-folder path of the script under appData (%APPDATA%).
func KeepAlivePath(appData string) (string, error) {
	if appData == "" {
		return "", errors.New("the APPDATA environment variable is not set; cannot locate the Startup folder (use --no-keepalive to skip the keep-alive)")
	}
	return filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", "Startup", KeepAliveFileName), nil
}

// InstallKeepAlive writes the Startup script if missing or different, then starts the keep-alive detached.
func InstallKeepAlive(_ context.Context, env *Env) *Failure {
	if !env.Config.KeepAlive {
		env.Log.Debugf("keep-alive skipped (--no-keepalive)")
		return nil
	}
	path, err := KeepAlivePath(env.Config.AppData)
	if err != nil {
		return fail(ExitRuntime, StatePartiallyConfigured, "keep-alive", err)
	}
	if cur, err := os.ReadFile(path); err != nil || !bytes.Equal(cur, []byte(KeepAliveScript)) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fail(ExitRuntime, StatePartiallyConfigured, "creating the Startup folder failed", err)
		}
		if err := os.WriteFile(path, []byte(KeepAliveScript), 0o644); err != nil {
			return fail(ExitRuntime, StatePartiallyConfigured, "writing "+path+" failed", err)
		}
	}
	// A keep-alive that is already running makes this a harmless extra session.
	if err := env.Starter.Start(KeepAliveCommand[0], KeepAliveCommand[1:]...); err != nil {
		return fail(ExitRuntime, StatePartiallyConfigured, "starting the keep-alive process failed", err)
	}
	return nil
}

// VerifyKeepAliveFile checks the Startup script exists with the expected content (the process is not checked).
func VerifyKeepAliveFile(appData string) error {
	path, err := KeepAlivePath(appData)
	if err != nil {
		return err
	}
	cur, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(cur, []byte(KeepAliveScript)) {
		return fmt.Errorf("%s content differs from the expected script", path)
	}
	return nil
}
