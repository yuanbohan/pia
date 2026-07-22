package coding

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yuanbohan/pia/internal/agent"
	skillcatalog "github.com/yuanbohan/pia/internal/coding/skills"
)

func TestBuildSystemPromptUsesCanonicalWorkspaceAndRealTools(t *testing.T) {
	realDirectory := t.TempDir()
	link := filepath.Join(t.TempDir(), "workspace-link")
	if err := os.Symlink(realDirectory, link); err != nil {
		t.Fatalf("create workspace symlink: %v", err)
	}

	workspace := openPromptWorkspace(t, link)
	tools := promptTools(t, workspace)
	prompt, err := buildSystemPrompt(workspace, tools, "")
	if err != nil {
		t.Fatalf("build system prompt: %v", err)
	}

	want := fmt.Sprintf(`You are an expert coding assistant operating inside pia, a coding agent harness. You help users by reading files, executing commands, editing code, and writing new files.

Available tools:
- read: Read file contents
- bash: Execute bash commands (ls, grep, find, etc.)
- edit: Make precise file edits with exact text replacement, including multiple disjoint edits in one call
- write: Create or overwrite files

Guidelines:
- Use bash for file operations like ls, rg, find
- Use read to examine files instead of cat or sed.
- Use edit for precise changes (edits[].oldText must match exactly)
- When changing multiple separate locations in one file, use one edit call with multiple entries in edits[] instead of multiple edit calls
- Each edits[].oldText is matched against the original file, not after earlier edits are applied. Do not emit overlapping or nested edits. Merge nearby changes into one edit.
- Keep edits[].oldText as small as possible while still being unique in the file. Do not pad with large unchanged regions.
- Use write only for new files or complete rewrites.
- Be concise in your responses
- Show file paths clearly when working with files

Complete the user's coding task autonomously in the selected workspace. Inspect the project, make focused changes, and verify the result before responding.

Additional pia guidelines:
- Read relevant files before changing them, and preserve unrelated user work.
- Prefer focused, reviewable changes over speculative redesign.
- Treat tool errors as information: inspect the result and recover when possible.
- Run relevant checks after changes, and do not claim success without verification.
- Summarize the result and verification in the final response.

Current working directory: %s`, workspace.Path())
	if prompt != want {
		t.Fatalf("system prompt does not match the adapted frozen-Pi baseline\ngot:\n%s\n\nwant:\n%s", prompt, want)
	}
	if strings.Contains(prompt, link) {
		t.Fatalf("prompt contains non-canonical workspace path %q\n%s", link, prompt)
	}
	if strings.Contains(prompt, "Project instructions from") {
		t.Fatalf("prompt contains an empty project-instructions section\n%s", prompt)
	}
}

func TestBuildSystemPromptIncludesOnlyNonEmptyPiaSkillCatalog(t *testing.T) {
	directory := t.TempDir()
	writePiaSkill(t, directory, "review-go", `name: review-go
description: Review Go changes.
`, "SKILL_BODY_SENTINEL")
	workspace := openPromptWorkspace(t, directory)
	discovery, err := discoverPiaSkills(workspace)
	if err != nil {
		t.Fatalf("discover Pia skills: %v", err)
	}

	prompt, err := buildSystemPrompt(workspace, promptTools(t, workspace, discovery.Entries...), discovery.Catalog)
	if err != nil {
		t.Fatalf("build system prompt: %v", err)
	}
	for _, fragment := range []string{
		"Project skills:",
		"<name>review-go</name>",
		"<location>.pia/skills/review-go/SKILL.md</location>",
		"- skill: Load complete project Skill instructions by catalog name",
		"use the skill tool",
	} {
		if !strings.Contains(prompt, fragment) {
			t.Errorf("prompt does not contain %q\n%s", fragment, prompt)
		}
	}
	if strings.Contains(prompt, "SKILL_BODY_SENTINEL") {
		t.Fatalf("prompt contains undisclosed Skill body\n%s", prompt)
	}

	emptyPrompt, err := buildSystemPrompt(workspace, promptTools(t, workspace), "")
	if err != nil {
		t.Fatalf("build empty-catalog system prompt: %v", err)
	}
	if strings.Contains(emptyPrompt, "Project skills:") || strings.Contains(emptyPrompt, "<available_skills>") {
		t.Fatalf("empty catalog produced a Skill prompt section\n%s", emptyPrompt)
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
			name: "newline-terminated AGENTS md preserves template framing",
			files: map[string]string{
				"AGENTS.md": "newline-terminated guidance\n",
			},
			wantName:    "AGENTS.md",
			wantContent: "newline-terminated guidance\n",
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

			prompt, err := buildSystemPrompt(workspace, promptTools(t, workspace), "")
			if err != nil {
				t.Fatalf("build system prompt: %v", err)
			}
			instructionPath := filepath.Join(workspace.Path(), test.wantName)
			wantSuffix := fmt.Sprintf(`<project_context>

Project-specific instructions and guidelines:

<project_instructions path="%s">
%s
</project_instructions>

</project_context>

Current working directory: %s`, instructionPath, test.wantContent, workspace.Path())
			if !strings.HasSuffix(prompt, wantSuffix) {
				t.Fatalf("prompt does not preserve the frozen-Pi project context and cwd order\ngot:\n%s\n\nwant suffix:\n%s", prompt, wantSuffix)
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

	prompt, err := buildSystemPrompt(workspace, promptTools(t, workspace), "")
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
	prompt, err := buildSystemPrompt(workspace, promptTools(t, workspace), "")
	if err != nil {
		t.Fatalf("build system prompt: %v", err)
	}
	if strings.Contains(prompt, "guidance") || strings.Contains(prompt, "<project_context>") {
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

	prompt, err := buildSystemPrompt(workspace, promptTools(t, workspace), "")
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

			prompt, err := buildSystemPrompt(workspace, promptTools(t, workspace), "")
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

	prompt, err := buildSystemPrompt(workspace, promptTools(t, workspace), "")
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

func promptTools(t *testing.T, workspace *Workspace, entries ...skillcatalog.Entry) []agent.Tool {
	t.Helper()
	tools, err := newCodingTools(workspace, entries)
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
