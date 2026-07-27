package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/yuanbohan/pia/internal/observation"
)

func TestLineObserverProjectsStartsAndFailureSettlementsOnly(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	observer := newLineObserver(&output)
	for _, event := range []observation.Event{
		observation.Advance{Phase: observation.PhaseStarted},
		observation.Run{Phase: observation.PhaseStarted, Mode: observation.RunModeInput},
		observation.NewToolStarted(0, "read", "Read main.go"),
		observation.NewToolSettled(0, "read", "Read main.go", observation.OutcomeSuccess),
		observation.NewToolStarted(1, "edit", "Edit main.go"),
		observation.NewToolSettled(1, "edit", "Edit main.go", observation.OutcomeError),
		observation.Compaction{
			Phase:  observation.PhaseStarted,
			Reason: observation.CompactionReasonThreshold,
		},
		observation.Compaction{
			Phase:   observation.PhaseSettled,
			Reason:  observation.CompactionReasonThreshold,
			Outcome: observation.OutcomeSuccess,
		},
		observation.Compaction{
			Phase:  observation.PhaseStarted,
			Reason: observation.CompactionReasonOverflow,
		},
		observation.Compaction{
			Phase:   observation.PhaseSettled,
			Reason:  observation.CompactionReasonOverflow,
			Outcome: observation.OutcomeError,
		},
	} {
		observer.Observe(event)
	}

	want := strings.Join([]string{
		"pia: Read main.go",
		"pia: Edit main.go",
		"pia: Edit main.go failed",
		"pia: Compact context (threshold)",
		"pia: Compact context (overflow)",
		"pia: Compact context (overflow) failed",
		"",
	}, "\n")
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
	if err := observer.Err(); err != nil {
		t.Fatalf("Err() = %v, want nil", err)
	}
}

func TestLineObserverEscapesControlCharactersIntoOneLine(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	observer := newLineObserver(&output)
	observer.Observe(observation.NewToolStarted(
		0,
		"bash",
		"Bash printf 'first\nsecond'\x1b[31m",
	))

	if got, want := output.String(), "pia: Bash printf 'first\\nsecond'\\u001b[31m\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestLineObserverStopsAfterFirstWriteFailure(t *testing.T) {
	t.Parallel()

	writeErr := errors.New("stderr unavailable")
	writer := &countingErrorWriter{err: writeErr}
	observer := newLineObserver(writer)

	observer.Observe(observation.NewToolStarted(0, "read", "Read first.go"))
	observer.Observe(observation.NewToolStarted(1, "read", "Read second.go"))

	if !errors.Is(observer.Err(), writeErr) {
		t.Fatalf("Err() = %v, want write error", observer.Err())
	}
	if writer.calls != 1 {
		t.Fatalf("writer calls = %d, want 1", writer.calls)
	}
}

type countingErrorWriter struct {
	err   error
	calls int
}

func (writer *countingErrorWriter) Write([]byte) (int, error) {
	writer.calls++
	return 0, writer.err
}
