package fileutil_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yuanbohan/pi-go/internal/coding/tools/fileutil"
)

func TestOpenRegularHostFileRequiresAbsoluteRegularFile(t *testing.T) {
	t.Parallel()

	if _, err := fileutil.OpenRegularHostFile("relative.txt"); err == nil || !strings.Contains(err.Error(), "absolute path is required") {
		t.Fatalf("OpenRegularHostFile(relative) error = %v, want absolute-path error", err)
	}

	path := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(path, []byte("outside"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	file, err := fileutil.OpenRegularHostFile(path)
	if err != nil {
		t.Fatalf("OpenRegularHostFile() error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestOpenDirectoryValidatesOpenedHandle(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	if err := os.Mkdir(filepath.Join(rootPath, "skills"), 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(rootPath, "regular.txt"), []byte("ordinary file"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatalf("OpenRoot() error = %v", err)
	}
	t.Cleanup(func() {
		if err := root.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	directory, err := fileutil.OpenDirectory(root, "skills")
	if err != nil {
		t.Fatalf("OpenDirectory() error = %v", err)
	}
	if err := directory.Close(); err != nil {
		t.Fatalf("directory Close() error = %v", err)
	}
	if _, err := fileutil.OpenDirectory(root, "regular.txt"); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("OpenDirectory(regular file) error = %v, want directory error", err)
	}
}
