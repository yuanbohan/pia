package agent

import (
	"fmt"
	"strings"

	"github.com/yuanbohan/pi-go/internal/ai"
)

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
