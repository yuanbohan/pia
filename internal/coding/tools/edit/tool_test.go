package edit_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yuanbohan/pia/internal/agent"
	"github.com/yuanbohan/pia/internal/coding/tools/edit"
	"github.com/yuanbohan/pia/internal/coding/tools/read"
)

func TestEditDefinition(t *testing.T) {
	t.Parallel()

	tool := newTool(t, t.TempDir())
	definition := tool.Definition()
	if got, want := definition.Schema.Name, "edit"; got != want {
		t.Fatalf("Name = %q, want %q", got, want)
	}
	if !strings.Contains(definition.Schema.Description, "workspace-relative") ||
		!strings.Contains(definition.Schema.Description, "original file") {
		t.Fatalf("Description = %q, want path boundary and original-file matching", definition.Schema.Description)
	}
	if definition.CanRunParallel {
		t.Fatal("CanRunParallel = true, want the default serial barrier")
	}

	var schema struct {
		Type                 string `json:"type"`
		AdditionalProperties *bool  `json:"additionalProperties"`
		Required             []string
		Properties           map[string]struct {
			Type     string `json:"type"`
			MinItems int    `json:"minItems"`
			Items    struct {
				Type                 string `json:"type"`
				AdditionalProperties *bool  `json:"additionalProperties"`
				Required             []string
				Properties           map[string]struct {
					Type string `json:"type"`
				}
			}
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
	if got, want := fmt.Sprint(schema.Required), "[path edits]"; got != want {
		t.Fatalf("required = %s, want %s", got, want)
	}
	if got := schema.Properties["path"].Type; got != "string" {
		t.Fatalf("path type = %q, want string", got)
	}
	edits := schema.Properties["edits"]
	if edits.Type != "array" || edits.MinItems != 1 {
		t.Fatalf("edits = type %q minItems %d, want array with minItems 1", edits.Type, edits.MinItems)
	}
	if edits.Items.Type != "object" {
		t.Fatalf("edits items type = %q, want object", edits.Items.Type)
	}
	if edits.Items.AdditionalProperties == nil || *edits.Items.AdditionalProperties {
		t.Fatalf("edits additionalProperties = %v, want false", edits.Items.AdditionalProperties)
	}
	if got, want := fmt.Sprint(edits.Items.Required), "[oldText newText]"; got != want {
		t.Fatalf("edits required = %s, want %s", got, want)
	}
	for _, name := range []string{"oldText", "newText"} {
		if got := edits.Items.Properties[name].Type; got != "string" {
			t.Fatalf("%s type = %q, want string", name, got)
		}
	}

	var _ agent.Tool = tool
}

func TestNewEditRejectsNilRoot(t *testing.T) {
	t.Parallel()

	if _, err := edit.New(nil); err == nil || !strings.Contains(err.Error(), "root is required") {
		t.Fatalf("New(nil) error = %v, want root-required error", err)
	}
}

func TestEditReplacesMultipleDisjointBlocksFromOriginalFile(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	target := filepath.Join(rootPath, "nested", "example.txt")
	writeFixtureFile(t, rootPath, "nested/example.txt", []byte("foo\nbar\nbaz\n"))
	if err := os.Chmod(target, 0o600); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}
	tool := newTool(t, rootPath)

	got, err := tool.Execute(context.Background(), json.RawMessage(`{
		"path":"nested/./example.txt",
		"edits":[
			{"oldText":"foo\n","newText":"foo bar\n"},
			{"oldText":"bar\n","newText":"BAR\n"}
		]
	}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if want := "Successfully replaced 2 block(s) in nested/example.txt"; got != want {
		t.Fatalf("Execute() = %q, want %q", got, want)
	}
	written, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got, want := string(written), "foo bar\nBAR\nbaz\n"; got != want {
		t.Fatalf("written content = %q, want %q", got, want)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Fatalf("permissions = %o, want %o", got, want)
	}
	entries, err := os.ReadDir(filepath.Dir(target))
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "example.txt" {
		t.Fatalf("target directory entries = %v, want only example.txt", entryNames(entries))
	}
}

func TestEditAllowsDeletingTheOnlyExactMatch(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	writeFixtureFile(t, rootPath, "example.txt", []byte("before\nremove me\nafter\n"))
	tool := newTool(t, rootPath)

	got, err := tool.Execute(context.Background(), json.RawMessage(`{
		"path":"example.txt",
		"edits":[{"oldText":"remove me\n","newText":""}]
	}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if want := "Successfully replaced 1 block(s) in example.txt"; got != want {
		t.Fatalf("Execute() = %q, want %q", got, want)
	}
	written, err := os.ReadFile(filepath.Join(rootPath, "example.txt"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got, want := string(written), "before\nafter\n"; got != want {
		t.Fatalf("written content = %q, want %q", got, want)
	}
}

func TestEditValidatesEveryMatchBeforeCommitting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		content   string
		edits     string
		wantError string
	}{
		{
			name:      "missing text",
			content:   "alpha\nbeta\n",
			edits:     `[{"oldText":"alpha\n","newText":"ALPHA\n"},{"oldText":"missing\n","newText":"MISSING\n"}]`,
			wantError: "could not find edits[1]",
		},
		{
			name:      "duplicate text",
			content:   "same\nsame\n",
			edits:     `[{"oldText":"same\n","newText":"changed\n"}]`,
			wantError: "multiple occurrences",
		},
		{
			name:      "overlapping duplicate text",
			content:   "aaa",
			edits:     `[{"oldText":"aa","newText":"changed"}]`,
			wantError: "multiple occurrences",
		},
		{
			name:      "overlapping regions",
			content:   "one\ntwo\nthree\n",
			edits:     `[{"oldText":"one\ntwo\n","newText":"ONE\nTWO\n"},{"oldText":"two\nthree\n","newText":"TWO\nTHREE\n"}]`,
			wantError: "overlap",
		},
		{
			name:      "no content change",
			content:   "unchanged\n",
			edits:     `[{"oldText":"unchanged\n","newText":"unchanged\n"}]`,
			wantError: "no changes",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			rootPath := t.TempDir()
			writeFixtureFile(t, rootPath, "example.txt", []byte(test.content))
			tool := newTool(t, rootPath)
			arguments := json.RawMessage(fmt.Sprintf(`{"path":"example.txt","edits":%s}`, test.edits))

			got, err := tool.Execute(context.Background(), arguments)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), test.wantError) {
				t.Fatalf("Execute() = %q, %v, want error containing %q", got, err, test.wantError)
			}
			written, readErr := os.ReadFile(filepath.Join(rootPath, "example.txt"))
			if readErr != nil {
				t.Fatalf("ReadFile() error = %v", readErr)
			}
			if got := string(written); got != test.content {
				t.Fatalf("failed edit changed content to %q, want original %q", got, test.content)
			}
		})
	}
}

func TestEditDoesNotGuessWithFuzzyMatching(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	content := "keep\nhello   \nafter\n"
	writeFixtureFile(t, rootPath, "example.txt", []byte(content))
	tool := newTool(t, rootPath)

	got, err := tool.Execute(context.Background(), json.RawMessage(`{
		"path":"example.txt",
		"edits":[{"oldText":"hello\n","newText":"changed\n"}]
	}`))
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "could not find") {
		t.Fatalf("Execute() = %q, %v, want exact-match failure", got, err)
	}
	written, readErr := os.ReadFile(filepath.Join(rootPath, "example.txt"))
	if readErr != nil {
		t.Fatalf("ReadFile() error = %v", readErr)
	}
	if got := string(written); got != content {
		t.Fatalf("fuzzy candidate changed content to %q, want original %q", got, content)
	}
}

func TestEditValidatesArguments(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	writeFixtureFile(t, rootPath, "example.txt", []byte("old"))
	tool := newTool(t, rootPath)
	tests := []struct {
		name      string
		arguments string
		want      string
	}{
		{name: "empty JSON", arguments: ``, want: "decode arguments"},
		{name: "malformed JSON", arguments: `{`, want: "decode arguments"},
		{name: "trailing JSON", arguments: `{"path":"example.txt","edits":[]}{} `, want: "one JSON object"},
		{name: "unknown top-level field", arguments: `{"path":"example.txt","edits":[],"extra":true}`, want: "unknown field"},
		{name: "unknown edit field", arguments: `{"path":"example.txt","edits":[{"oldText":"old","newText":"new","extra":true}]}`, want: "unknown field"},
		{name: "missing path", arguments: `{"edits":[{"oldText":"old","newText":"new"}]}`, want: "path is required"},
		{name: "blank path", arguments: `{"path":"  ","edits":[{"oldText":"old","newText":"new"}]}`, want: "path is required"},
		{name: "missing edits", arguments: `{"path":"example.txt"}`, want: "at least one replacement"},
		{name: "null edits", arguments: `{"path":"example.txt","edits":null}`, want: "at least one replacement"},
		{name: "empty edits", arguments: `{"path":"example.txt","edits":[]}`, want: "at least one replacement"},
		{name: "missing old text", arguments: `{"path":"example.txt","edits":[{"newText":"new"}]}`, want: "oldText is required"},
		{name: "null old text", arguments: `{"path":"example.txt","edits":[{"oldText":null,"newText":"new"}]}`, want: "oldText is required"},
		{name: "empty old text", arguments: `{"path":"example.txt","edits":[{"oldText":"","newText":"new"}]}`, want: "oldText must not be empty"},
		{name: "missing new text", arguments: `{"path":"example.txt","edits":[{"oldText":"old"}]}`, want: "newText is required"},
		{name: "null new text", arguments: `{"path":"example.txt","edits":[{"oldText":"old","newText":null}]}`, want: "newText is required"},
		{name: "non-string text", arguments: `{"path":"example.txt","edits":[{"oldText":1,"newText":"new"}]}`, want: "decode arguments"},
		{name: "absolute path", arguments: fmt.Sprintf(`{"path":%q,"edits":[{"oldText":"old","newText":"new"}]}`, filepath.Join(rootPath, "example.txt")), want: "workspace-relative"},
		{name: "parent traversal", arguments: `{"path":"../outside.txt","edits":[{"oldText":"old","newText":"new"}]}`, want: "workspace-relative"},
		{name: "nested parent component", arguments: `{"path":"dir/../example.txt","edits":[{"oldText":"old","newText":"new"}]}`, want: "must not contain .."},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := tool.Execute(context.Background(), json.RawMessage(test.arguments)); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Execute(%s) error = %v, want substring %q", test.arguments, err, test.want)
			}
		})
	}
}

func TestEditRejectsMissingAndInvalidUTF8Files(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	writeFixtureFile(t, rootPath, "invalid.txt", []byte{'o', 'l', 'd', 0xff})
	tool := newTool(t, rootPath)

	if _, err := tool.Execute(context.Background(), json.RawMessage(`{
		"path":"missing.txt",
		"edits":[{"oldText":"old","newText":"new"}]
	}`)); err == nil || !strings.Contains(err.Error(), "open") {
		t.Fatalf("missing Execute() error = %v, want open error", err)
	}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{
		"path":"invalid.txt",
		"edits":[{"oldText":"old","newText":"new"}]
	}`)); err == nil || !strings.Contains(err.Error(), "not valid UTF-8") {
		t.Fatalf("invalid UTF-8 Execute() error = %v, want UTF-8 error", err)
	}
	written, err := os.ReadFile(filepath.Join(rootPath, "invalid.txt"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if want := []byte{'o', 'l', 'd', 0xff}; string(written) != string(want) {
		t.Fatalf("invalid file changed to %v, want %v", written, want)
	}
}

func TestEditHonorsPreCanceledContextWithoutSideEffects(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	writeFixtureFile(t, rootPath, "example.txt", []byte("old"))
	tool := newTool(t, rootPath)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got, err := tool.Execute(ctx, json.RawMessage(`{
		"path":"example.txt",
		"edits":[{"oldText":"old","newText":"new"}]
	}`))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() = %q, %v, want context.Canceled", got, err)
	}
	written, readErr := os.ReadFile(filepath.Join(rootPath, "example.txt"))
	if readErr != nil {
		t.Fatalf("ReadFile() error = %v", readErr)
	}
	if got, want := string(written), "old"; got != want {
		t.Fatalf("written content = %q, want %q", got, want)
	}
}

func TestEditResultIsImmediatelyVisibleToRead(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	writeFixtureFile(t, rootPath, "shared.txt", []byte("first\nsecond\n"))
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatalf("OpenRoot() error = %v", err)
	}
	t.Cleanup(func() {
		if err := root.Close(); err != nil {
			t.Errorf("Root.Close() error = %v", err)
		}
	})
	editTool, err := edit.New(root)
	if err != nil {
		t.Fatalf("edit.New() error = %v", err)
	}
	readTool, err := read.New(root)
	if err != nil {
		t.Fatalf("read.New() error = %v", err)
	}

	if _, err := editTool.Execute(context.Background(), json.RawMessage(`{
		"path":"shared.txt",
		"edits":[{"oldText":"second\n","newText":"changed\n"}]
	}`)); err != nil {
		t.Fatalf("edit Execute() error = %v", err)
	}
	got, err := readTool.Execute(context.Background(), json.RawMessage(`{"path":"shared.txt"}`))
	if err != nil {
		t.Fatalf("read Execute() error = %v", err)
	}
	want := "Path: shared.txt\nLines: 1-2\nContent:\nfirst\nchanged\n\n[End of file.]"
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
