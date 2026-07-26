package agent

import (
	"errors"
	"fmt"
	"strings"

	"github.com/yuanbohan/pia/internal/ai"
)

func validateContinuationContext(messages []ai.Message) error {
	if len(messages) == 0 {
		return errors.New("agent: cannot continue with an empty Working Context")
	}

	switch messages[len(messages)-1].(type) {
	case ai.UserMessage:
	case ai.ToolResultMessage:
	case ai.AssistantMessage:
		return errors.New("agent: cannot continue from an assistant message")
	default:
		return fmt.Errorf(
			"agent: cannot continue from unsupported Working Context tail %T",
			messages[len(messages)-1],
		)
	}
	if err := validateContinuationToolProtocol(messages); err != nil {
		return fmt.Errorf("agent: cannot continue from unpaired tool calls or results: %w", err)
	}
	return nil
}

func validateContinuationToolProtocol(messages []ai.Message) error {
	pending := make(map[string]string)
	for index, message := range messages {
		switch message := message.(type) {
		case ai.UserMessage:
			if len(pending) > 0 {
				return fmt.Errorf("user message %d appears before pending tool results", index)
			}
		case ai.AssistantMessage:
			if len(pending) > 0 {
				return fmt.Errorf("assistant message %d appears before pending tool results", index)
			}
			if err := validateToolCallProtocol(message); err != nil {
				return fmt.Errorf("assistant message %d: %w", index, err)
			}
			for _, call := range toolCalls(message) {
				pending[call.ID] = call.Name
			}
		case ai.ToolResultMessage:
			name, ok := pending[message.ToolCallID]
			if !ok {
				return fmt.Errorf("tool result %d has no pending call %q", index, message.ToolCallID)
			}
			if name != message.ToolName {
				return fmt.Errorf(
					"tool result %d name %q does not match pending call %q",
					index,
					message.ToolName,
					name,
				)
			}
			delete(pending, message.ToolCallID)
		case nil:
			return fmt.Errorf("working context message %d is nil", index)
		}
	}
	if len(pending) > 0 {
		return errors.New("working context ends before all tool calls have results")
	}
	return nil
}

func validateToolCallProtocol(message ai.AssistantMessage) error {
	seen := make(map[string]struct{})
	count := 0
	for contentIndex, content := range message.Content {
		call, ok := assistantToolCall(content)
		if !ok {
			continue
		}
		count++
		if strings.TrimSpace(call.ID) == "" {
			return fmt.Errorf("tool call at content index %d has an empty ID", contentIndex)
		}
		if _, duplicate := seen[call.ID]; duplicate {
			return fmt.Errorf("tool calls contain duplicate ID %q", call.ID)
		}
		seen[call.ID] = struct{}{}
	}
	if message.StopReason == ai.StopReasonToolUse && count == 0 {
		return fmt.Errorf("toolUse stop reason without a tool call")
	}
	return nil
}

func toolCalls(message ai.AssistantMessage) []ai.ToolCall {
	var calls []ai.ToolCall
	for _, content := range message.Content {
		call, ok := assistantToolCall(content)
		if ok {
			calls = append(calls, ai.CloneToolCall(call))
		}
	}
	return calls
}

func assistantToolCall(content ai.AssistantContent) (ai.ToolCall, bool) {
	switch content := content.(type) {
	case ai.ToolCall:
		return content, true
	case *ai.ToolCall:
		if content != nil {
			return *content, true
		}
	}
	return ai.ToolCall{}, false
}
