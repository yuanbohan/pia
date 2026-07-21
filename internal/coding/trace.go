package coding

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/yuanbohan/pia/internal/ai"
)

// Trace is an intentionally unstable, sensitive diagnostic snapshot created
// after one coding Run settles. It contains no dedicated credential field, but
// ordinary transcript content may still contain secrets emitted by a tool.
type Trace struct {
	Workspace        string            `json:"workspace"`
	SystemPrompt     string            `json:"system_prompt"`
	Model            TraceModel        `json:"model"`
	Tools            []TraceTool       `json:"tools"`
	SkillDiagnostics []SkillDiagnostic `json:"skill_diagnostics,omitempty"`
	Transcript       []TraceMessage    `json:"transcript"`
	RunError         string            `json:"run_error,omitempty"`
}

type TraceModel struct {
	Provider        string `json:"provider"`
	Name            string `json:"name"`
	Thinking        bool   `json:"thinking"`
	ReasoningEffort string `json:"reasoning_effort"`
}

type TraceTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type TraceMessage struct {
	Role         string         `json:"role"`
	Content      []TraceContent `json:"content"`
	Usage        *TraceUsage    `json:"usage,omitempty"`
	StopReason   string         `json:"stop_reason,omitempty"`
	ErrorMessage string         `json:"error_message,omitempty"`
}

type TraceContent struct {
	Type       string           `json:"type"`
	Text       string           `json:"text,omitempty"`
	Thinking   string           `json:"thinking,omitempty"`
	ToolCall   *TraceToolCall   `json:"tool_call,omitempty"`
	ToolResult *TraceToolResult `json:"tool_result,omitempty"`
}

type TraceToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type TraceToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	ToolName   string `json:"tool_name"`
	Content    string `json:"content"`
	IsError    bool   `json:"is_error"`
}

type TraceUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

// BuildTrace converts interface-backed protocol messages into explicitly
// tagged JSON data. Tool-call arguments remain strings so malformed arguments
// in an error transcript cannot invalidate the enclosing trace document.
func BuildTrace(result RunResult, runErr error) (Trace, error) {
	trace := Trace{
		Workspace:    result.WorkspacePath,
		SystemPrompt: result.SystemPrompt,
		Model: TraceModel{
			Provider:        result.Model.Provider,
			Name:            result.Model.Name,
			Thinking:        result.Model.Thinking,
			ReasoningEffort: result.Model.ReasoningEffort,
		},
		Tools: make([]TraceTool, len(result.Tools)),
		SkillDiagnostics: append(
			[]SkillDiagnostic(nil),
			result.SkillDiagnostics...,
		),
	}
	if runErr != nil {
		trace.RunError = runErr.Error()
	}
	for index, tool := range result.Tools {
		trace.Tools[index] = TraceTool{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  bytes.Clone(tool.Parameters),
		}
	}
	if result.Transcript != nil {
		trace.Transcript = make([]TraceMessage, len(result.Transcript))
	}
	for index, message := range result.Transcript {
		converted, err := traceMessage(message)
		if err != nil {
			return Trace{}, fmt.Errorf("coding: build trace message %d: %w", index, err)
		}
		trace.Transcript[index] = converted
	}
	return trace, nil
}

func traceMessage(message ai.Message) (TraceMessage, error) {
	switch message := message.(type) {
	case ai.UserMessage:
		return traceUserMessage(message), nil
	case *ai.UserMessage:
		if message != nil {
			return traceUserMessage(*message), nil
		}
	case ai.AssistantMessage:
		return traceAssistantMessage(message)
	case *ai.AssistantMessage:
		if message != nil {
			return traceAssistantMessage(*message)
		}
	case ai.ToolResultMessage:
		return traceToolResultMessage(message), nil
	case *ai.ToolResultMessage:
		if message != nil {
			return traceToolResultMessage(*message), nil
		}
	}
	return TraceMessage{}, fmt.Errorf("unsupported message %T", message)
}

func traceUserMessage(message ai.UserMessage) TraceMessage {
	return TraceMessage{
		Role: "user",
		Content: []TraceContent{{
			Type: "text",
			Text: message.Content,
		}},
	}
}

func traceAssistantMessage(message ai.AssistantMessage) (TraceMessage, error) {
	converted := TraceMessage{
		Role:         "assistant",
		Content:      make([]TraceContent, len(message.Content)),
		Usage:        &TraceUsage{InputTokens: message.Usage.InputTokens, OutputTokens: message.Usage.OutputTokens},
		StopReason:   string(message.StopReason),
		ErrorMessage: message.ErrorMessage,
	}
	for index, content := range message.Content {
		block, err := traceAssistantContent(content)
		if err != nil {
			return TraceMessage{}, fmt.Errorf("content %d: %w", index, err)
		}
		converted.Content[index] = block
	}
	return converted, nil
}

func traceAssistantContent(content ai.AssistantContent) (TraceContent, error) {
	switch content := content.(type) {
	case ai.TextContent:
		return TraceContent{Type: "text", Text: content.Text}, nil
	case *ai.TextContent:
		if content != nil {
			return TraceContent{Type: "text", Text: content.Text}, nil
		}
	case ai.ThinkingContent:
		return TraceContent{Type: "thinking", Thinking: content.Thinking}, nil
	case *ai.ThinkingContent:
		if content != nil {
			return TraceContent{Type: "thinking", Thinking: content.Thinking}, nil
		}
	case ai.ToolCall:
		return traceToolCall(content), nil
	case *ai.ToolCall:
		if content != nil {
			return traceToolCall(*content), nil
		}
	}
	return TraceContent{}, fmt.Errorf("unsupported assistant content %T", content)
}

func traceToolCall(call ai.ToolCall) TraceContent {
	return TraceContent{
		Type: "tool_call",
		ToolCall: &TraceToolCall{
			ID:        call.ID,
			Name:      call.Name,
			Arguments: string(call.Arguments),
		},
	}
}

func traceToolResultMessage(message ai.ToolResultMessage) TraceMessage {
	return TraceMessage{
		Role: "tool",
		Content: []TraceContent{{
			Type: "tool_result",
			ToolResult: &TraceToolResult{
				ToolCallID: message.ToolCallID,
				ToolName:   message.ToolName,
				Content:    message.Content,
				IsError:    message.IsError,
			},
		}},
	}
}
