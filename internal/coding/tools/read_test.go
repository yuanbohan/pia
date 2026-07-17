package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/yuanbohan/pi-go/internal/agent"
	codingtools "github.com/yuanbohan/pi-go/internal/coding/tools"
)

func TestReadDefinition(t *testing.T) {
	t.Parallel()

	read := newRead(t, t.TempDir())
	definition := read.Definition()
	if got, want := definition.Schema.Name, "read"; got != want {
		t.Fatalf("Name = %q, want %q", got, want)
	}
	if !strings.Contains(definition.Schema.Description, "workspace-relative") {
		t.Fatalf("Description = %q, want workspace-relative boundary", definition.Schema.Description)
	}
	if !definition.CanRunParallel {
		t.Fatal("CanRunParallel = false, want true")
	}

	var schema struct {
		Type                 string `json:"type"`
		AdditionalProperties *bool  `json:"additionalProperties"`
		Required             []string
		Properties           map[string]struct {
			Type    string `json:"type"`
			Minimum int    `json:"minimum"`
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
	if got, want := fmt.Sprint(schema.Required), "[path]"; got != want {
		t.Fatalf("required = %s, want %s", got, want)
	}
	if got := schema.Properties["path"].Type; got != "string" {
		t.Fatalf("path type = %q, want string", got)
	}
	for _, name := range []string{"offset", "limit"} {
		property := schema.Properties[name]
		if property.Type != "integer" || property.Minimum != 1 {
			t.Fatalf("%s property = %#v, want integer minimum 1", name, property)
		}
	}

	var _ agent.Tool = read
}

func TestNewReadRejectsNilRoot(t *testing.T) {
	t.Parallel()

	if _, err := codingtools.NewRead(nil); err == nil || !strings.Contains(err.Error(), "root is required") {
		t.Fatalf("NewRead(nil) error = %v, want root-required error", err)
	}
}

func TestReadReturnsStableWorkspaceRelativeResult(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	writeTestFile(t, rootPath, "dir/file.txt", []byte("alpha\r\nbeta\nomega"))
	read := newRead(t, rootPath)

	got, err := read.Execute(context.Background(), json.RawMessage(`{"path":"dir/./file.txt"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := readResult("dir/file.txt", "1-3", "alpha\r\nbeta\nomega", "[End of file.]")
	if got != want {
		t.Fatalf("Execute() = %q, want %q", got, want)
	}
	if strings.Contains(got, rootPath) {
		t.Fatalf("Execute() exposed host root %q in %q", rootPath, got)
	}
}

func TestReadPaginatesWithOneBasedOffsetAndLimit(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	writeTestFile(t, rootPath, "notes.txt", []byte("one\ntwo\nthree\n"))
	writeTestFile(t, rootPath, "single.txt", []byte("single\n"))
	read := newRead(t, rootPath)

	got, err := read.Execute(context.Background(), json.RawMessage(`{"path":"notes.txt","offset":2,"limit":1}`))
	if err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}
	want := readResult("notes.txt", "2-2", "two\n", "[More content available. Continue with offset=3.]")
	if got != want {
		t.Fatalf("first Execute() = %q, want %q", got, want)
	}

	got, err = read.Execute(context.Background(), json.RawMessage(`{"path":"notes.txt","offset":3}`))
	if err != nil {
		t.Fatalf("second Execute() error = %v", err)
	}
	want = readResult("notes.txt", "3-3", "three\n", "[End of file.]")
	if got != want {
		t.Fatalf("second Execute() = %q, want %q", got, want)
	}

	got, err = read.Execute(context.Background(), json.RawMessage(`{"path":"single.txt","limit":1}`))
	if err != nil {
		t.Fatalf("single-line Execute() error = %v", err)
	}
	want = readResult("single.txt", "1-1", "single\n", "[End of file.]")
	if got != want {
		t.Fatalf("single-line Execute() = %q, want no empty trailing page: %q", got, want)
	}
}

func TestReadEmptyFileAndOffsetPastEOF(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	writeTestFile(t, rootPath, "empty.txt", nil)
	writeTestFile(t, rootPath, "one.txt", []byte("only line"))
	read := newRead(t, rootPath)

	got, err := read.Execute(context.Background(), json.RawMessage(`{"path":"empty.txt"}`))
	if err != nil {
		t.Fatalf("empty Execute() error = %v", err)
	}
	want := "Path: empty.txt\nLines: 0\nContent:\n[empty file]\n\n[End of file.]"
	if got != want {
		t.Fatalf("empty Execute() = %q, want %q", got, want)
	}

	for _, arguments := range []string{
		`{"path":"empty.txt","offset":2}`,
		`{"path":"one.txt","offset":2}`,
	} {
		if _, err := read.Execute(context.Background(), json.RawMessage(arguments)); err == nil || !strings.Contains(err.Error(), "past end of file") {
			t.Fatalf("Execute(%s) error = %v, want past-EOF error", arguments, err)
		}
	}
}

func TestReadAppliesLineAndByteLimitsBeforeTranscript(t *testing.T) {
	t.Parallel()

	t.Run("line limit", func(t *testing.T) {
		t.Parallel()
		rootPath := t.TempDir()
		writeTestFile(t, rootPath, "many.txt", []byte(strings.Repeat("x\n", 2001)))
		read := newRead(t, rootPath)

		got, err := read.Execute(context.Background(), json.RawMessage(`{"path":"many.txt"}`))
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		want := readResult("many.txt", "1-2000", strings.Repeat("x\n", 2000), "[More content available. Continue with offset=2001.]")
		if got != want {
			t.Fatalf("Execute() line-limited result mismatch: got len %d, want len %d", len(got), len(want))
		}
	})

	t.Run("byte limit keeps complete lines", func(t *testing.T) {
		t.Parallel()
		rootPath := t.TempDir()
		first := strings.Repeat("a", 30<<10) + "\n"
		second := strings.Repeat("b", 30<<10) + "\n"
		writeTestFile(t, rootPath, "wide.txt", []byte(first+second))
		read := newRead(t, rootPath)

		got, err := read.Execute(context.Background(), json.RawMessage(`{"path":"wide.txt"}`))
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		want := readResult("wide.txt", "1-1", first, "[More content available. Continue with offset=2.]")
		if got != want {
			t.Fatalf("Execute() byte-limited result mismatch: got len %d, want len %d", len(got), len(want))
		}
		if strings.Contains(got, "bbb") {
			t.Fatal("Execute() returned a partial second line")
		}
	})

	t.Run("one line cannot exceed byte limit", func(t *testing.T) {
		t.Parallel()
		rootPath := t.TempDir()
		writeTestFile(t, rootPath, "minified.txt", []byte(strings.Repeat("x", (50<<10)+1)))
		read := newRead(t, rootPath)

		got, err := read.Execute(context.Background(), json.RawMessage(`{"path":"minified.txt"}`))
		if err == nil || !strings.Contains(err.Error(), "line 1 exceeds the 51200-byte limit") {
			t.Fatalf("Execute() = %q, %v, want oversized-line error", got, err)
		}
		if got != "" {
			t.Fatalf("Execute() content = %q, want no partial line", got)
		}
	})

	t.Run("exact byte limit includes final newline", func(t *testing.T) {
		t.Parallel()
		rootPath := t.TempDir()
		content := strings.Repeat("x", (50<<10)-1) + "\n"
		writeTestFile(t, rootPath, "exact.txt", []byte(content))
		read := newRead(t, rootPath)

		got, err := read.Execute(context.Background(), json.RawMessage(`{"path":"exact.txt"}`))
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		want := readResult("exact.txt", "1-1", content, "[End of file.]")
		if got != want {
			t.Fatalf("Execute() exact-limit result mismatch: got len %d, want len %d", len(got), len(want))
		}
	})
}

func TestReadRejectsInvalidUTF8WithoutReplacement(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	writeTestFile(t, rootPath, "binary.dat", []byte{'o', 'k', '\n', 0xff})
	read := newRead(t, rootPath)

	got, err := read.Execute(context.Background(), json.RawMessage(`{"path":"binary.dat"}`))
	if err == nil || !strings.Contains(err.Error(), "not valid UTF-8") {
		t.Fatalf("Execute() = %q, %v, want UTF-8 error", got, err)
	}
	if strings.Contains(got, "�") {
		t.Fatalf("Execute() silently replaced invalid bytes: %q", got)
	}
}

func TestReadValidatesArguments(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	writeTestFile(t, rootPath, "file.txt", []byte("content"))
	read := newRead(t, rootPath)

	tests := []struct {
		name      string
		arguments string
		want      string
	}{
		{name: "empty JSON", arguments: ``, want: "decode arguments"},
		{name: "malformed JSON", arguments: `{`, want: "decode arguments"},
		{name: "trailing JSON", arguments: `{"path":"file.txt"}{}`, want: "one JSON object"},
		{name: "unknown field", arguments: `{"path":"file.txt","extra":true}`, want: "unknown field"},
		{name: "missing path", arguments: `{}`, want: "path is required"},
		{name: "blank path", arguments: `{"path":"  "}`, want: "path is required"},
		{name: "absolute path", arguments: fmt.Sprintf(`{"path":%q}`, filepath.Join(rootPath, "file.txt")), want: "workspace-relative"},
		{name: "parent traversal", arguments: `{"path":"../outside.txt"}`, want: "workspace-relative"},
		{name: "nested parent component", arguments: `{"path":"dir/../file.txt"}`, want: "must not contain .."},
		{name: "zero offset", arguments: `{"path":"file.txt","offset":0}`, want: "offset must be at least 1"},
		{name: "zero limit", arguments: `{"path":"file.txt","limit":0}`, want: "limit must be at least 1"},
		{name: "fractional limit", arguments: `{"path":"file.txt","limit":1.5}`, want: "decode arguments"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := read.Execute(context.Background(), json.RawMessage(test.arguments)); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Execute(%s) error = %v, want substring %q", test.arguments, err, test.want)
			}
		})
	}

	oversized := json.RawMessage(`{"path":"` + strings.Repeat("x", (8<<10)+1) + `"}`)
	if _, err := read.Execute(context.Background(), oversized); err == nil || !strings.Contains(err.Error(), "arguments exceed the 8192-byte limit") {
		t.Fatalf("Execute(oversized) error = %v, want bounded-arguments error", err)
	} else if len(err.Error()) > 200 {
		t.Fatalf("Execute(oversized) error length = %d, want bounded diagnostic", len(err.Error()))
	}
}

func TestReadRejectsMissingAndNonRegularTargets(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	if err := os.Mkdir(filepath.Join(rootPath, "directory"), 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	read := newRead(t, rootPath)

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
	read := newRead(t, rootPath)

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
	read := newRead(t, rootPath)

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

func TestReadHonorsPreCanceledContext(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	writeTestFile(t, rootPath, "file.txt", []byte("content"))
	read := newRead(t, rootPath)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got, err := read.Execute(ctx, json.RawMessage(`{"path":"file.txt"}`))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() = %q, %v, want context.Canceled", got, err)
	}
}

func TestReadIsSafeForConcurrentCalls(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	writeTestFile(t, rootPath, "file.txt", []byte("shared\ncontent\n"))
	read := newRead(t, rootPath)
	want := readResult("file.txt", "1-2", "shared\ncontent\n", "[End of file.]")

	const workers = 24
	var group sync.WaitGroup
	errorsCh := make(chan error, workers)
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			got, err := read.Execute(context.Background(), json.RawMessage(`{"path":"file.txt"}`))
			if err != nil {
				errorsCh <- err
				return
			}
			if got != want {
				errorsCh <- fmt.Errorf("result = %q, want %q", got, want)
			}
		}()
	}
	group.Wait()
	close(errorsCh)
	for err := range errorsCh {
		t.Error(err)
	}
}

func newRead(t *testing.T, rootPath string) *codingtools.Read {
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
	read, err := codingtools.NewRead(root)
	if err != nil {
		t.Fatalf("NewRead() error = %v", err)
	}
	return read
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
