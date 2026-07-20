package ai

import (
	"encoding/json"
	"fmt"
	"unicode/utf8"
)

const estimatedCharactersPerToken int64 = 4

// ContextUsageEstimate combines Provider-reported usage for a known prefix
// with a deterministic approximation for messages appended after that prefix.
type ContextUsageEstimate struct {
	Tokens         int64
	UsageTokens    int64
	TrailingTokens int64
	LastUsageIndex int
}

// RequestLimits are model facts supplied by the Provider consumer. They do
// not select a model or own any compaction policy.
type RequestLimits struct {
	ContextCapacity int64
	ModelMaxOutput  int64
	ContextSafety   int64
}

// IsZero reports whether request output limiting is disabled.
func (limits RequestLimits) IsZero() bool {
	return limits == (RequestLimits{})
}

// Validate checks a configured set of request limits. The all-zero value is a
// deliberate disabled value for Agent consumers that do not set model facts.
func (limits RequestLimits) Validate() error {
	if limits.IsZero() {
		return nil
	}
	if limits.ContextCapacity <= 0 {
		return fmt.Errorf("context capacity must be positive")
	}
	if limits.ModelMaxOutput <= 0 {
		return fmt.Errorf("model max output must be positive")
	}
	if limits.ContextSafety < 0 {
		return fmt.Errorf("context safety must not be negative")
	}
	return nil
}

// ClampOutputTokens applies the requested cap, the model output cap, and the
// remaining context capacity. A Provider request always receives room for at
// least one output token, matching the frozen Pi clamp.
func (limits RequestLimits) ClampOutputTokens(projectedInput, requested int64) int64 {
	if limits.IsZero() {
		return 0
	}
	limit := requested
	if limit <= 0 || limit > limits.ModelMaxOutput {
		limit = limits.ModelMaxOutput
	}
	available := limits.ContextCapacity - projectedInput - limits.ContextSafety
	if available < 1 {
		available = 1
	}
	if available < limit {
		limit = available
	}
	return limit
}

// EstimateRequestTokens estimates the complete model-visible input. A valid
// terminal assistant usage is preferred because it already includes the
// stable system prompt, tools, and the dialogue prefix sent for that turn.
func EstimateRequestTokens(request Request) ContextUsageEstimate {
	lastUsageIndex := -1
	var usageTokens int64
	for index, message := range request.Messages {
		assistant, ok := asAssistantMessage(message)
		if !ok || !hasValidUsage(assistant) {
			continue
		}
		lastUsageIndex = index
		usageTokens = assistant.Usage.TotalTokens()
	}

	if lastUsageIndex >= 0 {
		var trailingTokens int64
		for _, message := range request.Messages[lastUsageIndex+1:] {
			trailingTokens += EstimateMessageTokens(message)
		}
		return ContextUsageEstimate{
			Tokens:         usageTokens + trailingTokens,
			UsageTokens:    usageTokens,
			TrailingTokens: trailingTokens,
			LastUsageIndex: lastUsageIndex,
		}
	}

	tokens := EstimateTextTokens(request.SystemPrompt)
	if len(request.Tools) > 0 {
		if encoded, err := json.Marshal(request.Tools); err == nil {
			tokens += EstimateTextTokens(string(encoded))
		}
	}
	for _, message := range request.Messages {
		tokens += EstimateMessageTokens(message)
	}
	return ContextUsageEstimate{
		Tokens:         tokens,
		TrailingTokens: tokens,
		LastUsageIndex: -1,
	}
}

// EstimateTextTokens applies the frozen Pi characters-per-token heuristic.
func EstimateTextTokens(text string) int64 {
	characters := int64(utf8.RuneCountInString(text))
	return divideRoundUp(characters, estimatedCharactersPerToken)
}

// EstimateMessageTokens estimates one model-visible protocol message.
func EstimateMessageTokens(message Message) int64 {
	switch message := message.(type) {
	case UserMessage:
		return EstimateTextTokens(message.Content)
	case *UserMessage:
		if message != nil {
			return EstimateTextTokens(message.Content)
		}
	case AssistantMessage:
		return estimateAssistantTokens(message)
	case *AssistantMessage:
		if message != nil {
			return estimateAssistantTokens(*message)
		}
	case ToolResultMessage:
		return EstimateTextTokens(message.Content)
	case *ToolResultMessage:
		if message != nil {
			return EstimateTextTokens(message.Content)
		}
	}
	return 0
}

func estimateAssistantTokens(message AssistantMessage) int64 {
	var characters int64
	for _, content := range message.Content {
		switch content := content.(type) {
		case TextContent:
			characters += int64(utf8.RuneCountInString(content.Text))
		case *TextContent:
			if content != nil {
				characters += int64(utf8.RuneCountInString(content.Text))
			}
		case ThinkingContent:
			characters += int64(utf8.RuneCountInString(content.Thinking))
		case *ThinkingContent:
			if content != nil {
				characters += int64(utf8.RuneCountInString(content.Thinking))
			}
		case ToolCall:
			characters += toolCallCharacters(content)
		case *ToolCall:
			if content != nil {
				characters += toolCallCharacters(*content)
			}
		}
	}
	return divideRoundUp(characters, estimatedCharactersPerToken)
}

func toolCallCharacters(call ToolCall) int64 {
	return int64(utf8.RuneCountInString(call.Name) + utf8.RuneCount(call.Arguments))
}

func asAssistantMessage(message Message) (AssistantMessage, bool) {
	switch message := message.(type) {
	case AssistantMessage:
		return message, true
	case *AssistantMessage:
		if message != nil {
			return *message, true
		}
	}
	return AssistantMessage{}, false
}

func hasValidUsage(message AssistantMessage) bool {
	return message.StopReason != StopReasonError &&
		message.StopReason != StopReasonAborted &&
		message.Usage.TotalTokens() > 0
}

func divideRoundUp(value, divisor int64) int64 {
	if value == 0 {
		return 0
	}
	return (value + divisor - 1) / divisor
}
