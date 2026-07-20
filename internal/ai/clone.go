package ai

import (
	"bytes"
	"fmt"
	"reflect"
)

// CloneRequest returns an ownership-independent copy of a Provider request.
func CloneRequest(request Request) Request {
	cloned := Request{
		SystemPrompt:    request.SystemPrompt,
		Messages:        CloneMessages(request.Messages),
		MaxOutputTokens: request.MaxOutputTokens,
	}
	cloned.Tools = CloneToolSchemas(request.Tools)
	return cloned
}

// CloneToolSchemas returns ownership-independent model tool definitions.
func CloneToolSchemas(tools []ToolSchema) []ToolSchema {
	if tools == nil {
		return nil
	}
	cloned := make([]ToolSchema, len(tools))
	for index, tool := range tools {
		cloned[index] = tool
		cloned[index].Parameters = bytes.Clone(tool.Parameters)
	}
	return cloned
}

// CloneMessages returns an ownership-independent copy of ordered messages.
func CloneMessages(messages []Message) []Message {
	if messages == nil {
		return nil
	}
	cloned := make([]Message, len(messages))
	for index, message := range messages {
		cloned[index] = CloneMessage(message)
	}
	return cloned
}

// CloneMessage returns an ownership-independent copy of one message.
func CloneMessage(message Message) Message {
	switch message := message.(type) {
	case nil:
		return nil
	case UserMessage:
		return message
	case *UserMessage:
		if message == nil {
			return nil
		}
		return *message
	case AssistantMessage:
		return CloneAssistantMessage(message)
	case *AssistantMessage:
		if message == nil {
			return nil
		}
		return CloneAssistantMessage(*message)
	case ToolResultMessage:
		return message
	case *ToolResultMessage:
		if message == nil {
			return nil
		}
		return *message
	default:
		panic(fmt.Sprintf("ai: unsupported message type %T", message))
	}
}

// CloneAssistantMessage returns an ownership-independent terminal response.
func CloneAssistantMessage(message AssistantMessage) AssistantMessage {
	cloned := message
	if message.Content != nil {
		cloned.Content = make([]AssistantContent, len(message.Content))
		for index, content := range message.Content {
			cloned.Content[index] = cloneAssistantContent(content)
		}
	}
	return cloned
}

// CloneToolCall returns an ownership-independent tool request.
func CloneToolCall(toolCall ToolCall) ToolCall {
	toolCall.Arguments = bytes.Clone(toolCall.Arguments)
	return toolCall
}

func cloneAssistantContent(content AssistantContent) AssistantContent {
	switch content := content.(type) {
	case nil:
		return nil
	case TextContent:
		return content
	case *TextContent:
		if content == nil {
			return nil
		}
		return *content
	case ThinkingContent:
		return content
	case *ThinkingContent:
		if content == nil {
			return nil
		}
		return *content
	case ToolCall:
		return CloneToolCall(content)
	case *ToolCall:
		if content == nil {
			return nil
		}
		return CloneToolCall(*content)
	default:
		panic(fmt.Sprintf("ai: unsupported assistant content type %T", content))
	}
}

// CloneEvents returns ownership-independent copies of Provider stream events.
func CloneEvents(events []Event) ([]Event, error) {
	if events == nil {
		return nil, nil
	}
	cloned := make([]Event, len(events))
	for index, event := range events {
		copy, err := CloneEvent(event)
		if err != nil {
			return nil, fmt.Errorf("event %d: %w", index, err)
		}
		cloned[index] = copy
	}
	return cloned, nil
}

// CloneEvent returns an ownership-independent copy of one Provider event.
func CloneEvent(event Event) (Event, error) {
	if event == nil {
		return nil, fmt.Errorf("event is nil")
	}
	value := reflect.ValueOf(event)
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil, fmt.Errorf("event %T is nil", event)
		}
		event = value.Elem().Interface().(Event)
	}

	switch event := event.(type) {
	case StartEvent,
		TextStartEvent,
		TextDeltaEvent,
		TextEndEvent,
		ThinkingStartEvent,
		ThinkingDeltaEvent,
		ThinkingEndEvent,
		ToolCallStartEvent,
		ToolCallDeltaEvent:
		return event, nil
	case ToolCallEndEvent:
		event.ToolCall = CloneToolCall(event.ToolCall)
		return event, nil
	case DoneEvent:
		event.Message = CloneAssistantMessage(event.Message)
		return event, nil
	case ErrorEvent:
		event.Message = CloneAssistantMessage(event.Message)
		return event, nil
	default:
		return nil, fmt.Errorf("unsupported event %T", event)
	}
}
