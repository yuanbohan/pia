package coding

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/yuanbohan/pia/internal/agent"
	"github.com/yuanbohan/pia/internal/ai"
	"github.com/yuanbohan/pia/internal/observation"
)

var (
	// ErrSessionBusy means another Advance owns this Session.
	ErrSessionBusy = errors.New("coding: session is busy")
	// ErrSessionClosed means the Session is closing or closed.
	ErrSessionClosed = errors.New("coding: session is closed")
)

// SessionConfig contains the fixed DeepSeek coding profile inputs.
type SessionConfig struct {
	WorkspacePath  string
	DeepSeekAPIKey string
	Observer       observation.Observer
}

// SessionInfo is an ownership-independent snapshot of immutable Session
// composition data. It never contains Provider credentials.
type SessionInfo struct {
	WorkspacePath    string
	SystemPrompt     string
	Model            ModelInfo
	Tools            []ai.ToolSchema
	SkillDiagnostics []SkillDiagnostic
}

// AdvanceResult contains the complete authoritative Conversation History and
// admitted inputs handed back after one Advance settles.
type AdvanceResult struct {
	History            []ai.Message
	UnconsumedSteering []string
}

// FinalText concatenates text blocks from the last assistant message without
// exposing reasoning, tool calls, tool results, or earlier assistant text.
func (r AdvanceResult) FinalText() string {
	for index := len(r.History) - 1; index >= 0; index-- {
		var message ai.AssistantMessage
		switch candidate := r.History[index].(type) {
		case ai.AssistantMessage:
			message = candidate
		case *ai.AssistantMessage:
			if candidate == nil {
				continue
			}
			message = *candidate
		default:
			continue
		}

		var final strings.Builder
		for _, content := range message.Content {
			switch content := content.(type) {
			case ai.TextContent:
				final.WriteString(content.Text)
			case *ai.TextContent:
				if content != nil {
					final.WriteString(content.Text)
				}
			}
		}
		return final.String()
	}
	return ""
}

type sessionDependencies struct {
	Engine         *agent.Engine
	Provider       ai.Provider
	RequestLimits  ai.RequestLimits
	Compaction     compactionPolicy
	Observer       observation.Observer
	Info           SessionInfo
	CloseWorkspace func() error
}

type sessionLifetime uint8

const (
	sessionOpen sessionLifetime = iota
	sessionClosing
	sessionClosed
)

type activeAdvance struct {
	ctx               context.Context
	cancel            context.CancelCauseFunc
	acceptingSteering bool
	pendingSteering   []string
}

// sealAndDetachSteering transfers pending Steering ownership to the caller.
// Session.mu must be held.
func (control *activeAdvance) sealAndDetachSteering() []string {
	control.acceptingSteering = false
	steering := control.pendingSteering
	control.pendingSteering = nil
	return steering
}

// Session is the sole long-lived owner of one coding Conversation, its
// Workspace resources, and its execution lifecycle.
type Session struct {
	mu        sync.Mutex
	lifetime  sessionLifetime
	active    *activeAdvance
	closeDone chan struct{}
	closeErr  error

	engine         *agent.Engine
	provider       ai.Provider
	requestLimits  ai.RequestLimits
	compaction     compactionPolicy
	observer       observation.Observer
	info           SessionInfo
	closeWorkspace func() error

	history    []ai.Message
	projection *compactionProjection
}

func newSession(dependencies sessionDependencies) (*Session, error) {
	if dependencies.Engine == nil {
		return nil, errors.New("coding: Agent execution engine is required")
	}
	if dependencies.CloseWorkspace == nil {
		return nil, errors.New("coding: Workspace close operation is required")
	}
	if err := dependencies.Compaction.validate(); err != nil {
		return nil, fmt.Errorf("coding: compaction policy: %w", err)
	}
	if dependencies.Compaction.enabled() {
		if dependencies.Provider == nil {
			return nil, errors.New("coding: compaction Provider is required")
		}
		if dependencies.RequestLimits.IsZero() {
			return nil, errors.New("coding: compaction request limits are required")
		}
		if err := dependencies.RequestLimits.Validate(); err != nil {
			return nil, fmt.Errorf("coding: compaction request limits: %w", err)
		}
	}

	info := cloneSessionInfo(dependencies.Info)
	return &Session{
		closeDone:      make(chan struct{}),
		engine:         dependencies.Engine,
		provider:       dependencies.Provider,
		requestLimits:  dependencies.RequestLimits,
		compaction:     dependencies.Compaction,
		observer:       dependencies.Observer,
		info:           info,
		closeWorkspace: dependencies.CloseWorkspace,
	}, nil
}

// Info returns immutable Session composition data without exposing ownership.
func (s *Session) Info() SessionInfo {
	return cloneSessionInfo(s.info)
}

// Advance accepts one initial input and settles every execution, commit, and
// observation it starts before returning.
func (s *Session) Advance(
	ctx context.Context,
	input string,
) (result AdvanceResult, err error) {
	control, executionCtx, history, err := s.beginAdvance(ctx, input)
	if err != nil {
		return AdvanceResult{History: history}, err
	}

	s.observer.Observe(observation.Advance{Phase: observation.PhaseStarted})
	var unconsumedSteering []string
	defer func() {
		result.History = s.historySnapshot()
		result.UnconsumedSteering = unconsumedSteering
		s.observer.Observe(observation.Advance{
			Phase:   observation.PhaseSettled,
			Outcome: observation.OutcomeFromError(err),
		})
		s.finishAdvance(control)
	}()

	if advanceErr := s.executeInput(executionCtx, control, input); advanceErr != nil {
		unconsumedSteering = s.stopSteering(control)
		return result, fmt.Errorf("coding: run Agent: %w", advanceErr)
	}
	unconsumedSteering, err = s.settleSuccessfulExecution(
		control,
		executionCtx,
	)
	if err != nil {
		return result, fmt.Errorf("coding: run Agent: %w", err)
	}
	return result, nil
}

func (s *Session) executeInput(
	ctx context.Context,
	control *activeAdvance,
	input string,
) error {
	if err := s.compactBeforeRun(ctx, input); err != nil {
		return err
	}

	workingContext, err := s.workingContextSnapshot()
	if err != nil {
		return fmt.Errorf("coding: derive Working Context: %w", err)
	}
	steering := s.openSteering(control)
	result, runErr := s.engine.Run(ctx, workingContext, input, steering)
	s.pauseSteering(control)
	historyStart := s.appendRun(result.NewMessages)
	if runErr == nil || !s.compaction.enabled() {
		return runErr
	}

	terminalOffset, ok := recoverableOverflowTerminal(result.NewMessages)
	if !ok {
		return runErr
	}
	if err := s.compactAfterOverflow(ctx, historyStart+terminalOffset); err != nil {
		return fmt.Errorf("coding: recover context overflow: %w", err)
	}

	workingContext, err = s.workingContextSnapshot()
	if err != nil {
		return fmt.Errorf("coding: derive recovery Working Context: %w", err)
	}
	steering = s.openSteering(control)
	continuation, continueErr := s.engine.Continue(ctx, workingContext, steering)
	s.pauseSteering(control)
	s.appendRun(continuation.NewMessages)
	if continueErr != nil {
		if _, overflowedAgain := recoverableOverflowTerminal(continuation.NewMessages); overflowedAgain {
			return fmt.Errorf(
				"coding: context overflow recovery exhausted: %w",
				continueErr,
			)
		}
		return fmt.Errorf("coding: continue after context overflow: %w", continueErr)
	}
	return nil
}

func (s *Session) beginAdvance(
	callerCtx context.Context,
	input string,
) (*activeAdvance, context.Context, []ai.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.lifetime != sessionOpen {
		return nil, nil, ai.CloneMessages(s.history), ErrSessionClosed
	}
	if s.active != nil {
		return nil, nil, ai.CloneMessages(s.history), ErrSessionBusy
	}
	if cause := context.Cause(callerCtx); cause != nil {
		return nil, nil, ai.CloneMessages(s.history), cause
	}
	if err := validateInput(input); err != nil {
		return nil, nil, ai.CloneMessages(s.history), err
	}

	executionCtx, cancel := context.WithCancelCause(callerCtx)
	control := &activeAdvance{
		ctx:    executionCtx,
		cancel: cancel,
	}
	s.active = control
	return control, executionCtx, nil, nil
}

func validateInput(input string) error {
	if strings.TrimSpace(input) == "" {
		return errors.New("coding: input is required")
	}
	return nil
}

func (s *Session) settleSuccessfulExecution(
	control *activeAdvance,
	ctx context.Context,
) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	unconsumedSteering := control.sealAndDetachSteering()
	if len(unconsumedSteering) == 0 {
		// A completed terminal wins over cancellation that arrived after the
		// Engine had already consumed or sealed every accepted Steering input.
		return nil, nil
	}

	return unconsumedSteering, context.Cause(ctx)
}

func (s *Session) stopSteering(
	control *activeAdvance,
) []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return control.sealAndDetachSteering()
}

func (s *Session) finishAdvance(control *activeAdvance) {
	control.cancel(nil)

	s.mu.Lock()
	if s.active == control {
		s.active = nil
	}
	shouldClose := s.lifetime == sessionClosing
	s.mu.Unlock()

	if shouldClose {
		s.finishClose()
	}
}

// Cancel requests cancellation of the sole active Advance and never waits.
func (s *Session) Cancel() {
	s.mu.Lock()
	if s.active != nil {
		s.active.acceptingSteering = false
		s.active.cancel(context.Canceled)
	}
	s.mu.Unlock()
}

// Close permanently stops admission, cancels active work, and waits within ctx
// for settlement and the unique Workspace close.
func (s *Session) Close(ctx context.Context) error {
	shouldClose := false

	s.mu.Lock()
	switch s.lifetime {
	case sessionOpen:
		s.lifetime = sessionClosing
		if s.active != nil {
			s.active.acceptingSteering = false
			s.active.cancel(context.Canceled)
		} else {
			shouldClose = true
		}
	case sessionClosed:
		err := s.closeErr
		s.mu.Unlock()
		return err
	}
	done := s.closeDone
	s.mu.Unlock()

	if shouldClose {
		s.finishClose()
	}
	if err := waitForCompletion(ctx, done); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closeErr
}

func (s *Session) finishClose() {
	closeErr := s.closeWorkspace()
	if closeErr != nil {
		closeErr = fmt.Errorf("coding: close workspace: %w", closeErr)
	}

	s.mu.Lock()
	s.closeErr = closeErr
	s.lifetime = sessionClosed
	close(s.closeDone)
	s.mu.Unlock()
}

func waitForCompletion(ctx context.Context, done <-chan struct{}) error {
	select {
	case <-done:
		return nil
	default:
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		select {
		case <-done:
			return nil
		default:
			return context.Cause(ctx)
		}
	}
}

func (s *Session) appendRun(newMessages []ai.Message) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	start := len(s.history)
	s.history = append(s.history, newMessages...)
	return start
}

func (s *Session) historySnapshot() []ai.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return ai.CloneMessages(s.history)
}

func (s *Session) workingContextSnapshot() ([]ai.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return projectedMessages(s.history, s.projection)
}

func (s *Session) compactionSnapshot() ([]ai.Message, *compactionProjection) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var projection *compactionProjection
	if s.projection != nil {
		copy := *s.projection
		copy.Excluded = slices.Clone(s.projection.Excluded)
		projection = &copy
	}
	return ai.CloneMessages(s.history), projection
}

func (s *Session) publishProjection(projection compactionProjection) {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := projection
	copy.Excluded = slices.Clone(projection.Excluded)
	s.projection = &copy
}

func cloneSessionInfo(info SessionInfo) SessionInfo {
	info.Tools = ai.CloneToolSchemas(info.Tools)
	info.SkillDiagnostics = append([]SkillDiagnostic(nil), info.SkillDiagnostics...)
	return info
}
