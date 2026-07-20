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
