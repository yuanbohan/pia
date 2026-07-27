package coding

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/yuanbohan/pia/internal/agent"
	"github.com/yuanbohan/pia/internal/ai"
	"github.com/yuanbohan/pia/internal/observation"
)

func TestRunWithProviderDeliversNestedAdvanceAndCoreEvents(t *testing.T) {
	t.Parallel()

	terminal := textAssistant("done")
	provider := newConversationFaux(t, conversationAssistantStep(terminal))
	var events []observation.Event

	if _, err := runWithProvider(context.Background(), RunInput{
		WorkspacePath: t.TempDir(),
		Task:          "finish",
		Observer: func(event observation.Event) {
			events = append(events, event)
		},
	}, provider); err != nil {
		t.Fatalf("runWithProvider() error = %v", err)
	}

	want := []observation.Event{
		observation.Advance{Phase: observation.PhaseStarted},
		observation.Run{Phase: observation.PhaseStarted, Mode: observation.RunModeInput},
		observation.Message{Role: observation.MessageRoleUser},
		observation.Turn{Phase: observation.PhaseStarted},
		observation.Message{
			Role:       observation.MessageRoleAssistant,
			StopReason: ai.StopReasonStop,
		},
		observation.Turn{Phase: observation.PhaseSettled, Outcome: observation.OutcomeSuccess},
		observation.Run{Phase: observation.PhaseSettled, Outcome: observation.OutcomeSuccess},
		observation.Advance{Phase: observation.PhaseSettled, Outcome: observation.OutcomeSuccess},
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events =\n%#v\nwant\n%#v", events, want)
	}
}

func TestThresholdCompactionEventsSurroundOnlyRealAttempt(t *testing.T) {
	t.Parallel()

	first := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "first answer"}},
		Usage:      ai.Usage{InputTokens: 110, OutputTokens: 10},
		StopReason: ai.StopReasonStop,
	}
	summary := textAssistant("checkpoint")
	second := textAssistant("second answer")
	provider := newConversationFaux(t,
		conversationAssistantStep(first),
		conversationAssistantStep(summary),
		conversationAssistantStep(second),
	)
	var events []observation.Event
	var conversation *conversation
	projectionPublishedAtSettlement := false
	conversation = newObservedCompactingConversation(t, provider, func(event observation.Event) {
		events = append(events, event)
		compaction, ok := event.(observation.Compaction)
		if !ok ||
			compaction.Phase != observation.PhaseSettled ||
			compaction.Outcome != observation.OutcomeSuccess {
			return
		}
		conversation.mu.Lock()
		projectionPublishedAtSettlement = conversation.projection != nil
		conversation.mu.Unlock()
	})

	if _, err := conversation.run(context.Background(), "first"); err != nil {
		t.Fatalf("first run error = %v", err)
	}
	events = nil
	if _, err := conversation.run(context.Background(), "second"); err != nil {
		t.Fatalf("second run error = %v", err)
	}

	want := []observation.Event{
		observation.Advance{Phase: observation.PhaseStarted},
		observation.Compaction{
			Phase:  observation.PhaseStarted,
			Reason: observation.CompactionReasonThreshold,
		},
		observation.Compaction{
			Phase:   observation.PhaseSettled,
			Reason:  observation.CompactionReasonThreshold,
			Outcome: observation.OutcomeSuccess,
		},
		observation.Run{Phase: observation.PhaseStarted, Mode: observation.RunModeInput},
		observation.Message{Role: observation.MessageRoleUser},
		observation.Turn{Phase: observation.PhaseStarted},
		observation.Message{
			Role:       observation.MessageRoleAssistant,
			StopReason: ai.StopReasonStop,
		},
		observation.Turn{Phase: observation.PhaseSettled, Outcome: observation.OutcomeSuccess},
		observation.Run{Phase: observation.PhaseSettled, Outcome: observation.OutcomeSuccess},
		observation.Advance{Phase: observation.PhaseSettled, Outcome: observation.OutcomeSuccess},
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events =\n%#v\nwant\n%#v", events, want)
	}
	if !projectionPublishedAtSettlement {
		t.Fatal("successful compaction settled before projection publication")
	}
}

func TestCompactionFailureSettlesAttemptAndAdvanceAsError(t *testing.T) {
	t.Parallel()

	first := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "first answer"}},
		Usage:      ai.Usage{InputTokens: 110, OutputTokens: 10},
		StopReason: ai.StopReasonStop,
	}
	summaryFailure := ai.AssistantMessage{
		StopReason:   ai.StopReasonError,
		ErrorMessage: "summary failed",
	}
	provider := newConversationFaux(t,
		conversationAssistantStep(first),
		recoveryErrorStep(summaryFailure),
	)
	var events []observation.Event
	conversation := newObservedCompactingConversation(t, provider, func(event observation.Event) {
		events = append(events, event)
	})

	if _, err := conversation.run(context.Background(), "first"); err != nil {
		t.Fatalf("first run error = %v", err)
	}
	events = nil
	if _, err := conversation.run(context.Background(), "second"); err == nil {
		t.Fatal("second run error = nil, want compaction failure")
	}

	want := []observation.Event{
		observation.Advance{Phase: observation.PhaseStarted},
		observation.Compaction{
			Phase:  observation.PhaseStarted,
			Reason: observation.CompactionReasonThreshold,
		},
		observation.Compaction{
			Phase:   observation.PhaseSettled,
			Reason:  observation.CompactionReasonThreshold,
			Outcome: observation.OutcomeError,
		},
		observation.Advance{Phase: observation.PhaseSettled, Outcome: observation.OutcomeError},
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events =\n%#v\nwant\n%#v", events, want)
	}
}

func TestOverflowRecoveryUsesCompactionAndContinuationEvents(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	if err := os.WriteFile(workspace+"/main.go", []byte("package main\n"), 0o600); err != nil {
		t.Fatalf("write workspace fixture: %v", err)
	}
	call := ai.ToolCall{
		ID:        "read-1",
		Name:      "read",
		Arguments: []byte(`{"path":"main.go"}`),
	}
	overflow := ai.AssistantMessage{
		StopReason:   ai.StopReasonError,
		ErrorMessage: "context length exceeded",
	}
	provider := newConversationFaux(t,
		fauxToolStep(call),
		recoveryErrorStep(overflow),
		conversationAssistantStep(textAssistant("checkpoint")),
		conversationAssistantStep(textAssistant("recovered")),
	)
	var events []observation.Event

	if _, err := runWithProvider(context.Background(), RunInput{
		WorkspacePath: workspace,
		Task:          "read and finish",
		Observer: func(event observation.Event) {
			events = append(events, event)
		},
	}, provider); err != nil {
		t.Fatalf("runWithProvider() error = %v", err)
	}

	want := []observation.Event{
		observation.Advance{Phase: observation.PhaseStarted},
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
			StopReason: ai.StopReasonError,
		},
		observation.Turn{Phase: observation.PhaseSettled, Outcome: observation.OutcomeError},
		observation.Run{Phase: observation.PhaseSettled, Outcome: observation.OutcomeError},
		observation.Compaction{
			Phase:  observation.PhaseStarted,
			Reason: observation.CompactionReasonOverflow,
		},
		observation.Compaction{
			Phase:   observation.PhaseSettled,
			Reason:  observation.CompactionReasonOverflow,
			Outcome: observation.OutcomeSuccess,
		},
		observation.Run{Phase: observation.PhaseStarted, Mode: observation.RunModeContinuation},
		observation.Turn{Phase: observation.PhaseStarted},
		observation.Message{
			Role:       observation.MessageRoleAssistant,
			StopReason: ai.StopReasonStop,
		},
		observation.Turn{Phase: observation.PhaseSettled, Outcome: observation.OutcomeSuccess},
		observation.Run{Phase: observation.PhaseSettled, Outcome: observation.OutcomeSuccess},
		observation.Advance{Phase: observation.PhaseSettled, Outcome: observation.OutcomeSuccess},
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events =\n%#v\nwant\n%#v", events, want)
	}
}

func TestAdvanceKeepsActiveGuardThroughSettledObservation(t *testing.T) {
	t.Parallel()

	provider := newConversationFaux(t, conversationAssistantStep(textAssistant("done")))
	var conversation *conversation
	var reentryErr error
	observer := observation.Observer(func(event observation.Event) {
		advance, ok := event.(observation.Advance)
		if !ok || advance.Phase != observation.PhaseSettled {
			return
		}
		_, reentryErr = conversation.run(context.Background(), "re-enter")
	})
	conversation = newObservedConversation(t, provider, observer)

	if _, err := conversation.run(context.Background(), "first"); err != nil {
		t.Fatalf("run error = %v", err)
	}
	if !errors.Is(reentryErr, agent.ErrRunActive) {
		t.Fatalf("observer re-entry error = %v, want ErrRunActive", reentryErr)
	}
}

func newObservedConversation(
	t *testing.T,
	provider ai.Provider,
	observer observation.Observer,
) *conversation {
	t.Helper()

	core, err := agent.New(agent.Config{
		Provider: provider,
		Observer: observer,
	})
	if err != nil {
		t.Fatalf("agent.New() error = %v", err)
	}
	conversation, err := newConversation(conversationConfig{
		Core:     core,
		Observer: observer,
	})
	if err != nil {
		t.Fatalf("newConversation() error = %v", err)
	}
	return conversation
}

func newObservedCompactingConversation(
	t *testing.T,
	provider ai.Provider,
	observer observation.Observer,
) *conversation {
	t.Helper()

	limits := ai.RequestLimits{
		ContextCapacity: 1000,
		ModelMaxOutput:  400,
		ContextSafety:   10,
	}
	core, err := agent.New(agent.Config{
		Provider:      provider,
		SystemPrompt:  "system",
		RequestLimits: limits,
		Observer:      observer,
	})
	if err != nil {
		t.Fatalf("agent.New() error = %v", err)
	}
	conversation, err := newConversation(conversationConfig{
		Core:          core,
		Provider:      provider,
		SystemPrompt:  "system",
		RequestLimits: limits,
		Compaction:    testCompactionPolicy(),
		Observer:      observer,
	})
	if err != nil {
		t.Fatalf("newConversation() error = %v", err)
	}
	return conversation
}
