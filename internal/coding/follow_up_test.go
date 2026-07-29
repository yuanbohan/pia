package coding

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/yuanbohan/pia/internal/ai"
	"github.com/yuanbohan/pia/internal/observation"
)

func TestSessionFollowUpDrainsFIFOWithoutAnIdleGap(t *testing.T) {
	t.Parallel()

	initialStarted := make(chan struct{})
	initialRelease := make(chan struct{})
	firstFollowUpStarted := make(chan struct{})
	firstFollowUpRelease := make(chan struct{})
	provider := newScriptedRecoveryProvider(
		gatedEventFactory(initialStarted, initialRelease, ai.DoneEvent{
			Message: textAssistant("initial answer"),
		}),
		gatedEventFactory(firstFollowUpStarted, firstFollowUpRelease, ai.DoneEvent{
			Message: textAssistant("first follow-up answer"),
		}),
		eventStreamFactory(ai.DoneEvent{Message: textAssistant("second follow-up answer")}),
		eventStreamFactory(ai.DoneEvent{Message: textAssistant("third follow-up answer")}),
	)
	session := newLifecycleSession(t, provider, func() error { return nil })
	t.Cleanup(func() {
		if err := session.Close(context.Background()); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	returned := make(chan sessionAdvanceReturn, 1)
	go func() {
		result, err := session.Advance(context.Background(), "initial")
		returned <- sessionAdvanceReturn{result: result, err: err}
	}()
	<-initialStarted

	if err := session.FollowUp("first follow-up"); err != nil {
		t.Fatalf("first FollowUp() error = %v", err)
	}
	if err := session.FollowUp("second follow-up"); err != nil {
		t.Fatalf("second FollowUp() error = %v", err)
	}
	close(initialRelease)
	<-firstFollowUpStarted

	waitCtx, cancelWait := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancelWait()
	if err := session.Wait(waitCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Wait() between Follow-up Runs error = %v, want deadline", err)
	}
	if _, err := session.Advance(context.Background(), "must stay busy"); !errors.Is(err, ErrSessionBusy) {
		t.Fatalf("Advance() between Follow-up Runs error = %v, want ErrSessionBusy", err)
	}
	if err := session.FollowUp("third follow-up"); err != nil {
		t.Fatalf("third FollowUp() error = %v", err)
	}

	close(firstFollowUpRelease)
	got := receiveAdvance(t, returned)
	if got.err != nil {
		t.Fatalf("Advance() error = %v", got.err)
	}
	if len(got.result.UnconsumedFollowUps) != 0 {
		t.Fatalf(
			"UnconsumedFollowUps = %#v, want empty",
			got.result.UnconsumedFollowUps,
		)
	}
	if gotText, want := got.result.FinalText(), "third follow-up answer"; gotText != want {
		t.Fatalf("FinalText() = %q, want %q", gotText, want)
	}

	wantUsers := []string{
		"initial",
		"first follow-up",
		"second follow-up",
		"third follow-up",
	}
	if gotUsers := userMessageTexts(got.result.History); !reflect.DeepEqual(gotUsers, wantUsers) {
		t.Fatalf("History user messages = %#v, want %#v", gotUsers, wantUsers)
	}
	requests := provider.Requests()
	if gotCount, want := len(requests), len(wantUsers); gotCount != want {
		t.Fatalf("Provider requests = %d, want %d", gotCount, want)
	}
	for index, request := range requests {
		if gotUsers := userMessageTexts(request.Messages); !reflect.DeepEqual(
			gotUsers,
			wantUsers[:index+1],
		) {
			t.Fatalf(
				"Provider request %d user messages = %#v, want %#v",
				index,
				gotUsers,
				wantUsers[:index+1],
			)
		}
	}
}

func TestSessionFollowUpAdmissionRejectsIdleBlankSealedAndClosed(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	release := make(chan struct{})
	settling := make(chan struct{})
	releaseSettlement := make(chan struct{})
	var settleOnce sync.Once
	var events []observation.Event
	var eventsMu sync.Mutex
	observer := observation.Observer(func(event observation.Event) {
		eventsMu.Lock()
		events = append(events, event)
		eventsMu.Unlock()

		advance, ok := event.(observation.Advance)
		if ok && advance.Phase == observation.PhaseSettled {
			settleOnce.Do(func() { close(settling) })
			<-releaseSettlement
		}
	})
	provider := newScriptedRecoveryProvider(
		gatedEventFactory(started, release, ai.DoneEvent{
			Message: textAssistant("initial answer"),
		}),
		eventStreamFactory(ai.DoneEvent{Message: textAssistant("follow-up answer")}),
	)
	session := newObservedSession(t, provider, observer)

	if err := session.FollowUp("idle"); !errors.Is(err, ErrFollowUpUnavailable) {
		t.Fatalf("idle FollowUp() error = %v, want ErrFollowUpUnavailable", err)
	}
	if err := session.FollowUp(" \t\n"); err == nil ||
		errors.Is(err, ErrFollowUpUnavailable) ||
		errors.Is(err, ErrSessionClosed) {
		t.Fatalf("blank FollowUp() error = %v, want input validation error", err)
	}

	returned := make(chan sessionAdvanceReturn, 1)
	go func() {
		result, err := session.Advance(context.Background(), "initial")
		returned <- sessionAdvanceReturn{result: result, err: err}
	}()
	<-started
	if err := session.FollowUp("queued"); err != nil {
		t.Fatalf("active FollowUp() error = %v", err)
	}
	close(release)
	<-settling

	if err := session.FollowUp("too late"); !errors.Is(err, ErrFollowUpUnavailable) {
		t.Fatalf("sealed FollowUp() error = %v, want ErrFollowUpUnavailable", err)
	}
	if _, err := session.Advance(context.Background(), "still active"); !errors.Is(err, ErrSessionBusy) {
		t.Fatalf("Advance() during settled observation error = %v, want ErrSessionBusy", err)
	}
	close(releaseSettlement)

	got := receiveAdvance(t, returned)
	if got.err != nil {
		t.Fatalf("Advance() error = %v", got.err)
	}
	if err := session.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := session.FollowUp("closed"); !errors.Is(err, ErrSessionClosed) {
		t.Fatalf("closed FollowUp() error = %v, want ErrSessionClosed", err)
	}

	eventsMu.Lock()
	gotEvents := append([]observation.Event(nil), events...)
	eventsMu.Unlock()
	wantEvents := []observation.Event{
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
	if !reflect.DeepEqual(gotEvents, wantEvents) {
		t.Fatalf("events =\n%#v\nwant\n%#v", gotEvents, wantEvents)
	}
}

func TestSessionRunFailureHandsBackOnlyUnconsumedFollowUps(t *testing.T) {
	t.Parallel()

	initialStarted := make(chan struct{})
	initialRelease := make(chan struct{})
	failure := ai.AssistantMessage{
		StopReason:   ai.StopReasonError,
		ErrorMessage: "provider failed",
	}
	provider := newScriptedRecoveryProvider(
		gatedEventFactory(initialStarted, initialRelease, ai.DoneEvent{
			Message: textAssistant("initial answer"),
		}),
		eventStreamFactory(ai.ErrorEvent{Message: failure}),
	)
	session := newLifecycleSession(t, provider, func() error { return nil })
	t.Cleanup(func() {
		if err := session.Close(context.Background()); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	returned := make(chan sessionAdvanceReturn, 1)
	go func() {
		result, err := session.Advance(context.Background(), "initial")
		returned <- sessionAdvanceReturn{result: result, err: err}
	}()
	<-initialStarted
	if err := session.FollowUp("accepted then failed"); err != nil {
		t.Fatalf("first FollowUp() error = %v", err)
	}
	if err := session.FollowUp("not consumed"); err != nil {
		t.Fatalf("second FollowUp() error = %v", err)
	}
	close(initialRelease)

	got := receiveAdvance(t, returned)
	if got.err == nil {
		t.Fatal("Advance() error = nil, want Follow-up Run failure")
	}
	if want := []string{"not consumed"}; !reflect.DeepEqual(
		got.result.UnconsumedFollowUps,
		want,
	) {
		t.Fatalf(
			"UnconsumedFollowUps = %#v, want %#v",
			got.result.UnconsumedFollowUps,
			want,
		)
	}
	wantUsers := []string{"initial", "accepted then failed"}
	if gotUsers := userMessageTexts(got.result.History); !reflect.DeepEqual(gotUsers, wantUsers) {
		t.Fatalf("History user messages = %#v, want %#v", gotUsers, wantUsers)
	}
	if gotRequests := len(provider.Requests()); gotRequests != 2 {
		t.Fatalf("Provider requests = %d, want 2", gotRequests)
	}
}

func TestSessionPreRunFailureReturnsDequeuedFollowUpFirst(t *testing.T) {
	t.Parallel()

	initialStarted := make(chan struct{})
	initialRelease := make(chan struct{})
	initial := textAssistant("initial answer")
	initial.Usage = ai.Usage{InputTokens: 1_100, OutputTokens: 10}
	summaryFailure := ai.AssistantMessage{
		StopReason:   ai.StopReasonError,
		ErrorMessage: "summary failed",
	}
	provider := newScriptedRecoveryProvider(
		gatedEventFactory(initialStarted, initialRelease, ai.DoneEvent{Message: initial}),
		eventStreamFactory(ai.ErrorEvent{Message: summaryFailure}),
	)
	session := newRecoveryTestSession(t, provider)
	t.Cleanup(func() {
		if err := session.Close(context.Background()); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	returned := make(chan sessionAdvanceReturn, 1)
	go func() {
		result, err := session.Advance(context.Background(), "initial")
		returned <- sessionAdvanceReturn{result: result, err: err}
	}()
	<-initialStarted
	if err := session.FollowUp("dequeued before compaction"); err != nil {
		t.Fatalf("first FollowUp() error = %v", err)
	}
	if err := session.FollowUp("still pending"); err != nil {
		t.Fatalf("second FollowUp() error = %v", err)
	}
	close(initialRelease)

	got := receiveAdvance(t, returned)
	if got.err == nil {
		t.Fatal("Advance() error = nil, want pre-Run compaction failure")
	}
	wantUnconsumed := []string{"dequeued before compaction", "still pending"}
	if !reflect.DeepEqual(got.result.UnconsumedFollowUps, wantUnconsumed) {
		t.Fatalf(
			"UnconsumedFollowUps = %#v, want %#v",
			got.result.UnconsumedFollowUps,
			wantUnconsumed,
		)
	}
	if gotUsers := userMessageTexts(got.result.History); !reflect.DeepEqual(
		gotUsers,
		[]string{"initial"},
	) {
		t.Fatalf("History user messages = %#v, want only initial", gotUsers)
	}
	if gotRequests := len(provider.Requests()); gotRequests != 2 {
		t.Fatalf("Provider requests = %d, want initial Run plus compaction", gotRequests)
	}
}

func TestSessionCancellationHandsBackPendingFollowUps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		trigger func(*Session, context.CancelCauseFunc) error
	}{
		{
			name: "caller context",
			trigger: func(_ *Session, cancel context.CancelCauseFunc) error {
				cancel(errors.New("caller canceled"))
				return nil
			},
		},
		{
			name: "Cancel",
			trigger: func(session *Session, _ context.CancelCauseFunc) error {
				session.Cancel()
				return nil
			},
		},
		{
			name: "Close",
			trigger: func(session *Session, _ context.CancelCauseFunc) error {
				return session.Close(context.Background())
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			started := make(chan struct{})
			provider := newScriptedRecoveryProvider(func(ctx context.Context) ai.Stream {
				return &cancelBlockingStream{ctx: ctx, started: started}
			})
			session := newLifecycleSession(t, provider, func() error { return nil })
			ctx, cancel := context.WithCancelCause(context.Background())
			returned := make(chan sessionAdvanceReturn, 1)
			go func() {
				result, err := session.Advance(ctx, "initial")
				returned <- sessionAdvanceReturn{result: result, err: err}
			}()
			<-started
			if err := session.FollowUp("first pending"); err != nil {
				t.Fatalf("first FollowUp() error = %v", err)
			}
			if err := session.FollowUp("second pending"); err != nil {
				t.Fatalf("second FollowUp() error = %v", err)
			}

			if err := test.trigger(session, cancel); err != nil {
				t.Fatalf("control operation error = %v", err)
			}
			got := receiveAdvance(t, returned)
			if got.err == nil {
				t.Fatal("Advance() error = nil, want cancellation")
			}
			want := []string{"first pending", "second pending"}
			if !reflect.DeepEqual(got.result.UnconsumedFollowUps, want) {
				t.Fatalf(
					"UnconsumedFollowUps = %#v, want %#v",
					got.result.UnconsumedFollowUps,
					want,
				)
			}
			if gotUsers := userMessageTexts(got.result.History); !reflect.DeepEqual(
				gotUsers,
				[]string{"initial"},
			) {
				t.Fatalf("History user messages = %#v, want only initial", gotUsers)
			}

			if test.name != "Close" {
				if err := session.Close(context.Background()); err != nil {
					t.Fatalf("Close() error = %v", err)
				}
			}
		})
	}
}

func TestSessionCancelAfterSuccessfulRunStopsBeforePendingFollowUp(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	release := make(chan struct{})
	provider := newScriptedRecoveryProvider(
		gatedEventFactory(started, release, ai.DoneEvent{
			Message: textAssistant("current Run still completed"),
		}),
	)
	session := newLifecycleSession(t, provider, func() error { return nil })
	t.Cleanup(func() {
		if err := session.Close(context.Background()); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	returned := make(chan sessionAdvanceReturn, 1)
	go func() {
		result, err := session.Advance(context.Background(), "initial")
		returned <- sessionAdvanceReturn{result: result, err: err}
	}()
	<-started
	if err := session.FollowUp("must not start"); err != nil {
		t.Fatalf("FollowUp() error = %v", err)
	}

	session.Cancel()
	close(release)
	got := receiveAdvance(t, returned)
	if !errors.Is(got.err, context.Canceled) {
		t.Fatalf("Advance() error = %v, want context.Canceled", got.err)
	}
	if want := []string{"must not start"}; !reflect.DeepEqual(
		got.result.UnconsumedFollowUps,
		want,
	) {
		t.Fatalf(
			"UnconsumedFollowUps = %#v, want %#v",
			got.result.UnconsumedFollowUps,
			want,
		)
	}
	if gotText, want := got.result.FinalText(), "current Run still completed"; gotText != want {
		t.Fatalf("FinalText() = %q, want %q", gotText, want)
	}
	if gotRequests := len(provider.Requests()); gotRequests != 1 {
		t.Fatalf("Provider requests = %d, want 1", gotRequests)
	}
}

func TestSessionFollowUpCanRecoverOverflowBeforeDrainingNextInput(t *testing.T) {
	t.Parallel()

	initialStarted := make(chan struct{})
	initialRelease := make(chan struct{})
	overflow := ai.AssistantMessage{
		StopReason:   ai.StopReasonError,
		ErrorMessage: "context length exceeded",
	}
	provider := newScriptedRecoveryProvider(
		gatedEventFactory(initialStarted, initialRelease, ai.DoneEvent{
			Message: textAssistant("initial answer"),
		}),
		eventStreamFactory(ai.ErrorEvent{Message: overflow}),
		eventStreamFactory(ai.DoneEvent{Message: textAssistant("checkpoint")}),
		eventStreamFactory(ai.DoneEvent{Message: textAssistant("recovered follow-up")}),
		eventStreamFactory(ai.DoneEvent{Message: textAssistant("final follow-up")}),
	)
	session := newRecoveryTestSession(t, provider)
	t.Cleanup(func() {
		if err := session.Close(context.Background()); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	returned := make(chan sessionAdvanceReturn, 1)
	go func() {
		result, err := session.Advance(context.Background(), "initial")
		returned <- sessionAdvanceReturn{result: result, err: err}
	}()
	<-initialStarted
	if err := session.FollowUp("overflowing follow-up"); err != nil {
		t.Fatalf("first FollowUp() error = %v", err)
	}
	if err := session.FollowUp("after recovery"); err != nil {
		t.Fatalf("second FollowUp() error = %v", err)
	}
	close(initialRelease)

	got := receiveAdvance(t, returned)
	if got.err != nil {
		t.Fatalf("Advance() error = %v", got.err)
	}
	if len(got.result.UnconsumedFollowUps) != 0 {
		t.Fatalf(
			"UnconsumedFollowUps = %#v, want empty",
			got.result.UnconsumedFollowUps,
		)
	}
	wantUsers := []string{"initial", "overflowing follow-up", "after recovery"}
	if gotUsers := userMessageTexts(got.result.History); !reflect.DeepEqual(gotUsers, wantUsers) {
		t.Fatalf("History user messages = %#v, want %#v", gotUsers, wantUsers)
	}
	if gotText, want := got.result.FinalText(), "final follow-up"; gotText != want {
		t.Fatalf("FinalText() = %q, want %q", gotText, want)
	}
	if gotRequests := len(provider.Requests()); gotRequests != 5 {
		t.Fatalf(
			"Provider requests = %d, want initial, overflow, summary, continuation, final",
			gotRequests,
		)
	}
}

func TestSessionFollowUpRacingFinalSealIsConsumedOrRejected(t *testing.T) {
	for iteration := range 50 {
		started := make(chan struct{})
		release := make(chan struct{})
		provider := newScriptedRecoveryProvider(
			gatedEventFactory(started, release, ai.DoneEvent{
				Message: textAssistant("initial answer"),
			}),
			eventStreamFactory(ai.DoneEvent{Message: textAssistant("follow-up answer")}),
		)
		session := newLifecycleSession(t, provider, func() error { return nil })

		returned := make(chan sessionAdvanceReturn, 1)
		go func() {
			result, err := session.Advance(context.Background(), "initial")
			returned <- sessionAdvanceReturn{result: result, err: err}
		}()
		<-started

		race := make(chan struct{})
		admissionReturned := make(chan error, 1)
		go func() {
			<-race
			admissionReturned <- session.FollowUp("racing follow-up")
		}()
		go func() {
			<-race
			close(release)
		}()
		close(race)

		admissionErr := receiveError(t, admissionReturned)
		got := receiveAdvance(t, returned)
		if got.err != nil {
			t.Fatalf("iteration %d Advance() error = %v", iteration, got.err)
		}
		users := userMessageTexts(got.result.History)
		switch {
		case admissionErr == nil:
			if want := []string{"initial", "racing follow-up"}; !reflect.DeepEqual(users, want) {
				t.Fatalf(
					"iteration %d accepted users = %#v, want %#v",
					iteration,
					users,
					want,
				)
			}
		case errors.Is(admissionErr, ErrFollowUpUnavailable):
			if want := []string{"initial"}; !reflect.DeepEqual(users, want) {
				t.Fatalf(
					"iteration %d rejected users = %#v, want %#v",
					iteration,
					users,
					want,
				)
			}
		default:
			t.Fatalf("iteration %d FollowUp() error = %v", iteration, admissionErr)
		}
		if len(got.result.UnconsumedFollowUps) != 0 {
			t.Fatalf(
				"iteration %d UnconsumedFollowUps = %#v, want empty",
				iteration,
				got.result.UnconsumedFollowUps,
			)
		}
		if err := session.Close(context.Background()); err != nil {
			t.Fatalf("iteration %d Close() error = %v", iteration, err)
		}
	}
}

func gatedEventFactory(
	started chan<- struct{},
	release <-chan struct{},
	event ai.Event,
) recoveryStreamFactory {
	return func(context.Context) ai.Stream {
		close(started)
		return &gatedRecoveryStream{release: release, event: event}
	}
}

func userMessageTexts(messages []ai.Message) []string {
	var texts []string
	for _, message := range messages {
		switch message := message.(type) {
		case ai.UserMessage:
			texts = append(texts, message.Content)
		case *ai.UserMessage:
			if message != nil {
				texts = append(texts, message.Content)
			}
		}
	}
	return texts
}
