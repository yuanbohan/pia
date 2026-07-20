package coding

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"io"
	"io/fs"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/yuanbohan/pi-go/internal/ai"
	"github.com/yuanbohan/pi-go/internal/coding/tools/fileutil"
	"go.yaml.in/yaml/v3"
)

const (
	piaSkillsDirectory = ".pia/skills"
	piaSkillFilename   = "SKILL.md"

	maxPiaSkillCandidates          = 64
	maxPiaSkillDirectoryEntries    = maxPiaSkillCandidates * 4
	maxSkillFrontmatterBytes       = 16 << 10
	maxPortableSkillNameCharacters = 64
	maxSkillNameCharacters         = 256
	maxSkillDescriptionCharacters  = 1_024
	maxSkillCatalogTokens          = 4_096
	maxSkillDiagnostics            = 64
	maxUnknownFieldsInDiagnostic   = 8
)

var portableSkillNamePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// SkillDiagnostic is one bounded operator warning produced while taking the
// project Skill snapshot. Skill failures never block an otherwise valid Run.
type SkillDiagnostic struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

type piaSkillDiscovery struct {
	Catalog     string
	Diagnostics []SkillDiagnostic
}

type piaSkill struct {
	Name        string
	Description string
	Location    string
}

// discoverPiaSkills takes one project-local snapshot before the Conversation
// starts. It deliberately discovers only direct directories under
// .pia/skills; a later Run never reloads this data.
func discoverPiaSkills(workspace *Workspace) (piaSkillDiscovery, error) {
	if workspace == nil || workspace.Root() == nil {
		return piaSkillDiscovery{}, fmt.Errorf("coding: discover Pia skills: workspace is required")
	}

	skillsDirectory, err := openPiaSkillsDirectory(workspace.Root())
	if errors.Is(err, fs.ErrNotExist) {
		return piaSkillDiscovery{}, nil
	}
	if err != nil {
		return piaSkillDiscovery{
			Diagnostics: []SkillDiagnostic{{
				Path:    piaSkillsDirectory,
				Message: "could not list project Skills: " + boundedDiagnosticText(err.Error()),
			}},
		}, nil
	}
	entries, readErr := skillsDirectory.ReadDir(maxPiaSkillDirectoryEntries + 1)
	if errors.Is(readErr, io.EOF) {
		readErr = nil
	}
	if readErr != nil {
		joined := errors.Join(readErr, skillsDirectory.Close())
		return piaSkillDiscovery{
			Diagnostics: []SkillDiagnostic{{
				Path:    piaSkillsDirectory,
				Message: "could not read project Skills directory: " + boundedDiagnosticText(joined.Error()),
			}},
		}, nil
	}
	if len(entries) > maxPiaSkillDirectoryEntries {
		diagnostics := []SkillDiagnostic{{
			Path: piaSkillsDirectory,
			Message: fmt.Sprintf(
				"project Skills were ignored because the directory contains more than %d direct entries",
				maxPiaSkillDirectoryEntries,
			),
		}}
		if closeErr := skillsDirectory.Close(); closeErr != nil {
			diagnostics = append(diagnostics, SkillDiagnostic{
				Path:    piaSkillsDirectory,
				Message: "could not close project Skills directory: " + boundedDiagnosticText(closeErr.Error()),
			})
		}
		return piaSkillDiscovery{
			Diagnostics: diagnostics,
		}, nil
	}
	sort.Slice(entries, func(left, right int) bool {
		return entries[left].Name() < entries[right].Name()
	})

	var directoryNames []string
	var candidateDiagnostics []SkillDiagnostic
	for _, entry := range entries {
		name := entry.Name()
		isDirectory := entry.IsDir()
		isSymlink := entry.Type()&fs.ModeSymlink != 0
		if (isDirectory || isSymlink) && !utf8.ValidString(name) {
			candidateDiagnostics = append(candidateDiagnostics, SkillDiagnostic{
				Path:    piaSkillsDirectory,
				Message: "a direct Skill entry with a name that is not valid UTF-8 was ignored",
			})
			continue
		}
		// IsDir intentionally excludes directory symlinks. Pia Skill v1 has one
		// concrete project-owned directory per Skill and no symlink source model.
		if isDirectory {
			directoryNames = append(directoryNames, name)
		} else if isSymlink {
			candidateDiagnostics = append(candidateDiagnostics, SkillDiagnostic{
				Path:    path.Join(piaSkillsDirectory, name),
				Message: "symlink Skill directories are unsupported and were ignored",
			})
		}
	}

	var priorityDiagnostics []SkillDiagnostic
	if len(directoryNames) > maxPiaSkillCandidates {
		priorityDiagnostics = append(priorityDiagnostics, SkillDiagnostic{
			Path: piaSkillsDirectory,
			Message: fmt.Sprintf(
				"only the first %d direct Skill directories were inspected; %d tail directories were omitted",
				maxPiaSkillCandidates,
				len(directoryNames)-maxPiaSkillCandidates,
			),
		})
		directoryNames = directoryNames[:maxPiaSkillCandidates]
	}

	byName := make(map[string]piaSkill, len(directoryNames))
	for _, directoryName := range directoryNames {
		location := path.Join(piaSkillsDirectory, directoryName, piaSkillFilename)
		skill, diagnostics, ok := loadPiaSkill(skillsDirectory, directoryName, location)
		candidateDiagnostics = append(candidateDiagnostics, diagnostics...)
		if !ok {
			continue
		}
		if winner, exists := byName[skill.Name]; exists {
			candidateDiagnostics = append(candidateDiagnostics, SkillDiagnostic{
				Path: skill.Location,
				Message: fmt.Sprintf(
					"duplicate Skill name %q ignored; lexical path winner is %s",
					skill.Name,
					winner.Location,
				),
			})
			continue
		}
		byName[skill.Name] = skill
	}
	if closeErr := skillsDirectory.Close(); closeErr != nil {
		candidateDiagnostics = append(candidateDiagnostics, SkillDiagnostic{
			Path:    piaSkillsDirectory,
			Message: "could not close project Skills directory: " + boundedDiagnosticText(closeErr.Error()),
		})
	}

	skills := make([]piaSkill, 0, len(byName))
	for _, skill := range byName {
		skills = append(skills, skill)
	}
	sort.Slice(skills, func(left, right int) bool {
		if skills[left].Name == skills[right].Name {
			return skills[left].Location < skills[right].Location
		}
		return skills[left].Name < skills[right].Name
	})

	catalog, catalogDiagnostics := buildPiaSkillCatalog(skills)
	allDiagnostics := append(priorityDiagnostics, catalogDiagnostics...)
	allDiagnostics = append(allDiagnostics, candidateDiagnostics...)
	return piaSkillDiscovery{
		Catalog:     catalog,
		Diagnostics: limitSkillDiagnostics(allDiagnostics),
	}, nil
}

func openPiaSkillsDirectory(root *os.Root) (*os.File, error) {
	entry, err := root.Lstat(piaSkillsDirectory)
	if err != nil {
		return nil, err
	}
	if entry.Mode()&fs.ModeSymlink != 0 {
		return nil, fmt.Errorf("project Skills source is a symlink")
	}
	if !entry.IsDir() {
		return nil, fmt.Errorf("project Skills source is not a directory")
	}

	directory, err := fileutil.OpenDirectory(root, piaSkillsDirectory)
	if err != nil {
		return nil, err
	}
	opened, openStatErr := directory.Stat()
	current, currentStatErr := root.Lstat(piaSkillsDirectory)
	if joined := errors.Join(openStatErr, currentStatErr); joined != nil {
		return nil, errors.Join(fmt.Errorf("verify opened project Skills source: %w", joined), directory.Close())
	}
	// Root.OpenFile follows safe in-workspace symlinks. Rechecking the final
	// entry and its identity prevents a source swapped after the first Lstat
	// from turning that behavior into implicit symlink discovery.
	if current.Mode()&fs.ModeSymlink != 0 || !current.IsDir() || !os.SameFile(opened, current) {
		return nil, errors.Join(
			fmt.Errorf("project Skills source changed while it was being opened"),
			directory.Close(),
		)
	}
	return directory, nil
}

func loadPiaSkill(
	skillsDirectory *os.File,
	directoryName string,
	location string,
) (piaSkill, []SkillDiagnostic, bool) {
	skillDirectory, err := fileutil.OpenDirectoryAt(skillsDirectory, directoryName)
	if err != nil {
		return piaSkill{}, []SkillDiagnostic{{
			Path:    location,
			Message: "could not open a direct project Skill directory: " + boundedDiagnosticText(err.Error()),
		}}, false
	}

	file, err := fileutil.OpenRegularFileAt(skillDirectory, piaSkillFilename)
	if err != nil {
		joined := errors.Join(err, skillDirectory.Close())
		return piaSkill{}, []SkillDiagnostic{{
			Path:    location,
			Message: "could not open a direct regular project Skill: " + boundedDiagnosticText(joined.Error()),
		}}, false
	}

	frontmatter, readErr := readSkillFrontmatter(file)
	fileCloseErr := file.Close()
	directoryCloseErr := skillDirectory.Close()
	if joined := errors.Join(readErr, fileCloseErr, directoryCloseErr); joined != nil {
		return piaSkill{}, []SkillDiagnostic{{
			Path:    location,
			Message: "could not read bounded Skill frontmatter: " + boundedDiagnosticText(joined.Error()),
		}}, false
	}

	name, description, unknownFields, err := parseSkillFrontmatter(frontmatter)
	if err != nil {
		return piaSkill{}, []SkillDiagnostic{{
			Path:    location,
			Message: "invalid Skill frontmatter: " + boundedDiagnosticText(err.Error()),
		}}, false
	}

	var diagnostics []SkillDiagnostic
	if len(unknownFields) > 0 {
		diagnostics = append(diagnostics, SkillDiagnostic{
			Path:    location,
			Message: formatUnknownSkillFields(unknownFields),
		})
	}

	name = strings.TrimSpace(name)
	if name == "" {
		diagnostics = append(diagnostics, SkillDiagnostic{Path: location, Message: "required name is missing"})
		return piaSkill{}, diagnostics, false
	}
	nameCharacters := utf8.RuneCountInString(name)
	if nameCharacters > maxSkillNameCharacters {
		diagnostics = append(diagnostics, SkillDiagnostic{
			Path: location,
			Message: fmt.Sprintf(
				"name has %d characters and exceeds Pia's %d-character safety limit",
				nameCharacters,
				maxSkillNameCharacters,
			),
		})
		return piaSkill{}, diagnostics, false
	}
	if nameCharacters > maxPortableSkillNameCharacters || !portableSkillNamePattern.MatchString(name) {
		diagnostics = append(diagnostics, SkillDiagnostic{
			Path: location,
			Message: fmt.Sprintf(
				"name %q does not follow the portable 1-%d lowercase letters, digits, and hyphens format; Pia loaded it",
				boundedDiagnosticText(name),
				maxPortableSkillNameCharacters,
			),
		})
	}
	if name != directoryName {
		diagnostics = append(diagnostics, SkillDiagnostic{
			Path: location,
			Message: fmt.Sprintf(
				"frontmatter name %q does not match directory %q; Pia loaded the frontmatter name",
				boundedDiagnosticText(name),
				boundedDiagnosticText(directoryName),
			),
		})
	}

	description = strings.TrimSpace(description)
	if description == "" {
		diagnostics = append(diagnostics, SkillDiagnostic{Path: location, Message: "required description is missing"})
		return piaSkill{}, diagnostics, false
	}
	if utf8.RuneCountInString(description) > maxSkillDescriptionCharacters {
		description = truncateRunes(description, maxSkillDescriptionCharacters)
		diagnostics = append(diagnostics, SkillDiagnostic{
			Path: location,
			Message: fmt.Sprintf(
				"description was truncated to %d characters for the catalog",
				maxSkillDescriptionCharacters,
			),
		})
	}

	return piaSkill{
		Name:        name,
		Description: description,
		Location:    location,
	}, diagnostics, true
}

func readSkillFrontmatter(reader io.Reader) ([]byte, error) {
	prefix, err := io.ReadAll(io.LimitReader(reader, maxSkillFrontmatterBytes+1))
	if err != nil {
		return nil, err
	}
	firstEnd, firstHasNewline := skillLineEnd(prefix, 0)
	if firstEnd < 0 || !bytes.Equal(bytes.TrimSuffix(prefix[:firstEnd], []byte{'\r'}), []byte("---")) {
		return nil, fmt.Errorf("opening --- delimiter is required")
	}
	if !firstHasNewline {
		return nil, fmt.Errorf("closing --- delimiter is required")
	}
	contentStart := firstEnd + 1
	for lineStart := contentStart; lineStart <= len(prefix); {
		lineEnd, hasNewline := skillLineEnd(prefix, lineStart)
		if lineEnd < 0 {
			break
		}
		line := bytes.TrimSuffix(prefix[lineStart:lineEnd], []byte{'\r'})
		if bytes.Equal(line, []byte("---")) {
			closingEnd := lineEnd
			if hasNewline {
				closingEnd++
			}
			if closingEnd > maxSkillFrontmatterBytes {
				return nil, fmt.Errorf("frontmatter exceeds the %d-byte limit", maxSkillFrontmatterBytes)
			}
			frontmatter := prefix[contentStart:lineStart]
			if !utf8.Valid(frontmatter) {
				return nil, fmt.Errorf("frontmatter is not valid UTF-8")
			}
			return frontmatter, nil
		}
		if !hasNewline {
			break
		}
		lineStart = lineEnd + 1
	}

	if len(prefix) > maxSkillFrontmatterBytes {
		return nil, fmt.Errorf("frontmatter exceeds the %d-byte limit", maxSkillFrontmatterBytes)
	}
	return nil, fmt.Errorf("closing --- delimiter is required")
}

func skillLineEnd(content []byte, start int) (int, bool) {
	if start >= len(content) {
		return -1, false
	}
	if offset := bytes.IndexByte(content[start:], '\n'); offset >= 0 {
		return start + offset, true
	}
	return len(content), false
}

func parseSkillFrontmatter(frontmatter []byte) (string, string, []string, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(frontmatter, &document); err != nil {
		return "", "", nil, err
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return "", "", nil, fmt.Errorf("frontmatter must be a YAML mapping")
	}

	fields := document.Content[0].Content
	var name, description string
	var unknown []string
	seen := make(map[string]struct{}, len(fields)/2)
	for index := 0; index < len(fields); index += 2 {
		key := fields[index]
		value := fields[index+1]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
			return "", "", nil, fmt.Errorf("frontmatter keys must be strings")
		}
		if _, exists := seen[key.Value]; exists {
			return "", "", nil, fmt.Errorf("frontmatter field %q is duplicated", key.Value)
		}
		seen[key.Value] = struct{}{}
		switch key.Value {
		case "name":
			parsed, err := skillStringField("name", value)
			if err != nil {
				return "", "", nil, err
			}
			name = parsed
		case "description":
			parsed, err := skillStringField("description", value)
			if err != nil {
				return "", "", nil, err
			}
			description = parsed
		default:
			unknown = append(unknown, key.Value)
		}
	}
	sort.Strings(unknown)
	return name, description, unknown, nil
}

func skillStringField(name string, node *yaml.Node) (string, error) {
	if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
		return "", fmt.Errorf("%s must be a string", name)
	}
	return node.Value, nil
}

func buildPiaSkillCatalog(skills []piaSkill) (string, []SkillDiagnostic) {
	if len(skills) == 0 {
		return "", nil
	}

	full := renderPiaSkillCatalog(skills, maxSkillDescriptionCharacters)
	if ai.EstimateTextTokens(full) <= maxSkillCatalogTokens {
		return full, nil
	}

	included := len(skills)
	if ai.EstimateTextTokens(renderPiaSkillCatalog(skills, 0)) > maxSkillCatalogTokens {
		low, high := 0, len(skills)
		for low < high {
			middle := (low + high + 1) / 2
			if ai.EstimateTextTokens(renderPiaSkillCatalog(skills[:middle], 0)) <= maxSkillCatalogTokens {
				low = middle
			} else {
				high = middle - 1
			}
		}
		included = low
	}

	selected := skills[:included]
	descriptionCap := largestCatalogDescriptionCap(selected)
	catalog := renderPiaSkillCatalog(selected, descriptionCap)
	var diagnostics []SkillDiagnostic
	if catalogDescriptionsWereShortened(selected, descriptionCap) {
		diagnostics = append(diagnostics, SkillDiagnostic{
			Path: piaSkillsDirectory,
			Message: fmt.Sprintf(
				"Skill catalog descriptions were shortened to at most %d characters to stay within the %d-token estimate",
				descriptionCap,
				maxSkillCatalogTokens,
			),
		})
	}
	if included < len(skills) {
		diagnostics = append(diagnostics, SkillDiagnostic{
			Path: piaSkillsDirectory,
			Message: fmt.Sprintf(
				"Skill catalog omitted %d lexical tail entries to stay within the %d-token estimate",
				len(skills)-included,
				maxSkillCatalogTokens,
			),
		})
	}
	return catalog, diagnostics
}

func catalogDescriptionsWereShortened(skills []piaSkill, descriptionCap int) bool {
	for _, skill := range skills {
		if utf8.RuneCountInString(skill.Description) > descriptionCap {
			return true
		}
	}
	return false
}

func largestCatalogDescriptionCap(skills []piaSkill) int {
	low, high := 0, maxSkillDescriptionCharacters
	for low < high {
		middle := (low + high + 1) / 2
		if ai.EstimateTextTokens(renderPiaSkillCatalog(skills, middle)) <= maxSkillCatalogTokens {
			low = middle
		} else {
			high = middle - 1
		}
	}
	return low
}

func renderPiaSkillCatalog(skills []piaSkill, descriptionCap int) string {
	if len(skills) == 0 {
		return ""
	}

	var catalog strings.Builder
	catalog.WriteString("Project skills:\n")
	catalog.WriteString("These Pia Skill v1 entries are available in the selected workspace. When one matches the task, use the read tool on its location to load the complete SKILL.md before applying its instructions. Only the listed SKILL.md is a Skill entry; no supporting-resource behavior is implied.\n")
	catalog.WriteString("<available_skills>\n")
	for _, skill := range skills {
		catalog.WriteString("<skill>\n")
		fmt.Fprintf(&catalog, "<name>%s</name>\n", html.EscapeString(skill.Name))
		fmt.Fprintf(
			&catalog,
			"<description>%s</description>\n",
			html.EscapeString(truncateRunes(skill.Description, descriptionCap)),
		)
		fmt.Fprintf(&catalog, "<location>%s</location>\n", html.EscapeString(skill.Location))
		catalog.WriteString("</skill>\n")
	}
	catalog.WriteString("</available_skills>\n")
	return catalog.String()
}

func formatUnknownSkillFields(fields []string) string {
	shown := fields
	if len(shown) > maxUnknownFieldsInDiagnostic {
		shown = shown[:maxUnknownFieldsInDiagnostic]
	}
	quoted := make([]string, len(shown))
	for index, field := range shown {
		quoted[index] = fmt.Sprintf("%q", boundedDiagnosticText(field))
	}
	message := "unsupported frontmatter fields were ignored: " + strings.Join(quoted, ", ")
	if len(shown) < len(fields) {
		message += fmt.Sprintf("; %d more omitted", len(fields)-len(shown))
	}
	return boundedDiagnosticText(message)
}

func limitSkillDiagnostics(diagnostics []SkillDiagnostic) []SkillDiagnostic {
	if len(diagnostics) <= maxSkillDiagnostics {
		return diagnostics
	}
	limited := append([]SkillDiagnostic(nil), diagnostics[:maxSkillDiagnostics-1]...)
	limited = append(limited, SkillDiagnostic{
		Path: piaSkillsDirectory,
		Message: fmt.Sprintf(
			"%d additional Skill warnings were omitted",
			len(diagnostics)-(maxSkillDiagnostics-1),
		),
	})
	return limited
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit])
}

func boundedDiagnosticText(value string) string {
	const limit = 256
	value = strings.ReplaceAll(strings.TrimSpace(value), "\n", " ")
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return truncateRunes(value, limit-1) + "…"
}
