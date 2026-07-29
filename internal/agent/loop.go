package agent

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/yuanbohan/pia/internal/ai"
	"github.com/yuanbohan/pia/internal/observation"
)

// Run appends one user input to an ownership-independent Working Context and
// continues Provider turns until the model stops or execution fails.
func (e *Engine) Run(
	ctx context.Context,
	workingContext []ai.Message,
	userInput string,
) (result RunResult, err error) {
	if cause := context.Cause(ctx); cause != nil {
		return RunResult{}, cause
	}
	execution := newExecution(e, workingContext)
	runStart := len(execution.workingContext)
	execution.workingContext = append(
		execution.workingContext,
		ai.UserMessage{Content: userInput},
	)

	e.observer.Observe(observation.Run{
		Phase: observation.PhaseStarted,
		Mode:  observation.RunModeInput,
	})
	e.observer.Observe(observation.Message{Role: observation.MessageRoleUser})
	defer func() {
		e.observer.Observe(observation.Run{
			Phase:   observation.PhaseSettled,
			Outcome: observation.OutcomeFromError(err),
		})
	}()
	return execution.executeRun(ctx, runStart)
}

// Continue resumes Provider turns from an existing user or paired tool-result
// tail without appending another user message.
func (e *Engine) Continue(
	ctx context.Context,
	workingContext []ai.Message,
) (result RunResult, err error) {
	if cause := context.Cause(ctx); cause != nil {
		return RunResult{}, cause
	}
	execution := newExecution(e, workingContext)
	if err := validateContinuationContext(execution.workingContext); err != nil {
		return RunResult{}, err
	}
	runStart := len(execution.workingContext)

	e.observer.Observe(observation.Run{
		Phase: observation.PhaseStarted,
		Mode:  observation.RunModeContinuation,
	})
	defer func() {
		e.observer.Observe(observation.Run{
			Phase:   observation.PhaseSettled,
			Outcome: observation.OutcomeFromError(err),
		})
	}()
	return execution.executeRun(ctx, runStart)
}

func (e *execution) executeRun(ctx context.Context, runStart int) (RunResult, error) {
	continuingAfterTools := false
	for {
		if continuingAfterTools {
			if cause := context.Cause(ctx); cause != nil {
				return e.snapshotRun(runStart), cause
			}
		}

		e.engine.observer.Observe(observation.Turn{Phase: observation.PhaseStarted})
		request := e.requestSnapshot()
		message, turnErr := receiveAssistant(ctx, e.engine.provider.Stream(ctx, request))
		e.appendAssistant(message)
		e.engine.observeAssistant(message)
		calls := toolCalls(message)
		if turnErr != nil {
			if len(calls) > 0 {
				results := failedTurnToolResults(calls, message.StopReason)
				e.appendToolResults(results)
				e.engine.observeToolResultMessages(results)
			}
			e.engine.observer.Observe(observation.Turn{
				Phase:   observation.PhaseSettled,
				Outcome: observation.OutcomeError,
			})
			return e.snapshotRun(runStart), turnErr
		}

		if len(calls) == 0 {
			e.engine.observer.Observe(observation.Turn{
				Phase:   observation.PhaseSettled,
				Outcome: observation.OutcomeSuccess,
			})
			return e.snapshotRun(runStart), nil
		}

		var results []ai.ToolResultMessage
		var runErr error
		if message.StopReason == ai.StopReasonLength {
			results = truncatedToolResults(calls)
		} else {
			results, runErr = e.engine.executeToolBatch(ctx, calls)
		}
		e.appendToolResults(results)
		e.engine.observeToolResultMessages(results)
		if runErr != nil {
			e.engine.observer.Observe(observation.Turn{
				Phase:   observation.PhaseSettled,
				Outcome: observation.OutcomeError,
			})
			return e.snapshotRun(runStart), runErr
		}
		e.engine.observer.Observe(observation.Turn{
			Phase:   observation.PhaseSettled,
			Outcome: observation.OutcomeSuccess,
		})
		continuingAfterTools = true
	}
}

func (e *Engine) observeAssistant(message ai.AssistantMessage) {
	e.observer.Observe(observation.Message{
		Role:       observation.MessageRoleAssistant,
		StopReason: message.StopReason,
	})
}

func (e *Engine) observeToolResultMessages(results []ai.ToolResultMessage) {
	for _, result := range results {
		e.observer.Observe(observation.Message{
			Role:    observation.MessageRoleToolResult,
			IsError: result.IsError,
		})
	}
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
