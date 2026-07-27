package observation

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/yuanbohan/pia/internal/ai"
)

func TestObserverDeliversEventSynchronously(t *testing.T) {
	t.Parallel()

	var received Event
	observer := Observer(func(event Event) {
		received = event
	})
	want := Run{Phase: PhaseStarted, Mode: RunModeInput}

	observer.Observe(want)

	if received != want {
		t.Fatalf("received = %#v, want %#v", received, want)
	}
}

func TestNilObserverDoesNothing(t *testing.T) {
	t.Parallel()

	var observer Observer
	observer.Observe(Advance{Phase: PhaseStarted})
}

func TestOutcomeFromErrorUsesBinarySettlementPolicy(t *testing.T) {
	t.Parallel()

	if got := OutcomeFromError(nil); got != OutcomeSuccess {
		t.Fatalf("nil outcome = %q, want %q", got, OutcomeSuccess)
	}
	if got := OutcomeFromError(errors.New("failed")); got != OutcomeError {
		t.Fatalf("error outcome = %q, want %q", got, OutcomeError)
	}
}

func TestToolEventBoundsCopiedDisplayState(t *testing.T) {
	t.Parallel()

	name := strings.Repeat("n", maxToolNameBytes+20)
	summary := strings.Repeat("界", maxToolSummaryBytes)
	event := NewToolStarted(3, name, summary)

	if event.Phase != PhaseStarted {
		t.Fatalf("Phase = %q, want %q", event.Phase, PhaseStarted)
	}
	if event.Index != 3 {
		t.Fatalf("Index = %d, want 3", event.Index)
	}
	if len(event.Name) > maxToolNameBytes {
		t.Fatalf("name length = %d, want at most %d", len(event.Name), maxToolNameBytes)
	}
	if len(event.Summary) > maxToolSummaryBytes {
		t.Fatalf("summary length = %d, want at most %d", len(event.Summary), maxToolSummaryBytes)
	}
	if !utf8.ValidString(event.Name) || !utf8.ValidString(event.Summary) {
		t.Fatal("bounded tool display state is not valid UTF-8")
	}
	if !strings.HasSuffix(event.Name, truncationMarker) ||
		!strings.HasSuffix(event.Summary, truncationMarker) {
		t.Fatalf("bounded event = %#v, want visible truncation markers", event)
	}
}

func TestToolSettledCarriesOnlyBoundedIdentityAndOutcome(t *testing.T) {
	t.Parallel()

	event := NewToolSettled(1, "read", "Read main.go", OutcomeError)

	if event != (Tool{
		Phase:   PhaseSettled,
		Index:   1,
		Name:    "read",
		Summary: "Read main.go",
		Outcome: OutcomeError,
	}) {
		t.Fatalf("event = %#v", event)
	}
}

func TestMessageEventCarriesProtocolFactsWithoutMessageContent(t *testing.T) {
	t.Parallel()

	event := Message{
		Role:       MessageRoleAssistant,
		StopReason: ai.StopReasonAborted,
	}

	if event.Role != MessageRoleAssistant || event.StopReason != ai.StopReasonAborted {
		t.Fatalf("event = %#v", event)
	}
}

func TestEverySemanticFamilyImplementsEvent(t *testing.T) {
	t.Parallel()

	events := []Event{
		Advance{Phase: PhaseSettled, Outcome: OutcomeSuccess},
		Compaction{
			Phase:  PhaseStarted,
			Reason: CompactionReasonThreshold,
		},
		Run{Phase: PhaseStarted, Mode: RunModeContinuation},
		Turn{Phase: PhaseSettled, Outcome: OutcomeError},
		Message{Role: MessageRoleToolResult, IsError: true},
		NewToolStarted(0, "bash", "Bash make check"),
	}

	if len(events) != 6 {
		t.Fatalf("event family count = %d, want 6", len(events))
	}
}
