package ai_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/yuanbohan/pi-go/internal/ai"
)

func TestCloneRequestDeepCopiesNestedMutableData(t *testing.T) {
	t.Parallel()

	original := ai.Request{
		SystemPrompt: "system",
		Messages: []ai.Message{
			ai.UserMessage{Content: "inspect"},
			ai.AssistantMessage{
				Content: []ai.AssistantContent{
					ai.TextContent{Text: "reading"},
					ai.ToolCall{
						ID:        "call-1",
						Name:      "read",
						Arguments: json.RawMessage(`{"path":"main.go"}`),
					},
				},
				StopReason: ai.StopReasonToolUse,
			},
		},
		Tools: []ai.ToolSchema{{
			Name:       "read",
			Parameters: json.RawMessage(`{"type":"object"}`),
		}},
	}

	cloned := ai.CloneRequest(original)
	if !reflect.DeepEqual(cloned, original) {
		t.Fatalf("CloneRequest() = %#v, want %#v", cloned, original)
	}

	cloned.Messages[0] = ai.UserMessage{Content: "changed"}
	clonedAssistant := cloned.Messages[1].(ai.AssistantMessage)
	clonedAssistant.Content[0] = ai.TextContent{Text: "changed"}
	clonedCall := clonedAssistant.Content[1].(ai.ToolCall)
	clonedCall.Arguments[9] = 'X'
	cloned.Tools[0].Parameters[2] = 'X'

	if got, want := original.Messages[0], (ai.UserMessage{Content: "inspect"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("original user message = %#v, want %#v", got, want)
	}
	originalAssistant := original.Messages[1].(ai.AssistantMessage)
	if got, want := originalAssistant.Content[0], (ai.TextContent{Text: "reading"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("original text content = %#v, want %#v", got, want)
	}
	originalCall := originalAssistant.Content[1].(ai.ToolCall)
	if got, want := string(originalCall.Arguments), `{"path":"main.go"}`; got != want {
		t.Fatalf("original tool arguments = %q, want %q", got, want)
	}
	if got, want := string(original.Tools[0].Parameters), `{"type":"object"}`; got != want {
		t.Fatalf("original tool parameters = %q, want %q", got, want)
	}
}

func TestCloneEventDeepCopiesTerminalAndToolCallData(t *testing.T) {
	t.Parallel()

	arguments := json.RawMessage(`{"path":"main.go"}`)
	events := []ai.Event{
		ai.ToolCallEndEvent{
			ContentIndex: 0,
			ToolCall: ai.ToolCall{
				ID:        "call-1",
				Name:      "read",
				Arguments: arguments,
			},
		},
		ai.DoneEvent{Message: ai.AssistantMessage{
			Content: []ai.AssistantContent{ai.ToolCall{
				ID:        "call-1",
				Name:      "read",
				Arguments: arguments,
			}},
			StopReason: ai.StopReasonToolUse,
		}},
	}

	cloned, err := ai.CloneEvents(events)
	if err != nil {
		t.Fatalf("CloneEvents() error = %v", err)
	}
	if !reflect.DeepEqual(cloned, events) {
		t.Fatalf("CloneEvents() = %#v, want %#v", cloned, events)
	}

	end := cloned[0].(ai.ToolCallEndEvent)
	end.ToolCall.Arguments[9] = 'X'
	done := cloned[1].(ai.DoneEvent)
	doneCall := done.Message.Content[0].(ai.ToolCall)
	doneCall.Arguments[9] = 'Y'

	originalEnd := events[0].(ai.ToolCallEndEvent)
	if got, want := string(originalEnd.ToolCall.Arguments), `{"path":"main.go"}`; got != want {
		t.Fatalf("original end-event arguments = %q, want %q", got, want)
	}
	originalDone := events[1].(ai.DoneEvent)
	originalDoneCall := originalDone.Message.Content[0].(ai.ToolCall)
	if got, want := string(originalDoneCall.Arguments), `{"path":"main.go"}`; got != want {
		t.Fatalf("original terminal arguments = %q, want %q", got, want)
	}
}

func TestCloneEventsRejectsNilEvents(t *testing.T) {
	t.Parallel()

	if _, err := ai.CloneEvents([]ai.Event{nil}); err == nil {
		t.Fatal("CloneEvents() error = nil, want nil-event error")
	}
}
