//go:build !windows

package services

import "os/exec"

// Start runs exe in the background (the keep-alive itself is Windows-only; this keeps other platforms compiling).
func (DetachedStarter) Start(exe string, args ...string) error {
	return startAndRelease(exec.Command(exe, args...))
}
