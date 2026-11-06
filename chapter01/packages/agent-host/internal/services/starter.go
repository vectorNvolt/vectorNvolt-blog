// Detached process start for the Windows keep-alive (tests replace the Starter with a fake).
package services

import "os/exec"

// Starter starts a program in the background without waiting for it; the program must survive
// the exit of this process.
type Starter interface {
	Start(exe string, args ...string) error
}

// DetachedStarter is the real Starter; the platform detail is in starter_windows.go / starter_other.go.
type DetachedStarter struct{}

// startAndRelease starts cmd and drops the handle so the child is not waited for.
func startAndRelease(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
