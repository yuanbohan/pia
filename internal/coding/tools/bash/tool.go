// Package bash implements the model-facing shell tool used by the local coding
// Agent. It deliberately provides process control rather than filesystem
// containment: commands start in the workspace but run with the current user's
// host permissions and complete parent-process environment.
package bash

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/yuanbohan/pia/internal/agent"
	"github.com/yuanbohan/pia/internal/ai"
	"github.com/yuanbohan/pia/internal/coding/tools/toolargs"
)

const (
	maxTimeoutMilliseconds = 1<<31 - 1
	maxTimeoutSeconds      = float64(maxTimeoutMilliseconds) / 1000

	bashParametersSchema = `{
  "type": "object",
  "properties": {
    "command": {
      "type": "string",
      "description": "Bash command to execute. Empty commands are valid."
    },
    "timeout": {
      "type": "number",
      "description": "Optional timeout in seconds. There is no default timeout."
    }
  },
  "required": ["command"],
  "additionalProperties": false
}`
)

// Config supplies the host path required by exec.Cmd and an optional shell
// override. The composition root obtains WorkingDirectory from the canonical
// coding Workspace; unlike file tools, Bash cannot be contained by os.Root.
type Config struct {
	WorkingDirectory string
	ShellPath        string
}

// Tool executes each invocation in a fresh shell. The Go value has no mutable
// per-call state, but Definition keeps it a serial barrier because independent
// commands may still race through the shared workspace or other host resources.
type Tool struct {
	workingDirectory string
	shell            string
}

// New validates the fixed working directory and resolves the shell once so a
// later PATH or filesystem change cannot silently alter this Tool's executable.
func New(config Config) (*Tool, error) {
	if config.WorkingDirectory == "" {
		return nil, fmt.Errorf("coding tools: bash working directory is required")
	}
	info, err := os.Stat(config.WorkingDirectory)
	if err != nil {
		return nil, fmt.Errorf("coding tools: bash working directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("coding tools: bash working directory %q is not a directory", config.WorkingDirectory)
	}

	shell, err := resolveShell(config.ShellPath)
	if err != nil {
		return nil, fmt.Errorf("coding tools: bash shell: %w", err)
	}
	return &Tool{workingDirectory: config.WorkingDirectory, shell: shell}, nil
}

// Definition exposes frozen Pi's command and optional-timeout protocol. Bash
// intentionally leaves CanRunParallel false because it is a serial barrier.
func (t *Tool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Schema: ai.ToolSchema{
			Name: "bash",
			Description: "Execute a non-interactive shell command in the workspace. Returns merged stdout and stderr; " +
				"output is limited to the last 2000 lines or 50 KiB, with complete output saved to a temp file when truncated.",
			Parameters: json.RawMessage(bashParametersSchema),
		},
	}
}

// Execute starts one fresh shell, incrementally drains its output, and waits
// for the direct shell on every started path. Timeout and cancellation kill the
// original process group; a normal shell exit intentionally leaves background
// processes running to preserve frozen Pi's local CLI behavior.
func (t *Tool) Execute(ctx context.Context, rawArguments json.RawMessage) (string, error) {
	if cause := context.Cause(ctx); cause != nil {
		return "", cause
	}
	input, err := decodeArguments(rawArguments)
	if err != nil {
		return "", err
	}

	snapshot, err := runCommand(ctx, commandConfig{
		shell:            t.shell,
		workingDirectory: t.workingDirectory,
		command:          input.command,
		timeout:          input.timeout,
	})
	emptyText := ""
	var statusErr *exitStatusError
	if err == nil || errors.As(err, &statusErr) {
		emptyText = "(no output)"
	}
	return formatOutput(snapshot, emptyText), err
}

type arguments struct {
	command string
	timeout *time.Duration
}

type rawArguments struct {
	Command *string         `json:"command"`
	Timeout json.RawMessage `json:"timeout"`
}

func decodeArguments(raw json.RawMessage) (arguments, error) {
	decoded, err := toolargs.Decode[rawArguments](raw)
	if err != nil {
		return arguments{}, fmt.Errorf("bash: decode arguments: %w", err)
	}
	if decoded.Command == nil {
		return arguments{}, fmt.Errorf("bash: command is required")
	}

	input := arguments{command: *decoded.Command}
	if decoded.Timeout == nil {
		return input, nil
	}
	if bytes.Equal(bytes.TrimSpace(decoded.Timeout), []byte("null")) {
		return arguments{}, fmt.Errorf("bash: timeout must be a number of seconds")
	}

	var seconds float64
	if err := json.Unmarshal(decoded.Timeout, &seconds); err != nil {
		return arguments{}, fmt.Errorf("bash: timeout must be a number of seconds: %w", err)
	}
	if seconds <= 0 {
		return arguments{}, fmt.Errorf("bash: timeout must be greater than zero")
	}
	if seconds > maxTimeoutSeconds {
		return arguments{}, fmt.Errorf("bash: timeout exceeds the maximum %.3f seconds", maxTimeoutSeconds)
	}
	timeout := time.Duration(seconds * float64(time.Second))
	if timeout <= 0 {
		return arguments{}, fmt.Errorf("bash: timeout is too small to represent")
	}
	input.timeout = &timeout
	return input, nil
}

var _ agent.Tool = (*Tool)(nil)
