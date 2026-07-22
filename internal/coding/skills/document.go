package skills

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"unicode/utf8"

	"github.com/yuanbohan/pia/internal/coding/tools/fileutil"
)

// BodyLimitError reports that complete current instructions cannot fit in the
// caller-provided body budget. Size is the observed complete body byte count.
type BodyLimitError struct {
	Size  int64
	Limit int64
}

func (e *BodyLimitError) Error() string {
	return fmt.Sprintf("skill body has %d bytes and exceeds the %d-byte limit", e.Size, e.Limit)
}

// LoadCurrentBody reopens one frozen catalog location and returns the current
// bytes after its bounded frontmatter. It borrows root and owns every handle it
// opens for this call.
func LoadCurrentBody(
	ctx context.Context,
	root *os.Root,
	entry Entry,
	maxBodyBytes int64,
) (body []byte, err error) {
	if root == nil {
		return nil, fmt.Errorf("coding skills: root is required")
	}
	if maxBodyBytes < 0 {
		return nil, fmt.Errorf("coding skills: body limit must not be negative")
	}
	if cause := context.Cause(ctx); cause != nil {
		return nil, cause
	}

	source, err := openPiaSkillsDirectory(root)
	if err != nil {
		return nil, fmt.Errorf("open current project Skills source: %w", err)
	}
	defer joinClose(&body, &err, "project Skills source", source)

	directory, err := fileutil.OpenDirectoryAt(source, entry.Directory)
	if err != nil {
		return nil, fmt.Errorf("open current direct Skill directory: %w", err)
	}
	defer joinClose(&body, &err, "Skill directory", directory)

	file, err := fileutil.OpenRegularFileAt(directory, piaSkillFilename)
	if err != nil {
		return nil, fmt.Errorf("open current regular SKILL.md: %w", err)
	}
	defer joinClose(&body, &err, "SKILL.md", file)

	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect current SKILL.md: %w", err)
	}
	prefix, err := io.ReadAll(io.LimitReader(file, maxSkillFrontmatterBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read bounded current Skill frontmatter: %w", err)
	}
	_, _, bodyStart, err := skillDocumentSections(prefix)
	if err != nil {
		return nil, err
	}

	observedSize := max(info.Size()-int64(bodyStart), 0)
	if observedSize > maxBodyBytes {
		return nil, &BodyLimitError{Size: observedSize, Limit: maxBodyBytes}
	}
	if _, err := file.Seek(int64(bodyStart), io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek current Skill body: %w", err)
	}
	readLimit := maxBodyBytes
	if readLimit < math.MaxInt64 {
		readLimit++
	}
	body, err = io.ReadAll(io.LimitReader(file, readLimit))
	if err != nil {
		return nil, fmt.Errorf("read current Skill body: %w", err)
	}
	if int64(len(body)) > maxBodyBytes {
		observedSize = int64(len(body))
		if current, statErr := file.Stat(); statErr == nil {
			observedSize = max(observedSize, current.Size()-int64(bodyStart))
		}
		return nil, &BodyLimitError{Size: observedSize, Limit: maxBodyBytes}
	}
	if !utf8.Valid(body) {
		return nil, fmt.Errorf("skill body is not valid UTF-8")
	}
	if cause := context.Cause(ctx); cause != nil {
		return nil, cause
	}
	return body, nil
}

func readSkillFrontmatter(reader io.Reader) ([]byte, error) {
	prefix, err := io.ReadAll(io.LimitReader(reader, maxSkillFrontmatterBytes+1))
	if err != nil {
		return nil, err
	}
	contentStart, contentEnd, _, err := skillDocumentSections(prefix)
	if err != nil {
		return nil, err
	}
	frontmatter := prefix[contentStart:contentEnd]
	if !utf8.Valid(frontmatter) {
		return nil, fmt.Errorf("frontmatter is not valid UTF-8")
	}
	return frontmatter, nil
}

// skillDocumentSections locates the metadata and body boundaries in a bounded
// SKILL.md prefix without parsing current metadata. Discovery parses the
// returned frontmatter; activation needs only bodyStart so local metadata edits
// do not become a second identity check.
func skillDocumentSections(prefix []byte) (contentStart, contentEnd, bodyStart int, err error) {
	firstEnd, firstHasNewline := skillLineEnd(prefix, 0)
	if firstEnd < 0 || !bytes.Equal(bytes.TrimSuffix(prefix[:firstEnd], []byte{'\r'}), []byte("---")) {
		return 0, 0, 0, fmt.Errorf("opening --- delimiter is required")
	}
	if !firstHasNewline {
		return 0, 0, 0, fmt.Errorf("closing --- delimiter is required")
	}
	contentStart = firstEnd + 1
	for lineStart := contentStart; lineStart <= len(prefix); {
		lineEnd, hasNewline := skillLineEnd(prefix, lineStart)
		if lineEnd < 0 {
			break
		}
		line := bytes.TrimSuffix(prefix[lineStart:lineEnd], []byte{'\r'})
		if bytes.Equal(line, []byte("---")) {
			closingEnd := lineEnd
			if hasNewline {
				closingEnd++
			}
			if closingEnd > maxSkillFrontmatterBytes {
				return 0, 0, 0, fmt.Errorf("frontmatter exceeds the %d-byte limit", maxSkillFrontmatterBytes)
			}
			return contentStart, lineStart, closingEnd, nil
		}
		if !hasNewline {
			break
		}
		lineStart = lineEnd + 1
	}

	if len(prefix) > maxSkillFrontmatterBytes {
		return 0, 0, 0, fmt.Errorf("frontmatter exceeds the %d-byte limit", maxSkillFrontmatterBytes)
	}
	return 0, 0, 0, fmt.Errorf("closing --- delimiter is required")
}

func skillLineEnd(content []byte, start int) (int, bool) {
	if start >= len(content) {
		return -1, false
	}
	if offset := bytes.IndexByte(content[start:], '\n'); offset >= 0 {
		return start + offset, true
	}
	return len(content), false
}

func joinClose(body *[]byte, err *error, resource string, file *os.File) {
	if closeErr := file.Close(); closeErr != nil {
		*body = nil
		*err = errors.Join(*err, fmt.Errorf("close %s: %w", resource, closeErr))
	}
}
