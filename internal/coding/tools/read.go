// Package tools implements the model-facing tools used by the local coding
// Agent.
package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/yuanbohan/pi-go/internal/agent"
	"github.com/yuanbohan/pi-go/internal/ai"
	"github.com/yuanbohan/pi-go/internal/coding/tools/utils"
)

const (
	maxReadLines         = 2000
	maxReadBytes         = 50 << 10
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

// Read returns a bounded page from one UTF-8 regular file beneath its root.
// The shared os.Root is safe for concurrent use, and Read has no mutable call
// state, so one instance may execute concurrently.
type Read struct {
	root *os.Root
}

// NewRead binds a read tool to the workspace root owned by the composition
// layer. The tool borrows root and never closes it.
func NewRead(root *os.Root) (*Read, error) {
	if root == nil {
		return nil, fmt.Errorf("coding tools: read root is required")
	}
	return &Read{root: root}, nil
}

// Definition exposes read's model schema and explicit parallel-safe promise.
func (r *Read) Definition() agent.ToolDefinition {
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
func (r *Read) Execute(ctx context.Context, arguments json.RawMessage) (string, error) {
	if cause := context.Cause(ctx); cause != nil {
		return "", cause
	}

	input, err := decodeReadArguments(arguments)
	if err != nil {
		return "", err
	}
	rootPath, displayPath, err := utils.NormalizeWorkspacePath(input.Path)
	if err != nil {
		return "", fmt.Errorf("read: %w", err)
	}

	file, err := openRegularFileCandidate(r.root, rootPath)
	if err != nil {
		return "", fmt.Errorf("read %q: open: %w", displayPath, err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
	}()

	// Validate the opened handle, rather than a prior path lookup, so a path
	// replacement cannot make us validate one object and read another.
	info, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("read %q: inspect opened file: %w", displayPath, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("read %q: target is not a regular file", displayPath)
	}

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

	return formatReadResult(displayPath, page), nil
}

type readArguments struct {
	Path   string `json:"path"`
	Offset *int   `json:"offset"`
	Limit  *int   `json:"limit"`
}

func decodeReadArguments(arguments json.RawMessage) (readArguments, error) {
	// Decoder errors may quote model-provided field names. Bound the raw input
	// first so even a malformed invocation cannot create an oversized tool
	// result before any file content is read.
	if len(arguments) > maxReadArgumentsSize {
		return readArguments{}, fmt.Errorf("read: arguments exceed the %d-byte limit", maxReadArgumentsSize)
	}
	input, err := utils.DecodeArguments[readArguments](arguments)
	if err != nil {
		return readArguments{}, fmt.Errorf("read: decode arguments: %w", err)
	}
	if input.Offset != nil && *input.Offset < 1 {
		return readArguments{}, fmt.Errorf("read: offset must be at least 1")
	}
	if input.Limit != nil && *input.Limit < 1 {
		return readArguments{}, fmt.Errorf("read: limit must be at least 1")
	}
	return input, nil
}

type readPageResult struct {
	startLine int
	endLine   int
	content   []byte
	more      bool
}

func readPage(ctx context.Context, source io.Reader, offset, lineLimit int) (readPageResult, error) {
	reader := bufio.NewReaderSize(source, 32<<10)
	lineNumber := 1
	for lineNumber < offset {
		exists, exceeded, err := discardCompleteLine(ctx, reader)
		if err != nil {
			return readPageResult{}, err
		}
		if !exists {
			return readPageResult{}, fmt.Errorf("offset %d is past end of file", offset)
		}
		if exceeded {
			return readPageResult{}, fmt.Errorf("line %d exceeds the %d-byte limit; offset cannot bypass oversized lines", lineNumber, maxReadBytes)
		}
		lineNumber++
	}

	page := readPageResult{startLine: lineNumber}
	for linesRead := 0; linesRead < lineLimit; linesRead++ {
		remaining := maxReadBytes - len(page.content)
		line, exists, exceeded, err := readCompleteLine(ctx, reader, remaining)
		if err != nil {
			return readPageResult{}, err
		}
		if !exists {
			break
		}
		if exceeded {
			if len(page.content) == 0 {
				return readPageResult{}, fmt.Errorf("line %d exceeds the %d-byte limit; read returns complete lines only", lineNumber, maxReadBytes)
			}
			page.more = true
			break
		}
		page.content = append(page.content, line...)
		page.endLine = lineNumber
		lineNumber++
	}

	if page.endLine == 0 {
		if offset != 1 {
			return readPageResult{}, fmt.Errorf("offset %d is past end of file", offset)
		}
		return page, nil
	}
	if !page.more {
		more, err := readerHasData(ctx, reader)
		if err != nil {
			return readPageResult{}, err
		}
		page.more = more
	}

	// The model's text view must match the bytes later used by exact edit.
	// Replacing invalid UTF-8 would create text that cannot reliably match or
	// round-trip to the file, so invalid bytes are an explicit tool error.
	if !utf8.Valid(page.content) {
		return readPageResult{}, fmt.Errorf("selected content is not valid UTF-8")
	}
	return page, nil
}

func readCompleteLine(ctx context.Context, reader *bufio.Reader, captureLimit int) ([]byte, bool, bool, error) {
	var line []byte
	exists := false
	for {
		if cause := context.Cause(ctx); cause != nil {
			return nil, false, false, cause
		}
		fragment, err := reader.ReadSlice('\n')
		if len(fragment) > 0 {
			exists = true
			remaining := captureLimit - len(line)
			if remaining > 0 {
				amount := min(remaining, len(fragment))
				line = append(line, fragment[:amount]...)
			}
			if len(fragment) > max(remaining, 0) {
				// This invocation closes the file after returning, so draining an
				// oversized selected line would turn a bounded result into unbounded
				// disk I/O without producing any additional model-visible content.
				return line, exists, true, nil
			}
		}
		switch {
		case err == nil:
			return line, exists, false, nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF):
			return line, exists, false, nil
		default:
			return nil, false, false, err
		}
	}
}

func discardCompleteLine(ctx context.Context, reader *bufio.Reader) (bool, bool, error) {
	exists := false
	bytesRead := 0
	for {
		if cause := context.Cause(ctx); cause != nil {
			return false, false, cause
		}
		fragment, err := reader.ReadSlice('\n')
		exists = exists || len(fragment) > 0
		if len(fragment) > maxReadBytes-bytesRead {
			// Applying the same per-line bound while seeking prevents offset from
			// becoming a way to drain an arbitrarily large line before returning.
			return exists, true, nil
		}
		bytesRead += len(fragment)
		switch {
		case err == nil:
			return exists, false, nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF):
			return exists, false, nil
		default:
			return false, false, err
		}
	}
}

func readerHasData(ctx context.Context, reader *bufio.Reader) (bool, error) {
	if cause := context.Cause(ctx); cause != nil {
		return false, cause
	}
	_, err := reader.Peek(1)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, io.EOF):
		return false, nil
	default:
		return false, err
	}
}

func formatReadResult(path string, page readPageResult) string {
	if page.endLine == 0 {
		return fmt.Sprintf("Path: %s\nLines: 0\nContent:\n[empty file]\n\n[End of file.]", path)
	}

	var result strings.Builder
	fmt.Fprintf(&result, "Path: %s\nLines: %d-%d\nContent:\n", path, page.startLine, page.endLine)
	result.Write(page.content)
	if page.content[len(page.content)-1] != '\n' {
		result.WriteByte('\n')
	}
	result.WriteByte('\n')
	if page.more {
		fmt.Fprintf(&result, "[More content available. Continue with offset=%d.]", page.endLine+1)
	} else {
		result.WriteString("[End of file.]")
	}
	return result.String()
}

var _ agent.Tool = (*Read)(nil)
