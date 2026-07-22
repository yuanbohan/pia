// Package skills owns project-local Skill discovery, catalog disclosure, and
// activation-time document loading for the coding application.
package skills

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/yuanbohan/pia/internal/coding/tools/fileutil"
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

// Diagnostic is one bounded operator warning produced while taking the
// project Skill snapshot. Skill failures never block an otherwise valid Run.
type Diagnostic struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// Discovery is one Conversation-start Skill snapshot. Entries is exactly the
// set of winners rendered into Catalog.
type Discovery struct {
	Catalog     string
	Entries     []Entry
	Diagnostics []Diagnostic
}

// Entry is one catalog-visible Skill lookup. Directory records the direct
// source child independently from Name because Pia permits a diagnostic-only
// mismatch between directory and frontmatter names.
type Entry struct {
	Name        string
	Description string
	Directory   string
	Location    string
}

// Discover takes one project-local snapshot before the Conversation starts.
// It deliberately discovers only direct directories under .pia/skills; a
// later Run never reloads this data.
func Discover(root *os.Root) (Discovery, error) {
	if root == nil {
		return Discovery{}, fmt.Errorf("coding skills: root is required")
	}

	skillsDirectory, err := openPiaSkillsDirectory(root)
	if errors.Is(err, fs.ErrNotExist) {
		return Discovery{}, nil
	}
	if err != nil {
		return Discovery{
			Diagnostics: []Diagnostic{{
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
		return Discovery{
			Diagnostics: []Diagnostic{{
				Path:    piaSkillsDirectory,
				Message: "could not read project Skills directory: " + boundedDiagnosticText(joined.Error()),
			}},
		}, nil
	}
	if len(entries) > maxPiaSkillDirectoryEntries {
		diagnostics := []Diagnostic{{
			Path: piaSkillsDirectory,
			Message: fmt.Sprintf(
				"project Skills were ignored because the directory contains more than %d direct entries",
				maxPiaSkillDirectoryEntries,
			),
		}}
		if closeErr := skillsDirectory.Close(); closeErr != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Path:    piaSkillsDirectory,
				Message: "could not close project Skills directory: " + boundedDiagnosticText(closeErr.Error()),
			})
		}
		return Discovery{Diagnostics: diagnostics}, nil
	}
	sort.Slice(entries, func(left, right int) bool {
		return entries[left].Name() < entries[right].Name()
	})

	var directoryNames []string
	var candidateDiagnostics []Diagnostic
	for _, entry := range entries {
		name := entry.Name()
		isDirectory := entry.IsDir()
		isSymlink := entry.Type()&fs.ModeSymlink != 0
		if (isDirectory || isSymlink) && !utf8.ValidString(name) {
			candidateDiagnostics = append(candidateDiagnostics, Diagnostic{
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
			candidateDiagnostics = append(candidateDiagnostics, Diagnostic{
				Path:    path.Join(piaSkillsDirectory, name),
				Message: "symlink Skill directories are unsupported and were ignored",
			})
		}
	}

	var priorityDiagnostics []Diagnostic
	if len(directoryNames) > maxPiaSkillCandidates {
		priorityDiagnostics = append(priorityDiagnostics, Diagnostic{
			Path: piaSkillsDirectory,
			Message: fmt.Sprintf(
				"only the first %d direct Skill directories were inspected; %d tail directories were omitted",
				maxPiaSkillCandidates,
				len(directoryNames)-maxPiaSkillCandidates,
			),
		})
		directoryNames = directoryNames[:maxPiaSkillCandidates]
	}

	byName := make(map[string]Entry, len(directoryNames))
	for _, directoryName := range directoryNames {
		location := path.Join(piaSkillsDirectory, directoryName, piaSkillFilename)
		skill, diagnostics, ok := loadPiaSkill(skillsDirectory, directoryName, location)
		candidateDiagnostics = append(candidateDiagnostics, diagnostics...)
		if !ok {
			continue
		}
		if winner, exists := byName[skill.Name]; exists {
			candidateDiagnostics = append(candidateDiagnostics, Diagnostic{
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
		candidateDiagnostics = append(candidateDiagnostics, Diagnostic{
			Path:    piaSkillsDirectory,
			Message: "could not close project Skills directory: " + boundedDiagnosticText(closeErr.Error()),
		})
	}

	skills := make([]Entry, 0, len(byName))
	for _, skill := range byName {
		skills = append(skills, skill)
	}
	sort.Slice(skills, func(left, right int) bool {
		if skills[left].Name == skills[right].Name {
			return skills[left].Location < skills[right].Location
		}
		return skills[left].Name < skills[right].Name
	})

	catalog, catalogEntries, catalogDiagnostics := buildPiaSkillCatalog(skills)
	allDiagnostics := append(priorityDiagnostics, catalogDiagnostics...)
	allDiagnostics = append(allDiagnostics, candidateDiagnostics...)
	return Discovery{
		Catalog:     catalog,
		Entries:     catalogEntries,
		Diagnostics: limitSkillDiagnostics(allDiagnostics),
	}, nil
}

func loadPiaSkill(
	skillsDirectory *os.File,
	directoryName string,
	location string,
) (Entry, []Diagnostic, bool) {
	skillDirectory, err := fileutil.OpenDirectoryAt(skillsDirectory, directoryName)
	if err != nil {
		return Entry{}, []Diagnostic{{
			Path:    location,
			Message: "could not open a direct project Skill directory: " + boundedDiagnosticText(err.Error()),
		}}, false
	}

	file, err := fileutil.OpenRegularFileAt(skillDirectory, piaSkillFilename)
	if err != nil {
		joined := errors.Join(err, skillDirectory.Close())
		return Entry{}, []Diagnostic{{
			Path:    location,
			Message: "could not open a direct regular project Skill: " + boundedDiagnosticText(joined.Error()),
		}}, false
	}

	frontmatter, readErr := readSkillFrontmatter(file)
	fileCloseErr := file.Close()
	directoryCloseErr := skillDirectory.Close()
	if joined := errors.Join(readErr, fileCloseErr, directoryCloseErr); joined != nil {
		return Entry{}, []Diagnostic{{
			Path:    location,
			Message: "could not read bounded Skill frontmatter: " + boundedDiagnosticText(joined.Error()),
		}}, false
	}

	name, description, unknownFields, err := parseSkillFrontmatter(frontmatter)
	if err != nil {
		return Entry{}, []Diagnostic{{
			Path:    location,
			Message: "invalid Skill frontmatter: " + boundedDiagnosticText(err.Error()),
		}}, false
	}

	var diagnostics []Diagnostic
	if len(unknownFields) > 0 {
		diagnostics = append(diagnostics, Diagnostic{
			Path:    location,
			Message: formatUnknownSkillFields(unknownFields),
		})
	}

	name = strings.TrimSpace(name)
	if name == "" {
		diagnostics = append(diagnostics, Diagnostic{Path: location, Message: "required name is missing"})
		return Entry{}, diagnostics, false
	}
	nameCharacters := utf8.RuneCountInString(name)
	if nameCharacters > maxSkillNameCharacters {
		diagnostics = append(diagnostics, Diagnostic{
			Path: location,
			Message: fmt.Sprintf(
				"name has %d characters and exceeds Pia's %d-character safety limit",
				nameCharacters,
				maxSkillNameCharacters,
			),
		})
		return Entry{}, diagnostics, false
	}
	if nameCharacters > maxPortableSkillNameCharacters || !portableSkillNamePattern.MatchString(name) {
		diagnostics = append(diagnostics, Diagnostic{
			Path: location,
			Message: fmt.Sprintf(
				"name %q does not follow the portable 1-%d lowercase letters, digits, and hyphens format; Pia loaded it",
				boundedDiagnosticText(name),
				maxPortableSkillNameCharacters,
			),
		})
	}
	if name != directoryName {
		diagnostics = append(diagnostics, Diagnostic{
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
		diagnostics = append(diagnostics, Diagnostic{Path: location, Message: "required description is missing"})
		return Entry{}, diagnostics, false
	}
	if utf8.RuneCountInString(description) > maxSkillDescriptionCharacters {
		description = truncateRunes(description, maxSkillDescriptionCharacters)
		diagnostics = append(diagnostics, Diagnostic{
			Path: location,
			Message: fmt.Sprintf(
				"description was truncated to %d characters for the catalog",
				maxSkillDescriptionCharacters,
			),
		})
	}

	return Entry{
		Name:        name,
		Description: description,
		Directory:   directoryName,
		Location:    location,
	}, diagnostics, true
}
