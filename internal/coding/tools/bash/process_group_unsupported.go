//go:build !darwin && !linux

package bash

import (
	"fmt"
	"os/exec"
)

func configureProcessGroup(_ *exec.Cmd) error {
	return fmt.Errorf("bash process groups are supported only on macOS and Linux in Phase 1")
}

func killProcessGroup(_ *exec.Cmd) error {
	return nil
}
