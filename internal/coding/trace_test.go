package coding

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/yuanbohan/pia/internal/ai"
)

func TestBuildTraceRepresentsEveryTranscriptVariant(t *testing.T) {
	parameters := json.RawMessage(`{"type":"object"}`)
	arguments := json.RawMessage(`{"path":`)
	result := RunResult{
		WorkspacePath: "/workspace",
		SystemPrompt:  "stable prompt",
		Model: ModelInfo{
			Provider:        "deepseek",
			Name:            "deepseek-v4-pro",
			Thinking:        true,
			ReasoningEffort: "high",
		},
		Tools: []ai.ToolSchema{{
			Name:        "read",
			Description: "Read a file",
			Parameters:  parameters,
		}},
		SkillDiagnostics: []SkillDiagnostic{{
			Path:    ".pia/skills/broken/SKILL.md",
			Message: "required name is missing",
		}},
		Transcript: []ai.Message{
			ai.UserMessage{Content: "inspect"},
			ai.AssistantMessage{
				Content: []ai.AssistantContent{
					ai.ThinkingContent{Thinking: "reasoning"},
					ai.TextContent{Text: "partial answer"},
					ai.ToolCall{ID: "call-1", Name: "read", Arguments: arguments},
				},
				Usage:        ai.Usage{InputTokens: 12, OutputTokens: 7},
				StopReason:   ai.StopReasonError,
				ErrorMessage: "provider error",
			},
			ai.ToolResultMessage{
				ToolCallID: "call-1",
				ToolName:   "read",
				Content:    "synthetic-secret-deliberately-returned-by-tool",
				IsError:    true,
			},
		},
	}
	runErr := errors.New("run failed")

	trace, err := BuildTrace(result, runErr)
	if err != nil {
		t.Fatalf("BuildTrace() error = %v", err)
	}
	if trace.RunError != runErr.Error() {
		t.Fatalf("trace RunError = %q, want %q", trace.RunError, runErr)
	}
	if got, want := trace.SkillDiagnostics, result.SkillDiagnostics; !reflect.DeepEqual(got, want) {
		t.Fatalf("trace SkillDiagnostics = %#v, want %#v", got, want)
	}
	if got, want := len(trace.Transcript), 3; got != want {
		t.Fatalf("trace transcript length = %d, want %d", got, want)
	}
	assistant := trace.Transcript[1]
	if assistant.Role != "assistant" || assistant.StopReason != string(ai.StopReasonError) || assistant.ErrorMessage != "provider error" {
		t.Fatalf("assistant trace = %#v", assistant)
	}
	if assistant.Usage == nil || assistant.Usage.InputTokens != 12 || assistant.Usage.OutputTokens != 7 {
		t.Fatalf("assistant usage = %#v", assistant.Usage)
	}
	if got, want := assistant.Content[2].ToolCall.Arguments, `{"path":`; got != want {
		t.Fatalf("tool arguments = %q, want %q", got, want)
	}
	if got := trace.Transcript[2].Content[0].ToolResult.Content; got != "synthetic-secret-deliberately-returned-by-tool" {
		t.Fatalf("tool result content = %q", got)
	}

	parameters[0] = '['
	arguments[0] = '['
	result.SkillDiagnostics[0].Message = "changed through source"
	if got, want := string(trace.Tools[0].Parameters), `{"type":"object"}`; got != want {
		t.Fatalf("trace parameters changed through source alias: %q", got)
	}
	if got, want := trace.Transcript[1].Content[2].ToolCall.Arguments, `{"path":`; got != want {
		t.Fatalf("trace arguments changed through source alias: %q", got)
	}
	if got, want := trace.SkillDiagnostics[0].Message, "required name is missing"; got != want {
		t.Fatalf("trace diagnostic changed through source alias: %q", got)
	}

	encoded, err := json.MarshalIndent(trace, "", "  ")
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}
	text := string(encoded)
	for _, fragment := range []string{
		`"role": "user"`,
		`"type": "thinking"`,
		`"type": "text"`,
		`"type": "tool_call"`,
		`"type": "tool_result"`,
		`"arguments": "{\"path\":"`,
		`"run_error": "run failed"`,
		`"skill_diagnostics"`,
		`"path": ".pia/skills/broken/SKILL.md"`,
		`"message": "required name is missing"`,
		"synthetic-secret-deliberately-returned-by-tool",
	} {
		if !strings.Contains(text, fragment) {
			t.Errorf("trace JSON does not contain %q\n%s", fragment, text)
		}
	}
	if strings.Contains(text, "api_key") || strings.Contains(text, "final_text") {
		t.Fatalf("trace contains forbidden duplicate or credential field\n%s", text)
	}
}

func TestBuildTraceRejectsNilTranscriptEntry(t *testing.T) {
	_, err := BuildTrace(RunResult{Transcript: []ai.Message{nil}}, nil)
	if err == nil || !strings.Contains(err.Error(), "message 0") {
		t.Fatalf("BuildTrace() error = %v, want indexed message error", err)
	}
}
