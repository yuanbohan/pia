package edit_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditAllowsInternalAncestorSymlink(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	if err := os.Mkdir(filepath.Join(rootPath, "real"), 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	writeFixtureFile(t, rootPath, "real/file.txt", []byte("old"))
	if err := os.Symlink("real", filepath.Join(rootPath, "alias")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	tool := newTool(t, rootPath)

	if _, err := tool.Execute(context.Background(), json.RawMessage(`{
		"path":"alias/file.txt",
		"edits":[{"oldText":"old","newText":"new"}]
	}`)); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	written, err := os.ReadFile(filepath.Join(rootPath, "real", "file.txt"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got, want := string(written), "new"; got != want {
		t.Fatalf("written content = %q, want %q", got, want)
	}
}

func TestEditRejectsFinalSymlinksAndOtherNonRegularTargets(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	rootPath := filepath.Join(parent, "workspace")
	outsidePath := filepath.Join(parent, "outside")
	if err := os.Mkdir(rootPath, 0o755); err != nil {
		t.Fatalf("Mkdir(workspace) error = %v", err)
	}
	if err := os.Mkdir(outsidePath, 0o755); err != nil {
		t.Fatalf("Mkdir(outside) error = %v", err)
	}
	writeFixtureFile(t, rootPath, "inside.txt", []byte("INSIDE-CANARY"))
	writeFixtureFile(t, outsidePath, "outside.txt", []byte("OUTSIDE-CANARY"))
	if err := os.Mkdir(filepath.Join(rootPath, "directory"), 0o755); err != nil {
		t.Fatalf("Mkdir(directory) error = %v", err)
	}
	if err := os.Symlink("inside.txt", filepath.Join(rootPath, "internal-link")); err != nil {
		t.Fatalf("Symlink(internal) error = %v", err)
	}
	if err := os.Symlink(filepath.Join(outsidePath, "outside.txt"), filepath.Join(rootPath, "outside-link")); err != nil {
		t.Fatalf("Symlink(outside) error = %v", err)
	}
	if err := os.Symlink("missing.txt", filepath.Join(rootPath, "dangling-link")); err != nil {
		t.Fatalf("Symlink(dangling) error = %v", err)
	}
	if err := os.Symlink(outsidePath, filepath.Join(rootPath, "outside-parent")); err != nil {
		t.Fatalf("Symlink(outside parent) error = %v", err)
	}
	tool := newTool(t, rootPath)

	for _, test := range []struct {
		path string
		want string
	}{
		{path: "directory", want: "not a regular file"},
		{path: "internal-link", want: "symbolic link"},
		{path: "outside-link", want: "escapes"},
		{path: "dangling-link", want: "open"},
		{path: "outside-parent/outside.txt", want: "escapes"},
	} {
		t.Run(test.path, func(t *testing.T) {
			arguments := json.RawMessage(fmt.Sprintf(`{
				"path":%q,
				"edits":[{"oldText":"CANARY","newText":"changed"}]
			}`, test.path))
			got, err := tool.Execute(context.Background(), arguments)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Execute(%q) = %q, %v, want error containing %q", test.path, got, err, test.want)
			}
		})
	}

	for path, want := range map[string]string{
		filepath.Join(rootPath, "inside.txt"):     "INSIDE-CANARY",
		filepath.Join(outsidePath, "outside.txt"): "OUTSIDE-CANARY",
	} {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%q) error = %v", path, err)
		}
		if got := string(content); got != want {
			t.Fatalf("ReadFile(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestEditRootBoundarySurvivesFinalSymlinkSwap(t *testing.T) {
	parent := t.TempDir()
	rootPath := filepath.Join(parent, "workspace")
	outsidePath := filepath.Join(parent, "outside.txt")
	if err := os.Mkdir(rootPath, 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if err := os.WriteFile(outsidePath, []byte("OUTSIDE-SWAP-CANARY"), 0o644); err != nil {
		t.Fatalf("WriteFile(outside) error = %v", err)
	}
	target := filepath.Join(rootPath, "swap.txt")
	if err := os.WriteFile(target, []byte("inside"), 0o644); err != nil {
		t.Fatalf("WriteFile(target) error = %v", err)
	}
	tool := newTool(t, rootPath)

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
			temporary := filepath.Join(rootPath, fmt.Sprintf("swap-candidate-%d", index))
			var err error
			if index%2 == 0 {
				err = os.WriteFile(temporary, []byte("inside"), 0o644)
			} else {
				err = os.Symlink(outsidePath, temporary)
			}
			if err != nil {
				continue
			}
			if err := os.Rename(temporary, target); err != nil {
				_ = os.Remove(temporary)
			}
		}
	}()

	for range 300 {
		_, _ = tool.Execute(context.Background(), json.RawMessage(`{
			"path":"swap.txt",
			"edits":[{"oldText":"inside","newText":"changed"}]
		}`))
	}
	close(stop)
	<-done

	outside, err := os.ReadFile(outsidePath)
	if err != nil {
		t.Fatalf("ReadFile(outside) error = %v", err)
	}
	if got, want := string(outside), "OUTSIDE-SWAP-CANARY"; got != want {
		t.Fatalf("outside content = %q, want %q", got, want)
	}
}

func TestEditPinsAncestorDuringSymlinkSwap(t *testing.T) {
	parent := t.TempDir()
	rootPath := filepath.Join(parent, "workspace")
	insidePath := filepath.Join(rootPath, "inside")
	outsidePath := filepath.Join(parent, "outside")
	for _, path := range []string{rootPath, insidePath, outsidePath} {
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatalf("Mkdir(%q) error = %v", path, err)
		}
	}
	insideTarget := filepath.Join(insidePath, "edited.txt")
	outsideTarget := filepath.Join(outsidePath, "edited.txt")
	if err := os.WriteFile(insideTarget, []byte("inside"), 0o644); err != nil {
		t.Fatalf("WriteFile(inside) error = %v", err)
	}
	if err := os.WriteFile(outsideTarget, []byte("OUTSIDE-ANCESTOR-CANARY"), 0o644); err != nil {
		t.Fatalf("WriteFile(outside) error = %v", err)
	}
	alias := filepath.Join(rootPath, "alias")
	if err := os.Symlink("inside", alias); err != nil {
		t.Fatalf("Symlink(alias) error = %v", err)
	}
	tool := newTool(t, rootPath)

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
			temporary := filepath.Join(rootPath, fmt.Sprintf("alias-candidate-%d", index))
			linkTarget := "inside"
			if index%2 == 1 {
				linkTarget = outsidePath
			}
			if err := os.Symlink(linkTarget, temporary); err != nil {
				continue
			}
			if err := os.Rename(temporary, alias); err != nil {
				_ = os.Remove(temporary)
			}
		}
	}()

	for range 300 {
		_, _ = tool.Execute(context.Background(), json.RawMessage(`{
			"path":"alias/edited.txt",
			"edits":[{"oldText":"inside","newText":"changed"}]
		}`))
	}
	close(stop)
	<-done

	outside, err := os.ReadFile(outsideTarget)
	if err != nil {
		t.Fatalf("ReadFile(outside) error = %v", err)
	}
	if got, want := string(outside), "OUTSIDE-ANCESTOR-CANARY"; got != want {
		t.Fatalf("outside content = %q, want %q", got, want)
	}
	entries, err := os.ReadDir(insidePath)
	if err != nil {
		t.Fatalf("ReadDir(inside) error = %v", err)
	}
	for _, entry := range entries {
		if entry.Name() != "edited.txt" {
			t.Fatalf("inside directory contains a stranded replacement after ancestor swap: %q", entry.Name())
		}
	}
}
