package skills

import (
	"fmt"
	"html"
	"strings"
	"unicode/utf8"

	"github.com/yuanbohan/pia/internal/ai"
)

func buildPiaSkillCatalog(skills []Entry) (string, []Entry, []Diagnostic) {
	if len(skills) == 0 {
		return "", nil, nil
	}

	full := renderPiaSkillCatalog(skills, maxSkillDescriptionCharacters)
	if ai.EstimateTextTokens(full) <= maxSkillCatalogTokens {
		return full, append([]Entry(nil), skills...), nil
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
	var diagnostics []Diagnostic
	if catalogDescriptionsWereShortened(selected, descriptionCap) {
		diagnostics = append(diagnostics, Diagnostic{
			Path: piaSkillsDirectory,
			Message: fmt.Sprintf(
				"Skill catalog descriptions were shortened to at most %d characters to stay within the %d-token estimate",
				descriptionCap,
				maxSkillCatalogTokens,
			),
		})
	}
	if included < len(skills) {
		diagnostics = append(diagnostics, Diagnostic{
			Path: piaSkillsDirectory,
			Message: fmt.Sprintf(
				"Skill catalog omitted %d lexical tail entries to stay within the %d-token estimate",
				len(skills)-included,
				maxSkillCatalogTokens,
			),
		})
	}
	return catalog, append([]Entry(nil), selected...), diagnostics
}

func catalogDescriptionsWereShortened(skills []Entry, descriptionCap int) bool {
	for _, skill := range skills {
		if utf8.RuneCountInString(skill.Description) > descriptionCap {
			return true
		}
	}
	return false
}

func largestCatalogDescriptionCap(skills []Entry) int {
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

func renderPiaSkillCatalog(skills []Entry, descriptionCap int) string {
	if len(skills) == 0 {
		return ""
	}

	var catalog strings.Builder
	catalog.WriteString("Project skills:\n")
	catalog.WriteString("These Pia Skill v1 entries are available in the selected workspace. When one matches the task, use the skill tool with its exact catalog name to load the complete current instructions before applying them. Use read only for explicitly referenced project files or the documented oversized-Skill fallback; no supporting-resource behavior is implied.\n")
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

func limitSkillDiagnostics(diagnostics []Diagnostic) []Diagnostic {
	if len(diagnostics) <= maxSkillDiagnostics {
		return diagnostics
	}
	limited := append([]Diagnostic(nil), diagnostics[:maxSkillDiagnostics-1]...)
	limited = append(limited, Diagnostic{
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
