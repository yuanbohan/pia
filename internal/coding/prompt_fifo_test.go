//go:build darwin || linux

package coding

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestBuildSystemPromptRejectsFIFOWithoutFallingThrough(t *testing.T) {
	directory := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(directory, "AGENTS.md"), 0o600); err != nil {
		t.Fatalf("create instruction FIFO: %v", err)
	}
	writePromptFile(t, directory, "CLAUDE.md", []byte("lower-priority guidance"))
	workspace := openPromptWorkspace(t, directory)

	prompt, err := buildSystemPrompt(workspace, promptTools(t, workspace))
	if err == nil {
		t.Fatalf("build system prompt unexpectedly succeeded\n%s", prompt)
	}
	if strings.Contains(prompt, "lower-priority guidance") {
		t.Fatalf("prompt fell through to lower-priority instructions\n%s", prompt)
	}
}

func TestBuildSystemPromptRejectsUnreadableFileWithoutFallingThrough(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read files regardless of permission bits")
	}
	directory := t.TempDir()
	path := filepath.Join(directory, "AGENTS.md")
	writePromptFile(t, directory, "AGENTS.md", []byte("unreadable guidance"))
	if err := os.Chmod(path, 0); err != nil {
		t.Fatalf("remove instruction permissions: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
	writePromptFile(t, directory, "CLAUDE.md", []byte("lower-priority guidance"))
	workspace := openPromptWorkspace(t, directory)

	prompt, err := buildSystemPrompt(workspace, promptTools(t, workspace))
	if err == nil {
		t.Fatalf("build system prompt unexpectedly succeeded\n%s", prompt)
	}
	if strings.Contains(prompt, "lower-priority guidance") {
		t.Fatalf("prompt fell through to lower-priority instructions\n%s", prompt)
	}
}
