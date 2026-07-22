package coding

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	testPiaSkillsDirectory = ".pia/skills"
	testPiaSkillFilename   = "SKILL.md"
)

func writePiaSkill(t *testing.T, workspace, directory, frontmatter, body string) {
	t.Helper()
	skillDirectory := filepath.Join(workspace, testPiaSkillsDirectory, directory)
	if err := os.MkdirAll(skillDirectory, 0o755); err != nil {
		t.Fatalf("create Skill directory: %v", err)
	}
	content := "---\n" + frontmatter + "---\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(skillDirectory, testPiaSkillFilename), []byte(content), 0o600); err != nil {
		t.Fatalf("write Skill: %v", err)
	}
}

func skillDiagnosticsContain(diagnostics []SkillDiagnostic, fragment string) bool {
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic.Path+" "+diagnostic.Message, fragment) {
			return true
		}
	}
	return false
}
