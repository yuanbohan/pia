package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/yuanbohan/pia/internal/agent"
	"github.com/yuanbohan/pia/internal/coding/skills"
)

func TestDefinitionExposesBoundedNameLookupAndParallelSafety(t *testing.T) {
	tool := newTestTool(t, t.TempDir(), testEntry("review-go"))
	definition := tool.Definition()
	if got, want := definition.Schema.Name, "skill"; got != want {
		t.Fatalf("Name = %q, want %q", got, want)
	}
	if !definition.CanRunParallel {
		t.Fatal("CanRunParallel = false, want true")
	}
	var schema struct {
		Type                 string `json:"type"`
		AdditionalProperties *bool  `json:"additionalProperties"`
		Required             []string
		Properties           map[string]struct {
			Type      string `json:"type"`
			MaxLength int    `json:"maxLength"`
			Enum      []string
		}
	}
	if err := json.Unmarshal(definition.Schema.Parameters, &schema); err != nil {
		t.Fatalf("Parameters JSON error = %v", err)
	}
	if schema.Type != "object" || schema.AdditionalProperties == nil || *schema.AdditionalProperties {
		t.Fatalf("schema = %#v, want strict object", schema)
	}
	if got, want := fmt.Sprint(schema.Required), "[name]"; got != want {
		t.Fatalf("required = %s, want %s", got, want)
	}
	if property := schema.Properties["name"]; property.Type != "string" || property.MaxLength != maxNameCharacters || property.Enum != nil {
		t.Fatalf("name property = %#v, want bounded string without duplicated catalog enum", property)
	}
	var _ agent.Tool = tool
}

func TestDescribeInvocationReportsOnlySkillName(t *testing.T) {
	tool := newTestTool(t, t.TempDir(), testEntry("review-go"))

	if got, want := tool.DescribeInvocation(json.RawMessage(
		`{"name":"review-go"}`,
	)), "Skill review-go"; got != want {
		t.Fatalf("DescribeInvocation() = %q, want %q", got, want)
	}
}

func TestExecuteReturnsStructuredCurrentInstructionsWithoutDedupe(t *testing.T) {
	directory := t.TempDir()
	writeTestSkill(t, directory, "review-go", "name: review-go\ndescription: Review.\n", "BODY_V1")
	tool := newTestTool(t, directory, testEntry("review-go"))

	first, err := tool.Execute(context.Background(), json.RawMessage(`{"name":"review-go"}`))
	if err != nil {
		t.Fatalf("Execute(V1) error = %v", err)
	}
	for _, fragment := range []string{
		`<skill_content name="review-go" location=".pia/skills/review-go/SKILL.md">`,
		"Base directory: .pia/skills/review-go",
		"BODY_V1",
		"</skill_content>",
	} {
		if !strings.Contains(first, fragment) {
			t.Errorf("Execute(V1) does not contain %q\n%s", fragment, first)
		}
	}
	if strings.Contains(first, "description: Review.") {
		t.Fatalf("Execute(V1) retained frontmatter\n%s", first)
	}

	writeTestSkill(t, directory, "review-go", "name: deploy-prod\ndescription: Current metadata is not revalidated.\n", "BODY_V2")
	second, err := tool.Execute(context.Background(), json.RawMessage(`{"name":"review-go"}`))
	if err != nil {
		t.Fatalf("Execute(V2) error = %v", err)
	}
	if !strings.Contains(second, "BODY_V2") || strings.Contains(second, "BODY_V1") {
		t.Fatalf("Execute(V2) = %q, want latest body without dedupe", second)
	}
}

func TestExecuteEnforcesExactFinalResultLimitAndRecoversAfterShrink(t *testing.T) {
	directory := t.TempDir()
	writeTestSkill(t, directory, "bounded", "name: bounded\ndescription: Bounded.\n", "")
	tool := newTestTool(t, directory, testEntry("bounded"))

	empty, err := tool.Execute(context.Background(), json.RawMessage(`{"name":"bounded"}`))
	if err != nil {
		t.Fatalf("Execute(empty) error = %v", err)
	}
	overhead := len(empty)
	if overhead >= maxResultBytes {
		t.Fatalf("empty result size = %d, want below %d", overhead, maxResultBytes)
	}

	exactBody := strings.Repeat("x", maxResultBytes-overhead)
	writeTestSkill(t, directory, "bounded", "name: bounded\ndescription: Bounded.\n", exactBody)
	exact, err := tool.Execute(context.Background(), json.RawMessage(`{"name":"bounded"}`))
	if err != nil {
		t.Fatalf("Execute(exact) error = %v", err)
	}
	if got := len(exact); got != maxResultBytes {
		t.Fatalf("Execute(exact) size = %d, want %d", got, maxResultBytes)
	}

	writeTestSkill(t, directory, "bounded", "name: bounded\ndescription: Bounded.\n", exactBody+"x")
	oversized, err := tool.Execute(context.Background(), json.RawMessage(`{"name":"bounded"}`))
	if oversized != "" || err == nil {
		t.Fatalf("Execute(oversized) = %q, %v, want error without partial result", oversized, err)
	}
	for _, fragment := range []string{"not activated", fmt.Sprint(maxResultBytes + 1), fmt.Sprint(maxResultBytes), ".pia/skills/bounded/SKILL.md", "read"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Errorf("oversized error does not contain %q: %v", fragment, err)
		}
	}
	if strings.Contains(err.Error(), strings.Repeat("x", 32)) {
		t.Fatalf("oversized error exposed body content: %v", err)
	}

	writeTestSkill(t, directory, "bounded", "name: bounded\ndescription: Bounded.\n", "small again")
	if recovered, err := tool.Execute(context.Background(), json.RawMessage(`{"name":"bounded"}`)); err != nil || !strings.Contains(recovered, "small again") {
		t.Fatalf("Execute(recovered) = %q, %v, want success after shrink", recovered, err)
	}
}

func TestExecuteValidatesArgumentsAndCatalogMembership(t *testing.T) {
	directory := t.TempDir()
	writeTestSkill(t, directory, "review-go", "name: review-go\ndescription: Review.\n", "BODY")
	tool := newTestTool(t, directory, testEntry("review-go"))
	tests := []struct {
		name      string
		arguments string
		want      string
	}{
		{name: "malformed", arguments: `{`, want: "decode arguments"},
		{name: "unknown field", arguments: `{"name":"review-go","path":"SKILL.md"}`, want: "unknown field"},
		{name: "empty", arguments: `{"name":""}`, want: "name is required"},
		{name: "unknown", arguments: `{"name":"deploy-prod"}`, want: "not present in the Conversation Skill catalog"},
		{name: "oversized arguments", arguments: `{"name":"` + strings.Repeat("x", maxArgumentsBytes) + `"}`, want: "arguments exceed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := tool.Execute(context.Background(), json.RawMessage(test.arguments))
			if result != "" || err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Execute(%s) = %q, %v, want error containing %q", test.arguments, result, err, test.want)
			}
		})
	}
}

func TestNewValidatesEntriesAndExecuteIsConcurrentSafe(t *testing.T) {
	rootPath := t.TempDir()
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatalf("OpenRoot() error = %v", err)
	}
	t.Cleanup(func() { _ = root.Close() })
	if _, err := New(nil, []skills.Entry{testEntry("review-go")}); err == nil {
		t.Fatal("New(nil) succeeded")
	}
	if _, err := New(root, nil); err == nil {
		t.Fatal("New(empty entries) succeeded")
	}
	if _, err := New(root, []skills.Entry{testEntry("review-go"), testEntry("review-go")}); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("New(duplicate) error = %v, want duplicate", err)
	}
	invalid := testEntry("review-go")
	invalid.Directory = "../review-go"
	if _, err := New(root, []skills.Entry{invalid}); err == nil || !strings.Contains(err.Error(), "direct child") {
		t.Fatalf("New(escaping directory) error = %v, want direct-child failure", err)
	}
	invalid = testEntry("review-go")
	invalid.Location = ".pia/skills/other/SKILL.md"
	if _, err := New(root, []skills.Entry{invalid}); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("New(mismatched location) error = %v, want location failure", err)
	}

	writeTestSkill(t, rootPath, "review-go", "name: review-go\ndescription: Review.\n", "CONCURRENT_BODY")
	tool, err := New(root, []skills.Entry{testEntry("review-go")})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	var wait sync.WaitGroup
	errorsFound := make(chan error, 16)
	for range 16 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, executeErr := tool.Execute(context.Background(), json.RawMessage(`{"name":"review-go"}`))
			if executeErr != nil {
				errorsFound <- executeErr
				return
			}
			if !strings.Contains(result, "CONCURRENT_BODY") {
				errorsFound <- fmt.Errorf("result omitted body")
			}
		}()
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Errorf("concurrent Execute() error = %v", err)
	}
}

func newTestTool(t *testing.T, directory string, entries ...skills.Entry) *Tool {
	t.Helper()
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatalf("OpenRoot() error = %v", err)
	}
	t.Cleanup(func() {
		if err := root.Close(); err != nil {
			t.Errorf("Root.Close() error = %v", err)
		}
	})
	tool, err := New(root, entries)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return tool
}

func testEntry(name string) skills.Entry {
	return skills.Entry{
		Name:      name,
		Directory: name,
		Location:  filepath.ToSlash(filepath.Join(".pia", "skills", name, "SKILL.md")),
	}
}

func writeTestSkill(t *testing.T, workspace, directory, frontmatter, body string) {
	t.Helper()
	skillDirectory := filepath.Join(workspace, ".pia", "skills", directory)
	if err := os.MkdirAll(skillDirectory, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	content := "---\n" + frontmatter + "---\n" + body
	if err := os.WriteFile(filepath.Join(skillDirectory, "SKILL.md"), []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
