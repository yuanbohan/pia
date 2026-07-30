package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/yuanbohan/pia/internal/agent"
	"github.com/yuanbohan/pia/internal/ai"
)

func TestContinueUsesExistingUserTailWithoutAppendingInput(t *testing.T) {
	t.Parallel()

	initial := []ai.Message{
		ai.UserMessage{Content: "finish the accepted task"},
	}
	final := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "done"}},
		StopReason: ai.StopReasonStop,
	}
	provider := newFaux(t, assistantStep(final))
	runtime := newAgent(t, provider, "system")

	result, err := runtime.Continue(context.Background(), initial, emptySteeringSource{})
	if err != nil {
		t.Fatalf("Continue() error = %v", err)
	}
	if got, want := result.NewMessages, []ai.Message{final}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Continue() NewMessages = %#v, want %#v", got, want)
	}

	requests := provider.Requests()
	if got, want := len(requests), 1; got != want {
		t.Fatalf("Provider requests = %d, want %d", got, want)
	}
	if !reflect.DeepEqual(requests[0].Messages, initial) {
		t.Fatalf("Provider messages = %#v, want existing context %#v", requests[0].Messages, initial)
	}
}

func TestContinueAcceptsPairedToolResultTail(t *testing.T) {
	t.Parallel()

	call := ai.ToolCall{
		ID:        "call-1",
		Name:      "read",
		Arguments: json.RawMessage(`{"path":"main.go"}`),
	}
	initial := []ai.Message{
		ai.UserMessage{Content: "inspect the file"},
		ai.AssistantMessage{
			Content:    []ai.AssistantContent{call},
			StopReason: ai.StopReasonToolUse,
		},
		ai.ToolResultMessage{
			ToolCallID: call.ID,
			ToolName:   call.Name,
			Content:    "package main",
		},
	}
	final := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "inspection complete"}},
		StopReason: ai.StopReasonStop,
	}
	provider := newFaux(t, assistantStep(final))
	runtime := newAgent(t, provider, "system")

	result, err := runtime.Continue(context.Background(), initial, emptySteeringSource{})
	if err != nil {
		t.Fatalf("Continue() error = %v", err)
	}
	if got, want := result.NewMessages, []ai.Message{final}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Continue() NewMessages = %#v, want %#v", got, want)
	}
	if got := provider.Requests()[0].Messages; !reflect.DeepEqual(got, initial) {
		t.Fatalf("Provider messages = %#v, want paired context %#v", got, initial)
	}
}

func TestContinueReusesToolLoopAndReturnsOnlyNewMessages(t *testing.T) {
	t.Parallel()

	toolCall := ai.ToolCall{
		ID:        "call-1",
		Name:      "read",
		Arguments: json.RawMessage(`{"path":"main.go"}`),
	}
	callMessage := ai.AssistantMessage{
		Content:    []ai.AssistantContent{toolCall},
		StopReason: ai.StopReasonToolUse,
	}
	final := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "done"}},
		StopReason: ai.StopReasonStop,
	}
	provider := newFaux(t, assistantStep(callMessage), assistantStep(final))
	runtime := newAgentWithTools(t, provider, "system", &testTool{
		definition: agent.ToolDefinition{Schema: toolSchema("read"), CanRunParallel: true},
		execute: func(context.Context, json.RawMessage) (string, error) {
			return "package main", nil
		},
	})
	initial := []ai.Message{ai.UserMessage{Content: "inspect then finish"}}

	result, err := runtime.Continue(context.Background(), initial, emptySteeringSource{})
	if err != nil {
		t.Fatalf("Continue() error = %v", err)
	}
	want := []ai.Message{
		callMessage,
		ai.ToolResultMessage{
			ToolCallID: "call-1",
			ToolName:   "read",
			Content:    "package main",
		},
		final,
	}
	if !reflect.DeepEqual(result.NewMessages, want) {
		t.Fatalf("Continue() NewMessages = %#v, want %#v", result.NewMessages, want)
	}

	requests := provider.Requests()
	if got, want := len(requests), 2; got != want {
		t.Fatalf("Provider requests = %d, want %d", got, want)
	}
	wantSecond := append(ai.CloneMessages(initial), want[:2]...)
	if !reflect.DeepEqual(requests[1].Messages, wantSecond) {
		t.Fatalf("second Provider messages = %#v, want %#v", requests[1].Messages, wantSecond)
	}
}

func TestContinueReusesProviderFailureToolCallSettlement(t *testing.T) {
	t.Parallel()

	var executions int
	call := ai.ToolCall{
		ID:        "call-1",
		Name:      "read",
		Arguments: json.RawMessage(`{"path":"main.go"}`),
	}
	failed := ai.AssistantMessage{
		Content:      []ai.AssistantContent{call},
		StopReason:   ai.StopReasonError,
		ErrorMessage: "upstream disconnected",
	}
	runtime := newAgentWithTools(t, staticProvider{stream: &sliceStream{events: []ai.Event{
		ai.ErrorEvent{Message: failed},
	}}}, "system", &testTool{
		definition: agent.ToolDefinition{Schema: toolSchema("read"), CanRunParallel: true},
		execute: func(context.Context, json.RawMessage) (string, error) {
			executions++
			return "must not execute", nil
		},
	})
	initial := []ai.Message{ai.UserMessage{Content: "finish from this accepted input"}}

	result, err := runtime.Continue(context.Background(), initial, emptySteeringSource{})
	if err == nil || !strings.Contains(err.Error(), failed.ErrorMessage) {
		t.Fatalf("Continue() error = %v, want Provider failure", err)
	}
	if executions != 0 {
		t.Fatalf("tool executions = %d, want 0", executions)
	}
	if got, want := len(result.NewMessages), 2; got != want {
		t.Fatalf("Continue() NewMessages length = %d, want error assistant and settlement", got)
	}
	if !reflect.DeepEqual(result.NewMessages[0], ai.Message(failed)) {
		t.Fatalf("error assistant = %#v, want %#v", result.NewMessages[0], failed)
	}
	settlement, ok := result.NewMessages[1].(ai.ToolResultMessage)
	if !ok || settlement.ToolCallID != call.ID || !settlement.IsError ||
		!strings.Contains(settlement.Content, "not executed") {
		t.Fatalf("tool settlement = %#v, want same-ID not-executed result", result.NewMessages[1])
	}
}

func TestContinueRejectsInvalidContextWithoutCallingProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		messages []ai.Message
		want     string
	}{
		{
			name: "empty context",
			want: "empty",
		},
		{
			name: "assistant tail",
			messages: []ai.Message{
				ai.UserMessage{Content: "question"},
				ai.AssistantMessage{StopReason: ai.StopReasonStop},
			},
			want: "assistant",
		},
		{
			name: "unpaired tool result",
			messages: []ai.Message{
				ai.ToolResultMessage{ToolCallID: "missing", ToolName: "read", Content: "result"},
			},
			want: "paired",
		},
		{
			name: "incomplete tool result group",
			messages: []ai.Message{
				ai.UserMessage{Content: "inspect"},
				ai.AssistantMessage{
					Content: []ai.AssistantContent{
						ai.ToolCall{ID: "call-1", Name: "read", Arguments: json.RawMessage(`{}`)},
						ai.ToolCall{ID: "call-2", Name: "read", Arguments: json.RawMessage(`{}`)},
					},
					StopReason: ai.StopReasonToolUse,
				},
				ai.ToolResultMessage{ToolCallID: "call-1", ToolName: "read", Content: "first"},
			},
			want: "paired",
		},
		{
			name: "earlier orphaned tool call before user tail",
			messages: []ai.Message{
				ai.UserMessage{Content: "inspect"},
				ai.AssistantMessage{
					Content: []ai.AssistantContent{
						ai.ToolCall{ID: "call-1", Name: "read", Arguments: json.RawMessage(`{}`)},
					},
					StopReason: ai.StopReasonToolUse,
				},
				ai.UserMessage{Content: "continue anyway"},
			},
			want: "paired",
		},
		{
			name: "nil message before user tail",
			messages: []ai.Message{
				nil,
				ai.UserMessage{Content: "continue"},
			},
			want: "nil",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			provider := newFaux(t, assistantStep(ai.AssistantMessage{StopReason: ai.StopReasonStop}))
			runtime := newAgent(t, provider, "system")

			result, err := runtime.Continue(context.Background(), test.messages, emptySteeringSource{})
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), test.want) {
				t.Fatalf("Continue() error = %v, want fragment %q", err, test.want)
			}
			if len(result.NewMessages) != 0 {
				t.Fatalf("Continue() NewMessages = %#v, want empty result", result.NewMessages)
			}
			if got := len(provider.Requests()); got != 0 {
				t.Fatalf("Provider requests = %d, want 0", got)
			}
		})
	}
}

func TestPreCanceledContinueDoesNotMutateWorkingContext(t *testing.T) {
	t.Parallel()

	final := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "done"}},
		StopReason: ai.StopReasonStop,
	}
	provider := newFaux(t, assistantStep(final))
	runtime := newAgent(t, provider, "system")
	initial := []ai.Message{ai.UserMessage{Content: "finish this"}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := runtime.Continue(ctx, initial, emptySteeringSource{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Continue() error = %v, want context.Canceled", err)
	}
	if len(result.NewMessages) != 0 {
		t.Fatalf("Continue() NewMessages = %#v, want empty result", result.NewMessages)
	}
	if got := len(provider.Requests()); got != 0 {
		t.Fatalf("Provider requests after canceled Continue = %d, want 0", got)
	}

	result, err = runtime.Continue(context.Background(), initial, emptySteeringSource{})
	if err != nil {
		t.Fatalf("second Continue() error = %v", err)
	}
	if got, want := result.NewMessages, []ai.Message{final}; !reflect.DeepEqual(got, want) {
		t.Fatalf("second Continue() NewMessages = %#v, want %#v", got, want)
	}
	if got := provider.Requests()[0].Messages; !reflect.DeepEqual(got, initial) {
		t.Fatalf("Provider messages = %#v, want unchanged context %#v", got, initial)
	}
}
