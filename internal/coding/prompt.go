package coding

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/yuanbohan/pia/internal/agent"
	"github.com/yuanbohan/pia/internal/coding/tools/fileutil"
)

const maxProjectInstructionsBytes = 50 << 10

var projectInstructionCandidates = [...]string{
	"AGENTS.md",
	"AGENTS.MD",
	"CLAUDE.md",
	"CLAUDE.MD",
}

type toolPromptMetadata struct {
	snippet    string
	guidelines []string
}

func codingToolPromptMetadata(name string) (toolPromptMetadata, bool) {
	switch name {
	case "read":
		return toolPromptMetadata{
			snippet:    "Read file contents",
			guidelines: []string{"Use read to examine files instead of cat or sed."},
		}, true
	case "bash":
		return toolPromptMetadata{
			snippet: "Execute bash commands (ls, grep, find, etc.)",
		}, true
	case "edit":
		return toolPromptMetadata{
			snippet: "Make precise file edits with exact text replacement, including multiple disjoint edits in one call",
			guidelines: []string{
				"Use edit for precise changes (edits[].oldText must match exactly)",
				"When changing multiple separate locations in one file, use one edit call with multiple entries in edits[] instead of multiple edit calls",
				"Each edits[].oldText is matched against the original file, not after earlier edits are applied. Do not emit overlapping or nested edits. Merge nearby changes into one edit.",
				"Keep edits[].oldText as small as possible while still being unique in the file. Do not pad with large unchanged regions.",
			},
		}, true
	case "write":
		return toolPromptMetadata{
			snippet:    "Create or overwrite files",
			guidelines: []string{"Use write only for new files or complete rewrites."},
		}, true
	case "skill":
		return toolPromptMetadata{
			snippet: "Load complete project Skill instructions by catalog name",
			guidelines: []string{
				"When a listed project Skill matches the task, use skill with its exact catalog name before applying the instructions.",
				"Use read for files explicitly referenced by Skill instructions or to inspect an oversized SKILL.md after a skill error.",
			},
		}, true
	default:
		return toolPromptMetadata{}, false
	}
}

func buildSystemPrompt(workspace *Workspace, tools []agent.Tool, skillCatalog string) (string, error) {
	if workspace == nil || workspace.Root() == nil {
		return "", fmt.Errorf("coding: build system prompt: workspace is required")
	}

	var prompt strings.Builder
	prompt.WriteString("You are an expert coding assistant operating inside pia, a coding agent harness. You help users by reading files, executing commands, editing code, and writing new files.\n\n")
	prompt.WriteString("Available tools:\n")
	toolNames := make(map[string]struct{}, len(tools))
	toolGuidelines := make([]string, 0)
	for index, tool := range tools {
		if tool == nil {
			return "", fmt.Errorf("coding: build system prompt: tool %d is nil", index)
		}
		definition := tool.Definition()
		if strings.TrimSpace(definition.Schema.Name) == "" {
			return "", fmt.Errorf("coding: build system prompt: tool %d name is required", index)
		}
		name := definition.Schema.Name
		toolNames[name] = struct{}{}
		metadata, found := codingToolPromptMetadata(name)
		if !found {
			metadata.snippet = definition.Schema.Description
		}
		fmt.Fprintf(&prompt, "- %s: %s\n", name, metadata.snippet)
		toolGuidelines = append(toolGuidelines, metadata.guidelines...)
	}

	prompt.WriteString("\nGuidelines:\n")
	_, hasBash := toolNames["bash"]
	_, hasGrep := toolNames["grep"]
	_, hasFind := toolNames["find"]
	_, hasLS := toolNames["ls"]
	if hasBash && !hasGrep && !hasFind && !hasLS {
		prompt.WriteString("- Use bash for file operations like ls, rg, find\n")
	}
	for _, guideline := range toolGuidelines {
		fmt.Fprintf(&prompt, "- %s\n", guideline)
	}
	prompt.WriteString("- Be concise in your responses\n")
	prompt.WriteString("- Show file paths clearly when working with files\n")

	// Frozen Pi appends application-specific system prompt text after its default
	// body and before project context. Keep pia's narrower workflow guidance at
	// the same seam so the shared baseline remains directly comparable.
	prompt.WriteString("\nComplete the user's coding task autonomously in the selected workspace. Inspect the project, make focused changes, and verify the result before responding.\n\n")
	prompt.WriteString("Additional pia guidelines:\n")
	prompt.WriteString("- Read relevant files before changing them, and preserve unrelated user work.\n")
	prompt.WriteString("- Prefer focused, reviewable changes over speculative redesign.\n")
	prompt.WriteString("- Treat tool errors as information: inspect the result and recover when possible.\n")
	prompt.WriteString("- Run relevant checks after changes, and do not claim success without verification.\n")
	prompt.WriteString("- Summarize the result and verification in the final response.\n")

	name, instructions, found, err := loadProjectInstructions(workspace)
	if err != nil {
		return "", err
	}
	if found {
		prompt.WriteString("\n<project_context>\n\n")
		prompt.WriteString("Project-specific instructions and guidelines:\n\n")
		fmt.Fprintf(&prompt, "<project_instructions path=\"%s\">\n", filepath.Join(workspace.Path(), name))
		prompt.WriteString(instructions)
		prompt.WriteByte('\n')
		prompt.WriteString("</project_instructions>\n\n")
		prompt.WriteString("</project_context>\n")
	}
	if strings.TrimSpace(skillCatalog) != "" {
		prompt.WriteByte('\n')
		prompt.WriteString(skillCatalog)
	}

	fmt.Fprintf(&prompt, "\nCurrent working directory: %s", workspace.Path())

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
