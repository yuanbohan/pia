package write_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yuanbohan/pia/internal/coding/tools/write"
)

func newTool(t *testing.T, rootPath string) *write.Tool {
	t.Helper()
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatalf("OpenRoot() error = %v", err)
	}
	t.Cleanup(func() {
		if err := root.Close(); err != nil {
			t.Errorf("Root.Close() error = %v", err)
		}
	})
	tool, err := write.New(root)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return tool
}

func writeFixtureFile(t *testing.T, rootPath, relativePath string, content []byte) {
	t.Helper()
	path := filepath.Join(rootPath, relativePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", relativePath, err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", relativePath, err)
	}
}
