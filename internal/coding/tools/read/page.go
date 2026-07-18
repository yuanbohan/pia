package read

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

const (
	maxReadLines = 2000
	maxReadBytes = 50 << 10
)

type pageResult struct {
	startLine int
	endLine   int
	content   []byte
	more      bool
}

func readPage(ctx context.Context, source io.Reader, offset, lineLimit int) (pageResult, error) {
	reader := bufio.NewReaderSize(source, 32<<10)
	lineNumber := 1
	for lineNumber < offset {
		exists, exceeded, err := discardCompleteLine(ctx, reader)
		if err != nil {
			return pageResult{}, err
		}
		if !exists {
			return pageResult{}, fmt.Errorf("offset %d is past end of file", offset)
		}
		if exceeded {
			return pageResult{}, fmt.Errorf("line %d exceeds the %d-byte limit; offset cannot bypass oversized lines", lineNumber, maxReadBytes)
		}
		lineNumber++
	}

	page := pageResult{startLine: lineNumber}
	for linesRead := 0; linesRead < lineLimit; linesRead++ {
		remaining := maxReadBytes - len(page.content)
		line, exists, exceeded, err := readCompleteLine(ctx, reader, remaining)
		if err != nil {
			return pageResult{}, err
		}
		if !exists {
			break
		}
		if exceeded {
			if len(page.content) == 0 {
				return pageResult{}, fmt.Errorf("line %d exceeds the %d-byte limit; read returns complete lines only", lineNumber, maxReadBytes)
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
			return pageResult{}, fmt.Errorf("offset %d is past end of file", offset)
		}
		return page, nil
	}
	if !page.more {
		more, err := readerHasData(ctx, reader)
		if err != nil {
			return pageResult{}, err
		}
		page.more = more
	}

	// The model's text view must match the bytes later used by exact edit.
	// Replacing invalid UTF-8 would create text that cannot reliably match or
	// round-trip to the file, so invalid bytes are an explicit tool error.
	if !utf8.Valid(page.content) {
		return pageResult{}, fmt.Errorf("selected content is not valid UTF-8")
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

func formatResult(path string, page pageResult) string {
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
