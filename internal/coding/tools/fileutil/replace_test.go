package fileutil_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/yuanbohan/pi-go/internal/coding/tools/fileutil"
)

func TestReplaceRegularFileCancellationPreservesTargetAndRemovesTemporaryFile(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	target := filepath.Join(rootPath, "target.txt")
	if err := os.WriteFile(target, []byte("original"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatalf("OpenRoot() error = %v", err)
	}
	t.Cleanup(func() {
		if err := root.Close(); err != nil {
			t.Errorf("Root.Close() error = %v", err)
		}
	})
	ctx, cancel := context.WithCancel(context.Background())

	err = fileutil.ReplaceRegularFile(ctx, root, "target.txt", &cancelingReader{cancel: cancel})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ReplaceRegularFile() error = %v, want context.Canceled", err)
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got, want := string(content), "original"; got != want {
		t.Fatalf("target content = %q, want %q", got, want)
	}
	entries, err := os.ReadDir(rootPath)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "target.txt" {
		t.Fatalf("workspace contains temporary replacement after cancellation: %v", entries)
	}
}

type cancelingReader struct {
	cancel context.CancelFunc
	sent   bool
}

func (r *cancelingReader) Read(buffer []byte) (int, error) {
	if r.sent {
		return 0, io.EOF
	}
	r.sent = true
	n := copy(buffer, "replacement")
	r.cancel()
	return n, nil
}
