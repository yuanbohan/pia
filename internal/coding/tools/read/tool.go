// Package read implements the model-facing read tool used by the local coding
// Agent. Read-specific paging and opening rules stay here; shared coding-tool
// argument and workspace-file primitives live in sibling focused packages.
package read

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/yuanbohan/pi-go/internal/agent"
	"github.com/yuanbohan/pi-go/internal/ai"
	"github.com/yuanbohan/pi-go/internal/coding/tools/fileutil"
	"github.com/yuanbohan/pi-go/internal/coding/tools/toolargs"
)

const (
	maxReadArgumentsSize = 8 << 10

	readParametersSchema = `{
  "type": "object",
  "properties": {
    "path": {
      "type": "string",
      "minLength": 1,
      "description": "Workspace-relative path of the UTF-8 regular file to read, at most 4096 UTF-8 bytes."
    },
    "offset": {
      "type": "integer",
      "minimum": 1,
      "description": "Optional 1-based line number at which to start."
    },
    "limit": {
      "type": "integer",
      "minimum": 1,
      "description": "Optional maximum number of complete lines to return."
    }
  },
  "required": ["path"],
  "additionalProperties": false
}`
)

// Tool returns a bounded page from one UTF-8 regular file beneath its root.
// The shared os.Root is safe for concurrent use, and Tool has no mutable call
// state, so one instance may execute concurrently.
type Tool struct {
	root *os.Root
}

// New binds a read tool to the workspace root owned by the composition layer.
// The tool borrows root and never closes it.
func New(root *os.Root) (*Tool, error) {
	if root == nil {
		return nil, fmt.Errorf("coding tools: read root is required")
	}
	return &Tool{root: root}, nil
}

// Definition exposes read's model schema and explicit parallel-safe promise.
func (r *Tool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Schema: ai.ToolSchema{
			Name: "read",
			Description: "Read a bounded page of complete lines from a workspace-relative UTF-8 regular file. " +
				"Use the returned continuation offset to read more.",
			Parameters: json.RawMessage(readParametersSchema),
		},
		CanRunParallel: true,
	}
}

// Execute decodes one model invocation and returns a stable, bounded text
// result. Argument and file failures are call-local errors to the Agent.
func (r *Tool) Execute(ctx context.Context, rawArguments json.RawMessage) (string, error) {
	if cause := context.Cause(ctx); cause != nil {
		return "", cause
	}

	input, err := decodeArguments(rawArguments)
	if err != nil {
		return "", err
	}
	rootPath, displayPath, err := fileutil.NormalizeWorkspacePath(input.Path)
	if err != nil {
		return "", fmt.Errorf("read: %w", err)
	}

	file, err := fileutil.OpenRegularFile(r.root, rootPath)
	if err != nil {
		return "", fmt.Errorf("read %q: open: %w", displayPath, err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
	}()

	offset := 1
	if input.Offset != nil {
		offset = *input.Offset
	}
	lineLimit := maxReadLines
	if input.Limit != nil && *input.Limit < lineLimit {
		lineLimit = *input.Limit
	}
	page, err := readPage(ctx, file, offset, lineLimit)
	if err != nil {
		return "", fmt.Errorf("read %q: %w", displayPath, err)
	}
	if cause := context.Cause(ctx); cause != nil {
		return "", cause
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("read %q: close: %w", displayPath, err)
	}
	closed = true

	return formatResult(displayPath, page), nil
}

type arguments struct {
	Path   string `json:"path"`
	Offset *int   `json:"offset"`
	Limit  *int   `json:"limit"`
}

func decodeArguments(raw json.RawMessage) (arguments, error) {
	// Decoder errors may quote model-provided field names. Bound the raw input
	// first so even a malformed invocation cannot create an oversized tool
	// result before any file content is read.
	if len(raw) > maxReadArgumentsSize {
		return arguments{}, fmt.Errorf("read: arguments exceed the %d-byte limit", maxReadArgumentsSize)
	}
	input, err := toolargs.Decode[arguments](raw)
	if err != nil {
		return arguments{}, fmt.Errorf("read: decode arguments: %w", err)
	}
	if input.Offset != nil && *input.Offset < 1 {
		return arguments{}, fmt.Errorf("read: offset must be at least 1")
	}
	if input.Limit != nil && *input.Limit < 1 {
		return arguments{}, fmt.Errorf("read: limit must be at least 1")
	}
	return input, nil
}

var _ agent.Tool = (*Tool)(nil)
