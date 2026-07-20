//go:build darwin || linux

package bash

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefinitionExposesCommandAndTimeoutSchema(t *testing.T) {
	t.Parallel()

	tool := newTestTool(t, t.TempDir())
	definition := tool.Definition()
	if definition.Schema.Name != "bash" {
		t.Fatalf("Definition().Schema.Name = %q, want bash", definition.Schema.Name)
	}
	if definition.CanRunParallel {
		t.Fatal("Definition().CanRunParallel = true, want serial barrier")
	}

	var schema struct {
		Type                 string                     `json:"type"`
		AdditionalProperties *bool                      `json:"additionalProperties"`
		Properties           map[string]json.RawMessage `json:"properties"`
		Required             []string                   `json:"required"`
	}
	if err := json.Unmarshal(definition.Schema.Parameters, &schema); err != nil {
		t.Fatalf("Unmarshal(schema) error = %v", err)
	}
	if len(schema.Properties) != 2 || schema.Properties["command"] == nil || schema.Properties["timeout"] == nil {
		t.Fatalf("schema properties = %v, want command and timeout", schema.Properties)
	}
	if schema.Type != "object" || schema.AdditionalProperties == nil || *schema.AdditionalProperties {
		t.Fatalf("schema object boundary = type %q additionalProperties %v", schema.Type, schema.AdditionalProperties)
	}
	if len(schema.Required) != 1 || schema.Required[0] != "command" {
		t.Fatalf("schema required = %v, want [command]", schema.Required)
	}
}

func TestNewValidatesConfiguration(t *testing.T) {
	t.Parallel()

	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	tests := []struct {
		name   string
		config Config
		want   string
	}{
		{name: "missing working directory", config: Config{ShellPath: "/bin/sh"}, want: "working directory is required"},
		{name: "missing directory", config: Config{WorkingDirectory: filepath.Join(t.TempDir(), "missing"), ShellPath: "/bin/sh"}, want: "working directory"},
		{name: "regular file", config: Config{WorkingDirectory: file, ShellPath: "/bin/sh"}, want: "not a directory"},
		{name: "missing explicit shell", config: Config{WorkingDirectory: t.TempDir(), ShellPath: filepath.Join(t.TempDir(), "missing-shell")}, want: "shell"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := New(test.config)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("New() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestExecuteUsesWorkspaceAndCompleteParentEnvironment(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("PI_GO_BASH_PARENT_ENV", "from-zsh-parent")
	tool := newTestTool(t, workspace)

	result, err := tool.Execute(context.Background(), argumentsJSON(t,
		`printf 'cwd=%s\nenv=%s\n' "$PWD" "$PI_GO_BASH_PARENT_ENV"`, nil))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result, "cwd="+workspace+"\n") {
		t.Fatalf("Execute() result = %q, want workspace cwd", result)
	}
	if !strings.Contains(result, "env=from-zsh-parent\n") {
		t.Fatalf("Execute() result = %q, want inherited environment", result)
	}
}

func TestExecuteStartsFreshNonInteractiveShell(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	tool := newTestTool(t, workspace)
	first, err := tool.Execute(context.Background(), argumentsJSON(t,
		`mkdir -p nested && cd nested && export PI_GO_CALL_STATE=changed && printf '%s\n' "$PWD"`, nil))
	if err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}
	if !strings.Contains(first, filepath.Join(workspace, "nested")) {
		t.Fatalf("first Execute() result = %q, want nested cwd", first)
	}

	second, err := tool.Execute(context.Background(), argumentsJSON(t,
		`printf 'cwd=%s\nstate=%s\n' "$PWD" "${PI_GO_CALL_STATE-unset}"`, nil))
	if err != nil {
		t.Fatalf("second Execute() error = %v", err)
	}
	if !strings.Contains(second, "cwd="+workspace+"\n") || !strings.Contains(second, "state=unset\n") {
		t.Fatalf("second Execute() result = %q, want reset cwd and environment", second)
	}
}

func TestExecuteProvidesEOFOnStdin(t *testing.T) {
	t.Parallel()

	tool := newTestTool(t, t.TempDir())
	result, err := tool.Execute(context.Background(), argumentsJSON(t,
		`if read value; then printf 'read:%s\n' "$value"; else printf 'stdin-eof\n'; fi`, nil))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result != "stdin-eof\n" {
		t.Fatalf("Execute() result = %q, want stdin EOF", result)
	}
}

func TestExecuteCombinesOutputAndReportsExitFailure(t *testing.T) {
	t.Parallel()

	tool := newTestTool(t, t.TempDir())
	result, err := tool.Execute(context.Background(), argumentsJSON(t,
		`printf 'stdout-value\n'; printf 'stderr-value\n' >&2; exit 7`, nil))
	if err == nil || !strings.Contains(err.Error(), "code 7") {
		t.Fatalf("Execute() error = %v, want exit code 7", err)
	}
	if !strings.Contains(result, "stdout-value") || !strings.Contains(result, "stderr-value") {
		t.Fatalf("Execute() result = %q, want stdout and stderr", result)
	}
}

func TestExecuteAllowsEmptyCommand(t *testing.T) {
	t.Parallel()

	tool := newTestTool(t, t.TempDir())
	result, err := tool.Execute(context.Background(), argumentsJSON(t, "", nil))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result != "(no output)" {
		t.Fatalf("Execute() result = %q, want no-output marker", result)
	}
}

func TestExecuteUsesExplicitShellPath(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	shellPath := filepath.Join(directory, "test-shell")
	shell := "#!/bin/sh\nprintf 'custom-shell\\n' >&2\nexec /bin/sh \"$@\"\n"
	if err := os.WriteFile(shellPath, []byte(shell), 0o700); err != nil {
		t.Fatalf("WriteFile(shell) error = %v", err)
	}
	tool, err := New(Config{WorkingDirectory: directory, ShellPath: shellPath})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := tool.Execute(context.Background(), argumentsJSON(t, `printf 'command-output\n'`, nil))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result, "custom-shell\n") || !strings.Contains(result, "command-output\n") {
		t.Fatalf("Execute() result = %q, want explicit shell marker and command output", result)
	}
}

func TestNewUsesDefaultShellResolution(t *testing.T) {
	t.Parallel()

	tool, err := New(Config{WorkingDirectory: t.TempDir()})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	result, err := tool.Execute(context.Background(), argumentsJSON(t, `printf 'resolved\n'`, nil))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result != "resolved\n" {
		t.Fatalf("Execute() result = %q, want default shell output", result)
	}
}

func TestDecodeArgumentsIsStrict(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "non object", raw: `[]`, want: "JSON object"},
		{name: "missing command", raw: `{}`, want: "command is required"},
		{name: "null command", raw: `{"command":null}`, want: "command is required"},
		{name: "wrong command type", raw: `{"command":1}`, want: "decode arguments"},
		{name: "unknown field", raw: `{"command":"true","extra":1}`, want: "unknown field"},
		{name: "null timeout", raw: `{"command":"true","timeout":null}`, want: "timeout must be a number"},
		{name: "zero timeout", raw: `{"command":"true","timeout":0}`, want: "timeout must be greater than zero"},
		{name: "negative timeout", raw: `{"command":"true","timeout":-1}`, want: "timeout must be greater than zero"},
		{name: "timeout too large", raw: `{"command":"true","timeout":2147483.648}`, want: "timeout exceeds"},
		{name: "trailing document", raw: `{"command":"true"}{}`, want: "exactly one JSON object"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := decodeArguments(json.RawMessage(test.raw))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("decodeArguments() error = %v, want substring %q", err, test.want)
			}
		})
	}

	input, err := decodeArguments(json.RawMessage(`{"command":"","timeout":0.125}`))
	if err != nil {
		t.Fatalf("decodeArguments(valid) error = %v", err)
	}
	if input.command != "" || input.timeout == nil || input.timeout.Seconds() != 0.125 {
		t.Fatalf("decodeArguments(valid) = %#v, want empty command and 125ms timeout", input)
	}
}
