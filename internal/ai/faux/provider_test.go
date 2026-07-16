package faux_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/yuanbohan/pi-go/internal/ai"
	"github.com/yuanbohan/pi-go/internal/ai/faux"
)

func TestUsageTotalTokens(t *testing.T) {
	t.Parallel()

	usage := ai.Usage{InputTokens: 13, OutputTokens: 8}
	if got, want := usage.TotalTokens(), int64(21); got != want {
		t.Fatalf("TotalTokens() = %d, want %d", got, want)
	}
}

func TestProviderStreamsScriptedEventsAndThenEOFForever(t *testing.T) {
	t.Parallel()

	message := ai.AssistantMessage{
		Content: []ai.AssistantContent{
			ai.TextContent{Text: "hello"},
		},
		Usage:      ai.Usage{InputTokens: 3, OutputTokens: 1},
		StopReason: ai.StopReasonStop,
	}
	events := []ai.Event{
		ai.StartEvent{},
		ai.TextStartEvent{ContentIndex: 0},
		ai.TextDeltaEvent{ContentIndex: 0, Delta: "hel"},
		ai.TextDeltaEvent{ContentIndex: 0, Delta: "lo"},
		ai.TextEndEvent{ContentIndex: 0, Text: "hello"},
		ai.DoneEvent{Message: message},
	}
	provider, err := faux.New(faux.Step{Events: events})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	stream := provider.Stream(context.Background(), ai.Request{})
	for index, want := range events {
		got, receiveErr := stream.Receive()
		if receiveErr != nil {
			t.Fatalf("Receive() event %d error = %v", index, receiveErr)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Receive() event %d = %#v, want %#v", index, got, want)
		}
	}
	for attempt := 0; attempt < 2; attempt++ {
		got, receiveErr := stream.Receive()
		if got != nil || !errors.Is(receiveErr, io.EOF) {
			t.Fatalf("Receive() after terminal = (%#v, %v), want (nil, io.EOF)", got, receiveErr)
		}
	}
}

func TestProviderStreamsThinkingAndFragmentedToolCallInContentOrder(t *testing.T) {
	t.Parallel()

	arguments := json.RawMessage(`{"path":"main.go"}`)
	message := ai.AssistantMessage{
		Content: []ai.AssistantContent{
			ai.ThinkingContent{Thinking: "inspect first"},
			ai.ToolCall{ID: "call-1", Name: "read", Arguments: arguments},
		},
		StopReason: ai.StopReasonToolUse,
	}
	events := []ai.Event{
		ai.StartEvent{},
		ai.ThinkingStartEvent{ContentIndex: 0},
		ai.ThinkingDeltaEvent{ContentIndex: 0, Delta: "inspect "},
		ai.ThinkingDeltaEvent{ContentIndex: 0, Delta: "first"},
		ai.ThinkingEndEvent{ContentIndex: 0, Thinking: "inspect first"},
		ai.ToolCallStartEvent{ContentIndex: 1, ID: "call-1", Name: "read"},
		ai.ToolCallDeltaEvent{ContentIndex: 1, Delta: `{"path":`},
		ai.ToolCallDeltaEvent{ContentIndex: 1, Delta: `"main.go"}`},
		ai.ToolCallEndEvent{
			ContentIndex: 1,
			ToolCall:     ai.ToolCall{ID: "call-1", Name: "read", Arguments: arguments},
		},
		ai.DoneEvent{Message: message},
	}
	provider, err := faux.New(faux.Step{Events: events})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	stream := provider.Stream(context.Background(), ai.Request{})
	for index, want := range events {
		got, receiveErr := stream.Receive()
		if receiveErr != nil {
			t.Fatalf("Receive() event %d error = %v", index, receiveErr)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Receive() event %d = %#v, want %#v", index, got, want)
		}
	}
}

func TestNewRejectsInvalidSteps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		step faux.Step
		want string
	}{
		{
			name: "missing terminal",
			step: faux.Step{Events: []ai.Event{ai.StartEvent{}}},
			want: "terminal",
		},
		{
			name: "event after terminal",
			step: faux.Step{Events: []ai.Event{
				ai.StartEvent{},
				ai.DoneEvent{Message: ai.AssistantMessage{StopReason: ai.StopReasonStop}},
				ai.StartEvent{},
			}},
			want: "after terminal",
		},
		{
			name: "delta before block start",
			step: faux.Step{Events: []ai.Event{
				ai.StartEvent{},
				ai.TextDeltaEvent{ContentIndex: 0, Delta: "hello"},
				ai.ErrorEvent{Message: ai.AssistantMessage{
					StopReason:   ai.StopReasonError,
					ErrorMessage: "broken",
				}},
			}},
			want: "text delta",
		},
		{
			name: "done with error reason",
			step: faux.Step{Events: []ai.Event{
				ai.StartEvent{},
				ai.DoneEvent{Message: ai.AssistantMessage{StopReason: ai.StopReasonError}},
			}},
			want: "done event",
		},
		{
			name: "content index does not map to final content",
			step: faux.Step{Events: []ai.Event{
				ai.StartEvent{},
				ai.TextStartEvent{ContentIndex: 1},
				ai.TextDeltaEvent{ContentIndex: 1, Delta: "hello"},
				ai.TextEndEvent{ContentIndex: 1, Text: "hello"},
				ai.DoneEvent{Message: ai.AssistantMessage{
					Content:    []ai.AssistantContent{ai.TextContent{Text: "hello"}},
					StopReason: ai.StopReasonStop,
				}},
			}},
			want: "content index",
		},
		{
			name: "terminal content does not match deltas",
			step: faux.Step{Events: []ai.Event{
				ai.StartEvent{},
				ai.TextStartEvent{ContentIndex: 0},
				ai.TextDeltaEvent{ContentIndex: 0, Delta: "streamed"},
				ai.TextEndEvent{ContentIndex: 0, Text: "streamed"},
				ai.DoneEvent{Message: ai.AssistantMessage{
					Content:    []ai.AssistantContent{ai.TextContent{Text: "different"}},
					StopReason: ai.StopReasonStop,
				}},
			}},
			want: "does not match streamed content",
		},
		{
			name: "error with normal reason",
			step: faux.Step{Events: []ai.Event{
				ai.ErrorEvent{Message: ai.AssistantMessage{StopReason: ai.StopReasonStop}},
			}},
			want: "error event",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := faux.New(test.step)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("New() error = %v, want error containing %q", err, test.want)
			}
		})
	}
}

func TestProviderConsumesStepsAndRecordsRequestSnapshots(t *testing.T) {
	t.Parallel()

	first := faux.Step{Events: []ai.Event{
		ai.StartEvent{},
		ai.DoneEvent{Message: ai.AssistantMessage{StopReason: ai.StopReasonStop}},
	}}
	second := faux.Step{Events: []ai.Event{
		ai.ErrorEvent{Message: ai.AssistantMessage{
			StopReason:   ai.StopReasonError,
			ErrorMessage: "provider failed",
		}},
	}}
	provider, err := faux.New(first, second)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	request := ai.Request{
		SystemPrompt: "You are a coding agent. Current working directory: /work",
		Messages: []ai.Message{
			ai.UserMessage{Content: "fix the bug"},
		},
		Tools: []ai.ToolSchema{{
			Name:        "read",
			Description: "Read a file",
			Parameters:  json.RawMessage(`{"type":"object"}`),
		}},
	}
	firstStream := provider.Stream(context.Background(), request)
	secondStream := provider.Stream(context.Background(), request)

	firstTerminal := receiveTerminal(t, firstStream)
	if _, ok := firstTerminal.(ai.DoneEvent); !ok {
		t.Fatalf("first terminal = %T, want ai.DoneEvent", firstTerminal)
	}
	secondTerminal := receiveTerminal(t, secondStream)
	if _, ok := secondTerminal.(ai.ErrorEvent); !ok {
		t.Fatalf("second terminal = %T, want ai.ErrorEvent", secondTerminal)
	}

	request.SystemPrompt = "changed"
	request.Messages[0] = ai.UserMessage{Content: "changed"}
	request.Tools[0].Parameters[0] = '['
	recorded := provider.Requests()
	if got, want := len(recorded), 2; got != want {
		t.Fatalf("len(Requests()) = %d, want %d", got, want)
	}
	if got, want := recorded[0].SystemPrompt, "You are a coding agent. Current working directory: /work"; got != want {
		t.Fatalf("recorded SystemPrompt = %q, want %q", got, want)
	}
	if got, want := recorded[0].Messages[0], (ai.UserMessage{Content: "fix the bug"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("recorded message = %#v, want %#v", got, want)
	}
	if got, want := string(recorded[0].Tools[0].Parameters), `{"type":"object"}`; got != want {
		t.Fatalf("recorded parameters = %q, want %q", got, want)
	}

	recorded[0].Tools[0].Parameters[0] = '['
	if got, want := string(provider.Requests()[0].Tools[0].Parameters), `{"type":"object"}`; got != want {
		t.Fatalf("Requests() returned aliased parameters %q, want %q", got, want)
	}
}

func TestProviderReturnsProtocolErrorWhenStepsAreExhausted(t *testing.T) {
	t.Parallel()

	provider, err := faux.New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	stream := provider.Stream(context.Background(), ai.Request{})

	event, receiveErr := stream.Receive()
	if receiveErr != nil {
		t.Fatalf("Receive() error = %v", receiveErr)
	}
	errorEvent, ok := event.(ai.ErrorEvent)
	if !ok {
		t.Fatalf("Receive() event = %T, want ai.ErrorEvent", event)
	}
	if got, want := errorEvent.Message.StopReason, ai.StopReasonError; got != want {
		t.Fatalf("StopReason = %q, want %q", got, want)
	}
	if got, want := errorEvent.Message.ErrorMessage, "faux response queue exhausted"; got != want {
		t.Fatalf("ErrorMessage = %q, want %q", got, want)
	}
	if event, receiveErr = stream.Receive(); event != nil || !errors.Is(receiveErr, io.EOF) {
		t.Fatalf("Receive() after error = (%#v, %v), want (nil, io.EOF)", event, receiveErr)
	}
}

func TestCancelBeforeFirstReceiveReturnsAbortedThenEOF(t *testing.T) {
	t.Parallel()

	provider, err := faux.New(textStep("unread"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stream := provider.Stream(ctx, ai.Request{})

	event, receiveErr := stream.Receive()
	if receiveErr != nil {
		t.Fatalf("Receive() error = %v", receiveErr)
	}
	errorEvent, ok := event.(ai.ErrorEvent)
	if !ok {
		t.Fatalf("Receive() event = %T, want ai.ErrorEvent", event)
	}
	if got, want := errorEvent.Message.StopReason, ai.StopReasonAborted; got != want {
		t.Fatalf("StopReason = %q, want %q", got, want)
	}
	if got, want := errorEvent.Message.ErrorMessage, "request was aborted"; got != want {
		t.Fatalf("ErrorMessage = %q, want %q", got, want)
	}
	if got := len(errorEvent.Message.Content); got != 0 {
		t.Fatalf("len(Content) = %d, want 0", got)
	}
	if event, receiveErr = stream.Receive(); event != nil || !errors.Is(receiveErr, io.EOF) {
		t.Fatalf("Receive() after aborted = (%#v, %v), want (nil, io.EOF)", event, receiveErr)
	}
}

func TestCancelKeepsReceivedTextAndCompletedToolCalls(t *testing.T) {
	t.Parallel()

	incompleteArguments := json.RawMessage(`{"path":"unread.go"}`)
	completedArguments := json.RawMessage(`{"path":"done.go"}`)
	finalMessage := ai.AssistantMessage{
		Content: []ai.AssistantContent{
			ai.ToolCall{ID: "call-1", Name: "read", Arguments: incompleteArguments},
			ai.TextContent{Text: "working on it"},
			ai.ToolCall{ID: "call-2", Name: "read", Arguments: completedArguments},
		},
		StopReason: ai.StopReasonToolUse,
	}
	step := faux.Step{Events: []ai.Event{
		ai.StartEvent{},
		ai.ToolCallStartEvent{ContentIndex: 0, ID: "call-1", Name: "read"},
		ai.ToolCallDeltaEvent{ContentIndex: 0, Delta: `{"path":`},
		ai.TextStartEvent{ContentIndex: 1},
		ai.TextDeltaEvent{ContentIndex: 1, Delta: "working"},
		ai.ToolCallStartEvent{ContentIndex: 2, ID: "call-2", Name: "read"},
		ai.ToolCallDeltaEvent{ContentIndex: 2, Delta: string(completedArguments)},
		ai.ToolCallEndEvent{
			ContentIndex: 2,
			ToolCall:     ai.ToolCall{ID: "call-2", Name: "read", Arguments: completedArguments},
		},
		ai.ToolCallDeltaEvent{ContentIndex: 0, Delta: `"unread.go"}`},
		ai.ToolCallEndEvent{
			ContentIndex: 0,
			ToolCall:     ai.ToolCall{ID: "call-1", Name: "read", Arguments: incompleteArguments},
		},
		ai.TextDeltaEvent{ContentIndex: 1, Delta: " on it"},
		ai.TextEndEvent{ContentIndex: 1, Text: "working on it"},
		ai.DoneEvent{Message: finalMessage},
	}}
	provider, err := faux.New(step)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	stream := provider.Stream(ctx, ai.Request{})

	for index := 0; index < 8; index++ {
		if _, receiveErr := stream.Receive(); receiveErr != nil {
			t.Fatalf("Receive() event %d error = %v", index, receiveErr)
		}
	}
	cancel()
	event, receiveErr := stream.Receive()
	if receiveErr != nil {
		t.Fatalf("Receive() after cancel error = %v", receiveErr)
	}
	errorEvent, ok := event.(ai.ErrorEvent)
	if !ok {
		t.Fatalf("Receive() after cancel event = %T, want ai.ErrorEvent", event)
	}
	wantContent := []ai.AssistantContent{
		ai.TextContent{Text: "working"},
		ai.ToolCall{ID: "call-2", Name: "read", Arguments: completedArguments},
	}
	if !reflect.DeepEqual(errorEvent.Message.Content, wantContent) {
		t.Fatalf("aborted Content = %#v, want %#v", errorEvent.Message.Content, wantContent)
	}
	if got, want := errorEvent.Message.StopReason, ai.StopReasonAborted; got != want {
		t.Fatalf("StopReason = %q, want %q", got, want)
	}
}

func TestCancelKeepsReceivedThinking(t *testing.T) {
	t.Parallel()

	provider, err := faux.New(faux.Step{Events: []ai.Event{
		ai.StartEvent{},
		ai.ThinkingStartEvent{ContentIndex: 0},
		ai.ThinkingDeltaEvent{ContentIndex: 0, Delta: "inspect"},
		ai.ThinkingDeltaEvent{ContentIndex: 0, Delta: " first"},
		ai.ThinkingEndEvent{ContentIndex: 0, Thinking: "inspect first"},
		ai.DoneEvent{Message: ai.AssistantMessage{
			Content:    []ai.AssistantContent{ai.ThinkingContent{Thinking: "inspect first"}},
			StopReason: ai.StopReasonStop,
		}},
	}})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	stream := provider.Stream(ctx, ai.Request{})

	for index := 0; index < 3; index++ {
		if _, receiveErr := stream.Receive(); receiveErr != nil {
			t.Fatalf("Receive() event %d error = %v", index, receiveErr)
		}
	}
	cancel()
	event, receiveErr := stream.Receive()
	if receiveErr != nil {
		t.Fatalf("Receive() after cancel error = %v", receiveErr)
	}
	errorEvent, ok := event.(ai.ErrorEvent)
	if !ok {
		t.Fatalf("Receive() after cancel event = %T, want ai.ErrorEvent", event)
	}
	wantContent := []ai.AssistantContent{ai.ThinkingContent{Thinking: "inspect"}}
	if !reflect.DeepEqual(errorEvent.Message.Content, wantContent) {
		t.Fatalf("aborted Content = %#v, want %#v", errorEvent.Message.Content, wantContent)
	}
}

func receiveTerminal(t *testing.T, stream ai.Stream) ai.Event {
	t.Helper()

	for {
		event, err := stream.Receive()
		if err != nil {
			t.Fatalf("Receive() before terminal error = %v", err)
		}
		switch event.(type) {
		case ai.DoneEvent, ai.ErrorEvent:
			return event
		}
	}
}

func textStep(text string) faux.Step {
	return faux.Step{Events: []ai.Event{
		ai.StartEvent{},
		ai.TextStartEvent{ContentIndex: 0},
		ai.TextDeltaEvent{ContentIndex: 0, Delta: text},
		ai.TextEndEvent{ContentIndex: 0, Text: text},
		ai.DoneEvent{Message: ai.AssistantMessage{
			Content:    []ai.AssistantContent{ai.TextContent{Text: text}},
			StopReason: ai.StopReasonStop,
		}},
	}}
}
