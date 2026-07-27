package agent

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/yuanbohan/pia/internal/ai"
	"github.com/yuanbohan/pia/internal/observation"
)

// Run appends one user input and continues Provider turns until the model no
// longer requests tools or the Run reaches a Provider/cancellation failure.
func (a *Agent) Run(ctx context.Context, userInput string) (result RunResult, err error) {
	runStart, err := a.beginRun(ctx, userInput)
	if err != nil {
		return RunResult{}, err
	}
	defer a.endRun()

	a.observer.Observe(observation.Run{
		Phase: observation.PhaseStarted,
		Mode:  observation.RunModeInput,
	})
	a.observer.Observe(observation.Message{Role: observation.MessageRoleUser})
	defer func() {
		a.observer.Observe(observation.Run{
			Phase:   observation.PhaseSettled,
			Outcome: observation.OutcomeFromError(err),
		})
	}()
	return a.executeRun(ctx, runStart)
}

// Continue resumes Provider turns from an existing user or paired tool-result
// tail without appending another user message.
func (a *Agent) Continue(ctx context.Context) (result RunResult, err error) {
	runStart, err := a.beginContinue(ctx)
	if err != nil {
		return RunResult{}, err
	}
	defer a.endRun()

	a.observer.Observe(observation.Run{
		Phase: observation.PhaseStarted,
		Mode:  observation.RunModeContinuation,
	})
	defer func() {
		a.observer.Observe(observation.Run{
			Phase:   observation.PhaseSettled,
			Outcome: observation.OutcomeFromError(err),
		})
	}()
	return a.executeRun(ctx, runStart)
}

func (a *Agent) executeRun(ctx context.Context, runStart int) (RunResult, error) {
	continuingAfterTools := false
	for {
		if continuingAfterTools {
			if cause := context.Cause(ctx); cause != nil {
				return a.snapshotRun(runStart), cause
			}
		}

		a.observer.Observe(observation.Turn{Phase: observation.PhaseStarted})
		request := a.requestSnapshot()
		message, turnErr := receiveAssistant(ctx, a.provider.Stream(ctx, request))
		a.appendAssistant(message)
		a.observeAssistant(message)
		calls := toolCalls(message)
		if turnErr != nil {
			if len(calls) > 0 {
				results := failedTurnToolResults(calls, message.StopReason)
				a.appendToolResults(results)
				a.observeToolResultMessages(results)
			}
			a.observer.Observe(observation.Turn{
				Phase:   observation.PhaseSettled,
				Outcome: observation.OutcomeError,
			})
			return a.snapshotRun(runStart), turnErr
		}

		if len(calls) == 0 {
			a.observer.Observe(observation.Turn{
				Phase:   observation.PhaseSettled,
				Outcome: observation.OutcomeSuccess,
			})
			return a.snapshotRun(runStart), nil
		}

		var results []ai.ToolResultMessage
		var runErr error
		if message.StopReason == ai.StopReasonLength {
			results = truncatedToolResults(calls)
		} else {
			results, runErr = a.executeToolBatch(ctx, calls)
		}
		a.appendToolResults(results)
		a.observeToolResultMessages(results)
		if runErr != nil {
			a.observer.Observe(observation.Turn{
				Phase:   observation.PhaseSettled,
				Outcome: observation.OutcomeError,
			})
			return a.snapshotRun(runStart), runErr
		}
		a.observer.Observe(observation.Turn{
			Phase:   observation.PhaseSettled,
			Outcome: observation.OutcomeSuccess,
		})
		continuingAfterTools = true
	}
}

func (a *Agent) observeAssistant(message ai.AssistantMessage) {
	a.observer.Observe(observation.Message{
		Role:       observation.MessageRoleAssistant,
		StopReason: message.StopReason,
	})
}

func (a *Agent) observeToolResultMessages(results []ai.ToolResultMessage) {
	for _, result := range results {
		a.observer.Observe(observation.Message{
			Role:    observation.MessageRoleToolResult,
			IsError: result.IsError,
		})
	}
}

func (a *Agent) beginRun(ctx context.Context, userInput string) (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.active {
		return 0, ErrRunActive
	}
	if cause := context.Cause(ctx); cause != nil {
		return 0, cause
	}

	a.active = true
	runStart := len(a.workingContext)
	a.workingContext = append(a.workingContext, ai.UserMessage{Content: userInput})
	return runStart, nil
}

func (a *Agent) beginContinue(ctx context.Context) (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.active {
		return 0, ErrRunActive
	}
	if cause := context.Cause(ctx); cause != nil {
		return 0, cause
	}
	if err := validateContinuationContext(a.workingContext); err != nil {
		return 0, err
	}

	a.active = true
	return len(a.workingContext), nil
}

func (a *Agent) endRun() {
	a.mu.Lock()
	a.active = false
	a.mu.Unlock()
}

func receiveAssistant(ctx context.Context, stream ai.Stream) (ai.AssistantMessage, error) {
	if stream == nil {
		return assistantFromProtocolError("provider returned a nil stream")
	}

	for {
		event, receiveErr := stream.Receive()

		switch event := event.(type) {
		case ai.DoneEvent:
			return assistantFromDone(event.Message)
		case *ai.DoneEvent:
			if event != nil {
				return assistantFromDone(event.Message)
			}
		case ai.ErrorEvent:
			return assistantFromErrorEvent(ctx, event.Message)
		case *ai.ErrorEvent:
			if event != nil {
				return assistantFromErrorEvent(ctx, event.Message)
			}
		}

		if receiveErr != nil {
			return assistantFromReceiveError(ctx, receiveErr)
		}

		switch event.(type) {
		case *ai.DoneEvent:
			return assistantFromProtocolError("provider returned a nil done event")
		case *ai.ErrorEvent:
			return assistantFromProtocolError("provider returned a nil error event")
		case nil:
			return assistantFromProtocolError("provider returned a nil event without an error")
		default:
			// Formation events are not authoritative Working Context messages.
		}
	}
}

func assistantFromDone(message ai.AssistantMessage) (ai.AssistantMessage, error) {
	switch message.StopReason {
	case ai.StopReasonStop, ai.StopReasonLength, ai.StopReasonToolUse:
		if err := validateToolCallProtocol(message); err != nil {
			return assistantFromProtocolError(err.Error())
		}
		return message, nil
	default:
		return assistantFromProtocolError(fmt.Sprintf("done event has invalid stop reason %q", message.StopReason))
	}
}

func assistantFromErrorEvent(ctx context.Context, message ai.AssistantMessage) (ai.AssistantMessage, error) {
	if message.StopReason == ai.StopReasonError || message.StopReason == ai.StopReasonAborted {
		if err := validateToolCallProtocol(message); err != nil {
			return assistantFromProtocolError(err.Error())
		}
	}
	switch message.StopReason {
	case ai.StopReasonError:
		if message.ErrorMessage == "" {
			return message, errors.New("agent: provider response failed")
		}
		return message, fmt.Errorf("agent: provider response failed: %s", message.ErrorMessage)
	case ai.StopReasonAborted:
		if cause := context.Cause(ctx); cause != nil {
			return message, cause
		}
		if message.ErrorMessage == "" {
			return message, errors.New("agent: provider aborted without a context cause")
		}
		return message, fmt.Errorf("agent: provider aborted without a context cause: %s", message.ErrorMessage)
	default:
		return assistantFromProtocolError(fmt.Sprintf("error event has invalid stop reason %q", message.StopReason))
	}
}

func assistantFromReceiveError(ctx context.Context, receiveErr error) (ai.AssistantMessage, error) {
	if cause := context.Cause(ctx); cause != nil {
		return ai.AssistantMessage{
			StopReason:   ai.StopReasonAborted,
			ErrorMessage: cause.Error(),
		}, cause
	}

	var err error
	if errors.Is(receiveErr, io.EOF) {
		err = errors.New("agent: provider stream ended before terminal event")
	} else {
		err = fmt.Errorf("agent: receive provider stream: %w", receiveErr)
	}
	return ai.AssistantMessage{
		StopReason:   ai.StopReasonError,
		ErrorMessage: err.Error(),
	}, err
}

func assistantFromProtocolError(message string) (ai.AssistantMessage, error) {
	err := fmt.Errorf("agent: provider protocol: %s", message)
	return ai.AssistantMessage{
		StopReason:   ai.StopReasonError,
		ErrorMessage: err.Error(),
	}, err
}
