package skills

import (
	"fmt"
	"sort"

	"go.yaml.in/yaml/v3"
)

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
