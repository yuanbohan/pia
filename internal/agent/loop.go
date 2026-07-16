package agent

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/yuanbohan/pi-go/internal/ai"
)

// Run appends one user input and completes one Provider turn.
func (a *Agent) Run(ctx context.Context, userInput string) (RunResult, error) {
	request, earlyResult, err := a.beginRun(ctx, userInput)
	if err != nil {
		return earlyResult, err
	}
	defer a.endRun()

	stream := a.provider.Stream(ctx, request)
	message, runErr := receiveAssistant(ctx, stream)
	return a.appendTerminalAndSnapshot(message), runErr
}

func (a *Agent) beginRun(ctx context.Context, userInput string) (ai.Request, RunResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.active {
		return ai.Request{}, RunResult{}, ErrRunActive
	}
	if cause := context.Cause(ctx); cause != nil {
		return ai.Request{}, RunResult{Transcript: ai.CloneMessages(a.transcript)}, cause
	}

	a.active = true
	a.transcript = append(a.transcript, ai.UserMessage{Content: userInput})
	request := ai.CloneRequest(ai.Request{
		SystemPrompt: a.systemPrompt,
		Messages:     a.transcript,
	})
	return request, RunResult{}, nil
}

func (a *Agent) appendTerminalAndSnapshot(message ai.AssistantMessage) RunResult {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.transcript = append(a.transcript, ai.CloneAssistantMessage(message))
	return RunResult{Transcript: ai.CloneMessages(a.transcript)}
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
			// Formation events are not authoritative transcript entries.
		}
	}
}

func assistantFromDone(message ai.AssistantMessage) (ai.AssistantMessage, error) {
	switch message.StopReason {
	case ai.StopReasonStop, ai.StopReasonLength:
		if hasToolCall(message) {
			return message, errors.New("agent: provider requested a tool before the tool loop is available")
		}
		return message, nil
	case ai.StopReasonToolUse:
		return message, errors.New("agent: provider requested a tool before the tool loop is available")
	default:
		return assistantFromProtocolError(fmt.Sprintf("done event has invalid stop reason %q", message.StopReason))
	}
}

func assistantFromErrorEvent(ctx context.Context, message ai.AssistantMessage) (ai.AssistantMessage, error) {
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

func hasToolCall(message ai.AssistantMessage) bool {
	for _, content := range message.Content {
		switch content.(type) {
		case ai.ToolCall, *ai.ToolCall:
			return true
		}
	}
	return false
}
