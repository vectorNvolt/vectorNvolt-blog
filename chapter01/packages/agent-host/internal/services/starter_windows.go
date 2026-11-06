//go:build windows

package services

import (
	"os/exec"
	"syscall"
)

const (
	detachedProcess     = 0x00000008 // DETACHED_PROCESS
	createNewProcessGrp = 0x00000200 // CREATE_NEW_PROCESS_GROUP
)

// Start runs exe detached from this console and process group, so it keeps running after this process exits.
func (DetachedStarter) Start(exe string, args ...string) error {
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: detachedProcess | createNewProcessGrp, HideWindow: true}
	return startAndRelease(cmd)
}
