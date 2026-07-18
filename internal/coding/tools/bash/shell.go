package bash

import (
	"fmt"
	"os"
	"os/exec"
)

func resolveShell(explicitPath string) (string, error) {
	if explicitPath != "" {
		if _, err := os.Stat(explicitPath); err != nil {
			return "", fmt.Errorf("custom shell path %q: %w", explicitPath, err)
		}
		return explicitPath, nil
	}

	// Preserve frozen Pi's Unix resolution order instead of consulting SHELL:
	// the user's interactive shell may be zsh, while model commands rely on
	// Bash-compatible syntax and run in a fresh non-login process.
	if _, err := os.Stat("/bin/bash"); err == nil {
		return "/bin/bash", nil
	}
	if path, err := exec.LookPath("bash"); err == nil {
		return path, nil
	}
	if path, err := exec.LookPath("sh"); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("no bash or sh executable found")
}
