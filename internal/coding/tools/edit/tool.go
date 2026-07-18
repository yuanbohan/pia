// Package edit implements exact, model-facing replacements for the local
// coding Agent. Matching policy stays here; shared argument, path, regular-file
// opening, and replacement-commit primitives live in sibling focused packages.
package edit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/yuanbohan/pi-go/internal/agent"
	"github.com/yuanbohan/pi-go/internal/ai"
	"github.com/yuanbohan/pi-go/internal/coding/tools/fileutil"
	"github.com/yuanbohan/pi-go/internal/coding/tools/toolargs"
)

const editParametersSchema = `{
  "type": "object",
  "properties": {
    "path": {
      "type": "string",
      "minLength": 1,
      "description": "Workspace-relative path of the existing UTF-8 regular file to edit, at most 4096 UTF-8 bytes."
    },
    "edits": {
      "type": "array",
      "minItems": 1,
      "description": "Exact, non-overlapping replacements matched against the original file. Every oldText must occur exactly once.",
      "items": {
        "type": "object",
        "properties": {
          "oldText": {
            "type": "string",
            "minLength": 1,
            "description": "Exact text to replace, including whitespace and line endings."
          },
          "newText": {
            "type": "string",
            "description": "Replacement text. Empty text deletes the matched block."
          }
        },
        "required": ["oldText", "newText"],
        "additionalProperties": false
      }
    }
  },
  "required": ["path", "edits"],
  "additionalProperties": false
}`

// Tool applies one all-or-nothing set of exact replacements beneath its root.
type Tool struct {
	root *os.Root
}

// New binds an edit tool to the workspace root owned by the composition layer.
// The tool borrows root and never closes it.
func New(root *os.Root) (*Tool, error) {
	if root == nil {
		return nil, fmt.Errorf("coding tools: edit root is required")
	}
	return &Tool{root: root}, nil
}

// Definition exposes edit's model schema. Edit intentionally keeps the default
// serial barrier because it reads and then mutates shared workspace state.
func (e *Tool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Schema: ai.ToolSchema{
			Name: "edit",
			Description: "Replace one or more exact, unique, non-overlapping text blocks in an existing " +
				"workspace-relative regular file. Every edit is matched against the original file before any change is committed.",
			Parameters: json.RawMessage(editParametersSchema),
		},
	}
}

// Execute validates all replacements against one opened file snapshot and then
// publishes the complete result through the shared replacement commit.
func (e *Tool) Execute(ctx context.Context, rawArguments json.RawMessage) (string, error) {
	if cause := context.Cause(ctx); cause != nil {
		return "", cause
	}
	input, err := decodeArguments(rawArguments)
	if err != nil {
		return "", err
	}
	rootPath, displayPath, err := fileutil.NormalizeWorkspacePath(input.path)
	if err != nil {
		return "", fmt.Errorf("edit: %w", err)
	}

	// Pin the resolved parent before opening or replacing the file. An ancestor
	// symlink swap therefore cannot redirect the read and commit to different
	// directories, and the final rename remains beneath the selected workspace.
	parent, err := e.root.OpenRoot(filepath.Dir(rootPath))
	if err != nil {
		return "", fmt.Errorf("edit %q: open workspace parent: %w", displayPath, err)
	}
	defer func() { _ = parent.Close() }()
	name := filepath.Base(rootPath)

	file, err := fileutil.OpenRegularFile(parent, name)
	if err != nil {
		return "", fmt.Errorf("edit %q: open: %w", displayPath, err)
	}
	content, readErr := readAll(ctx, file)
	closeErr := file.Close()
	if readErr != nil {
		return "", fmt.Errorf("edit %q: read: %w", displayPath, readErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("edit %q: close: %w", displayPath, closeErr)
	}
	if cause := context.Cause(ctx); cause != nil {
		return "", cause
	}
	// Edit is a text protocol. Rejecting invalid UTF-8 keeps its exact-match view
	// consistent with read and avoids publishing replacement characters or
	// otherwise changing bytes the model could not observe faithfully.
	if !utf8.Valid(content) {
		return "", fmt.Errorf("edit %q: file is not valid UTF-8", displayPath)
	}

	updated, err := applyExactReplacements(string(content), input.replacements, displayPath)
	if err != nil {
		return "", err
	}
	if err := fileutil.ReplaceRegularFile(ctx, parent, name, strings.NewReader(updated)); err != nil {
		return "", fmt.Errorf("edit %q: %w", displayPath, err)
	}
	return fmt.Sprintf("Successfully replaced %d block(s) in %s", len(input.replacements), displayPath), nil
}

type arguments struct {
	path         string
	replacements []replacement
}

type rawArguments struct {
	Path  string           `json:"path"`
	Edits []rawReplacement `json:"edits"`
}

type rawReplacement struct {
	OldText *string `json:"oldText"`
	NewText *string `json:"newText"`
}

func decodeArguments(raw json.RawMessage) (arguments, error) {
	input, err := toolargs.Decode[rawArguments](raw)
	if err != nil {
		return arguments{}, fmt.Errorf("edit: decode arguments: %w", err)
	}
	if len(input.Edits) == 0 {
		return arguments{}, fmt.Errorf("edit: edits must contain at least one replacement")
	}
	replacements := make([]replacement, len(input.Edits))
	for index, edit := range input.Edits {
		if edit.OldText == nil {
			return arguments{}, fmt.Errorf("edit: edits[%d].oldText is required", index)
		}
		if *edit.OldText == "" {
			return arguments{}, fmt.Errorf("edit: edits[%d].oldText must not be empty", index)
		}
		if edit.NewText == nil {
			return arguments{}, fmt.Errorf("edit: edits[%d].newText is required", index)
		}
		replacements[index] = replacement{oldText: *edit.OldText, newText: *edit.NewText}
	}
	return arguments{path: input.Path, replacements: replacements}, nil
}

func readAll(ctx context.Context, source io.Reader) ([]byte, error) {
	var result bytes.Buffer
	buffer := make([]byte, 32<<10)
	for {
		if cause := context.Cause(ctx); cause != nil {
			return nil, cause
		}
		read, err := source.Read(buffer)
		if read > 0 {
			_, _ = result.Write(buffer[:read])
		}
		if cause := context.Cause(ctx); cause != nil {
			return nil, cause
		}
		switch {
		case errors.Is(err, io.EOF):
			return result.Bytes(), nil
		case err != nil:
			return nil, err
		}
	}
}

var _ agent.Tool = (*Tool)(nil)
