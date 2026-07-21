package openaicompatible

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yuanbohan/pia/internal/ai"
)

// Profile describes the small set of OpenAI-compatible protocol extensions
// that differ between concrete providers. It is not a provider registry or a
// general capability matrix.
type Profile struct {
	ReplayReasoning bool
	StreamUsage     bool
	Thinking        bool
}

type requestPayload struct {
	Model           string           `json:"model"`
	Messages        []requestMessage `json:"messages"`
	Tools           []requestTool    `json:"tools,omitempty"`
	Stream          bool             `json:"stream"`
	StreamOptions   *streamOptions   `json:"stream_options,omitempty"`
	Thinking        *thinkingOption  `json:"thinking,omitempty"`
	ReasoningEffort string           `json:"reasoning_effort,omitempty"`
	MaxTokens       int64            `json:"max_tokens,omitempty"`
}

type requestMessage struct {
	Role             string            `json:"role"`
	Content          *string           `json:"content"`
	ReasoningContent string            `json:"reasoning_content,omitempty"`
	ToolCalls        []requestToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string            `json:"tool_call_id,omitempty"`
}

type requestToolCall struct {
	ID       string              `json:"id"`
	Type     string              `json:"type"`
	Function requestToolFunction `json:"function"`
}

type requestToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type requestTool struct {
	Type     string                    `json:"type"`
	Function requestFunctionDefinition `json:"function"`
}

type requestFunctionDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type thinkingOption struct {
	Type string `json:"type"`
}

func buildRequestPayload(
	model string,
	request ai.Request,
	profile Profile,
	reasoningEffort string,
) (requestPayload, error) {
	systemPrompt := request.SystemPrompt
	messages := make([]requestMessage, 0, len(request.Messages)+1)
	messages = append(messages, requestMessage{
		Role:    "system",
		Content: &systemPrompt,
	})

	for index, message := range request.Messages {
		converted, err := convertMessage(message, profile)
		if err != nil {
			return requestPayload{}, fmt.Errorf("message %d: %w", index, err)
		}
		messages = append(messages, converted)
	}

	tools := make([]requestTool, 0, len(request.Tools))
	for _, tool := range request.Tools {
		if !json.Valid(tool.Parameters) {
			return requestPayload{}, fmt.Errorf("tool %q parameters are not valid JSON", tool.Name)
		}
		tools = append(tools, requestTool{
			Type: "function",
			Function: requestFunctionDefinition{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  append(json.RawMessage(nil), tool.Parameters...),
			},
		})
	}

	payload := requestPayload{
		Model:           model,
		Messages:        messages,
		Tools:           tools,
		Stream:          true,
		ReasoningEffort: reasoningEffort,
		MaxTokens:       request.MaxOutputTokens,
	}
	if profile.StreamUsage {
		payload.StreamOptions = &streamOptions{IncludeUsage: true}
	}
	if profile.Thinking {
		payload.Thinking = &thinkingOption{Type: "enabled"}
	}

	return payload, nil
}

func convertMessage(message ai.Message, profile Profile) (requestMessage, error) {
	switch typed := message.(type) {
	case ai.UserMessage:
		return userRequestMessage(typed), nil
	case *ai.UserMessage:
		if typed == nil {
			return requestMessage{}, fmt.Errorf("nil *ai.UserMessage")
		}
		return userRequestMessage(*typed), nil
	case ai.AssistantMessage:
		return assistantRequestMessage(typed, profile)
	case *ai.AssistantMessage:
		if typed == nil {
			return requestMessage{}, fmt.Errorf("nil *ai.AssistantMessage")
		}
		return assistantRequestMessage(*typed, profile)
	case ai.ToolResultMessage:
		return toolResultRequestMessage(typed), nil
	case *ai.ToolResultMessage:
		if typed == nil {
			return requestMessage{}, fmt.Errorf("nil *ai.ToolResultMessage")
		}
		return toolResultRequestMessage(*typed), nil
	case nil:
		return requestMessage{}, fmt.Errorf("nil message")
	default:
		return requestMessage{}, fmt.Errorf("unsupported message type %T", message)
	}
}

func userRequestMessage(message ai.UserMessage) requestMessage {
	content := message.Content
	return requestMessage{
		Role:    "user",
		Content: &content,
	}
}

func toolResultRequestMessage(message ai.ToolResultMessage) requestMessage {
	content := message.Content
	return requestMessage{
		Role:       "tool",
		Content:    &content,
		ToolCallID: message.ToolCallID,
	}
}

func assistantRequestMessage(message ai.AssistantMessage, profile Profile) (requestMessage, error) {
	var text strings.Builder
	var reasoning []string
	toolCalls := make([]requestToolCall, 0)
	hasText := false

	for index, content := range message.Content {
		switch typed := content.(type) {
		case ai.TextContent:
			hasText = true
			text.WriteString(typed.Text)
		case *ai.TextContent:
			if typed == nil {
				return requestMessage{}, fmt.Errorf("assistant content %d: nil *ai.TextContent", index)
			}
			hasText = true
			text.WriteString(typed.Text)
		case ai.ThinkingContent:
			if profile.ReplayReasoning {
				reasoning = append(reasoning, typed.Thinking)
			}
		case *ai.ThinkingContent:
			if typed == nil {
				return requestMessage{}, fmt.Errorf("assistant content %d: nil *ai.ThinkingContent", index)
			}
			if profile.ReplayReasoning {
				reasoning = append(reasoning, typed.Thinking)
			}
		case ai.ToolCall:
			toolCalls = append(toolCalls, convertToolCall(typed))
		case *ai.ToolCall:
			if typed == nil {
				return requestMessage{}, fmt.Errorf("assistant content %d: nil *ai.ToolCall", index)
			}
			toolCalls = append(toolCalls, convertToolCall(*typed))
		case nil:
			return requestMessage{}, fmt.Errorf("assistant content %d: nil content", index)
		default:
			return requestMessage{}, fmt.Errorf("assistant content %d: unsupported type %T", index, content)
		}
	}

	converted := requestMessage{
		Role:             "assistant",
		ReasoningContent: strings.Join(reasoning, "\n"),
		ToolCalls:        toolCalls,
	}
	if hasText {
		value := text.String()
		converted.Content = &value
	} else if len(message.Content) == 0 &&
		(message.StopReason == ai.StopReasonError || message.StopReason == ai.StopReasonAborted) {
		value := ""
		converted.Content = &value
	}

	return converted, nil
}

func convertToolCall(call ai.ToolCall) requestToolCall {
	return requestToolCall{
		ID:   call.ID,
		Type: "function",
		Function: requestToolFunction{
			Name:      call.Name,
			Arguments: string(call.Arguments),
		},
	}
}
