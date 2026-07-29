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

func TestSessionWithProviderDeliversNestedAdvanceAndEngineEvents(t *testing.T) {
	t.Parallel()

	terminal := textAssistant("done")
	provider := newCodingFaux(t, codingAssistantStep(terminal))
	var events []observation.Event

	if _, err := advanceTestSessionWithProvider(context.Background(), testSessionAdvance{
		WorkspacePath: t.TempDir(),
		Input:         "finish",
		Observer: func(event observation.Event) {
			events = append(events, event)
		},
	}, provider); err != nil {
		t.Fatalf("advance test Session error = %v", err)
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
	provider := newCodingFaux(t,
		codingAssistantStep(first),
		codingAssistantStep(summary),
		codingAssistantStep(second),
	)
	var events []observation.Event
	var session *Session
	projectionPublishedAtSettlement := false
	session = newObservedCompactingSession(t, provider, func(event observation.Event) {
		events = append(events, event)
		compaction, ok := event.(observation.Compaction)
		if !ok ||
			compaction.Phase != observation.PhaseSettled ||
			compaction.Outcome != observation.OutcomeSuccess {
			return
		}
		session.mu.Lock()
		projectionPublishedAtSettlement = session.projection != nil
		session.mu.Unlock()
	})

	if _, err := session.advanceHistory(context.Background(), "first"); err != nil {
		t.Fatalf("first run error = %v", err)
	}
	events = nil
	if _, err := session.advanceHistory(context.Background(), "second"); err != nil {
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
	provider := newCodingFaux(t,
		codingAssistantStep(first),
		recoveryErrorStep(summaryFailure),
	)
	var events []observation.Event
	session := newObservedCompactingSession(t, provider, func(event observation.Event) {
		events = append(events, event)
	})

	if _, err := session.advanceHistory(context.Background(), "first"); err != nil {
		t.Fatalf("first run error = %v", err)
	}
	events = nil
	if _, err := session.advanceHistory(context.Background(), "second"); err == nil {
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
	provider := newCodingFaux(t,
		fauxToolStep(call),
		recoveryErrorStep(overflow),
		codingAssistantStep(textAssistant("checkpoint")),
		codingAssistantStep(textAssistant("recovered")),
	)
	var events []observation.Event

	if _, err := advanceTestSessionWithProvider(context.Background(), testSessionAdvance{
		WorkspacePath: workspace,
		Input:         "read and finish",
		Observer: func(event observation.Event) {
			events = append(events, event)
		},
	}, provider); err != nil {
		t.Fatalf("advance test Session error = %v", err)
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

	provider := newCodingFaux(t, codingAssistantStep(textAssistant("done")))
	var session *Session
	var reentryErr error
	observer := observation.Observer(func(event observation.Event) {
		advance, ok := event.(observation.Advance)
		if !ok || advance.Phase != observation.PhaseSettled {
			return
		}
		_, reentryErr = session.advanceHistory(context.Background(), "re-enter")
	})
	session = newObservedSession(t, provider, observer)

	if _, err := session.advanceHistory(context.Background(), "first"); err != nil {
		t.Fatalf("run error = %v", err)
	}
	if !errors.Is(reentryErr, ErrSessionBusy) {
		t.Fatalf("observer re-entry error = %v, want ErrSessionBusy", reentryErr)
	}
}

func newObservedSession(
	t *testing.T,
	provider ai.Provider,
	observer observation.Observer,
) *Session {
	t.Helper()

	engine, err := agent.New(agent.Config{
		Provider: provider,
		Observer: observer,
	})
	if err != nil {
		t.Fatalf("agent.New() error = %v", err)
	}
	session, err := newSession(sessionDependencies{
		Engine:         engine,
		Observer:       observer,
		CloseWorkspace: func() error { return nil },
	})
	if err != nil {
		t.Fatalf("newSession() error = %v", err)
	}
	return session
}

func newObservedCompactingSession(
	t *testing.T,
	provider ai.Provider,
	observer observation.Observer,
) *Session {
	t.Helper()

	limits := ai.RequestLimits{
		ContextCapacity: 1000,
		ModelMaxOutput:  400,
		ContextSafety:   10,
	}
	engine, err := agent.New(agent.Config{
		Provider:      provider,
		SystemPrompt:  "system",
		RequestLimits: limits,
		Observer:      observer,
	})
	if err != nil {
		t.Fatalf("agent.New() error = %v", err)
	}
	session, err := newSession(sessionDependencies{
		Engine:         engine,
		Provider:       provider,
		RequestLimits:  limits,
		Compaction:     testCompactionPolicy(),
		Observer:       observer,
		CloseWorkspace: func() error { return nil },
		Info:           SessionInfo{SystemPrompt: "system"},
	})
	if err != nil {
		t.Fatalf("newSession() error = %v", err)
	}
	return session
}
