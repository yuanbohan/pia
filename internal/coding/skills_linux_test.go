//go:build linux

package coding

import (
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"
)

func TestDiscoverPiaSkillsRejectsNonUTF8DirectoryName(t *testing.T) {
	directory := t.TempDir()
	skillsDirectory := filepath.Join(directory, piaSkillsDirectory)
	invalidName := string([]byte{'i', 'n', 'v', 'a', 'l', 'i', 'd', '-', 0xff})
	skillDirectory := filepath.Join(skillsDirectory, invalidName)
	if err := os.MkdirAll(skillDirectory, 0o755); err != nil {
		t.Fatalf("create non-UTF-8 Skill directory: %v", err)
	}
	content := "---\nname: unreachable\ndescription: Its advertised location would not survive JSON encoding.\n---\n"
	if err := os.WriteFile(filepath.Join(skillDirectory, piaSkillFilename), []byte(content), 0o600); err != nil {
		t.Fatalf("write Skill in non-UTF-8 directory: %v", err)
	}

	workspace := openPromptWorkspace(t, directory)
	discovery, err := discoverPiaSkills(workspace)
	if err != nil {
		t.Fatalf("discover Pia skills: %v", err)
	}
	if discovery.Catalog != "" {
		t.Fatalf("non-UTF-8 Skill location entered catalog\n%s", discovery.Catalog)
	}
	if !skillDiagnosticsContain(discovery.Diagnostics, "not valid UTF-8") {
		t.Fatalf("diagnostics = %#v, want non-UTF-8 directory warning", discovery.Diagnostics)
	}
	for _, diagnostic := range discovery.Diagnostics {
		if !utf8.ValidString(diagnostic.Path) || !utf8.ValidString(diagnostic.Message) {
			t.Fatalf("diagnostic contains invalid UTF-8: %#v", diagnostic)
		}
	}
}
