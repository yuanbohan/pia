package coding

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yuanbohan/pi-go/internal/agent"
)

func TestBuildSystemPromptUsesCanonicalWorkspaceAndRealTools(t *testing.T) {
	realDirectory := t.TempDir()
	link := filepath.Join(t.TempDir(), "workspace-link")
	if err := os.Symlink(realDirectory, link); err != nil {
		t.Fatalf("create workspace symlink: %v", err)
	}

	workspace := openPromptWorkspace(t, link)
	tools := promptTools(t, workspace)
	prompt, err := buildSystemPrompt(workspace, tools)
	if err != nil {
		t.Fatalf("build system prompt: %v", err)
	}

	for _, fragment := range []string{
		"You are pia, a coding agent",
		"Current working directory: " + workspace.Path(),
		"Run relevant checks",
		"Keep the final response concise",
	} {
		if !strings.Contains(prompt, fragment) {
			t.Errorf("prompt does not contain %q\n%s", fragment, prompt)
		}
	}
	for _, tool := range tools {
		schema := tool.Definition().Schema
		fragment := "- " + schema.Name + ": " + schema.Description
		if !strings.Contains(prompt, fragment) {
			t.Errorf("prompt does not describe actual tool %q\n%s", schema.Name, prompt)
		}
	}
	if strings.Contains(prompt, link) {
		t.Fatalf("prompt contains non-canonical workspace path %q\n%s", link, prompt)
	}
	if strings.Contains(prompt, "Project instructions from") {
		t.Fatalf("prompt contains an empty project-instructions section\n%s", prompt)
	}
}

func TestBuildSystemPromptInstructionPrecedence(t *testing.T) {
	tests := []struct {
		name        string
		files       map[string]string
		wantName    string
		wantContent string
		notContent  string
	}{
		{
			name: "AGENTS md wins over CLAUDE md",
			files: map[string]string{
				"AGENTS.md": "agents guidance",
				"CLAUDE.md": "claude guidance",
			},
			wantName:    "AGENTS.md",
			wantContent: "agents guidance",
			notContent:  "claude guidance",
		},
		{
			name: "uppercase CLAUDE fallback",
			files: map[string]string{
				"CLAUDE.MD": "uppercase guidance",
			},
			wantName:    "CLAUDE.MD",
			wantContent: "uppercase guidance",
		},
		{
			name: "uppercase AGENTS wins over CLAUDE",
			files: map[string]string{
				"AGENTS.MD": "uppercase agents guidance",
				"CLAUDE.md": "lower-priority claude guidance",
			},
			wantName:    "AGENTS.MD",
			wantContent: "uppercase agents guidance",
			notContent:  "lower-priority claude guidance",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			for name, content := range test.files {
				writePromptFile(t, directory, name, []byte(content))
			}
			workspace := openPromptWorkspace(t, directory)

			prompt, err := buildSystemPrompt(workspace, promptTools(t, workspace))
			if err != nil {
				t.Fatalf("build system prompt: %v", err)
			}
			if !strings.Contains(prompt, "Project instructions from "+test.wantName+":") {
				t.Fatalf("prompt does not identify selected instructions\n%s", prompt)
			}
			if !strings.Contains(prompt, test.wantContent) {
				t.Fatalf("prompt does not contain selected instructions\n%s", prompt)
			}
			if test.notContent != "" && strings.Contains(prompt, test.notContent) {
				t.Fatalf("prompt contains lower-priority instructions\n%s", prompt)
			}
		})
	}
}

func TestBuildSystemPromptAcceptsInstructionSizeLimit(t *testing.T) {
	directory := t.TempDir()
	instructions := strings.Repeat("x", maxProjectInstructionsBytes)
	writePromptFile(t, directory, "AGENTS.md", []byte(instructions))
	workspace := openPromptWorkspace(t, directory)

	prompt, err := buildSystemPrompt(workspace, promptTools(t, workspace))
	if err != nil {
		t.Fatalf("build system prompt: %v", err)
	}
	if !strings.Contains(prompt, instructions) {
		t.Fatal("prompt does not contain instructions at the exact size limit")
	}
}

func TestBuildSystemPromptIgnoresAncestorAndChildInstructions(t *testing.T) {
	parent := t.TempDir()
	directory := filepath.Join(parent, "workspace")
	if err := os.MkdirAll(filepath.Join(directory, "child"), 0o755); err != nil {
		t.Fatalf("create workspace directories: %v", err)
	}
	writePromptFile(t, parent, "AGENTS.md", []byte("ancestor guidance"))
	writePromptFile(t, filepath.Join(directory, "child"), "AGENTS.md", []byte("child guidance"))

	workspace := openPromptWorkspace(t, directory)
	prompt, err := buildSystemPrompt(workspace, promptTools(t, workspace))
	if err != nil {
		t.Fatalf("build system prompt: %v", err)
	}
	if strings.Contains(prompt, "guidance") || strings.Contains(prompt, "Project instructions from") {
		t.Fatalf("prompt discovered instructions outside the workspace root\n%s", prompt)
	}
}

func TestBuildSystemPromptFollowsInternalInstructionSymlink(t *testing.T) {
	directory := t.TempDir()
	writePromptFile(t, directory, "guidance.txt", []byte("internal linked guidance"))
	if err := os.Symlink("guidance.txt", filepath.Join(directory, "AGENTS.md")); err != nil {
		t.Fatalf("create internal instruction symlink: %v", err)
	}
	workspace := openPromptWorkspace(t, directory)

	prompt, err := buildSystemPrompt(workspace, promptTools(t, workspace))
	if err != nil {
		t.Fatalf("build system prompt: %v", err)
	}
	if !strings.Contains(prompt, "internal linked guidance") {
		t.Fatalf("prompt does not contain internally linked instructions\n%s", prompt)
	}
}

func TestBuildSystemPromptRejectsInvalidHigherPriorityCandidate(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, directory string)
	}{
		{
			name: "escaping symlink",
			setup: func(t *testing.T, directory string) {
				outside := filepath.Join(t.TempDir(), "outside.txt")
				if err := os.WriteFile(outside, []byte("outside secret"), 0o600); err != nil {
					t.Fatalf("write outside target: %v", err)
				}
				if err := os.Symlink(outside, filepath.Join(directory, "AGENTS.md")); err != nil {
					t.Fatalf("create escaping symlink: %v", err)
				}
			},
		},
		{
			name: "dangling symlink",
			setup: func(t *testing.T, directory string) {
				if err := os.Symlink("missing.txt", filepath.Join(directory, "AGENTS.md")); err != nil {
					t.Fatalf("create dangling symlink: %v", err)
				}
			},
		},
		{
			name: "directory",
			setup: func(t *testing.T, directory string) {
				if err := os.Mkdir(filepath.Join(directory, "AGENTS.md"), 0o755); err != nil {
					t.Fatalf("create instruction directory: %v", err)
				}
			},
		},
		{
			name: "invalid UTF-8",
			setup: func(t *testing.T, directory string) {
				writePromptFile(t, directory, "AGENTS.md", []byte{0xff, 0xfe})
			},
		},
		{
			name: "over size limit",
			setup: func(t *testing.T, directory string) {
				writePromptFile(t, directory, "AGENTS.md", []byte(strings.Repeat("x", maxProjectInstructionsBytes+1)))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			test.setup(t, directory)
			writePromptFile(t, directory, "CLAUDE.md", []byte("lower-priority guidance"))
			workspace := openPromptWorkspace(t, directory)

			prompt, err := buildSystemPrompt(workspace, promptTools(t, workspace))
			if err == nil {
				t.Fatalf("build system prompt unexpectedly succeeded\n%s", prompt)
			}
			if strings.Contains(err.Error(), "outside secret") || strings.Contains(prompt, "lower-priority guidance") {
				t.Fatalf("failure exposed content or fell through: prompt=%q error=%v", prompt, err)
			}
		})
	}
}

func TestBuildSystemPromptReturnsStableString(t *testing.T) {
	directory := t.TempDir()
	writePromptFile(t, directory, "AGENTS.md", []byte("original guidance"))
	workspace := openPromptWorkspace(t, directory)

	prompt, err := buildSystemPrompt(workspace, promptTools(t, workspace))
	if err != nil {
		t.Fatalf("build system prompt: %v", err)
	}
	writePromptFile(t, directory, "AGENTS.md", []byte("changed guidance"))

	if !strings.Contains(prompt, "original guidance") || strings.Contains(prompt, "changed guidance") {
		t.Fatalf("assembled prompt changed with its source file\n%s", prompt)
	}
}

func openPromptWorkspace(t *testing.T, path string) *Workspace {
	t.Helper()
	workspace, err := OpenWorkspace(path)
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	t.Cleanup(func() {
		if err := workspace.Close(); err != nil {
			t.Errorf("close workspace: %v", err)
		}
	})
	return workspace
}

func promptTools(t *testing.T, workspace *Workspace) []agent.Tool {
	t.Helper()
	tools, err := newCodingTools(workspace)
	if err != nil {
		t.Fatalf("create coding tools: %v", err)
	}
	return tools
}

func writePromptFile(t *testing.T, directory, name string, content []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(directory, name), content, 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
