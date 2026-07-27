// Package skill implements the model-facing activation tool for project-local
// Skills discovered by the coding application.
package skill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"os"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/yuanbohan/pia/internal/agent"
	"github.com/yuanbohan/pia/internal/ai"
	"github.com/yuanbohan/pia/internal/coding/skills"
	"github.com/yuanbohan/pia/internal/coding/tools/toolargs"
)

const (
	maxArgumentsBytes = 4 << 10
	maxNameCharacters = 256
	maxResultBytes    = 50 << 10

	parametersSchema = `{
  "type": "object",
  "properties": {
    "name": {
      "type": "string",
      "minLength": 1,
      "maxLength": 256,
      "description": "Exact name of one Skill listed in the Conversation's project Skill catalog."
    }
  },
  "required": ["name"],
  "additionalProperties": false
}`
)

// Tool resolves only the immutable catalog entries supplied at construction.
// It has no activation state; every Execute call reloads current instructions.
type Tool struct {
	root    *os.Root
	entries map[string]skills.Entry
}

// New binds the tool to a borrowed workspace root and one catalog snapshot.
func New(root *os.Root, entries []skills.Entry) (*Tool, error) {
	if root == nil {
		return nil, fmt.Errorf("coding tools: skill root is required")
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("coding tools: at least one catalog Skill is required")
	}

	byName := make(map[string]skills.Entry, len(entries))
	for index, entry := range entries {
		if err := validateEntry(entry); err != nil {
			return nil, fmt.Errorf("coding tools: Skill entry %d: %w", index, err)
		}
		if _, exists := byName[entry.Name]; exists {
			return nil, fmt.Errorf("coding tools: duplicate Skill name %q", entry.Name)
		}
		byName[entry.Name] = entry
	}
	return &Tool{root: root, entries: byName}, nil
}

// Definition exposes an exact-name lookup without duplicating catalog names
// into a second model-visible enum.
func (t *Tool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Schema: ai.ToolSchema{
			Name: "skill",
			Description: "Load the complete current instructions for one project Skill " +
				"by its exact catalog name.",
			Parameters: json.RawMessage(parametersSchema),
		},
		CanRunParallel: true,
	}
}

// DescribeInvocation reports only the catalog identity requested by the model.
func (t *Tool) DescribeInvocation(rawArguments json.RawMessage) string {
	input, err := decodeArguments(rawArguments)
	if err != nil {
		return "Skill"
	}
	return "Skill " + input.Name
}

// Execute returns one full structured result or one call-local error. It never
// returns a truncated preview or consults activation/cache state.
func (t *Tool) Execute(ctx context.Context, rawArguments json.RawMessage) (string, error) {
	if cause := context.Cause(ctx); cause != nil {
		return "", cause
	}
	input, err := decodeArguments(rawArguments)
	if err != nil {
		return "", err
	}
	entry, exists := t.entries[input.Name]
	if !exists {
		return "", fmt.Errorf("skill: name %q is not present in the Conversation Skill catalog", input.Name)
	}

	prefix, suffix := resultEnvelope(entry)
	bodyBudget := int64(maxResultBytes - len(prefix) - len(suffix))
	body, err := skills.LoadCurrentBody(ctx, t.root, entry, bodyBudget)
	if err != nil {
		var limitErr *skills.BodyLimitError
		if errors.As(err, &limitErr) {
			actual := int64(len(prefix)+len(suffix)) + limitErr.Size
			return "", oversizedError(entry, actual)
		}
		return "", fmt.Errorf("skill %q at %q was not activated: load current instructions: %w", entry.Name, entry.Location, err)
	}

	var result strings.Builder
	result.Grow(len(prefix) + len(body) + len(suffix))
	result.WriteString(prefix)
	result.Write(body)
	result.WriteString(suffix)
	if result.Len() > maxResultBytes {
		return "", oversizedError(entry, int64(result.Len()))
	}
	return result.String(), nil
}

type arguments struct {
	Name string `json:"name"`
}

func decodeArguments(raw json.RawMessage) (arguments, error) {
	if len(raw) > maxArgumentsBytes {
		return arguments{}, fmt.Errorf("skill: arguments exceed the %d-byte limit", maxArgumentsBytes)
	}
	input, err := toolargs.Decode[arguments](raw)
	if err != nil {
		return arguments{}, fmt.Errorf("skill: decode arguments: %w", err)
	}
	if input.Name == "" {
		return arguments{}, fmt.Errorf("skill: name is required")
	}
	if !utf8.ValidString(input.Name) {
		return arguments{}, fmt.Errorf("skill: name is not valid UTF-8")
	}
	if utf8.RuneCountInString(input.Name) > maxNameCharacters {
		return arguments{}, fmt.Errorf("skill: name exceeds the %d-character limit", maxNameCharacters)
	}
	return input, nil
}

func validateEntry(entry skills.Entry) error {
	if entry.Name == "" || !utf8.ValidString(entry.Name) || utf8.RuneCountInString(entry.Name) > maxNameCharacters {
		return fmt.Errorf("name is required, valid UTF-8, and at most %d characters", maxNameCharacters)
	}
	if entry.Directory == "" || entry.Directory == "." || entry.Directory == ".." || strings.Contains(entry.Directory, "/") {
		return fmt.Errorf("directory must be one direct child")
	}
	expectedLocation := path.Join(".pia/skills", entry.Directory, "SKILL.md")
	if entry.Location != expectedLocation {
		return fmt.Errorf("location %q does not match %q", entry.Location, expectedLocation)
	}
	return nil
}

func resultEnvelope(entry skills.Entry) (string, string) {
	name := html.EscapeString(entry.Name)
	location := html.EscapeString(entry.Location)
	base := html.EscapeString(path.Dir(entry.Location))
	prefix := fmt.Sprintf(
		"<skill_content name=\"%s\" location=\"%s\">\n"+
			"Base directory: %s\n"+
			"The following is the complete Skill instructions after frontmatter:\n",
		name,
		location,
		base,
	)
	return prefix, "\n</skill_content>"
}

func oversizedError(entry skills.Entry, actual int64) error {
	return fmt.Errorf(
		"skill %q was not activated: complete result has %d bytes and exceeds the %d-byte limit; source remains at %s; use read with offsets or reduce SKILL.md before retrying",
		entry.Name,
		actual,
		maxResultBytes,
		entry.Location,
	)
}

var _ agent.Tool = (*Tool)(nil)
