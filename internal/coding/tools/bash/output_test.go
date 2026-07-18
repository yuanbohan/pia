package bash

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOutputAccumulatorPreservesSmallOutput(t *testing.T) {
	t.Parallel()

	output := newOutputAccumulator()
	if err := output.append([]byte("first\nsecond\n")); err != nil {
		t.Fatalf("append() error = %v", err)
	}
	snapshot, err := output.finish()
	if err != nil {
		t.Fatalf("finish() error = %v", err)
	}
	if snapshot.content != "first\nsecond\n" {
		t.Fatalf("snapshot content = %q, want original output", snapshot.content)
	}
	if snapshot.truncation.truncated || snapshot.fullOutputPath != "" {
		t.Fatalf("snapshot = %#v, want no truncation or temp file", snapshot)
	}
	if snapshot.truncation.totalLines != 2 || snapshot.truncation.totalBytes != 13 {
		t.Fatalf("truncation = %#v, want 2 lines and 13 bytes", snapshot.truncation)
	}
}

func TestOutputAccumulatorKeepsLastTwoThousandLinesAndPersistsRawOutput(t *testing.T) {
	t.Parallel()

	var raw strings.Builder
	for line := 1; line <= 2100; line++ {
		fmt.Fprintf(&raw, "line-%04d\n", line)
	}

	output := newOutputAccumulator()
	rawBytes := []byte(raw.String())
	cut := len(rawBytes) / 2
	if err := output.append(rawBytes[:cut]); err != nil {
		t.Fatalf("first append() error = %v", err)
	}
	if err := output.append(rawBytes[cut:]); err != nil {
		t.Fatalf("second append() error = %v", err)
	}
	snapshot, err := output.finish()
	if err != nil {
		t.Fatalf("finish() error = %v", err)
	}
	removeFullOutput(t, snapshot.fullOutputPath)

	if !snapshot.truncation.truncated || snapshot.truncation.truncatedBy != truncationByLines {
		t.Fatalf("truncation = %#v, want line truncation", snapshot.truncation)
	}
	if snapshot.truncation.totalLines != 2100 || snapshot.truncation.outputLines != 2000 {
		t.Fatalf("truncation = %#v, want 2000 of 2100 lines", snapshot.truncation)
	}
	if !strings.HasPrefix(snapshot.content, "line-0101\n") || !strings.HasSuffix(snapshot.content, "line-2100") {
		t.Fatalf("snapshot tail boundaries are wrong (length %d)", len(snapshot.content))
	}
	assertFullOutput(t, snapshot.fullOutputPath, rawBytes)
	formatted := formatOutput(snapshot, "")
	wantFooter := fmt.Sprintf("[Showing lines 101-2100 of 2100. Full output: %s]", snapshot.fullOutputPath)
	if !strings.HasSuffix(formatted, wantFooter) {
		t.Fatalf("formatOutput() = %q, want suffix %q", formatted, wantFooter)
	}
}

func TestOutputAccumulatorKeepsUtf8SafeTailOfOneLongLine(t *testing.T) {
	t.Parallel()

	raw := bytes.Repeat([]byte("€"), 20_000)
	output := newOutputAccumulator()
	if err := output.append(raw); err != nil {
		t.Fatalf("append() error = %v", err)
	}
	snapshot, err := output.finish()
	if err != nil {
		t.Fatalf("finish() error = %v", err)
	}
	removeFullOutput(t, snapshot.fullOutputPath)

	if !snapshot.truncation.truncated || snapshot.truncation.truncatedBy != truncationByBytes {
		t.Fatalf("truncation = %#v, want byte truncation", snapshot.truncation)
	}
	if !snapshot.truncation.lastLinePartial || snapshot.truncation.totalLines != 1 {
		t.Fatalf("truncation = %#v, want one partial line", snapshot.truncation)
	}
	if len(snapshot.content) > maxOutputBytes || !strings.HasSuffix(string(raw), snapshot.content) {
		t.Fatalf("snapshot content has %d bytes, want UTF-8-safe tail within %d", len(snapshot.content), maxOutputBytes)
	}
	assertFullOutput(t, snapshot.fullOutputPath, raw)
}

func TestOutputAccumulatorDecodesUtf8AcrossChunks(t *testing.T) {
	t.Parallel()

	output := newOutputAccumulator()
	euro := []byte("€")
	if err := output.append(euro[:1]); err != nil {
		t.Fatalf("first append() error = %v", err)
	}
	if err := output.append(euro[1:]); err != nil {
		t.Fatalf("second append() error = %v", err)
	}
	snapshot, err := output.finish()
	if err != nil {
		t.Fatalf("finish() error = %v", err)
	}
	if snapshot.content != "€" {
		t.Fatalf("snapshot content = %q, want one decoded rune", snapshot.content)
	}
}

func TestOutputAccumulatorReplacesIncompleteFinalUtf8(t *testing.T) {
	t.Parallel()

	output := newOutputAccumulator()
	if err := output.append([]byte{0xe2, 0x82}); err != nil {
		t.Fatalf("append() error = %v", err)
	}
	snapshot, err := output.finish()
	if err != nil {
		t.Fatalf("finish() error = %v", err)
	}
	if snapshot.content != "�" {
		t.Fatalf("snapshot content = %q, want replacement rune", snapshot.content)
	}
	if snapshot.truncation.totalBytes != len([]byte("�")) {
		t.Fatalf("total decoded bytes = %d, want replacement-rune bytes", snapshot.truncation.totalBytes)
	}
}

func TestOutputAccumulatorRejectsAppendAfterFinish(t *testing.T) {
	t.Parallel()

	output := newOutputAccumulator()
	if _, err := output.finish(); err != nil {
		t.Fatalf("finish() error = %v", err)
	}
	if err := output.append([]byte("late")); err == nil || !strings.Contains(err.Error(), "finished") {
		t.Fatalf("append() error = %v, want finished error", err)
	}
}

func TestOutputAccumulatorDoesNotRetryFailedPersistence(t *testing.T) {
	missingTempDirectory := filepath.Join(t.TempDir(), "missing")
	t.Setenv("TMPDIR", missingTempDirectory)

	output := newOutputAccumulator()
	err := output.append(bytes.Repeat([]byte("x"), maxOutputBytes+1))
	if err == nil || !strings.Contains(err.Error(), "create complete-output temp file") {
		t.Fatalf("append() error = %v, want temp-file creation failure", err)
	}
	if err := os.Mkdir(missingTempDirectory, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	snapshot, err := output.finish()
	if err != nil {
		t.Fatalf("finish() error = %v, want no misleading persistence retry", err)
	}
	if snapshot.fullOutputPath != "" {
		t.Fatalf("full output path = %q, want none after persistence failure", snapshot.fullOutputPath)
	}
	if formatted := formatOutput(snapshot, ""); strings.Contains(formatted, "Full output:") {
		t.Fatalf("formatOutput() = %q, want no misleading complete-output path", formatted)
	}
}

func assertFullOutput(t *testing.T, path string, want []byte) {
	t.Helper()
	if path == "" {
		t.Fatal("full output path is empty")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(full output) error = %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("full output has %d bytes, want exact %d-byte raw stream", len(got), len(want))
	}
}

func removeFullOutput(t *testing.T, path string) {
	t.Helper()
	if path != "" {
		t.Cleanup(func() { _ = os.Remove(path) })
	}
}
