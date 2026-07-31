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

func TestSessionSteeringBatchesAtSafeBoundaryWithinSameRun(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	release := make(chan struct{})
	firstAssistant := textAssistant("first answer")
	finalAssistant := textAssistant("final answer")
	provider := newScriptedRecoveryProvider(
		gatedEventFactory(started, release, ai.DoneEvent{Message: firstAssistant}),
		eventStreamFactory(ai.DoneEvent{Message: finalAssistant}),
	)
	var events []observation.Event
	var eventsMu sync.Mutex
	session := newObservedSession(t, provider, func(event observation.Event) {
		eventsMu.Lock()
		events = append(events, event)
		eventsMu.Unlock()
	})
	t.Cleanup(func() {
		if err := session.Close(context.Background()); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	initialInputs := []string{"initial", "initial constraint"}
	returned := make(chan sessionAdvanceReturn, 1)
	go func() {
		result, err := session.Advance(
			context.Background(),
			initialInputs,
		)
		returned <- sessionAdvanceReturn{result: result, err: err}
	}()
	<-started
	initialInputs[0] = "caller mutation"

	steeringInputs := []string{"first correction", "second correction"}
	requireTrySteerAccepted(
		t,
		session,
		steeringInputs...,
	)
	steeringInputs[0] = "caller mutation"
	close(release)

	got := receiveAdvance(t, returned)
	if got.err != nil {
		t.Fatalf("Advance() error = %v", got.err)
	}
	wantUsers := []string{
		"initial",
		"initial constraint",
		"first correction",
		"second correction",
	}
	if users := userMessageTexts(got.result.History); !reflect.DeepEqual(users, wantUsers) {
		t.Fatalf("History user messages = %#v, want %#v", users, wantUsers)
	}

	requests := provider.Requests()
	if len(requests) != 2 {
		t.Fatalf("Provider request count = %d, want 2", len(requests))
	}
	if users := userMessageTexts(requests[1].Messages); !reflect.DeepEqual(users, wantUsers) {
		t.Fatalf("second request user messages = %#v, want %#v", users, wantUsers)
	}
	if !reflect.DeepEqual(requests[1].Messages[2], ai.Message(firstAssistant)) {
		t.Fatalf(
			"second request message 2 = %#v, want first assistant",
			requests[1].Messages[2],
		)
	}

	eventsMu.Lock()
	gotEvents := append([]observation.Event(nil), events...)
	eventsMu.Unlock()
	var runStarts int
	var userMessages int
	for _, event := range gotEvents {
		switch event := event.(type) {
		case observation.Run:
			if event.Phase == observation.PhaseStarted {
				runStarts++
			}
		case observation.Message:
			if event.Role == observation.MessageRoleUser {
				userMessages++
			}
		}
	}
	if runStarts != 1 {
		t.Fatalf("Run started events = %d, want 1", runStarts)
	}
	if userMessages != len(wantUsers) {
		t.Fatalf("user Message events = %d, want %d", userMessages, len(wantUsers))
	}
}

func TestSessionTrySteerDistinguishesUnavailableInvalidAndClosed(t *testing.T) {
	t.Parallel()

	runSettled := make(chan struct{})
	releaseSettlement := make(chan struct{})
	var settleOnce sync.Once
	observer := observation.Observer(func(event observation.Event) {
		run, ok := event.(observation.Run)
		if ok && run.Phase == observation.PhaseSettled {
			settleOnce.Do(func() { close(runSettled) })
			<-releaseSettlement
		}
	})
	provider := newScriptedRecoveryProvider(
		eventStreamFactory(ai.DoneEvent{Message: textAssistant("done")}),
	)
	session := newObservedSession(t, provider, observer)

	if accepted, err := session.TrySteer([]string{"idle"}); accepted || err != nil {
		t.Fatalf("idle TrySteer() = (%t, %v), want (false, nil)", accepted, err)
	}
	for _, inputs := range [][]string{
		nil,
		{"valid", " \t\n"},
	} {
		if accepted, err := session.TrySteer(inputs); accepted ||
			err == nil ||
			errors.Is(err, ErrSessionClosed) {
			t.Fatalf(
				"invalid TrySteer(%#v) = (%t, %v), want input validation error",
				inputs,
				accepted,
				err,
			)
		}
	}

	returned := make(chan sessionAdvanceReturn, 1)
	go func() {
		result, err := session.Advance(context.Background(), []string{"initial"})
		returned <- sessionAdvanceReturn{result: result, err: err}
	}()
	<-runSettled

	if accepted, err := session.TrySteer([]string{"too late"}); accepted || err != nil {
		t.Fatalf(
			"sealed TrySteer() = (%t, %v), want (false, nil)",
			accepted,
			err,
		)
	}
	close(releaseSettlement)
	if got := receiveAdvance(t, returned); got.err != nil {
		t.Fatalf("Advance() error = %v", got.err)
	}

	if err := session.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if accepted, err := session.TrySteer([]string{"closed"}); accepted ||
		!errors.Is(err, ErrSessionClosed) {
		t.Fatalf(
			"closed TrySteer() = (%t, %v), want (false, ErrSessionClosed)",
			accepted,
			err,
		)
	}
}

func TestSessionProviderFailureCommitsAcceptedUnconsumedSteering(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	release := make(chan struct{})
	failure := ai.AssistantMessage{
		StopReason:   ai.StopReasonError,
		ErrorMessage: "provider failed",
	}
	provider := newScriptedRecoveryProvider(
		gatedEventFactory(started, release, ai.ErrorEvent{Message: failure}),
	)
	session := newLifecycleSession(t, provider, func() error { return nil })
	t.Cleanup(func() {
		if err := session.Close(context.Background()); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	returned := make(chan sessionAdvanceReturn, 1)
	go func() {
		result, err := session.Advance(context.Background(), []string{"initial"})
		returned <- sessionAdvanceReturn{result: result, err: err}
	}()
	<-started

	requireTrySteerAccepted(
		t,
		session,
		[]string{"first correction", "second correction"}...,
	)
	close(release)

	got := receiveAdvance(t, returned)
	if got.err == nil {
		t.Fatal("Advance() error = nil, want Provider failure")
	}
	wantUsers := []string{"initial", "first correction", "second correction"}
	if users := userMessageTexts(got.result.History); !reflect.DeepEqual(
		users,
		wantUsers,
	) {
		t.Fatalf("History user messages = %#v, want %#v", users, wantUsers)
	}
}

func TestSessionProviderFailureDoesNotHandBackConsumedSteering(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	release := make(chan struct{})
	failure := ai.AssistantMessage{
		StopReason:   ai.StopReasonError,
		ErrorMessage: "steered turn failed",
	}
	provider := newScriptedRecoveryProvider(
		gatedEventFactory(
			started,
			release,
			ai.DoneEvent{Message: textAssistant("initial answer")},
		),
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
		result, err := session.Advance(context.Background(), []string{"initial"})
		returned <- sessionAdvanceReturn{result: result, err: err}
	}()
	<-started
	requireTrySteerAccepted(t, session, "consumed correction")
	close(release)

	got := receiveAdvance(t, returned)
	if got.err == nil {
		t.Fatal("Advance() error = nil, want Provider failure")
	}
	if want := []string{"initial", "consumed correction"}; !reflect.DeepEqual(
		userMessageTexts(got.result.History),
		want,
	) {
		t.Fatalf(
			"History user messages = %#v, want %#v",
			userMessageTexts(got.result.History),
			want,
		)
	}
	requests := provider.Requests()
	if len(requests) != 2 {
		t.Fatalf("Provider request count = %d, want 2", len(requests))
	}
	if users := userMessageTexts(requests[1].Messages); !reflect.DeepEqual(
		users,
		[]string{"initial", "consumed correction"},
	) {
		t.Fatalf("second request user messages = %#v", users)
	}
}

func TestSessionCancellationSealsAdmissionAndCommitsPendingSteering(t *testing.T) {
	t.Parallel()

	callerCause := errors.New("caller canceled")
	tests := []struct {
		name       string
		trigger    func(*Session, context.CancelCauseFunc) error
		wantCause  error
		wantClosed bool
	}{
		{
			name: "caller context",
			trigger: func(_ *Session, cancel context.CancelCauseFunc) error {
				cancel(callerCause)
				return nil
			},
			wantCause: callerCause,
		},
		{
			name: "Cancel",
			trigger: func(session *Session, _ context.CancelCauseFunc) error {
				session.Cancel()
				return nil
			},
			wantCause: context.Canceled,
		},
		{
			name: "Close",
			trigger: func(session *Session, _ context.CancelCauseFunc) error {
				return session.Close(context.Background())
			},
			wantCause:  context.Canceled,
			wantClosed: true,
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
			t.Cleanup(func() {
				if err := session.Close(context.Background()); err != nil {
					t.Errorf("Close() error = %v", err)
				}
			})
			ctx, cancel := context.WithCancelCause(context.Background())

			returned := make(chan sessionAdvanceReturn, 1)
			go func() {
				result, err := session.Advance(ctx, []string{"initial"})
				returned <- sessionAdvanceReturn{result: result, err: err}
			}()
			<-started
			requireTrySteerAccepted(t, session, "pending correction")

			if err := test.trigger(session, cancel); err != nil {
				t.Fatalf("control operation error = %v", err)
			}
			accepted, err := session.TrySteer([]string{"after cancellation"})
			if test.wantClosed {
				if accepted || !errors.Is(err, ErrSessionClosed) {
					t.Fatalf(
						"post-close TrySteer() = (%t, %v), want (false, ErrSessionClosed)",
						accepted,
						err,
					)
				}
			} else if accepted || err != nil {
				t.Fatalf(
					"post-cancel TrySteer() = (%t, %v), want (false, nil)",
					accepted,
					err,
				)
			}

			got := receiveAdvance(t, returned)
			if !errors.Is(got.err, test.wantCause) {
				t.Fatalf("Advance() error = %v, want %v", got.err, test.wantCause)
			}
			wantUsers := []string{"initial", "pending correction"}
			if users := userMessageTexts(got.result.History); !reflect.DeepEqual(
				users,
				wantUsers,
			) {
				t.Fatalf("History user messages = %#v, want %#v", users, wantUsers)
			}
		})
	}
}

func TestSessionCancelAfterSuccessfulTerminalCommitsPendingSteering(t *testing.T) {
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
		result, err := session.Advance(context.Background(), []string{"initial"})
		returned <- sessionAdvanceReturn{result: result, err: err}
	}()
	<-started
	requireTrySteerAccepted(t, session, "must not be consumed")

	session.Cancel()
	close(release)
	got := receiveAdvance(t, returned)
	if !errors.Is(got.err, context.Canceled) {
		t.Fatalf("Advance() error = %v, want context.Canceled", got.err)
	}
	wantUsers := []string{"initial", "must not be consumed"}
	if users := userMessageTexts(got.result.History); !reflect.DeepEqual(
		users,
		wantUsers,
	) {
		t.Fatalf("History user messages = %#v, want %#v", users, wantUsers)
	}
	if gotText, want := got.result.FinalText(), "current Run still completed"; gotText != want {
		t.Fatalf("FinalText() = %q, want %q", gotText, want)
	}
	if gotRequests := len(provider.Requests()); gotRequests != 1 {
		t.Fatalf("Provider requests = %d, want 1", gotRequests)
	}
}

func TestSessionSteeringSurvivesOverflowRecovery(t *testing.T) {
	t.Parallel()

	runStarted := make(chan struct{})
	releaseRun := make(chan struct{})
	compactionStarted := make(chan struct{})
	releaseCompaction := make(chan struct{})
	overflow := ai.AssistantMessage{
		StopReason:   ai.StopReasonError,
		ErrorMessage: "context length exceeded",
	}
	recovered := textAssistant("recovered")
	provider := newScriptedRecoveryProvider(
		eventStreamFactory(ai.DoneEvent{Message: textAssistant("earlier answer")}),
		gatedEventFactory(runStarted, releaseRun, ai.ErrorEvent{Message: overflow}),
		gatedEventFactory(
			compactionStarted,
			releaseCompaction,
			ai.DoneEvent{Message: textAssistant("checkpoint")},
		),
		eventStreamFactory(ai.DoneEvent{Message: recovered}),
	)
	session := newRecoveryTestSession(t, provider)
	t.Cleanup(func() {
		if err := session.Close(context.Background()); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	if _, err := session.Advance(context.Background(), []string{"earlier"}); err != nil {
		t.Fatalf("earlier Advance() error = %v", err)
	}

	returned := make(chan sessionAdvanceReturn, 1)
	go func() {
		result, err := session.Advance(context.Background(), []string{"initial"})
		returned <- sessionAdvanceReturn{result: result, err: err}
	}()
	<-runStarted
	requireTrySteerAccepted(t, session, "preserved correction")
	close(releaseRun)

	<-compactionStarted
	if accepted, err := session.TrySteer([]string{"during compaction"}); accepted || err != nil {
		t.Fatalf(
			"compaction TrySteer() = (%t, %v), want (false, nil)",
			accepted,
			err,
		)
	}
	close(releaseCompaction)

	got := receiveAdvance(t, returned)
	if got.err != nil {
		t.Fatalf("Advance() error = %v", got.err)
	}
	wantUsers := []string{"earlier", "initial", "preserved correction"}
	if users := userMessageTexts(got.result.History); !reflect.DeepEqual(users, wantUsers) {
		t.Fatalf("History user messages = %#v, want %#v", users, wantUsers)
	}

	requests := provider.Requests()
	if len(requests) != 4 {
		t.Fatalf("Provider request count = %d, want 4", len(requests))
	}
	users := userMessageTexts(requests[3].Messages)
	wantTail := []string{"initial", "preserved correction"}
	if len(users) < len(wantTail) ||
		!reflect.DeepEqual(users[len(users)-len(wantTail):], wantTail) {
		t.Fatalf(
			"recovery request user messages = %#v, want tail %#v",
			users,
			wantTail,
		)
	}
}

func TestSessionTrySteerRacingFinalSealIsConsumedOrUnavailable(t *testing.T) {
	for iteration := range 50 {
		started := make(chan struct{})
		release := make(chan struct{})
		provider := newScriptedRecoveryProvider(
			gatedEventFactory(started, release, ai.DoneEvent{
				Message: textAssistant("initial answer"),
			}),
			eventStreamFactory(ai.DoneEvent{Message: textAssistant("steered answer")}),
		)
		session := newLifecycleSession(t, provider, func() error { return nil })

		returned := make(chan sessionAdvanceReturn, 1)
		go func() {
			result, err := session.Advance(context.Background(), []string{"initial"})
			returned <- sessionAdvanceReturn{result: result, err: err}
		}()
		<-started

		race := make(chan struct{})
		admissionReturned := make(chan trySteerReturn, 1)
		go func() {
			<-race
			accepted, err := session.TrySteer([]string{"racing correction"})
			admissionReturned <- trySteerReturn{accepted: accepted, err: err}
		}()
		go func() {
			<-race
			close(release)
		}()
		close(race)

		admission := receiveTrySteer(t, admissionReturned)
		got := receiveAdvance(t, returned)
		if got.err != nil {
			t.Fatalf("iteration %d Advance() error = %v", iteration, got.err)
		}
		users := userMessageTexts(got.result.History)
		switch {
		case admission.accepted && admission.err == nil:
			if want := []string{"initial", "racing correction"}; !reflect.DeepEqual(users, want) {
				t.Fatalf(
					"iteration %d accepted users = %#v, want %#v",
					iteration,
					users,
					want,
				)
			}
		case !admission.accepted && admission.err == nil:
			if want := []string{"initial"}; !reflect.DeepEqual(users, want) {
				t.Fatalf(
					"iteration %d unavailable users = %#v, want %#v",
					iteration,
					users,
					want,
				)
			}
		default:
			t.Fatalf(
				"iteration %d TrySteer() = (%t, %v), want accepted or unavailable",
				iteration,
				admission.accepted,
				admission.err,
			)
		}
		if err := session.Close(context.Background()); err != nil {
			t.Fatalf("iteration %d Close() error = %v", iteration, err)
		}
	}
}

func requireTrySteerAccepted(t *testing.T, session *Session, inputs ...string) {
	t.Helper()

	accepted, err := session.TrySteer(inputs)
	if !accepted || err != nil {
		t.Fatalf("TrySteer(%q) = (%t, %v), want (true, nil)", inputs, accepted, err)
	}
}

type trySteerReturn struct {
	accepted bool
	err      error
}

func receiveTrySteer(
	t *testing.T,
	returned <-chan trySteerReturn,
) trySteerReturn {
	t.Helper()

	select {
	case result := <-returned:
		return result
	case <-time.After(2 * time.Second):
		t.Fatal("TrySteer did not settle")
		return trySteerReturn{}
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
