package coding

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

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

	returned := make(chan sessionAdvanceReturn, 1)
	go func() {
		result, err := session.Advance(context.Background(), "initial")
		returned <- sessionAdvanceReturn{result: result, err: err}
	}()
	<-started

	if err := session.Steer("first correction"); err != nil {
		t.Fatalf("first Steer() error = %v", err)
	}
	if err := session.Steer("second correction"); err != nil {
		t.Fatalf("second Steer() error = %v", err)
	}
	close(release)

	got := receiveAdvance(t, returned)
	if got.err != nil {
		t.Fatalf("Advance() error = %v", got.err)
	}
	if len(got.result.UnconsumedSteering) != 0 {
		t.Fatalf(
			"UnconsumedSteering = %#v, want empty",
			got.result.UnconsumedSteering,
		)
	}
	wantUsers := []string{"initial", "first correction", "second correction"}
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
	if !reflect.DeepEqual(requests[1].Messages[1], ai.Message(firstAssistant)) {
		t.Fatalf(
			"second request message 1 = %#v, want first assistant",
			requests[1].Messages[1],
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

func TestSessionSteerAdmissionRejectsIdleBlankSealedAndClosed(t *testing.T) {
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

	if err := session.Steer("idle"); !errors.Is(err, ErrSteerUnavailable) {
		t.Fatalf("idle Steer() error = %v, want ErrSteerUnavailable", err)
	}
	if err := session.Steer(" \t\n"); err == nil ||
		errors.Is(err, ErrSteerUnavailable) ||
		errors.Is(err, ErrSessionClosed) {
		t.Fatalf("blank Steer() error = %v, want input validation error", err)
	}

	returned := make(chan sessionAdvanceReturn, 1)
	go func() {
		result, err := session.Advance(context.Background(), "initial")
		returned <- sessionAdvanceReturn{result: result, err: err}
	}()
	<-runSettled

	if err := session.Steer("too late"); !errors.Is(err, ErrSteerUnavailable) {
		t.Fatalf("sealed Steer() error = %v, want ErrSteerUnavailable", err)
	}
	close(releaseSettlement)
	if got := receiveAdvance(t, returned); got.err != nil {
		t.Fatalf("Advance() error = %v", got.err)
	}

	if err := session.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := session.Steer("closed"); !errors.Is(err, ErrSessionClosed) {
		t.Fatalf("closed Steer() error = %v, want ErrSessionClosed", err)
	}
}

func TestSessionProviderFailureHandsBackUnconsumedInputs(t *testing.T) {
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
		result, err := session.Advance(context.Background(), "initial")
		returned <- sessionAdvanceReturn{result: result, err: err}
	}()
	<-started

	if err := session.Steer("first correction"); err != nil {
		t.Fatalf("first Steer() error = %v", err)
	}
	if err := session.Steer("second correction"); err != nil {
		t.Fatalf("second Steer() error = %v", err)
	}
	if err := session.FollowUp("later task"); err != nil {
		t.Fatalf("FollowUp() error = %v", err)
	}
	close(release)

	got := receiveAdvance(t, returned)
	if got.err == nil {
		t.Fatal("Advance() error = nil, want Provider failure")
	}
	if want := []string{"first correction", "second correction"}; !reflect.DeepEqual(
		got.result.UnconsumedSteering,
		want,
	) {
		t.Fatalf(
			"UnconsumedSteering = %#v, want %#v",
			got.result.UnconsumedSteering,
			want,
		)
	}
	if want := []string{"later task"}; !reflect.DeepEqual(
		got.result.UnconsumedFollowUps,
		want,
	) {
		t.Fatalf(
			"UnconsumedFollowUps = %#v, want %#v",
			got.result.UnconsumedFollowUps,
			want,
		)
	}
	if users := userMessageTexts(got.result.History); !reflect.DeepEqual(
		users,
		[]string{"initial"},
	) {
		t.Fatalf("History user messages = %#v, want only initial", users)
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
		result, err := session.Advance(context.Background(), "initial")
		returned <- sessionAdvanceReturn{result: result, err: err}
	}()
	<-started
	if err := session.Steer("consumed correction"); err != nil {
		t.Fatalf("Steer() error = %v", err)
	}
	close(release)

	got := receiveAdvance(t, returned)
	if got.err == nil {
		t.Fatal("Advance() error = nil, want Provider failure")
	}
	if len(got.result.UnconsumedSteering) != 0 {
		t.Fatalf(
			"UnconsumedSteering = %#v, want empty",
			got.result.UnconsumedSteering,
		)
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

func TestSessionCancelSealsAdmissionAndHandsBackPendingInputs(t *testing.T) {
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

	returned := make(chan sessionAdvanceReturn, 1)
	go func() {
		result, err := session.Advance(context.Background(), "initial")
		returned <- sessionAdvanceReturn{result: result, err: err}
	}()
	<-started
	if err := session.Steer("pending correction"); err != nil {
		t.Fatalf("Steer() error = %v", err)
	}
	if err := session.FollowUp("pending follow-up"); err != nil {
		t.Fatalf("FollowUp() error = %v", err)
	}

	session.Cancel()
	if err := session.Steer("after cancel"); !errors.Is(err, ErrSteerUnavailable) {
		t.Fatalf("post-Cancel Steer() error = %v, want ErrSteerUnavailable", err)
	}
	if err := session.FollowUp("after cancel"); !errors.Is(err, ErrFollowUpUnavailable) {
		t.Fatalf("post-Cancel FollowUp() error = %v, want ErrFollowUpUnavailable", err)
	}

	got := receiveAdvance(t, returned)
	if !errors.Is(got.err, context.Canceled) {
		t.Fatalf("Advance() error = %v, want context.Canceled", got.err)
	}
	if want := []string{"pending correction"}; !reflect.DeepEqual(
		got.result.UnconsumedSteering,
		want,
	) {
		t.Fatalf(
			"UnconsumedSteering = %#v, want %#v",
			got.result.UnconsumedSteering,
			want,
		)
	}
	if want := []string{"pending follow-up"}; !reflect.DeepEqual(
		got.result.UnconsumedFollowUps,
		want,
	) {
		t.Fatalf(
			"UnconsumedFollowUps = %#v, want %#v",
			got.result.UnconsumedFollowUps,
			want,
		)
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
	if _, err := session.Advance(context.Background(), "earlier"); err != nil {
		t.Fatalf("earlier Advance() error = %v", err)
	}

	returned := make(chan sessionAdvanceReturn, 1)
	go func() {
		result, err := session.Advance(context.Background(), "initial")
		returned <- sessionAdvanceReturn{result: result, err: err}
	}()
	<-runStarted
	if err := session.Steer("preserved correction"); err != nil {
		t.Fatalf("Steer() error = %v", err)
	}
	close(releaseRun)

	<-compactionStarted
	if err := session.Steer("during compaction"); !errors.Is(err, ErrSteerUnavailable) {
		t.Fatalf(
			"compaction Steer() error = %v, want ErrSteerUnavailable",
			err,
		)
	}
	close(releaseCompaction)

	got := receiveAdvance(t, returned)
	if got.err != nil {
		t.Fatalf("Advance() error = %v", got.err)
	}
	if len(got.result.UnconsumedSteering) != 0 {
		t.Fatalf(
			"UnconsumedSteering = %#v, want empty",
			got.result.UnconsumedSteering,
		)
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

func TestSessionSteerRacingFinalSealIsConsumedOrRejected(t *testing.T) {
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
			result, err := session.Advance(context.Background(), "initial")
			returned <- sessionAdvanceReturn{result: result, err: err}
		}()
		<-started

		race := make(chan struct{})
		admissionReturned := make(chan error, 1)
		go func() {
			<-race
			admissionReturned <- session.Steer("racing correction")
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
			if want := []string{"initial", "racing correction"}; !reflect.DeepEqual(users, want) {
				t.Fatalf(
					"iteration %d accepted users = %#v, want %#v",
					iteration,
					users,
					want,
				)
			}
		case errors.Is(admissionErr, ErrSteerUnavailable):
			if want := []string{"initial"}; !reflect.DeepEqual(users, want) {
				t.Fatalf(
					"iteration %d rejected users = %#v, want %#v",
					iteration,
					users,
					want,
				)
			}
		default:
			t.Fatalf("iteration %d Steer() error = %v", iteration, admissionErr)
		}
		if len(got.result.UnconsumedSteering) != 0 {
			t.Fatalf(
				"iteration %d UnconsumedSteering = %#v, want empty",
				iteration,
				got.result.UnconsumedSteering,
			)
		}
		if err := session.Close(context.Background()); err != nil {
			t.Fatalf("iteration %d Close() error = %v", iteration, err)
		}
	}
}
