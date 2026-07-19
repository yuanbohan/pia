package coding

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"unicode/utf8"

	"github.com/yuanbohan/pi-go/internal/agent"
	"github.com/yuanbohan/pi-go/internal/coding/tools/fileutil"
)

const maxProjectInstructionsBytes = 50 << 10

var projectInstructionCandidates = [...]string{
	"AGENTS.md",
	"AGENTS.MD",
	"CLAUDE.md",
	"CLAUDE.MD",
}

func buildSystemPrompt(workspace *Workspace, tools []agent.Tool) (string, error) {
	if workspace == nil || workspace.Root() == nil {
		return "", fmt.Errorf("coding: build system prompt: workspace is required")
	}

	var prompt strings.Builder
	prompt.WriteString("You are pia, a coding agent. The pia command name is temporary.\n\n")
	prompt.WriteString("Complete the user's coding task autonomously in the selected workspace. Inspect the project, make focused changes, and verify the result before responding.\n\n")
	prompt.WriteString("Available tools:\n")
	for index, tool := range tools {
		if tool == nil {
			return "", fmt.Errorf("coding: build system prompt: tool %d is nil", index)
		}
		definition := tool.Definition()
		if strings.TrimSpace(definition.Schema.Name) == "" {
			return "", fmt.Errorf("coding: build system prompt: tool %d name is required", index)
		}
		fmt.Fprintf(&prompt, "- %s: %s\n", definition.Schema.Name, definition.Schema.Description)
	}

	prompt.WriteString("\nGuidelines:\n")
	prompt.WriteString("- Read relevant files before changing them, and preserve unrelated user work.\n")
	prompt.WriteString("- Prefer focused, reviewable changes over speculative redesign.\n")
	prompt.WriteString("- Treat tool errors as information: inspect the result and recover when possible.\n")
	prompt.WriteString("- Run relevant checks after changes, and do not claim success without verification.\n")
	prompt.WriteString("- Keep the final response concise and summarize the result and verification.\n\n")
	fmt.Fprintf(&prompt, "Current working directory: %s\n", workspace.Path())

	name, instructions, found, err := loadProjectInstructions(workspace)
	if err != nil {
		return "", err
	}
	if found {
		fmt.Fprintf(&prompt, "\nProject instructions from %s:\n<project_instructions>\n", name)
		prompt.WriteString(instructions)
		if !strings.HasSuffix(instructions, "\n") {
			prompt.WriteByte('\n')
		}
		prompt.WriteString("</project_instructions>\n")
	}

	return prompt.String(), nil
}

func loadProjectInstructions(workspace *Workspace) (string, string, bool, error) {
	entries, err := fs.ReadDir(workspace.Root().FS(), ".")
	if err != nil {
		return "", "", false, fmt.Errorf("coding: list project instruction candidates: %w", err)
	}
	exactNames := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		exactNames[entry.Name()] = struct{}{}
	}

	for _, name := range projectInstructionCandidates {
		// Directory enumeration preserves the entry's actual spelling. Without
		// this check, a case-insensitive filesystem can resolve CLAUDE.md to an
		// entry named CLAUDE.MD and silently change the documented precedence.
		if _, exists := exactNames[name]; !exists {
			continue
		}
		// Lstat selects the first directory entry without following it. Opening
		// happens only after selection, so a broken or unsafe higher-priority
		// candidate remains visible as configuration failure instead of silently
		// changing the instruction source.
		_, err := workspace.Root().Lstat(name)
		if err != nil {
			return "", "", false, fmt.Errorf("coding: inspect project instructions %q: %w", name, err)
		}

		instructions, err := readProjectInstructions(workspace, name)
		if err != nil {
			return "", "", false, err
		}
		return name, instructions, true, nil
	}
	return "", "", false, nil
}

func readProjectInstructions(workspace *Workspace, name string) (string, error) {
	file, err := fileutil.OpenRegularFile(workspace.Root(), name)
	if err != nil {
		return "", fmt.Errorf("coding: open project instructions %q: %w", name, err)
	}

	content, readErr := io.ReadAll(io.LimitReader(file, maxProjectInstructionsBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return "", fmt.Errorf("coding: read project instructions %q: %w", name, errors.Join(readErr, closeErr))
	}
	if closeErr != nil {
		return "", fmt.Errorf("coding: close project instructions %q: %w", name, closeErr)
	}
	if len(content) > maxProjectInstructionsBytes {
		return "", fmt.Errorf("coding: project instructions %q exceed the %d-byte limit", name, maxProjectInstructionsBytes)
	}
	if !utf8.Valid(content) {
		return "", fmt.Errorf("coding: project instructions %q are not valid UTF-8", name)
	}
	return string(content), nil
}
