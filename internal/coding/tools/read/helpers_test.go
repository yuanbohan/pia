package read_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yuanbohan/pi-go/internal/coding/tools/read"
)

func newTool(t *testing.T, rootPath string) *read.Tool {
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
	tool, err := read.New(root)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return tool
}

func writeTestFile(t *testing.T, rootPath, relativePath string, content []byte) {
	t.Helper()
	path := filepath.Join(rootPath, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func readResult(path, lines, content, footer string) string {
	result := fmt.Sprintf("Path: %s\nLines: %s\nContent:\n%s", path, lines, content)
	if !strings.HasSuffix(content, "\n") {
		result += "\n"
	}
	return result + "\n" + footer
}
