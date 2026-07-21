package bash

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"
)

const (
	maxOutputLines        = 2000
	maxOutputBytes        = 50 << 10
	maxRollingOutputBytes = maxOutputBytes * 2
)

type truncationKind string

const (
	truncationByLines truncationKind = "lines"
	truncationByBytes truncationKind = "bytes"
)

type truncationResult struct {
	truncated       bool
	truncatedBy     truncationKind
	totalLines      int
	totalBytes      int
	outputLines     int
	outputBytes     int
	lastLinePartial bool
}

type outputSnapshot struct {
	content        string
	truncation     truncationResult
	fullOutputPath string
	lastLineBytes  int
}

// outputAccumulator keeps transcript memory bounded while preserving raw bytes
// in a temp file once output crosses either model-visible limit. The temp file
// is intentionally not removed: frozen Pi returns its path so the model or
// operator can inspect output omitted from the transcript.
type outputAccumulator struct {
	raw               bytes.Buffer
	fullFile          *os.File
	fullPath          string
	persistenceFailed bool

	pendingUTF8              []byte
	tail                     string
	tailBytes                int
	tailStartsAtLineBoundary bool

	totalRawBytes     int
	totalDecodedBytes int
	completedLines    int
	hasOpenLine       bool
	currentLineBytes  int
	finished          bool
}

func newOutputAccumulator() *outputAccumulator {
	return &outputAccumulator{tailStartsAtLineBoundary: true}
}

func (a *outputAccumulator) append(data []byte) error {
	if a.finished {
		return errors.New("bash: cannot append to a finished output accumulator")
	}
	if len(data) == 0 {
		return nil
	}

	a.totalRawBytes += len(data)
	a.decode(data, false)
	if a.fullFile != nil || a.shouldPersist() {
		if err := a.ensureFullFile(); err != nil {
			a.persistenceFailed = true
			return err
		}
		if _, err := a.fullFile.Write(data); err != nil {
			path := a.fullPath
			a.persistenceFailed = true
			a.discardFullFile()
			return fmt.Errorf("bash: write complete output %q: %w", path, err)
		}
		return nil
	}
	_, _ = a.raw.Write(data)
	return nil
}

func (a *outputAccumulator) finish() (outputSnapshot, error) {
	if !a.finished {
		a.finished = true
		a.decode(nil, true)
		if a.shouldPersist() && !a.persistenceFailed {
			if err := a.ensureFullFile(); err != nil {
				return a.snapshot(), err
			}
		}
		if a.fullFile != nil {
			if err := a.fullFile.Close(); err != nil {
				a.fullFile = nil
				return a.snapshot(), fmt.Errorf("bash: close complete output %q: %w", a.fullPath, err)
			}
			a.fullFile = nil
		}
	}
	return a.snapshot(), nil
}

func (a *outputAccumulator) decode(data []byte, final bool) {
	input := make([]byte, 0, len(a.pendingUTF8)+len(data))
	input = append(input, a.pendingUTF8...)
	input = append(input, data...)
	a.pendingUTF8 = a.pendingUTF8[:0]

	var decoded strings.Builder
	for len(input) > 0 {
		if !utf8.FullRune(input) {
			if !final {
				a.pendingUTF8 = append(a.pendingUTF8, input...)
				break
			}
			decoded.WriteRune(utf8.RuneError)
			break
		}
		r, size := utf8.DecodeRune(input)
		decoded.WriteRune(r)
		input = input[size:]
	}
	a.appendDecoded(decoded.String())
}

func (a *outputAccumulator) appendDecoded(text string) {
	if text == "" {
		return
	}
	decodedBytes := len(text)
	a.totalDecodedBytes += decodedBytes
	a.tail += text
	a.tailBytes += decodedBytes
	if a.tailBytes > maxRollingOutputBytes*2 {
		a.trimTail()
	}

	newlines := strings.Count(text, "\n")
	if newlines == 0 {
		a.currentLineBytes += decodedBytes
		a.hasOpenLine = true
		return
	}
	a.completedLines += newlines
	lastNewline := strings.LastIndexByte(text, '\n')
	tail := text[lastNewline+1:]
	a.currentLineBytes = len(tail)
	a.hasOpenLine = tail != ""
}

func (a *outputAccumulator) trimTail() {
	buffer := []byte(a.tail)
	if len(buffer) <= maxRollingOutputBytes {
		a.tailBytes = len(buffer)
		return
	}
	start := len(buffer) - maxRollingOutputBytes
	for start < len(buffer) && !utf8.RuneStart(buffer[start]) {
		start++
	}
	a.tailStartsAtLineBoundary = start == 0 && a.tailStartsAtLineBoundary || start > 0 && buffer[start-1] == '\n'
	a.tail = string(buffer[start:])
	a.tailBytes = len(buffer[start:])
}

func (a *outputAccumulator) totalLines() int {
	if a.hasOpenLine {
		return a.completedLines + 1
	}
	return a.completedLines
}

func (a *outputAccumulator) shouldPersist() bool {
	return a.totalRawBytes > maxOutputBytes ||
		a.totalDecodedBytes > maxOutputBytes ||
		a.totalLines() > maxOutputLines
}

func (a *outputAccumulator) ensureFullFile() error {
	if a.fullFile != nil || a.fullPath != "" {
		return nil
	}
	file, err := os.CreateTemp("", "pia-bash-*.log")
	if err != nil {
		return fmt.Errorf("bash: create complete-output temp file: %w", err)
	}
	if a.raw.Len() > 0 {
		if _, err := file.Write(a.raw.Bytes()); err != nil {
			_ = file.Close()
			_ = os.Remove(file.Name())
			return fmt.Errorf("bash: initialize complete-output temp file: %w", err)
		}
	}
	a.raw.Reset()
	a.fullFile = file
	a.fullPath = file.Name()
	return nil
}

func (a *outputAccumulator) discardFullFile() {
	if a.fullFile != nil {
		_ = a.fullFile.Close()
	}
	if a.fullPath != "" {
		_ = os.Remove(a.fullPath)
	}
	a.fullFile = nil
	a.fullPath = ""
}

func (a *outputAccumulator) snapshot() outputSnapshot {
	totalLines := a.totalLines()
	truncated := totalLines > maxOutputLines || a.totalDecodedBytes > maxOutputBytes
	if !truncated {
		return outputSnapshot{
			content: a.tail,
			truncation: truncationResult{
				totalLines:  totalLines,
				totalBytes:  a.totalDecodedBytes,
				outputLines: totalLines,
				outputBytes: a.totalDecodedBytes,
			},
			fullOutputPath: a.fullPath,
			lastLineBytes:  a.currentLineBytes,
		}
	}

	text := a.tail
	if !a.tailStartsAtLineBoundary {
		if firstNewline := strings.IndexByte(text, '\n'); firstNewline >= 0 {
			text = text[firstNewline+1:]
		}
	}
	content, outputLines, outputBytes, truncatedBy, partial := truncateTail(text)
	if truncatedBy == "" {
		if a.totalDecodedBytes > maxOutputBytes {
			truncatedBy = truncationByBytes
		} else {
			truncatedBy = truncationByLines
		}
	}
	return outputSnapshot{
		content: content,
		truncation: truncationResult{
			truncated:       true,
			truncatedBy:     truncatedBy,
			totalLines:      totalLines,
			totalBytes:      a.totalDecodedBytes,
			outputLines:     outputLines,
			outputBytes:     outputBytes,
			lastLinePartial: partial,
		},
		fullOutputPath: a.fullPath,
		lastLineBytes:  a.currentLineBytes,
	}
}

func truncateTail(content string) (string, int, int, truncationKind, bool) {
	lines := splitOutputLines(content)
	selected := make([]string, 0, min(len(lines), maxOutputLines))
	selectedBytes := 0
	truncatedBy := truncationKind("")
	partial := false

	for index := len(lines) - 1; index >= 0 && len(selected) < maxOutputLines; index-- {
		line := lines[index]
		lineBytes := len(line)
		if len(selected) > 0 {
			lineBytes++
		}
		if selectedBytes+lineBytes > maxOutputBytes {
			truncatedBy = truncationByBytes
			if len(selected) == 0 {
				line = utf8Suffix(line, maxOutputBytes)
				selected = append(selected, line)
				partial = true
			}
			break
		}
		selected = append(selected, line)
		selectedBytes += lineBytes
	}
	if len(selected) == maxOutputLines && len(lines) > len(selected) && truncatedBy == "" {
		truncatedBy = truncationByLines
	}
	for left, right := 0, len(selected)-1; left < right; left, right = left+1, right-1 {
		selected[left], selected[right] = selected[right], selected[left]
	}
	result := strings.Join(selected, "\n")
	return result, len(selected), len(result), truncatedBy, partial
}

func splitOutputLines(content string) []string {
	if content == "" {
		return nil
	}
	lines := strings.Split(content, "\n")
	if strings.HasSuffix(content, "\n") {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func utf8Suffix(text string, maxBytes int) string {
	buffer := []byte(text)
	if len(buffer) <= maxBytes {
		return text
	}
	start := len(buffer) - maxBytes
	for start < len(buffer) && !utf8.RuneStart(buffer[start]) {
		start++
	}
	return string(buffer[start:])
}

func formatOutput(snapshot outputSnapshot, emptyText string) string {
	text := snapshot.content
	if text == "" {
		text = emptyText
	}
	if !snapshot.truncation.truncated {
		return text
	}

	startLine := snapshot.truncation.totalLines - snapshot.truncation.outputLines + 1
	endLine := snapshot.truncation.totalLines
	fullOutputSuffix := "]"
	if snapshot.fullOutputPath != "" {
		fullOutputSuffix = fmt.Sprintf(". Full output: %s]", snapshot.fullOutputPath)
	}
	var footer string
	if snapshot.truncation.lastLinePartial {
		footer = fmt.Sprintf(
			"[Showing last %s of line %d (line is %s)%s",
			formatSize(snapshot.truncation.outputBytes), endLine, formatSize(snapshot.lastLineBytes), fullOutputSuffix,
		)
	} else if snapshot.truncation.truncatedBy == truncationByLines {
		footer = fmt.Sprintf(
			"[Showing lines %d-%d of %d%s",
			startLine, endLine, snapshot.truncation.totalLines, fullOutputSuffix,
		)
	} else {
		footer = fmt.Sprintf(
			"[Showing lines %d-%d of %d (%s limit)%s",
			startLine, endLine, snapshot.truncation.totalLines, formatSize(maxOutputBytes), fullOutputSuffix,
		)
	}
	if text == "" {
		return footer
	}
	return text + "\n\n" + footer
}

func formatSize(size int) string {
	switch {
	case size < 1024:
		return fmt.Sprintf("%dB", size)
	case size < 1024*1024:
		return fmt.Sprintf("%.1fKB", float64(size)/1024)
	default:
		return fmt.Sprintf("%.1fMB", float64(size)/(1024*1024))
	}
}
