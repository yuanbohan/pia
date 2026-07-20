//go:build darwin || linux

package read_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestReadRejectsFIFOWithoutWaitingForAWriter(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	fifoPath := filepath.Join(rootPath, "events.fifo")
	if err := syscall.Mkfifo(fifoPath, 0o600); err != nil {
		t.Fatalf("Mkfifo() error = %v", err)
	}
	read := newTool(t, rootPath)

	type outcome struct {
		content string
		err     error
	}
	for _, arguments := range []json.RawMessage{
		json.RawMessage(`{"path":"events.fifo"}`),
		json.RawMessage(fmt.Sprintf(`{"path":%q}`, fifoPath)),
	} {
		result := make(chan outcome, 1)
		go func() {
			content, err := read.Execute(context.Background(), arguments)
			result <- outcome{content: content, err: err}
		}()

		timer := time.NewTimer(2 * time.Second)
		select {
		case got := <-result:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			if got.err == nil || !strings.Contains(got.err.Error(), "not a regular file") {
				t.Fatalf("Execute(%s) = %q, %v, want regular-file error", arguments, got.content, got.err)
			}
		case <-timer.C:
			t.Fatalf("Execute(%s) blocked opening a FIFO with no writer", arguments)
		}
	}
}
