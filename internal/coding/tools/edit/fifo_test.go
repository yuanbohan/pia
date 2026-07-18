//go:build darwin || linux

package edit_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestEditRejectsFIFOWithoutWaitingForAWriter(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(rootPath, "events.fifo"), 0o600); err != nil {
		t.Fatalf("Mkfifo() error = %v", err)
	}
	tool := newTool(t, rootPath)

	type outcome struct {
		content string
		err     error
	}
	result := make(chan outcome, 1)
	go func() {
		content, err := tool.Execute(context.Background(), json.RawMessage(`{
			"path":"events.fifo",
			"edits":[{"oldText":"event","newText":"changed"}]
		}`))
		result <- outcome{content: content, err: err}
	}()

	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case got := <-result:
		if got.err == nil || !strings.Contains(got.err.Error(), "not a regular file") {
			t.Fatalf("Execute() = %q, %v, want regular-file error", got.content, got.err)
		}
	case <-timer.C:
		t.Fatal("Execute() blocked opening a FIFO with no writer")
	}
}
