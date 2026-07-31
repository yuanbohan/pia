package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/yuanbohan/pia/internal/ai"
	"github.com/yuanbohan/pia/internal/observation"
)

// Run appends one or more user inputs to an ownership-independent Working
// Context and continues Provider turns until the model stops or execution
// fails. Each input remains a separate user message.
func (e *Engine) Run(
	ctx context.Context,
	workingContext []ai.Message,
	userInputs []string,
	steering SteeringSource,
) (result RunResult, err error) {
	if steering == nil {
		return RunResult{}, errors.New("agent: steering source is required")
	}
	if cause := context.Cause(ctx); cause != nil {
		return RunResult{}, cause
	}
	if err := validateUserInputs(userInputs); err != nil {
		return RunResult{}, err
	}
	execution := newExecution(e, workingContext, steering)
	runStart := len(execution.workingContext)

	e.observer.Observe(observation.Run{
		Phase: observation.PhaseStarted,
		Mode:  observation.RunModeInput,
	})
	execution.appendUserInputs(userInputs)
	defer func() {
		e.observer.Observe(observation.Run{
			Phase:   observation.PhaseSettled,
			Outcome: observation.OutcomeFromError(err),
		})
	}()
	if _, err := execution.consumeSteering(ctx, steering.Drain); err != nil {
		return execution.snapshotRun(runStart), err
	}
	return execution.executeRun(ctx, runStart)
}

func validateUserInputs(inputs []string) error {
	if len(inputs) == 0 {
		return errors.New("agent: at least one user input is required")
	}
	for index, input := range inputs {
		if strings.TrimSpace(input) == "" {
			return fmt.Errorf("agent: user input %d is required", index)
		}
	}
	return nil
}

// Continue resumes Provider turns from an existing user or paired tool-result
// tail without appending another user message.
func (e *Engine) Continue(
	ctx context.Context,
	workingContext []ai.Message,
	steering SteeringSource,
) (result RunResult, err error) {
	if steering == nil {
		return RunResult{}, errors.New("agent: steering source is required")
	}
	if cause := context.Cause(ctx); cause != nil {
		return RunResult{}, cause
	}
	execution := newExecution(e, workingContext, steering)
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
	if _, err := execution.consumeSteering(ctx, steering.Drain); err != nil {
		return execution.snapshotRun(runStart), err
	}
	return execution.executeRun(ctx, runStart)
}

func (e *execution) executeRun(ctx context.Context, runStart int) (RunResult, error) {
	for {
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
			consumed, err := e.consumeFinalSteering(ctx)
			if err != nil {
				return e.snapshotRun(runStart), err
			}
			if !consumed {
				return e.snapshotRun(runStart), nil
			}
			continue
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
		if _, err := e.consumeSteering(ctx, e.steering.Drain); err != nil {
			return e.snapshotRun(runStart), err
		}
	}
}

func (e *execution) consumeSteering(
	ctx context.Context,
	drain func() []string,
) (bool, error) {
	if cause := context.Cause(ctx); cause != nil {
		return false, cause
	}
	inputs := drain()
	e.appendSteering(inputs)
	if cause := context.Cause(ctx); cause != nil {
		return len(inputs) > 0, cause
	}
	return len(inputs) > 0, nil
}

func (e *execution) appendSteering(inputs []string) {
	e.appendUserInputs(inputs)
}

func (e *execution) appendUserInputs(inputs []string) {
	for _, input := range inputs {
		e.workingContext = append(
			e.workingContext,
			ai.UserMessage{Content: strings.Clone(input)},
		)
		e.engine.observer.Observe(observation.Message{
			Role: observation.MessageRoleUser,
		})
	}
}

func (e *execution) consumeFinalSteering(ctx context.Context) (bool, error) {
	inputs := e.steering.DrainOrSeal()
	e.appendSteering(inputs)
	if len(inputs) == 0 {
		return false, nil
	}
	if cause := context.Cause(ctx); cause != nil {
		return true, cause
	}
	return true, nil
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
