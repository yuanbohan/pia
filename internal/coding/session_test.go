package coding

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yuanbohan/pia/internal/agent"
	"github.com/yuanbohan/pia/internal/ai"
	"github.com/yuanbohan/pia/internal/ai/provider/faux"
)

func TestSessionAdvancesCommitHistoryAndDeriveNextWorkingContext(t *testing.T) {
	t.Parallel()

	firstAssistant := textAssistant("first answer")
	secondAssistant := textAssistant("second answer")
	provider := newCodingFaux(t,
		codingAssistantStep(firstAssistant),
		codingAssistantStep(secondAssistant),
	)
	session := newTestSession(t, provider)
	t.Cleanup(func() {
		if err := session.Close(context.Background()); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	first, err := session.Advance(context.Background(), []string{"first question"})
	if err != nil {
		t.Fatalf("first Advance() error = %v", err)
	}
	first.History[0] = ai.UserMessage{Content: "caller mutation"}

	second, err := session.Advance(context.Background(), []string{"second question"})
	if err != nil {
		t.Fatalf("second Advance() error = %v", err)
	}
	wantHistory := []ai.Message{
		ai.UserMessage{Content: "first question"},
		firstAssistant,
		ai.UserMessage{Content: "second question"},
		secondAssistant,
	}
	if !reflect.DeepEqual(second.History, wantHistory) {
		t.Fatalf("second History = %#v, want %#v", second.History, wantHistory)
	}
	requests := provider.Requests()
	if got, want := requests[1].Messages, wantHistory[:3]; !reflect.DeepEqual(got, want) {
		t.Fatalf("second Provider messages = %#v, want %#v", got, want)
	}
}

func TestSessionRejectsInvalidAdvanceWithoutChangingHistory(t *testing.T) {
	t.Parallel()

	provider := newCodingFaux(t)
	session := newTestSession(t, provider)

	for _, inputs := range [][]string{
		nil,
		{"valid", " \t\n"},
	} {
		result, advanceErr := session.Advance(context.Background(), inputs)
		if advanceErr == nil || len(result.History) != 0 {
			t.Fatalf(
				"invalid Advance(%#v) = (%#v, %v), want unchanged History and error",
				inputs,
				result,
				advanceErr,
			)
		}
	}

	ctx, cancel := context.WithCancelCause(context.Background())
	cancelErr := errors.New("caller already canceled")
	cancel(cancelErr)
	canceled, err := session.Advance(ctx, []string{"not accepted"})
	if !errors.Is(err, cancelErr) || len(canceled.History) != 0 {
		t.Fatalf("pre-canceled Advance = (%#v, %v), want unchanged History and cause", canceled, err)
	}
	if got := len(provider.Requests()); got != 0 {
		t.Fatalf("Provider requests = %d, want 0", got)
	}
}

func TestSessionInfoIsOwnershipIndependent(t *testing.T) {
	t.Parallel()

	provider := newCodingFaux(t)
	engine, err := agent.New(agent.Config{Provider: provider})
	if err != nil {
		t.Fatalf("agent.New() error = %v", err)
	}
	parameters := []byte(`{"type":"object"}`)
	dependencies := sessionDependencies{
		Engine: engine,
		Info: SessionInfo{
			WorkspacePath: "/workspace",
			Tools: []ai.ToolSchema{{
				Name:       "read",
				Parameters: parameters,
			}},
			SkillDiagnostics: []SkillDiagnostic{{Message: "warning"}},
		},
		CloseWorkspace: func() error { return nil },
	}
	session, err := newSession(dependencies)
	if err != nil {
		t.Fatalf("newSession() error = %v", err)
	}

	parameters[0] = '['
	first := session.Info()
	first.Tools[0].Parameters[0] = '['
	first.SkillDiagnostics[0].Message = "caller mutation"
	second := session.Info()
	if got, want := string(second.Tools[0].Parameters), `{"type":"object"}`; got != want {
		t.Fatalf("Info tool parameters = %q, want %q", got, want)
	}
	if got, want := second.SkillDiagnostics[0].Message, "warning"; got != want {
		t.Fatalf("Info diagnostic = %q, want %q", got, want)
	}
}

func TestNewSessionRequiresWorkspaceCloseOperation(t *testing.T) {
	t.Parallel()

	provider := newCodingFaux(t)
	engine, err := agent.New(agent.Config{Provider: provider})
	if err != nil {
		t.Fatalf("agent.New() error = %v", err)
	}
	if _, err := newSession(sessionDependencies{Engine: engine}); err == nil {
		t.Fatal("newSession() error = nil, want missing Workspace close operation")
	}
}

func TestSessionCancelReturnsWithoutWaitingForUncooperativeProvider(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	release := make(chan struct{})
	provider := newScriptedRecoveryProvider(func(context.Context) ai.Stream {
		close(started)
		return &gatedRecoveryStream{
			release: release,
			event:   ai.DoneEvent{Message: textAssistant("completed while cancellation was pending")},
		}
	})
	session := newLifecycleSession(t, provider, func() error { return nil })
	returned := make(chan error, 1)
	go func() {
		_, err := session.Advance(context.Background(), []string{"wait"})
		returned <- err
	}()
	<-started

	session.Cancel()
	select {
	case err := <-returned:
		t.Fatalf("Advance settled during Cancel() with error %v, want Cancel to return independently", err)
	default:
	}

	close(release)
	if err := receiveError(t, returned); err != nil {
		t.Fatalf("Advance() error = %v, want completed Provider result", err)
	}
}

func TestSessionCancelCommitsAbortedTerminalAndRemainsReusable(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	provider := newScriptedRecoveryProvider(
		func(ctx context.Context) ai.Stream {
			return &cancelBlockingStream{ctx: ctx, started: started}
		},
		eventStreamFactory(ai.DoneEvent{Message: textAssistant("second answer")}),
	)
	session := newLifecycleSession(t, provider, func() error { return nil })
	returned := make(chan sessionAdvanceReturn, 1)
	go func() {
		result, err := session.Advance(context.Background(), []string{"first input"})
		returned <- sessionAdvanceReturn{result: result, err: err}
	}()
	<-started
	session.Cancel()

	first := receiveAdvance(t, returned)
	if !errors.Is(first.err, context.Canceled) {
		t.Fatalf("canceled Advance error = %v, want context.Canceled", first.err)
	}
	if got, want := len(first.result.History), 2; got != want {
		t.Fatalf("canceled History length = %d, want %d", got, want)
	}
	terminal, ok := first.result.History[1].(ai.AssistantMessage)
	if !ok || terminal.StopReason != ai.StopReasonAborted {
		t.Fatalf("canceled terminal = %#v, want aborted assistant", first.result.History[1])
	}

	second, err := session.Advance(context.Background(), []string{"second input"})
	if err != nil {
		t.Fatalf("second Advance() error = %v", err)
	}
	if got, want := second.FinalText(), "second answer"; got != want {
		t.Fatalf("second FinalText() = %q, want %q", got, want)
	}
}

func TestSessionCloseIdleRunsUniqueCleanupAndSharesItsError(t *testing.T) {
	t.Parallel()

	closeErr := errors.New("close sentinel")
	var closeCalls atomic.Int32
	session := newLifecycleSession(t, newCodingFaux(t), func() error {
		closeCalls.Add(1)
		return closeErr
	})

	const callers = 8
	var group sync.WaitGroup
	group.Add(callers)
	returned := make(chan error, callers)
	for range callers {
		go func() {
			defer group.Done()
			returned <- session.Close(context.Background())
		}()
	}
	group.Wait()
	close(returned)
	for err := range returned {
		if !errors.Is(err, closeErr) {
			t.Fatalf("Close() error = %v, want shared cleanup error", err)
		}
	}
	if got := closeCalls.Load(); got != 1 {
		t.Fatalf("Workspace close calls = %d, want 1", got)
	}
	if _, err := session.Advance(context.Background(), []string{"not accepted"}); !errors.Is(err, ErrSessionClosed) {
		t.Fatalf("Advance() after Close error = %v, want ErrSessionClosed", err)
	}
}

func TestSessionCloseBusyCancelsAdmissionAndHonorsCallerDeadline(t *testing.T) {
	t.Parallel()

	provider := &blockingCodingProvider{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	closeErr := errors.New("close sentinel")
	var closeCalls atomic.Int32
	session := newLifecycleSession(t, provider, func() error {
		closeCalls.Add(1)
		return closeErr
	})
	advanceReturned := make(chan error, 1)
	go func() {
		_, err := session.Advance(context.Background(), []string{"wait"})
		advanceReturned <- err
	}()
	<-provider.started

	closeCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := session.Close(closeCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("bounded Close() error = %v, want deadline", err)
	}
	if got := closeCalls.Load(); got != 0 {
		t.Fatalf("Workspace closed %d times before Advance settlement", got)
	}
	if _, err := session.Advance(context.Background(), []string{"must be rejected"}); !errors.Is(err, ErrSessionClosed) {
		t.Fatalf("Advance() while closing error = %v, want ErrSessionClosed", err)
	}
	if accepted, err := session.TrySteer([]string{"must also be rejected"}); accepted ||
		!errors.Is(err, ErrSessionClosed) {
		t.Fatalf(
			"TrySteer() while closing = (%t, %v), want (false, ErrSessionClosed)",
			accepted,
			err,
		)
	}

	close(provider.release)
	_ = receiveError(t, advanceReturned)
	if err := session.Close(context.Background()); !errors.Is(err, closeErr) {
		t.Fatalf("settled Close() error = %v, want cleanup error", err)
	}
	if got := closeCalls.Load(); got != 1 {
		t.Fatalf("Workspace close calls = %d, want 1", got)
	}
}

func TestSessionCloseReturnsCleanupResultNotAdvanceFailure(t *testing.T) {
	t.Parallel()

	provider := &cancelAwareProvider{started: make(chan struct{})}
	session := newLifecycleSession(t, provider, func() error { return nil })
	advanceReturned := make(chan error, 1)
	go func() {
		_, err := session.Advance(context.Background(), []string{"wait"})
		advanceReturned <- err
	}()
	<-provider.started

	if err := session.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v, want only cleanup result", err)
	}
	if err := receiveError(t, advanceReturned); !errors.Is(err, context.Canceled) {
		t.Fatalf("Advance() error = %v, want cancellation", err)
	}
}

func (s *Session) advanceHistory(ctx context.Context, input string) ([]ai.Message, error) {
	result, err := s.Advance(ctx, []string{input})
	return result.History, err
}

func newTestSession(t *testing.T, provider *faux.Provider) *Session {
	t.Helper()
	engine, err := agent.New(agent.Config{Provider: provider, SystemPrompt: "system"})
	if err != nil {
		t.Fatalf("agent.New() error = %v", err)
	}
	session, err := newSession(sessionDependencies{
		Engine:         engine,
		CloseWorkspace: func() error { return nil },
	})
	if err != nil {
		t.Fatalf("newSession() error = %v", err)
	}
	return session
}

func newLifecycleSession(
	t *testing.T,
	provider ai.Provider,
	closeWorkspace func() error,
) *Session {
	t.Helper()
	engine, err := agent.New(agent.Config{Provider: provider, SystemPrompt: "system"})
	if err != nil {
		t.Fatalf("agent.New() error = %v", err)
	}
	session, err := newSession(sessionDependencies{
		Engine:         engine,
		CloseWorkspace: closeWorkspace,
	})
	if err != nil {
		t.Fatalf("newSession() error = %v", err)
	}
	return session
}

func receiveError(t *testing.T, returned <-chan error) error {
	t.Helper()
	select {
	case err := <-returned:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("operation did not settle")
		return nil
	}
}

func receiveAdvance(
	t *testing.T,
	returned <-chan sessionAdvanceReturn,
) sessionAdvanceReturn {
	t.Helper()
	select {
	case result := <-returned:
		return result
	case <-time.After(2 * time.Second):
		t.Fatal("Advance did not settle")
		return sessionAdvanceReturn{}
	}
}

type sessionAdvanceReturn struct {
	result AdvanceResult
	err    error
}
