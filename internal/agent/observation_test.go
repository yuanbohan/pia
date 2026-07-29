package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/yuanbohan/pia/internal/agent"
	"github.com/yuanbohan/pia/internal/ai"
	"github.com/yuanbohan/pia/internal/ai/provider/faux"
	"github.com/yuanbohan/pia/internal/observation"
)

func TestRunEmitsEngineSemanticEventsInSettlementOrder(t *testing.T) {
	t.Parallel()

	toolTerminal := toolAssistant(
		ai.StopReasonToolUse,
		toolCall("read-1", "read", "main.go"),
	)
	finalTerminal := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "done"}},
		StopReason: ai.StopReasonStop,
	}
	provider := newFaux(t, assistantStep(toolTerminal), assistantStep(finalTerminal))
	read := &testTool{
		definition: agent.ToolDefinition{
			Schema:         toolSchema("read"),
			CanRunParallel: true,
		},
		describe: func(json.RawMessage) string { return "Read main.go" },
		execute:  func(context.Context, json.RawMessage) (string, error) { return "contents", nil },
	}
	var events []observation.Event
	runtime, err := agent.New(agent.Config{
		Provider: provider,
		Tools:    []agent.Tool{read},
		Observer: func(event observation.Event) {
			events = append(events, event)
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := runtime.Run(context.Background(), nil, "inspect"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	want := []observation.Event{
		observation.Run{Phase: observation.PhaseStarted, Mode: observation.RunModeInput},
		observation.Message{Role: observation.MessageRoleUser},
		observation.Turn{Phase: observation.PhaseStarted},
		observation.Message{
			Role:       observation.MessageRoleAssistant,
			StopReason: ai.StopReasonToolUse,
		},
		observation.NewToolStarted(0, "read", "Read main.go"),
		observation.NewToolSettled(0, "read", "Read main.go", observation.OutcomeSuccess),
		observation.Message{Role: observation.MessageRoleToolResult},
		observation.Turn{Phase: observation.PhaseSettled, Outcome: observation.OutcomeSuccess},
		observation.Turn{Phase: observation.PhaseStarted},
		observation.Message{
			Role:       observation.MessageRoleAssistant,
			StopReason: ai.StopReasonStop,
		},
		observation.Turn{Phase: observation.PhaseSettled, Outcome: observation.OutcomeSuccess},
		observation.Run{Phase: observation.PhaseSettled, Outcome: observation.OutcomeSuccess},
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events =\n%#v\nwant\n%#v", events, want)
	}
}

func TestRunEmitsErrorSettlementsWithoutCopyingProviderError(t *testing.T) {
	t.Parallel()

	terminal := ai.AssistantMessage{
		StopReason:   ai.StopReasonError,
		ErrorMessage: "sensitive provider failure",
	}
	provider := newFaux(t, faux.Step{
		Events: []ai.Event{ai.ErrorEvent{Message: terminal}},
	})
	var events []observation.Event
	runtime, err := agent.New(agent.Config{
		Provider: provider,
		Observer: func(event observation.Event) {
			events = append(events, event)
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := runtime.Run(context.Background(), nil, "fail"); err == nil {
		t.Fatal("Run() error = nil, want Provider failure")
	}

	want := []observation.Event{
		observation.Run{Phase: observation.PhaseStarted, Mode: observation.RunModeInput},
		observation.Message{Role: observation.MessageRoleUser},
		observation.Turn{Phase: observation.PhaseStarted},
		observation.Message{
			Role:       observation.MessageRoleAssistant,
			StopReason: ai.StopReasonError,
		},
		observation.Turn{Phase: observation.PhaseSettled, Outcome: observation.OutcomeError},
		observation.Run{Phase: observation.PhaseSettled, Outcome: observation.OutcomeError},
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events =\n%#v\nwant\n%#v", events, want)
	}
}

func TestAgentWithoutObserverDoesNotDescribeToolInvocation(t *testing.T) {
	t.Parallel()

	descriptionCalls := 0
	read := &testTool{
		definition: agent.ToolDefinition{Schema: toolSchema("read")},
		describe: func(json.RawMessage) string {
			descriptionCalls++
			return "Read main.go"
		},
		execute: func(context.Context, json.RawMessage) (string, error) {
			return "contents", nil
		},
	}
	provider := newFaux(t,
		assistantStep(toolAssistant(
			ai.StopReasonToolUse,
			toolCall("read-1", "read", "main.go"),
		)),
		assistantStep(ai.AssistantMessage{StopReason: ai.StopReasonStop}),
	)
	runtime := newAgentWithTools(t, provider, "system", read)

	if _, err := runtime.Run(context.Background(), nil, "inspect"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if descriptionCalls != 0 {
		t.Fatalf("DescribeInvocation() calls = %d, want 0 without observer", descriptionCalls)
	}
}

func TestSerialToolErrorSettlesOnlyThatToolAsError(t *testing.T) {
	t.Parallel()

	read := &testTool{
		definition: agent.ToolDefinition{Schema: toolSchema("read")},
		describe:   func(json.RawMessage) string { return "Read missing.go" },
		execute: func(context.Context, json.RawMessage) (string, error) {
			return "", errors.New("missing")
		},
	}
	edit := &testTool{
		definition: agent.ToolDefinition{Schema: toolSchema("edit")},
		describe:   func(json.RawMessage) string { return "Edit main.go" },
		execute: func(context.Context, json.RawMessage) (string, error) {
			return "updated", nil
		},
	}
	provider := newFaux(t,
		assistantStep(toolAssistant(
			ai.StopReasonToolUse,
			toolCall("read-1", "read", "missing.go"),
			toolCall("edit-1", "edit", "main.go"),
		)),
		assistantStep(ai.AssistantMessage{StopReason: ai.StopReasonStop}),
	)
	var events []observation.Event
	runtime, err := agent.New(agent.Config{
		Provider: provider,
		Tools:    []agent.Tool{read, edit},
		Observer: func(event observation.Event) {
			events = append(events, event)
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := runtime.Run(context.Background(), nil, "inspect then update"); err != nil {
		t.Fatalf("Run() error = %v, want call-local tool error to remain in the Turn", err)
	}

	var toolEvents []observation.Event
	for _, event := range events {
		if _, ok := event.(observation.Tool); ok {
			toolEvents = append(toolEvents, event)
		}
	}
	want := []observation.Event{
		observation.NewToolStarted(0, "read", "Read missing.go"),
		observation.NewToolSettled(0, "read", "Read missing.go", observation.OutcomeError),
		observation.NewToolStarted(1, "edit", "Edit main.go"),
		observation.NewToolSettled(1, "edit", "Edit main.go", observation.OutcomeSuccess),
	}
	if !reflect.DeepEqual(toolEvents, want) {
		t.Fatalf("tool events =\n%#v\nwant\n%#v", toolEvents, want)
	}

	last := events[len(events)-1]
	if want := (observation.Run{
		Phase:   observation.PhaseSettled,
		Outcome: observation.OutcomeSuccess,
	}); last != want {
		t.Fatalf("last event = %#v, want %#v", last, want)
	}
}

func TestParallelToolSettlementsUseCompletionOrderAndResultsUseSourceOrder(t *testing.T) {
	t.Parallel()

	aStarted := make(chan struct{})
	bSettled := make(chan struct{})
	releaseA := make(chan struct{})
	read := &testTool{
		definition: agent.ToolDefinition{
			Schema:         toolSchema("read"),
			CanRunParallel: true,
		},
		describe: func(arguments json.RawMessage) string {
			label, _ := argumentLabel(arguments)
			return "Read " + label
		},
		execute: func(_ context.Context, arguments json.RawMessage) (string, error) {
			label, err := argumentLabel(arguments)
			if err != nil {
				return "", err
			}
			if label == "A" {
				close(aStarted)
				<-releaseA
			} else {
				<-aStarted
				close(bSettled)
			}
			return "result-" + label, nil
		},
	}
	provider := newFaux(t,
		assistantStep(toolAssistant(
			ai.StopReasonToolUse,
			toolCall("A", "read", "A"),
			toolCall("B", "read", "B"),
		)),
		assistantStep(ai.AssistantMessage{StopReason: ai.StopReasonStop}),
	)
	var eventsMu sync.Mutex
	var events []observation.Event
	runtime, err := agent.New(agent.Config{
		Provider: provider,
		Tools:    []agent.Tool{read},
		Observer: func(event observation.Event) {
			eventsMu.Lock()
			events = append(events, event)
			eventsMu.Unlock()
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	returned := runInBackground(runtime, context.Background(), "parallel")
	waitSignal(t, bSettled, "B did not settle first")
	close(releaseA)
	got := waitRun(t, returned)
	if got.err != nil {
		t.Fatalf("Run() error = %v", got.err)
	}

	eventsMu.Lock()
	captured := append([]observation.Event(nil), events...)
	eventsMu.Unlock()
	var started []int
	var settled []int
	for _, event := range captured {
		tool, ok := event.(observation.Tool)
		if !ok {
			continue
		}
		if tool.Phase == observation.PhaseStarted {
			started = append(started, tool.Index)
		} else {
			settled = append(settled, tool.Index)
		}
	}
	if want := []int{0, 1}; !reflect.DeepEqual(started, want) {
		t.Fatalf("tool-started indices = %v, want %v", started, want)
	}
	if want := []int{1, 0}; !reflect.DeepEqual(settled, want) {
		t.Fatalf("tool-settled indices = %v, want %v", settled, want)
	}

	results := runToolResults(got.result.NewMessages)
	gotIDs := []string{results[0].ToolCallID, results[1].ToolCallID}
	if want := []string{"A", "B"}; !reflect.DeepEqual(gotIDs, want) {
		t.Fatalf("tool-result message order = %v, want %v", gotIDs, want)
	}
}
