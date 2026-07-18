// Package write implements the model-facing write tool used by the local
// coding Agent. Model protocol and parent-directory policy stay here, while
// the replacement primitive shared with edit belongs to the sibling fileutil
// package and strict JSON decoding belongs to toolargs.
package write

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuanbohan/pi-go/internal/agent"
	"github.com/yuanbohan/pi-go/internal/ai"
	"github.com/yuanbohan/pi-go/internal/coding/tools/fileutil"
	"github.com/yuanbohan/pi-go/internal/coding/tools/toolargs"
)

const writeParametersSchema = `{
  "type": "object",
  "properties": {
    "path": {
      "type": "string",
      "minLength": 1,
      "description": "Workspace-relative path of the regular file to create or completely overwrite, at most 4096 UTF-8 bytes."
    },
    "content": {
      "type": "string",
      "description": "Complete UTF-8 text content to write. Empty content is valid."
    }
  },
  "required": ["path", "content"],
  "additionalProperties": false
}`

// Tool creates or completely replaces one regular file beneath its root.
// It does not create symbolic links or write through a final symbolic link.
type Tool struct {
	root *os.Root
}

// New binds a write tool to the workspace root owned by the composition layer.
// The tool borrows root and never closes it.
func New(root *os.Root) (*Tool, error) {
	if root == nil {
		return nil, fmt.Errorf("coding tools: write root is required")
	}
	return &Tool{root: root}, nil
}

// Definition exposes write's model schema. Write intentionally keeps the
// default serial barrier because it mutates the shared workspace.
func (w *Tool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Schema: ai.ToolSchema{
			Name: "write",
			Description: "Write complete content to a workspace-relative regular file. " +
				"Creates the file and parent directories when missing, or atomically replaces an existing regular file.",
			Parameters: json.RawMessage(writeParametersSchema),
		},
	}
}

// Execute validates one model invocation and commits complete content without
// exposing a partially written target path.
func (w *Tool) Execute(ctx context.Context, rawArguments json.RawMessage) (string, error) {
	if cause := context.Cause(ctx); cause != nil {
		return "", cause
	}
	input, err := decodeArguments(rawArguments)
	if err != nil {
		return "", err
	}
	rootPath, displayPath, err := fileutil.NormalizeWorkspacePath(input.Path)
	if err != nil {
		return "", fmt.Errorf("write: %w", err)
	}

	parentPath := filepath.Dir(rootPath)
	// Parent creation is intentionally not rolled back after a later failure:
	// removing directories could race with another actor, while write's atomic
	// visibility guarantee applies only to the final file entry.
	if err := w.root.MkdirAll(parentPath, 0o777); err != nil {
		return "", fmt.Errorf("write %q: create workspace parent: %w", displayPath, err)
	}
	if cause := context.Cause(ctx); cause != nil {
		return "", cause
	}

	// Pin the resolved parent once so an ancestor symlink swap cannot place the
	// temporary file and its final rename in different directories.
	parent, err := w.root.OpenRoot(parentPath)
	if err != nil {
		return "", fmt.Errorf("write %q: open workspace parent: %w", displayPath, err)
	}
	// Closing this sub-root is descriptor cleanup, not part of the replacement
	// commit. A successful rename must not become a reported write failure that
	// encourages a duplicate model retry merely because cleanup later fails.
	defer func() { _ = parent.Close() }()

	content := *input.Content
	if err := fileutil.ReplaceRegularFile(ctx, parent, filepath.Base(rootPath), strings.NewReader(content)); err != nil {
		return "", fmt.Errorf("write %q: %w", displayPath, err)
	}
	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), displayPath), nil
}

type arguments struct {
	Path    string  `json:"path"`
	Content *string `json:"content"`
}

func decodeArguments(raw json.RawMessage) (arguments, error) {
	input, err := toolargs.Decode[arguments](raw)
	if err != nil {
		return arguments{}, fmt.Errorf("write: decode arguments: %w", err)
	}
	if input.Content == nil {
		return arguments{}, fmt.Errorf("write: content is required")
	}
	return input, nil
}

var _ agent.Tool = (*Tool)(nil)
