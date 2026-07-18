package write_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yuanbohan/pi-go/internal/agent"
	"github.com/yuanbohan/pi-go/internal/coding/tools/read"
	"github.com/yuanbohan/pi-go/internal/coding/tools/write"
)

func TestWriteDefinition(t *testing.T) {
	t.Parallel()

	write := newTool(t, t.TempDir())
	definition := write.Definition()
	if got, want := definition.Schema.Name, "write"; got != want {
		t.Fatalf("Name = %q, want %q", got, want)
	}
	if !strings.Contains(definition.Schema.Description, "workspace-relative") {
		t.Fatalf("Description = %q, want workspace-relative boundary", definition.Schema.Description)
	}
	if definition.CanRunParallel {
		t.Fatal("CanRunParallel = true, want the default serial barrier")
	}

	var schema struct {
		Type                 string `json:"type"`
		AdditionalProperties *bool  `json:"additionalProperties"`
		Required             []string
		Properties           map[string]struct {
			Type string `json:"type"`
		}
	}
	if err := json.Unmarshal(definition.Schema.Parameters, &schema); err != nil {
		t.Fatalf("Parameters JSON error = %v", err)
	}
	if schema.Type != "object" {
		t.Fatalf("schema type = %q, want object", schema.Type)
	}
	if schema.AdditionalProperties == nil || *schema.AdditionalProperties {
		t.Fatalf("additionalProperties = %v, want false", schema.AdditionalProperties)
	}
	if got, want := fmt.Sprint(schema.Required), "[path content]"; got != want {
		t.Fatalf("required = %s, want %s", got, want)
	}
	for _, name := range []string{"path", "content"} {
		if got := schema.Properties[name].Type; got != "string" {
			t.Fatalf("%s type = %q, want string", name, got)
		}
	}

	var _ agent.Tool = write
}

func TestNewWriteRejectsNilRoot(t *testing.T) {
	t.Parallel()

	if _, err := write.New(nil); err == nil || !strings.Contains(err.Error(), "root is required") {
		t.Fatalf("New(nil) error = %v, want root-required error", err)
	}
}

func TestWriteCreatesParentsAndReportsActualUTF8Bytes(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	write := newTool(t, rootPath)
	content := "你好\n"
	arguments := json.RawMessage(fmt.Sprintf(`{"path":"nested/./hello.txt","content":%q}`, content))

	got, err := write.Execute(context.Background(), arguments)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := "Successfully wrote 7 bytes to nested/hello.txt"
	if got != want {
		t.Fatalf("Execute() = %q, want %q", got, want)
	}
	if strings.Contains(got, rootPath) || strings.Contains(got, content) {
		t.Fatalf("Execute() exposed host path or file content: %q", got)
	}

	written, err := os.ReadFile(filepath.Join(rootPath, "nested", "hello.txt"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got, want := string(written), content; got != want {
		t.Fatalf("written content = %q, want %q", got, want)
	}
}

func TestWriteOverwritesRegularFileAndPreservesPermissions(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	target := filepath.Join(rootPath, "config.txt")
	if err := os.WriteFile(target, []byte("old content"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Chmod(target, 0o600); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}
	write := newTool(t, rootPath)

	if _, err := write.Execute(context.Background(), json.RawMessage(`{"path":"config.txt","content":"new content"}`)); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	written, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got, want := string(written), "new content"; got != want {
		t.Fatalf("written content = %q, want %q", got, want)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Fatalf("permissions = %o, want %o", got, want)
	}
	entries, err := os.ReadDir(rootPath)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "config.txt" {
		t.Fatalf("workspace entries = %v, want only config.txt", entryNames(entries))
	}
}

func TestWriteAcceptsEmptyAndLargeContent(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	write := newTool(t, rootPath)

	if got, err := write.Execute(context.Background(), json.RawMessage(`{"path":"empty.txt","content":""}`)); err != nil {
		t.Fatalf("empty Execute() error = %v", err)
	} else if got != "Successfully wrote 0 bytes to empty.txt" {
		t.Fatalf("empty Execute() = %q", got)
	}
	if info, err := os.Stat(filepath.Join(rootPath, "empty.txt")); err != nil {
		t.Fatalf("Stat(empty) error = %v", err)
	} else if info.Size() != 0 {
		t.Fatalf("empty file size = %d, want 0", info.Size())
	}

	content := strings.Repeat("x", 16<<10)
	arguments := json.RawMessage(fmt.Sprintf(`{"path":"large.txt","content":%q}`, content))
	if _, err := write.Execute(context.Background(), arguments); err != nil {
		t.Fatalf("large Execute() error = %v; write content must not inherit read's argument limit", err)
	}
	if written, err := os.ReadFile(filepath.Join(rootPath, "large.txt")); err != nil {
		t.Fatalf("ReadFile(large) error = %v", err)
	} else if got := string(written); got != content {
		t.Fatalf("large content length = %d, want %d", len(got), len(content))
	}
}

func TestWriteValidatesArguments(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	write := newTool(t, rootPath)
	tests := []struct {
		name      string
		arguments string
		want      string
	}{
		{name: "empty JSON", arguments: ``, want: "decode arguments"},
		{name: "malformed JSON", arguments: `{`, want: "decode arguments"},
		{name: "trailing JSON", arguments: `{"path":"file.txt","content":"x"}{}`, want: "one JSON object"},
		{name: "unknown field", arguments: `{"path":"file.txt","content":"x","extra":true}`, want: "unknown field"},
		{name: "missing path", arguments: `{"content":"x"}`, want: "path is required"},
		{name: "missing content", arguments: `{"path":"file.txt"}`, want: "content is required"},
		{name: "null content", arguments: `{"path":"file.txt","content":null}`, want: "content is required"},
		{name: "non-string content", arguments: `{"path":"file.txt","content":1}`, want: "decode arguments"},
		{name: "blank path", arguments: `{"path":"  ","content":"x"}`, want: "path is required"},
		{name: "absolute path", arguments: fmt.Sprintf(`{"path":%q,"content":"x"}`, filepath.Join(rootPath, "file.txt")), want: "workspace-relative"},
		{name: "parent traversal", arguments: `{"path":"../outside.txt","content":"x"}`, want: "workspace-relative"},
		{name: "nested parent component", arguments: `{"path":"dir/../file.txt","content":"x"}`, want: "must not contain .."},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := write.Execute(context.Background(), json.RawMessage(test.arguments)); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Execute(%s) error = %v, want substring %q", test.arguments, err, test.want)
			}
		})
	}
}

func TestWriteHonorsPreCanceledContextWithoutSideEffects(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	write := newTool(t, rootPath)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got, err := write.Execute(ctx, json.RawMessage(`{"path":"new/file.txt","content":"not written"}`))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() = %q, %v, want context.Canceled", got, err)
	}
	if _, err := os.Lstat(filepath.Join(rootPath, "new")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Lstat(new parent) error = %v, want not-exist", err)
	}
}

func TestWriteResultIsImmediatelyVisibleToRead(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatalf("OpenRoot() error = %v", err)
	}
	t.Cleanup(func() {
		if err := root.Close(); err != nil {
			t.Errorf("Root.Close() error = %v", err)
		}
	})
	write, err := write.New(root)
	if err != nil {
		t.Fatalf("write.New() error = %v", err)
	}
	read, err := read.New(root)
	if err != nil {
		t.Fatalf("read.New() error = %v", err)
	}

	if _, err := write.Execute(context.Background(), json.RawMessage(`{"path":"shared.txt","content":"first\nsecond\n"}`)); err != nil {
		t.Fatalf("write Execute() error = %v", err)
	}
	got, err := read.Execute(context.Background(), json.RawMessage(`{"path":"shared.txt"}`))
	if err != nil {
		t.Fatalf("read Execute() error = %v", err)
	}
	want := "Path: shared.txt\nLines: 1-2\nContent:\nfirst\nsecond\n\n[End of file.]"
	if got != want {
		t.Fatalf("read Execute() = %q, want %q", got, want)
	}
}

func entryNames(entries []os.DirEntry) []string {
	names := make([]string, len(entries))
	for index, entry := range entries {
		names[index] = entry.Name()
	}
	return names
}
