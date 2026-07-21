//go:build darwin || linux

package bash_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yuanbohan/pia/internal/coding"
	"github.com/yuanbohan/pia/internal/coding/tools/bash"
	"github.com/yuanbohan/pia/internal/coding/tools/edit"
	"github.com/yuanbohan/pia/internal/coding/tools/read"
)

func TestBashCreatedFileIsVisibleToRead(t *testing.T) {
	t.Parallel()

	workspace := openWorkspace(t)
	bashTool := newBashTool(t, workspace)
	readTool, err := read.New(workspace.Root())
	if err != nil {
		t.Fatalf("read.New() error = %v", err)
	}

	if _, err := bashTool.Execute(context.Background(), json.RawMessage(
		`{"command":"mkdir -p generated && printf 'from bash\n' > generated/result.txt"}`,
	)); err != nil {
		t.Fatalf("bash Execute() error = %v", err)
	}
	result, err := readTool.Execute(context.Background(), json.RawMessage(`{"path":"generated/result.txt"}`))
	if err != nil {
		t.Fatalf("read Execute() error = %v", err)
	}
	if !strings.Contains(result, "from bash\n") {
		t.Fatalf("read result = %q, want bash-created content", result)
	}
}

func TestEditResultIsVisibleToBash(t *testing.T) {
	t.Parallel()

	workspace := openWorkspace(t)
	if err := os.WriteFile(filepath.Join(workspace.Path(), "message.txt"), []byte("before\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	editTool, err := edit.New(workspace.Root())
	if err != nil {
		t.Fatalf("edit.New() error = %v", err)
	}
	bashTool := newBashTool(t, workspace)

	if _, err := editTool.Execute(context.Background(), json.RawMessage(
		`{"path":"message.txt","edits":[{"oldText":"before","newText":"after"}]}`,
	)); err != nil {
		t.Fatalf("edit Execute() error = %v", err)
	}
	result, err := bashTool.Execute(context.Background(), json.RawMessage(`{"command":"cat message.txt"}`))
	if err != nil {
		t.Fatalf("bash Execute() error = %v", err)
	}
	if result != "after\n" {
		t.Fatalf("bash result = %q, want edited content", result)
	}
}

func openWorkspace(t *testing.T) *coding.Workspace {
	t.Helper()
	workspace, err := coding.OpenWorkspace(t.TempDir())
	if err != nil {
		t.Fatalf("OpenWorkspace() error = %v", err)
	}
	t.Cleanup(func() {
		if err := workspace.Close(); err != nil {
			t.Errorf("Workspace.Close() error = %v", err)
		}
	})
	return workspace
}

func newBashTool(t *testing.T, workspace *coding.Workspace) *bash.Tool {
	t.Helper()
	tool, err := bash.New(bash.Config{WorkingDirectory: workspace.Path(), ShellPath: "/bin/sh"})
	if err != nil {
		t.Fatalf("bash.New() error = %v", err)
	}
	return tool
}
