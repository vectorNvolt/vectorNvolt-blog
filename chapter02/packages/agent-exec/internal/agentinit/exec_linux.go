package agentinit

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// ExecStarter starts real processes with stdout and stderr inherited (the container log).
type ExecStarter struct{}

type execProcess struct{ cmd *exec.Cmd }

func (ExecStarter) Start(spec Spec) (Process, error) {
	cmd := exec.Command(spec.Path, spec.Args...)
	cmd.Env = spec.Env
	cmd.ExtraFiles = spec.ExtraFiles
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if spec.Cred != nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{
			Uid: spec.Cred.Uid, Gid: spec.Cred.Gid, Groups: []uint32{spec.Cred.Gid},
		}}
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &execProcess{cmd: cmd}, nil
}

func (p *execProcess) Wait() (int, error) {
	err := p.cmd.Wait()
	var ee *exec.ExitError
	if err == nil {
		return 0, nil
	}
	if errors.As(err, &ee) {
		return ee.ExitCode(), nil // -1 when killed by a signal
	}
	return -1, err
}

func (p *execProcess) Signal(sig os.Signal) error { return p.cmd.Process.Signal(sig) }
