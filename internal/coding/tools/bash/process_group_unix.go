//go:build darwin || linux

package bash

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func configureProcessGroup(command *exec.Cmd) error {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return nil
}

func killProcessGroup(command *exec.Cmd) error {
	if command.Process == nil {
		return nil
	}
	// A negative PID addresses the process group created for this invocation.
	// Killing only exec.Cmd.Process would leave grandchildren such as test
	// runners or shell pipelines alive after timeout or Run cancellation.
	err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	if err == nil || errors.Is(err, syscall.ESRCH) {
		return nil
	}
	if directErr := command.Process.Kill(); directErr != nil && !errors.Is(directErr, os.ErrProcessDone) {
		return errors.Join(err, directErr)
	}
	return err
}
