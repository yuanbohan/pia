package read_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadRejectsMissingAndNonRegularTargets(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	if err := os.Mkdir(filepath.Join(rootPath, "directory"), 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	read := newTool(t, rootPath)

	if _, err := read.Execute(context.Background(), json.RawMessage(`{"path":"missing.txt"}`)); err == nil || !strings.Contains(err.Error(), "open") {
		t.Fatalf("missing Execute() error = %v, want open error", err)
	}
	if _, err := read.Execute(context.Background(), json.RawMessage(`{"path":"directory"}`)); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("directory Execute() error = %v, want regular-file error", err)
	}
}

func TestReadAllowsInternalSymlinkAndRejectsEscapingSymlinks(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	rootPath := filepath.Join(parent, "workspace")
	outsidePath := filepath.Join(parent, "outside")
	if err := os.Mkdir(rootPath, 0o755); err != nil {
		t.Fatalf("Mkdir(root) error = %v", err)
	}
	if err := os.Mkdir(outsidePath, 0o755); err != nil {
		t.Fatalf("Mkdir(outside) error = %v", err)
	}
	writeTestFile(t, rootPath, "inside.txt", []byte("inside\n"))
	writeTestFile(t, outsidePath, "canary.txt", []byte("OUTSIDE-CANARY"))
	if err := os.Symlink("inside.txt", filepath.Join(rootPath, "internal-link")); err != nil {
		t.Fatalf("Symlink(internal) error = %v", err)
	}
	if err := os.Symlink(filepath.Join(outsidePath, "canary.txt"), filepath.Join(rootPath, "final-link")); err != nil {
		t.Fatalf("Symlink(final) error = %v", err)
	}
	if err := os.Symlink(outsidePath, filepath.Join(rootPath, "ancestor-link")); err != nil {
		t.Fatalf("Symlink(ancestor) error = %v", err)
	}
	if err := os.Symlink("missing.txt", filepath.Join(rootPath, "dangling-link")); err != nil {
		t.Fatalf("Symlink(dangling) error = %v", err)
	}
	read := newTool(t, rootPath)

	got, err := read.Execute(context.Background(), json.RawMessage(`{"path":"internal-link"}`))
	if err != nil {
		t.Fatalf("internal symlink Execute() error = %v", err)
	}
	want := readResult("internal-link", "1-1", "inside\n", "[End of file.]")
	if got != want {
		t.Fatalf("internal symlink Execute() = %q, want %q", got, want)
	}

	for _, path := range []string{"final-link", "ancestor-link/canary.txt", "dangling-link"} {
		t.Run(path, func(t *testing.T) {
			got, err := read.Execute(context.Background(), json.RawMessage(fmt.Sprintf(`{"path":%q}`, path)))
			if err == nil {
				t.Fatalf("Execute(%q) = %q, nil, want boundary error", path, got)
			}
			if strings.Contains(got, "OUTSIDE-CANARY") {
				t.Fatalf("Execute(%q) exposed outside canary: %q", path, got)
			}
		})
	}
}

func TestReadRootBoundarySurvivesSymlinkSwap(t *testing.T) {
	parent := t.TempDir()
	rootPath := filepath.Join(parent, "workspace")
	outsidePath := filepath.Join(parent, "outside.txt")
	if err := os.Mkdir(rootPath, 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	writeTestFile(t, rootPath, "inside.txt", []byte("inside\n"))
	if err := os.WriteFile(outsidePath, []byte("OUTSIDE-SWAP-CANARY"), 0o644); err != nil {
		t.Fatalf("WriteFile(outside) error = %v", err)
	}
	target := filepath.Join(rootPath, "swap")
	if err := os.Symlink("inside.txt", target); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	read := newTool(t, rootPath)

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for index := 0; ; index++ {
			select {
			case <-stop:
				return
			default:
			}
			temporary := filepath.Join(rootPath, fmt.Sprintf("swap-%d", index))
			linkTarget := "inside.txt"
			if index%2 == 1 {
				linkTarget = outsidePath
			}
			if err := os.Symlink(linkTarget, temporary); err != nil {
				continue
			}
			if err := os.Rename(temporary, target); err != nil {
				_ = os.Remove(temporary)
			}
		}
	}()

	for range 300 {
		got, err := read.Execute(context.Background(), json.RawMessage(`{"path":"swap"}`))
		if err == nil && strings.Contains(got, "OUTSIDE-SWAP-CANARY") {
			close(stop)
			<-done
			t.Fatalf("Execute() escaped root during symlink swap: %q", got)
		}
	}
	close(stop)
	<-done
}
