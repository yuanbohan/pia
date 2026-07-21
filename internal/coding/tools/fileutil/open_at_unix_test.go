//go:build darwin || linux

package fileutil_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/yuanbohan/pia/internal/coding/tools/fileutil"
)

func TestOpenAtUsesDirectoryHandleAndRejectsFinalSymlinks(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	skill := filepath.Join(source, "skill")
	if err := os.MkdirAll(skill, 0o755); err != nil {
		t.Fatalf("create source Skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("original"), 0o600); err != nil {
		t.Fatalf("write original Skill: %v", err)
	}
	if err := os.Symlink("skill", filepath.Join(source, "linked-skill")); err != nil {
		t.Fatalf("create directory symlink: %v", err)
	}
	if err := os.Symlink("SKILL.md", filepath.Join(skill, "LINK.md")); err != nil {
		t.Fatalf("create file symlink: %v", err)
	}

	sourceHandle, err := os.Open(source)
	if err != nil {
		t.Fatalf("open source handle: %v", err)
	}
	t.Cleanup(func() {
		if err := sourceHandle.Close(); err != nil {
			t.Errorf("close source handle: %v", err)
		}
	})
	if err := os.Rename(source, filepath.Join(root, "moved-source")); err != nil {
		t.Fatalf("move source after opening handle: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(source, "skill"), 0o755); err != nil {
		t.Fatalf("create replacement source: %v", err)
	}
	if err := os.WriteFile(filepath.Join(source, "skill", "SKILL.md"), []byte("replacement"), 0o600); err != nil {
		t.Fatalf("write replacement Skill: %v", err)
	}

	skillHandle, err := fileutil.OpenDirectoryAt(sourceHandle, "skill")
	if err != nil {
		t.Fatalf("open Skill relative to source handle: %v", err)
	}
	t.Cleanup(func() {
		if err := skillHandle.Close(); err != nil {
			t.Errorf("close Skill handle: %v", err)
		}
	})
	file, err := fileutil.OpenRegularFileAt(skillHandle, "SKILL.md")
	if err != nil {
		t.Fatalf("open SKILL.md relative to Skill handle: %v", err)
	}
	content, err := io.ReadAll(file)
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("read original SKILL.md: read=%v close=%v", err, closeErr)
	}
	if got, want := string(content), "original"; got != want {
		t.Fatalf("content = %q, want pinned %q", got, want)
	}

	if _, err := fileutil.OpenDirectoryAt(sourceHandle, "linked-skill"); err == nil {
		t.Fatal("directory symlink opened relative to source handle")
	}
	if _, err := fileutil.OpenRegularFileAt(skillHandle, "LINK.md"); err == nil {
		t.Fatal("file symlink opened relative to Skill handle")
	}
	if _, err := fileutil.OpenDirectoryAt(sourceHandle, "../source"); err == nil {
		t.Fatal("directory-relative open accepted a non-direct child")
	}
}
