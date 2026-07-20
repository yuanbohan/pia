package coding

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yuanbohan/pi-go/internal/ai"
)

func TestDiscoverPiaSkillsBuildsDeterministicMetadataOnlyCatalog(t *testing.T) {
	directory := t.TempDir()
	writePiaSkill(t, directory, "zeta", `name: zeta
description: Use <carefully> & report findings.
`, "ZETA_BODY_SENTINEL")
	writePiaSkill(t, directory, "alpha", `name: alpha
description: Start here.
`, "ALPHA_BODY_SENTINEL")
	writeOtherSkillRoot(t, directory, ".agents", "ignored-agents", "AGENTS_SENTINEL")
	writeOtherSkillRoot(t, directory, ".claude", "ignored-claude", "CLAUDE_SENTINEL")
	writePiaSkill(t, filepath.Join(directory, "nested-project"), "ignored-nested", `name: ignored-nested
description: Nested project skill.
`, "NESTED_SENTINEL")

	workspace := openPromptWorkspace(t, directory)
	discovery, err := discoverPiaSkills(workspace)
	if err != nil {
		t.Fatalf("discover Pia skills: %v", err)
	}
	if len(discovery.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", discovery.Diagnostics)
	}

	catalog := discovery.Catalog
	for _, fragment := range []string{
		"<available_skills>",
		"<name>alpha</name>",
		"<description>Start here.</description>",
		"<location>.pia/skills/alpha/SKILL.md</location>",
		"<name>zeta</name>",
		"Use &lt;carefully&gt; &amp; report findings.",
		"<location>.pia/skills/zeta/SKILL.md</location>",
	} {
		if !strings.Contains(catalog, fragment) {
			t.Errorf("catalog does not contain %q\n%s", fragment, catalog)
		}
	}
	if strings.Index(catalog, "<name>alpha</name>") > strings.Index(catalog, "<name>zeta</name>") {
		t.Fatalf("catalog is not ordered by stable skill name\n%s", catalog)
	}
	for _, forbidden := range []string{
		"ALPHA_BODY_SENTINEL",
		"ZETA_BODY_SENTINEL",
		"AGENTS_SENTINEL",
		"CLAUDE_SENTINEL",
		"NESTED_SENTINEL",
	} {
		if strings.Contains(catalog, forbidden) {
			t.Fatalf("catalog contains undisclosed content %q\n%s", forbidden, catalog)
		}
	}
	if got := ai.EstimateTextTokens(catalog); got > maxSkillCatalogTokens {
		t.Fatalf("catalog estimate = %d tokens, want at most %d", got, maxSkillCatalogTokens)
	}
}

func TestDiscoverPiaSkillsTreatsMissingRootAsEmpty(t *testing.T) {
	workspace := openPromptWorkspace(t, t.TempDir())
	discovery, err := discoverPiaSkills(workspace)
	if err != nil {
		t.Fatalf("discover Pia skills: %v", err)
	}
	if discovery.Catalog != "" || len(discovery.Diagnostics) != 0 {
		t.Fatalf("discovery = %#v, want empty", discovery)
	}
}

func TestDiscoverPiaSkillsRejectsSymlinkSource(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "skill-store")
	writePiaSkillFile(t, target, "review-go", `name: review-go
description: Must remain undiscovered through a source symlink.
`, "SYMLINKED_SOURCE_BODY")
	piaDirectory := filepath.Join(directory, ".pia")
	if err := os.MkdirAll(piaDirectory, 0o755); err != nil {
		t.Fatalf("create .pia directory: %v", err)
	}
	if err := os.Symlink("../skill-store", filepath.Join(piaDirectory, "skills")); err != nil {
		t.Fatalf("create project-local Skill source symlink: %v", err)
	}

	workspace := openPromptWorkspace(t, directory)
	discovery, err := discoverPiaSkills(workspace)
	if err != nil {
		t.Fatalf("discover Pia skills: %v", err)
	}
	if discovery.Catalog != "" {
		t.Fatalf("symlinked Skill source entered catalog\n%s", discovery.Catalog)
	}
	if !skillDiagnosticsContain(discovery.Diagnostics, "source is a symlink") {
		t.Fatalf("diagnostics = %#v, want source-symlink warning", discovery.Diagnostics)
	}
}

func TestLoadPiaSkillUsesVerifiedSourceHandleAfterReplacement(t *testing.T) {
	directory := t.TempDir()
	writePiaSkill(t, directory, "stable", `name: stable
description: Original metadata from the verified source.
`, "ORIGINAL_BODY")
	workspace := openPromptWorkspace(t, directory)
	source, err := openPiaSkillsDirectory(workspace.Root())
	if err != nil {
		t.Fatalf("open verified Skill source: %v", err)
	}

	sourcePath := filepath.Join(directory, piaSkillsDirectory)
	if err := os.Rename(sourcePath, sourcePath+"-old"); err != nil {
		_ = source.Close()
		t.Fatalf("move verified Skill source: %v", err)
	}
	writePiaSkill(t, directory, "stable", `name: stable
description: Replacement metadata must stay undiscovered.
`, "REPLACEMENT_BODY")

	location := path.Join(piaSkillsDirectory, "stable", piaSkillFilename)
	skill, diagnostics, ok := loadPiaSkill(source, "stable", location)
	closeErr := source.Close()
	if closeErr != nil {
		t.Fatalf("close verified Skill source: %v", closeErr)
	}
	if !ok || len(diagnostics) != 0 {
		t.Fatalf("load from verified source = (%#v, %#v, %v), want valid Skill", skill, diagnostics, ok)
	}
	if got, want := skill.Description, "Original metadata from the verified source."; got != want {
		t.Fatalf("description = %q, want pinned source metadata %q", got, want)
	}
}

func TestDiscoverPiaSkillsDoesNotRecurseWithinPiaRoot(t *testing.T) {
	directory := t.TempDir()
	nestedDirectory := filepath.Join(directory, piaSkillsDirectory, "group", "nested")
	if err := os.MkdirAll(nestedDirectory, 0o755); err != nil {
		t.Fatalf("create nested Skill directory: %v", err)
	}
	content := "---\nname: nested\ndescription: Must stay undiscovered.\n---\nNESTED_BODY_SENTINEL\n"
	if err := os.WriteFile(filepath.Join(nestedDirectory, piaSkillFilename), []byte(content), 0o600); err != nil {
		t.Fatalf("write nested Skill: %v", err)
	}

	workspace := openPromptWorkspace(t, directory)
	discovery, err := discoverPiaSkills(workspace)
	if err != nil {
		t.Fatalf("discover Pia skills: %v", err)
	}
	if discovery.Catalog != "" || strings.Contains(fmt.Sprint(discovery.Diagnostics), "NESTED_BODY_SENTINEL") {
		t.Fatalf("nested Skill was discovered: %#v", discovery)
	}
	if !skillDiagnosticsContain(discovery.Diagnostics, ".pia/skills/group/SKILL.md") {
		t.Fatalf("diagnostics = %#v, want only direct group candidate", discovery.Diagnostics)
	}
}

func TestDiscoverPiaSkillsDoesNotInspectBodyEncoding(t *testing.T) {
	directory := t.TempDir()
	skillDirectory := filepath.Join(directory, piaSkillsDirectory, "binary-body")
	if err := os.MkdirAll(skillDirectory, 0o755); err != nil {
		t.Fatalf("create skill directory: %v", err)
	}
	content := append([]byte("---\nname: binary-body\ndescription: Metadata remains discoverable.\n---\n"), 0xff, 0xfe)
	if err := os.WriteFile(filepath.Join(skillDirectory, piaSkillFilename), content, 0o600); err != nil {
		t.Fatalf("write skill with invalid body encoding: %v", err)
	}

	workspace := openPromptWorkspace(t, directory)
	discovery, err := discoverPiaSkills(workspace)
	if err != nil {
		t.Fatalf("discover Pia skills: %v", err)
	}
	if !strings.Contains(discovery.Catalog, "<name>binary-body</name>") {
		t.Fatalf("body encoding affected frontmatter discovery\n%s\n%#v", discovery.Catalog, discovery.Diagnostics)
	}
}

func TestDiscoverPiaSkillsWarnsWithoutRejectingCosmeticViolations(t *testing.T) {
	directory := t.TempDir()
	writePiaSkill(t, directory, "directory-name", `name: Display_Name
description: Still usable.
unknown-field: ignored
`, "BODY_SENTINEL")

	workspace := openPromptWorkspace(t, directory)
	discovery, err := discoverPiaSkills(workspace)
	if err != nil {
		t.Fatalf("discover Pia skills: %v", err)
	}
	if !strings.Contains(discovery.Catalog, "<name>Display_Name</name>") {
		t.Fatalf("cosmetically invalid skill was rejected\n%s", discovery.Catalog)
	}
	for _, fragment := range []string{"does not follow", "does not match", "unknown-field"} {
		if !skillDiagnosticsContain(discovery.Diagnostics, fragment) {
			t.Errorf("diagnostics do not contain %q: %#v", fragment, discovery.Diagnostics)
		}
	}
}

func TestDiscoverPiaSkillsSkipsStructurallyInvalidAndUnsafeCandidates(t *testing.T) {
	directory := t.TempDir()
	writePiaSkill(t, directory, "missing-name", `description: Missing name.
`, "MISSING_NAME_BODY")
	writePiaSkill(t, directory, "missing-description", `name: missing-description
`, "MISSING_DESCRIPTION_BODY")
	writePiaSkill(t, directory, "bad-yaml", "name: [unterminated\n", "BAD_YAML_BODY")
	writePiaSkill(t, directory, "duplicate-key", "name: first\nname: second\ndescription: Duplicate key.\n", "DUPLICATE_KEY_BODY")
	writePiaSkill(t, directory, "huge-name", "name: "+strings.Repeat("n", maxSkillNameCharacters+1)+"\ndescription: Too large.\n", "HUGE_NAME_BODY")
	writePiaSkill(t, directory, "huge-frontmatter", "name: huge-frontmatter\ndescription: "+strings.Repeat("x", maxSkillFrontmatterBytes)+"\n", "HUGE_FRONTMATTER_BODY")

	outside := filepath.Join(t.TempDir(), "outside-skill.md")
	if err := os.WriteFile(outside, []byte("---\nname: escaped\ndescription: Escaped.\n---\nOUTSIDE_SECRET"), 0o600); err != nil {
		t.Fatalf("write outside skill: %v", err)
	}
	escapedDirectory := filepath.Join(directory, piaSkillsDirectory, "escaped")
	if err := os.MkdirAll(escapedDirectory, 0o755); err != nil {
		t.Fatalf("create escaped skill directory: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(escapedDirectory, piaSkillFilename)); err != nil {
		t.Fatalf("create escaping skill symlink: %v", err)
	}
	outsideDirectory := filepath.Join(t.TempDir(), "outside-directory")
	writePiaSkill(t, outsideDirectory, "linked", "name: linked\ndescription: Linked directory.\n", "LINKED_DIRECTORY_SECRET")
	if err := os.Symlink(
		filepath.Join(outsideDirectory, piaSkillsDirectory, "linked"),
		filepath.Join(directory, piaSkillsDirectory, "linked-directory"),
	); err != nil {
		t.Fatalf("create linked Skill directory: %v", err)
	}

	workspace := openPromptWorkspace(t, directory)
	discovery, err := discoverPiaSkills(workspace)
	if err != nil {
		t.Fatalf("discover Pia skills: %v", err)
	}
	if discovery.Catalog != "" {
		t.Fatalf("invalid candidates entered catalog\n%s", discovery.Catalog)
	}
	if len(discovery.Diagnostics) < 8 {
		t.Fatalf("diagnostics = %#v, want one warning per skipped candidate", discovery.Diagnostics)
	}
	if !skillDiagnosticsContain(discovery.Diagnostics, "symlink Skill directories") {
		t.Fatalf("diagnostics = %#v, want linked-directory warning", discovery.Diagnostics)
	}
	for _, forbidden := range []string{"MISSING_NAME_BODY", "MISSING_DESCRIPTION_BODY", "BAD_YAML_BODY", "DUPLICATE_KEY_BODY", "HUGE_NAME_BODY", "HUGE_FRONTMATTER_BODY", "OUTSIDE_SECRET", "LINKED_DIRECTORY_SECRET"} {
		if strings.Contains(fmt.Sprint(discovery.Diagnostics), forbidden) {
			t.Fatalf("diagnostics exposed skill body or outside content %q: %#v", forbidden, discovery.Diagnostics)
		}
	}
}

func TestDiscoverPiaSkillsUsesLexicalWinnerAndBoundsDescriptions(t *testing.T) {
	directory := t.TempDir()
	writePiaSkill(t, directory, "b-path", `name: duplicate
description: Losing description.
`, "LOSING_BODY")
	writePiaSkill(t, directory, "a-path", `name: duplicate
description: Winning description.
`, "WINNING_BODY")
	writePiaSkill(t, directory, "long-description", "name: long-description\ndescription: "+strings.Repeat("d", maxSkillDescriptionCharacters+1)+"\n", "LONG_BODY")

	workspace := openPromptWorkspace(t, directory)
	discovery, err := discoverPiaSkills(workspace)
	if err != nil {
		t.Fatalf("discover Pia skills: %v", err)
	}
	if strings.Count(discovery.Catalog, "<name>duplicate</name>") != 1 ||
		!strings.Contains(discovery.Catalog, "Winning description.") ||
		strings.Contains(discovery.Catalog, "Losing description.") {
		t.Fatalf("catalog did not keep lexical duplicate winner\n%s", discovery.Catalog)
	}
	if strings.Contains(discovery.Catalog, strings.Repeat("d", maxSkillDescriptionCharacters+1)) {
		t.Fatalf("catalog contains unbounded description\n%s", discovery.Catalog)
	}
	for _, fragment := range []string{"duplicate", "truncated"} {
		if !skillDiagnosticsContain(discovery.Diagnostics, fragment) {
			t.Errorf("diagnostics do not contain %q: %#v", fragment, discovery.Diagnostics)
		}
	}
}

func TestDiscoverPiaSkillsEnforcesCandidateAndCatalogBudgets(t *testing.T) {
	directory := t.TempDir()
	for index := 0; index < maxPiaSkillCandidates+1; index++ {
		name := fmt.Sprintf("%s-%02d", strings.Repeat("n", 220), index)
		writePiaSkill(t, directory, fmt.Sprintf("skill-%02d", index), "name: "+name+"\ndescription: "+strings.Repeat("d", maxSkillDescriptionCharacters)+"\n", "BODY_SENTINEL")
	}

	workspace := openPromptWorkspace(t, directory)
	discovery, err := discoverPiaSkills(workspace)
	if err != nil {
		t.Fatalf("discover Pia skills: %v", err)
	}
	if got := ai.EstimateTextTokens(discovery.Catalog); got > maxSkillCatalogTokens {
		t.Fatalf("catalog estimate = %d tokens, want at most %d", got, maxSkillCatalogTokens)
	}
	if got := len(discovery.Diagnostics); got != maxSkillDiagnostics {
		t.Fatalf("diagnostic count = %d, want bounded count %d", got, maxSkillDiagnostics)
	}
	for _, fragment := range []string{"64", "catalog", "omitted"} {
		if !skillDiagnosticsContain(discovery.Diagnostics, fragment) {
			t.Errorf("diagnostics do not contain %q: %#v", fragment, discovery.Diagnostics)
		}
	}
	if strings.Contains(discovery.Catalog, "BODY_SENTINEL") {
		t.Fatal("catalog contains a skill body sentinel")
	}
	if !skillDiagnosticsContain(discovery.Diagnostics, "additional Skill warnings") {
		t.Fatalf("diagnostics = %#v, want bounded-warning summary", discovery.Diagnostics)
	}
}

func TestDiscoverPiaSkillsBoundsDirectoryEnumerationAtInput(t *testing.T) {
	directory := t.TempDir()
	writePiaSkill(t, directory, "valid", `name: valid
description: Would be valid below the source ceiling.
`, "VALID_BODY_SENTINEL")
	skillsDirectory := filepath.Join(directory, piaSkillsDirectory)
	for index := 0; index < maxPiaSkillDirectoryEntries; index++ {
		name := filepath.Join(skillsDirectory, fmt.Sprintf("ordinary-%03d.txt", index))
		if err := os.WriteFile(name, []byte("ordinary project file"), 0o600); err != nil {
			t.Fatalf("write oversized directory entry: %v", err)
		}
	}

	workspace := openPromptWorkspace(t, directory)
	discovery, err := discoverPiaSkills(workspace)
	if err != nil {
		t.Fatalf("discover Pia skills: %v", err)
	}
	if discovery.Catalog != "" {
		t.Fatalf("oversized Skill source entered catalog\n%s", discovery.Catalog)
	}
	if got, want := len(discovery.Diagnostics), 1; got != want {
		t.Fatalf("diagnostic count = %d, want %d: %#v", got, want, discovery.Diagnostics)
	}
	for _, fragment := range []string{"more than", fmt.Sprint(maxPiaSkillDirectoryEntries), "ignored"} {
		if !skillDiagnosticsContain(discovery.Diagnostics, fragment) {
			t.Errorf("diagnostics do not contain %q: %#v", fragment, discovery.Diagnostics)
		}
	}
	if strings.Contains(fmt.Sprint(discovery.Diagnostics), "VALID_BODY_SENTINEL") {
		t.Fatalf("diagnostics exposed Skill body: %#v", discovery.Diagnostics)
	}
}

func TestBuildPiaSkillCatalogReportsOnlyAppliedBudgetActions(t *testing.T) {
	skills := make([]piaSkill, maxPiaSkillCandidates)
	for index := range skills {
		skills[index] = piaSkill{
			Name:        fmt.Sprintf("%s-%02d", strings.Repeat("n", 220), index),
			Description: "x",
			Location:    fmt.Sprintf(".pia/skills/skill-%02d/SKILL.md", index),
		}
	}

	catalog, diagnostics := buildPiaSkillCatalog(skills)
	if got := ai.EstimateTextTokens(catalog); got > maxSkillCatalogTokens {
		t.Fatalf("catalog estimate = %d tokens, want at most %d", got, maxSkillCatalogTokens)
	}
	if !skillDiagnosticsContain(diagnostics, "omitted") {
		t.Fatalf("diagnostics = %#v, want entry-omission warning", diagnostics)
	}
	if skillDiagnosticsContain(diagnostics, "descriptions were shortened") {
		t.Fatalf("diagnostics falsely claim short descriptions were shortened: %#v", diagnostics)
	}
}

func TestBuildPiaSkillCatalogShortensDescriptionsBeforeOmittingEntries(t *testing.T) {
	skills := make([]piaSkill, maxPiaSkillCandidates)
	for index := range skills {
		skills[index] = piaSkill{
			Name:        fmt.Sprintf("skill-%02d", index),
			Description: strings.Repeat("d", maxSkillDescriptionCharacters),
			Location:    fmt.Sprintf(".pia/skills/skill-%02d/SKILL.md", index),
		}
	}

	catalog, diagnostics := buildPiaSkillCatalog(skills)
	if got := ai.EstimateTextTokens(catalog); got > maxSkillCatalogTokens {
		t.Fatalf("catalog estimate = %d tokens, want at most %d", got, maxSkillCatalogTokens)
	}
	if got, want := strings.Count(catalog, "<skill>"), len(skills); got != want {
		t.Fatalf("catalog Skill count = %d, want all %d", got, want)
	}
	if !skillDiagnosticsContain(diagnostics, "descriptions were shortened") {
		t.Fatalf("diagnostics = %#v, want description-shortening warning", diagnostics)
	}
	if skillDiagnosticsContain(diagnostics, "entries") {
		t.Fatalf("diagnostics unexpectedly omitted entries: %#v", diagnostics)
	}
}

func writePiaSkill(t *testing.T, workspace, directory, frontmatter, body string) {
	t.Helper()
	skillRoot := filepath.Join(workspace, piaSkillsDirectory)
	writePiaSkillFile(t, skillRoot, directory, frontmatter, body)
}

func writePiaSkillFile(t *testing.T, skillRoot, directory, frontmatter, body string) {
	t.Helper()
	skillDirectory := filepath.Join(skillRoot, directory)
	if err := os.MkdirAll(skillDirectory, 0o755); err != nil {
		t.Fatalf("create skill directory: %v", err)
	}
	content := "---\n" + frontmatter + "---\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(skillDirectory, piaSkillFilename), []byte(content), 0o600); err != nil {
		t.Fatalf("write skill: %v", err)
	}
}

func writeOtherSkillRoot(t *testing.T, workspace, root, directory, body string) {
	t.Helper()
	skillDirectory := filepath.Join(workspace, root, "skills", directory)
	if err := os.MkdirAll(skillDirectory, 0o755); err != nil {
		t.Fatalf("create other skill directory: %v", err)
	}
	content := "---\nname: " + directory + "\ndescription: Ignored.\n---\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(skillDirectory, piaSkillFilename), []byte(content), 0o600); err != nil {
		t.Fatalf("write other skill: %v", err)
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
