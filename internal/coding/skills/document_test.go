package skills

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestLoadCurrentBodyReadsLatestContentWithoutRevalidatingMetadata(t *testing.T) {
	directory := t.TempDir()
	writeSkillDocument(t, directory, "review-go", "name: review-go\ndescription: Original.\n", "BODY_V1\n")
	root := openTestRoot(t, directory)
	entry := Entry{
		Name:      "review-go",
		Directory: "review-go",
		Location:  ".pia/skills/review-go/SKILL.md",
	}

	body, err := LoadCurrentBody(context.Background(), root, entry, 1024)
	if err != nil {
		t.Fatalf("LoadCurrentBody(V1) error = %v", err)
	}
	if got, want := string(body), "BODY_V1\n"; got != want {
		t.Fatalf("LoadCurrentBody(V1) = %q, want %q", got, want)
	}

	writeSkillDocument(t, directory, "review-go", "name: [temporarily invalid YAML\n", "BODY_V2\n")
	body, err = LoadCurrentBody(context.Background(), root, entry, 1024)
	if err != nil {
		t.Fatalf("LoadCurrentBody(V2) error = %v", err)
	}
	if got, want := string(body), "BODY_V2\n"; got != want {
		t.Fatalf("LoadCurrentBody(V2) = %q, want latest body %q", got, want)
	}
}

func TestLoadCurrentBodyEnforcesCompleteBodyLimit(t *testing.T) {
	directory := t.TempDir()
	writeSkillDocument(t, directory, "bounded", "name: bounded\ndescription: Bounded.\n", "12345")
	root := openTestRoot(t, directory)
	entry := Entry{Name: "bounded", Directory: "bounded", Location: ".pia/skills/bounded/SKILL.md"}

	body, err := LoadCurrentBody(context.Background(), root, entry, 5)
	if err != nil {
		t.Fatalf("LoadCurrentBody(exact limit) error = %v", err)
	}
	if got, want := string(body), "12345"; got != want {
		t.Fatalf("LoadCurrentBody(exact limit) = %q, want %q", got, want)
	}

	body, err = LoadCurrentBody(context.Background(), root, entry, 4)
	if body != nil {
		t.Fatalf("LoadCurrentBody(over limit) body = %q, want nil", body)
	}
	var limitErr *BodyLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("LoadCurrentBody(over limit) error = %v, want BodyLimitError", err)
	}
	if limitErr.Size != 5 || limitErr.Limit != 4 {
		t.Fatalf("BodyLimitError = %#v, want size 5 limit 4", limitErr)
	}
}

func TestLoadCurrentBodyReopensCurrentFileAndSourceWithoutStaleFallback(t *testing.T) {
	directory := t.TempDir()
	writeSkillDocument(t, directory, "review-go", "name: review-go\ndescription: Review.\n", "BODY_V1")
	root := openTestRoot(t, directory)
	entry := Entry{Name: "review-go", Directory: "review-go", Location: ".pia/skills/review-go/SKILL.md"}
	skillDirectory := filepath.Join(directory, piaSkillsDirectory, "review-go")
	target := filepath.Join(skillDirectory, piaSkillFilename)

	replacement := filepath.Join(skillDirectory, "replacement.md")
	if err := os.WriteFile(
		replacement,
		[]byte("---\nname: changed\ndescription: Current metadata is not revalidated.\n---\nBODY_V2"),
		0o600,
	); err != nil {
		t.Fatalf("write replacement Skill: %v", err)
	}
	if err := os.Rename(replacement, target); err != nil {
		t.Fatalf("replace Skill atomically: %v", err)
	}
	if body, err := LoadCurrentBody(context.Background(), root, entry, 1024); err != nil || string(body) != "BODY_V2" {
		t.Fatalf("LoadCurrentBody(replaced file) = %q, %v, want BODY_V2", body, err)
	}

	source := filepath.Join(directory, piaSkillsDirectory)
	oldSource := filepath.Join(directory, ".pia", "old-skills")
	if err := os.Rename(source, oldSource); err != nil {
		t.Fatalf("rename old Skill source: %v", err)
	}
	writeSkillDocument(t, directory, "review-go", "name: review-go\ndescription: Review.\n", "BODY_V3")
	if body, err := LoadCurrentBody(context.Background(), root, entry, 1024); err != nil || string(body) != "BODY_V3" {
		t.Fatalf("LoadCurrentBody(replaced source) = %q, %v, want BODY_V3", body, err)
	}

	if err := os.Remove(filepath.Join(directory, piaSkillsDirectory, "review-go", piaSkillFilename)); err != nil {
		t.Fatalf("remove current Skill: %v", err)
	}
	if body, err := LoadCurrentBody(context.Background(), root, entry, 1024); body != nil || !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("LoadCurrentBody(deleted) = %q, %v, want not-exist failure without stale body", body, err)
	}
}

func TestLoadCurrentBodyRejectsInvalidDocumentViews(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		want    string
	}{
		{name: "opening delimiter", content: []byte("name: broken\n---\nbody"), want: "opening --- delimiter"},
		{name: "closing delimiter", content: []byte("---\nname: broken\nbody"), want: "closing --- delimiter"},
		{name: "frontmatter limit", content: []byte("---\n" + strings.Repeat("x", maxSkillFrontmatterBytes) + "\n---\nbody"), want: "frontmatter exceeds"},
		{name: "body utf8", content: append([]byte("---\nname: broken\n---\n"), 0xff), want: "body is not valid UTF-8"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			writeRawSkillDocument(t, directory, "broken", test.content)
			root := openTestRoot(t, directory)
			entry := Entry{Name: "broken", Directory: "broken", Location: ".pia/skills/broken/SKILL.md"}

			body, err := LoadCurrentBody(context.Background(), root, entry, 1024)
			if body != nil || err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("LoadCurrentBody() = %q, %v, want error containing %q", body, err, test.want)
			}
		})
	}
}

func TestLoadCurrentBodyRejectsCurrentSymlinkAndHonorsCancellation(t *testing.T) {
	directory := t.TempDir()
	writeSkillDocument(t, directory, "review-go", "name: review-go\ndescription: Review.\n", "BODY")
	root := openTestRoot(t, directory)
	entry := Entry{Name: "review-go", Directory: "review-go", Location: ".pia/skills/review-go/SKILL.md"}
	target := filepath.Join(directory, piaSkillsDirectory, "review-go", piaSkillFilename)
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("---\nname: outside\n---\nSECRET"), 0o600); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatalf("remove Skill file: %v", err)
	}
	if err := os.Symlink(outside, target); err != nil {
		t.Fatalf("replace Skill with symlink: %v", err)
	}

	if body, err := LoadCurrentBody(context.Background(), root, entry, 1024); body != nil || err == nil {
		t.Fatalf("LoadCurrentBody(symlink) = %q, %v, want failure", body, err)
	}

	cause := errors.New("cancel Skill load")
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(cause)
	if body, err := LoadCurrentBody(ctx, root, entry, 1024); body != nil || !errors.Is(err, cause) {
		t.Fatalf("LoadCurrentBody(canceled) = %q, %v, want cause", body, err)
	}
}

func openTestRoot(t *testing.T, directory string) *os.Root {
	t.Helper()
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatalf("open test root: %v", err)
	}
	t.Cleanup(func() {
		if err := root.Close(); err != nil {
			t.Errorf("close test root: %v", err)
		}
	})
	return root
}

func writeSkillDocument(t *testing.T, workspace, directory, frontmatter, body string) {
	t.Helper()
	writeRawSkillDocument(t, workspace, directory, []byte("---\n"+frontmatter+"---\n"+body))
}

func writeRawSkillDocument(t *testing.T, workspace, directory string, content []byte) {
	t.Helper()
	skillDirectory := filepath.Join(workspace, piaSkillsDirectory, directory)
	if err := os.MkdirAll(skillDirectory, 0o755); err != nil {
		t.Fatalf("create Skill directory: %v", err)
	}
	if !utf8.ValidString(directory) {
		t.Fatal("test helper requires a UTF-8 directory name")
	}
	if err := os.WriteFile(filepath.Join(skillDirectory, piaSkillFilename), content, 0o600); err != nil {
		t.Fatalf("write Skill document: %v", err)
	}
}
