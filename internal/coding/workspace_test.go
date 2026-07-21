package coding_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yuanbohan/pia/internal/coding"
)

func TestOpenWorkspaceOwnsCanonicalRoot(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	target := filepath.Join(parent, "workspace")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	alias := filepath.Join(parent, "alias")
	if err := os.Symlink(target, alias); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	workspace, err := coding.OpenWorkspace(alias)
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}

	wantPath, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatalf("EvalSymlinks() error = %v", err)
	}
	wantPath, err = filepath.Abs(wantPath)
	if err != nil {
		t.Fatalf("Abs() error = %v", err)
	}
	if got := workspace.Path(); got != wantPath {
		t.Fatalf("Path() = %q, want %q", got, wantPath)
	}

	file, err := workspace.Root().Open("main.go")
	if err != nil {
		t.Fatalf("Root().Open() error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("file.Close() error = %v", err)
	}
	if err := workspace.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := workspace.Root().Open("main.go"); err == nil {
		t.Fatal("Root().Open() after Close() error = nil, want error")
	}
}

func TestOpenWorkspaceRejectsInvalidRoots(t *testing.T) {
	t.Parallel()

	file := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(file, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "empty", path: " ", want: "path is required"},
		{name: "missing", path: filepath.Join(t.TempDir(), "missing"), want: "open workspace"},
		{name: "regular file", path: file, want: "open workspace"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := coding.OpenWorkspace(test.path)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("OpenWorkspace() error = %v, want substring %q", err, test.want)
			}
		})
	}
}
