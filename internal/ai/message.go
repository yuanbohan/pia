package ai

import "encoding/json"

// Message is one ordered entry in the model transcript.
type Message interface {
	isMessage()
}

// UserMessage carries the user's text input.
type UserMessage struct {
	Content string
}

func (UserMessage) isMessage() {}

// AssistantMessage is the authoritative result of one Provider response.
type AssistantMessage struct {
	Content      []AssistantContent
	Usage        Usage
	StopReason   StopReason
	ErrorMessage string
}

func (AssistantMessage) isMessage() {}

// ToolResultMessage connects one tool result to the tool call that requested it.
type ToolResultMessage struct {
	ToolCallID string
	ToolName   string
	Content    string
	IsError    bool
}

func (ToolResultMessage) isMessage() {}

// AssistantContent is one ordered block produced by the model.
type AssistantContent interface {
	isAssistantContent()
}

// TextContent is model-visible answer text.
type TextContent struct {
	Text string
}

func (TextContent) isAssistantContent() {}

// ThinkingContent is reasoning content exposed by a Provider.
type ThinkingContent struct {
	Thinking string
}

func (ThinkingContent) isAssistantContent() {}

// ToolCall requests that the Agent invoke one named tool.
// Arguments remain raw JSON until the concrete tool decodes and validates them.
type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

func (ToolCall) isAssistantContent() {}

// Usage reports the token counts observed for one Provider response.
type Usage struct {
	InputTokens  int64
	OutputTokens int64
}

// TotalTokens derives the total so there is no duplicate value to keep in sync.
func (u Usage) TotalTokens() int64 {
	return u.InputTokens + u.OutputTokens
}

// StopReason explains why the Provider response ended.
type StopReason string

const (
	StopReasonStop    StopReason = "stop"
	StopReasonLength  StopReason = "length"
	StopReasonToolUse StopReason = "toolUse"
	StopReasonError   StopReason = "error"
	StopReasonAborted StopReason = "aborted"
)
