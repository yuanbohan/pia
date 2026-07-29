package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/yuanbohan/pia/internal/agent"
	"github.com/yuanbohan/pia/internal/ai"
	"github.com/yuanbohan/pia/internal/ai/provider/faux"
)

func TestNewRequiresProvider(t *testing.T) {
	t.Parallel()

	if _, err := agent.New(agent.Config{}); err == nil {
		t.Fatal("New() error = nil, want missing-provider error")
	}
}

func TestRunClampsEachRequestOutputToProjectedContext(t *testing.T) {
	t.Parallel()

	first := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "first"}},
		Usage:      ai.Usage{InputTokens: 80, OutputTokens: 5},
		StopReason: ai.StopReasonStop,
	}
	second := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "second"}},
		StopReason: ai.StopReasonStop,
	}
	provider := newFaux(t, assistantStep(first), assistantStep(second))
	runtime, err := agent.New(agent.Config{
		Provider:     provider,
		SystemPrompt: "system",
		RequestLimits: ai.RequestLimits{
			ContextCapacity: 100,
			ModelMaxOutput:  50,
			ContextSafety:   10,
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	firstResult, err := runtime.Run(context.Background(), nil, "first")
	if err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
	firstContext := firstResult.NewMessages
	if _, err := runtime.Run(context.Background(), firstContext, "12345"); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}

	requests := provider.Requests()
	if got, want := requests[0].MaxOutputTokens, int64(50); got != want {
		t.Fatalf("first MaxOutputTokens = %d, want %d", got, want)
	}
	// The latest valid usage describes 85 tokens; the new five-character user
	// message estimates to two more, leaving 3 tokens after the safety margin.
	if got, want := requests[1].MaxOutputTokens, int64(3); got != want {
		t.Fatalf("second MaxOutputTokens = %d, want %d", got, want)
	}
}

func TestRunSendsCompleteRequestAndReturnsTerminalDelta(t *testing.T) {
	t.Parallel()

	message := ai.AssistantMessage{
		Content: []ai.AssistantContent{
			ai.ThinkingContent{Thinking: "inspect first"},
			ai.TextContent{Text: "done"},
		},
		Usage:      ai.Usage{InputTokens: 7, OutputTokens: 4},
		StopReason: ai.StopReasonStop,
	}
	provider := newFaux(t, assistantStep(message))
	runtime := newAgent(t, provider, "stable system prompt")

	result, err := runtime.Run(context.Background(), nil, "inspect the project")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	wantMessages := []ai.Message{
		ai.UserMessage{Content: "inspect the project"},
		message,
	}
	if !reflect.DeepEqual(result.NewMessages, wantMessages) {
		t.Fatalf("Run() NewMessages = %#v, want %#v", result.NewMessages, wantMessages)
	}

	requests := provider.Requests()
	if got, want := len(requests), 1; got != want {
		t.Fatalf("len(Requests()) = %d, want %d", got, want)
	}
	wantRequest := ai.Request{
		SystemPrompt: "stable system prompt",
		Messages: []ai.Message{
			ai.UserMessage{Content: "inspect the project"},
		},
	}
	if !reflect.DeepEqual(requests[0], wantRequest) {
		t.Fatalf("Provider request = %#v, want %#v", requests[0], wantRequest)
	}
}

func TestRunTreatsEmptyAssistantAsNormalCompletion(t *testing.T) {
	t.Parallel()

	message := ai.AssistantMessage{StopReason: ai.StopReasonStop}
	provider := newFaux(t, faux.Step{Events: []ai.Event{
		ai.StartEvent{},
		ai.DoneEvent{Message: message},
	}})
	runtime := newAgent(t, provider, "")

	result, err := runtime.Run(context.Background(), nil, "respond if needed")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := result.NewMessages[1], ai.Message(message); !reflect.DeepEqual(got, want) {
		t.Fatalf("terminal message = %#v, want %#v", got, want)
	}
}

func TestRunTreatsLengthStopAsNormalCompletion(t *testing.T) {
	t.Parallel()

	message := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "truncated answer"}},
		StopReason: ai.StopReasonLength,
	}
	provider := newFaux(t, assistantStep(message))
	runtime := newAgent(t, provider, "system")

	result, err := runtime.Run(context.Background(), nil, "answer within the limit")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := result.NewMessages[1], ai.Message(message); !reflect.DeepEqual(got, want) {
		t.Fatalf("terminal message = %#v, want %#v", got, want)
	}
}

func TestRunsUseExplicitWorkingContextAndReturnIndependentDeltas(t *testing.T) {
	t.Parallel()

	first := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "first answer"}},
		StopReason: ai.StopReasonStop,
	}
	second := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "second answer"}},
		StopReason: ai.StopReasonStop,
	}
	provider := newFaux(t, assistantStep(first), assistantStep(second))
	runtime := newAgent(t, provider, "system")

	firstResult, err := runtime.Run(context.Background(), nil, "first question")
	if err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
	returnedAssistant := firstResult.NewMessages[1].(ai.AssistantMessage)
	returnedAssistant.Content[0] = ai.TextContent{Text: "caller mutation"}

	secondResult, err := runtime.Run(
		context.Background(),
		[]ai.Message{
			ai.UserMessage{Content: "first question"},
			first,
		},
		"second question",
	)
	if err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	wantDelta := []ai.Message{
		ai.UserMessage{Content: "second question"},
		second,
	}
	if !reflect.DeepEqual(secondResult.NewMessages, wantDelta) {
		t.Fatalf("second Run() NewMessages = %#v, want %#v", secondResult.NewMessages, wantDelta)
	}

	requests := provider.Requests()
	if got, want := len(requests), 2; got != want {
		t.Fatalf("len(Requests()) = %d, want %d", got, want)
	}
	wantSecondRequest := []ai.Message{
		ai.UserMessage{Content: "first question"},
		first,
		ai.UserMessage{Content: "second question"},
	}
	if !reflect.DeepEqual(requests[1].Messages, wantSecondRequest) {
		t.Fatalf("second Provider messages = %#v, want %#v", requests[1].Messages, wantSecondRequest)
	}
}

func TestProviderCannotMutateAgentWorkingContextThroughRequest(t *testing.T) {
	t.Parallel()

	provider := &requestMutatingProvider{message: ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "answer"}},
		StopReason: ai.StopReasonStop,
	}}
	runtime := newAgent(t, provider, "system")

	result, err := runtime.Run(context.Background(), nil, "original user input")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := result.NewMessages[0], (ai.UserMessage{Content: "original user input"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("user message after Provider mutation = %#v, want %#v", got, want)
	}
}

func TestProviderCannotMutateNestedWorkingContextThroughRequest(t *testing.T) {
	t.Parallel()

	first := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "first answer"}},
		StopReason: ai.StopReasonStop,
	}
	second := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "second answer"}},
		StopReason: ai.StopReasonStop,
	}
	third := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "third answer"}},
		StopReason: ai.StopReasonStop,
	}
	provider := &nestedRequestMutatingProvider{responses: []ai.AssistantMessage{first, second, third}}
	runtime := newAgent(t, provider, "system")

	firstResult, err := runtime.Run(context.Background(), nil, "first question")
	if err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
	history := ai.CloneMessages(firstResult.NewMessages)
	secondResult, err := runtime.Run(context.Background(), history, "second question")
	if err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	history = append(history, ai.CloneMessages(secondResult.NewMessages)...)
	if _, err := runtime.Run(context.Background(), history, "third question"); err != nil {
		t.Fatalf("third Run() error = %v", err)
	}
	if got, want := provider.previousTexts, []string{"first answer", "first answer"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("previous assistant texts = %v, want %v after request mutations", got, want)
	}
}

func TestProviderTerminalMutationCannotAffectAgentWorkingContext(t *testing.T) {
	t.Parallel()

	arguments := []byte(`{"path":"main.go"}`)
	message := ai.AssistantMessage{
		Content: []ai.AssistantContent{ai.ToolCall{
			ID:        "call-1",
			Name:      "read",
			Arguments: arguments,
		}},
		StopReason: ai.StopReasonToolUse,
	}
	provider := &responseProvider{messages: []ai.AssistantMessage{
		message,
		{Content: []ai.AssistantContent{ai.TextContent{Text: "done"}}, StopReason: ai.StopReasonStop},
		{Content: []ai.AssistantContent{ai.TextContent{Text: "continued"}}, StopReason: ai.StopReasonStop},
	}}
	runtime := newAgentWithTools(t, provider, "system", &testTool{
		definition: agent.ToolDefinition{Schema: toolSchema("read"), CanRunParallel: true},
		execute: func(context.Context, json.RawMessage) (string, error) {
			return "file contents", nil
		},
	})

	firstResult, err := runtime.Run(context.Background(), nil, "inspect")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	arguments[9] = 'X'

	if _, err := runtime.Run(context.Background(), firstResult.NewMessages, "continue"); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	requests := provider.Requests()
	if got, want := len(requests), 3; got != want {
		t.Fatalf("Provider requests = %d, want %d", got, want)
	}
	terminal := requests[2].Messages[1].(ai.AssistantMessage)
	call := terminal.Content[0].(ai.ToolCall)
	if got, want := string(call.Arguments), `{"path":"main.go"}`; got != want {
		t.Fatalf("stored tool arguments = %q, want %q", got, want)
	}
}

func TestRunAppendsProviderErrorTerminalExactlyOnce(t *testing.T) {
	t.Parallel()

	message := ai.AssistantMessage{
		StopReason:   ai.StopReasonError,
		ErrorMessage: "upstream unavailable",
	}
	provider := newFaux(t, faux.Step{Events: []ai.Event{
		ai.ErrorEvent{Message: message},
	}})
	runtime := newAgent(t, provider, "system")

	result, err := runtime.Run(context.Background(), nil, "try once")
	if err == nil || !strings.Contains(err.Error(), "upstream unavailable") {
		t.Fatalf("Run() error = %v, want upstream error", err)
	}
	want := []ai.Message{
		ai.UserMessage{Content: "try once"},
		message,
	}
	if !reflect.DeepEqual(result.NewMessages, want) {
		t.Fatalf("Run() NewMessages = %#v, want exactly %#v", result.NewMessages, want)
	}
}

func TestPreCanceledRunDoesNotMutateWorkingContextOrCallProvider(t *testing.T) {
	t.Parallel()

	provider := newFaux(t, assistantStep(ai.AssistantMessage{StopReason: ai.StopReasonStop}))
	runtime := newAgent(t, provider, "system")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := runtime.Run(ctx, nil, "not accepted")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if len(result.NewMessages) != 0 {
		t.Fatalf("Run() NewMessages = %#v, want unchanged empty transcript", result.NewMessages)
	}
	if got := len(provider.Requests()); got != 0 {
		t.Fatalf("len(Requests()) = %d, want 0", got)
	}
}

func TestCancelAfterAcceptanceWaitsForReceiveAndAppendsOneAbortedTerminal(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	cancelObserved := make(chan struct{})
	settle := make(chan struct{})
	provider := &settlingCancelProvider{
		started:        started,
		cancelObserved: cancelObserved,
		settle:         settle,
	}
	runtime := newAgent(t, provider, "system")
	ctx, cancel := context.WithCancelCause(context.Background())
	cause := errors.New("operator stopped the run")
	type runReturn struct {
		result agent.RunResult
		err    error
	}
	returned := make(chan runReturn, 1)
	go func() {
		result, err := runtime.Run(ctx, nil, "accepted input")
		returned <- runReturn{result: result, err: err}
	}()

	<-started
	cancel(cause)
	<-cancelObserved
	select {
	case got := <-returned:
		t.Fatalf("Run() returned before Receive settled: %#v", got)
	default:
	}
	close(settle)
	got := <-returned
	if !errors.Is(got.err, cause) {
		t.Fatalf("Run() error = %v, want cancellation cause", got.err)
	}
	if gotLen, want := len(got.result.NewMessages), 2; gotLen != want {
		t.Fatalf("len(NewMessages) = %d, want %d", gotLen, want)
	}
	terminal, ok := got.result.NewMessages[1].(ai.AssistantMessage)
	if !ok {
		t.Fatalf("terminal = %T, want ai.AssistantMessage", got.result.NewMessages[1])
	}
	if terminal.StopReason != ai.StopReasonAborted {
		t.Fatalf("terminal StopReason = %q, want %q", terminal.StopReason, ai.StopReasonAborted)
	}
}

func TestRunPreservesProviderAbortedTerminalAndContextCause(t *testing.T) {
	t.Parallel()

	message := ai.AssistantMessage{
		Content:      []ai.AssistantContent{ai.TextContent{Text: "received so far"}},
		StopReason:   ai.StopReasonAborted,
		ErrorMessage: "provider observed cancellation",
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	cause := errors.New("operator stopped the run")
	runtime := newAgent(t, staticProvider{stream: &terminalWithErrorStream{
		event: ai.ErrorEvent{Message: message},
		err:   cause,
		beforeReturn: func() {
			cancel(cause)
		},
	}}, "system")

	result, err := runtime.Run(ctx, nil, "inspect")
	if !errors.Is(err, cause) {
		t.Fatalf("Run() error = %v, want cancellation cause", err)
	}
	if got, want := result.NewMessages[1], ai.Message(message); !reflect.DeepEqual(got, want) {
		t.Fatalf("terminal message = %#v, want Provider terminal %#v", got, want)
	}
}

func TestRunSynthesizesErrorWhenStreamEndsBeforeTerminal(t *testing.T) {
	t.Parallel()

	runtime := newAgent(t, staticProvider{stream: eofStream{}}, "system")

	result, err := runtime.Run(context.Background(), nil, "broken stream")
	if err == nil || !strings.Contains(err.Error(), "before terminal") {
		t.Fatalf("Run() error = %v, want missing-terminal error", err)
	}
	if got, want := len(result.NewMessages), 2; got != want {
		t.Fatalf("len(NewMessages) = %d, want %d", got, want)
	}
	terminal := result.NewMessages[1].(ai.AssistantMessage)
	if terminal.StopReason != ai.StopReasonError {
		t.Fatalf("terminal StopReason = %q, want %q", terminal.StopReason, ai.StopReasonError)
	}
}

func TestRunSynthesizesErrorForNonEOFReceiveFailure(t *testing.T) {
	t.Parallel()

	receiveErr := errors.New("connection reset")
	runtime := newAgent(t, staticProvider{stream: errorStream{err: receiveErr}}, "system")

	result, err := runtime.Run(context.Background(), nil, "broken stream")
	if !errors.Is(err, receiveErr) {
		t.Fatalf("Run() error = %v, want wrapped receive error", err)
	}
	terminal := result.NewMessages[1].(ai.AssistantMessage)
	if terminal.StopReason != ai.StopReasonError {
		t.Fatalf("terminal StopReason = %q, want %q", terminal.StopReason, ai.StopReasonError)
	}
	if !strings.Contains(terminal.ErrorMessage, "connection reset") {
		t.Fatalf("terminal ErrorMessage = %q, want receive failure", terminal.ErrorMessage)
	}
}

func TestRunSynthesizesProtocolErrorForInvalidTerminal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		event ai.Event
	}{
		{
			name: "done with error reason",
			event: ai.DoneEvent{Message: ai.AssistantMessage{
				StopReason: ai.StopReasonError,
			}},
		},
		{
			name: "error with stop reason",
			event: ai.ErrorEvent{Message: ai.AssistantMessage{
				StopReason: ai.StopReasonStop,
			}},
		},
		{
			name:  "nil event without error",
			event: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			runtime := newAgent(t, staticProvider{stream: &sliceStream{
				events: []ai.Event{test.event},
			}}, "system")

			result, err := runtime.Run(context.Background(), nil, "invalid response")
			if err == nil || !strings.Contains(err.Error(), "provider protocol") {
				t.Fatalf("Run() error = %v, want protocol error", err)
			}
			terminal := result.NewMessages[1].(ai.AssistantMessage)
			if terminal.StopReason != ai.StopReasonError {
				t.Fatalf("terminal StopReason = %q, want %q", terminal.StopReason, ai.StopReasonError)
			}
		})
	}
}

func TestRunSynthesizesErrorWhenProviderReturnsNilStream(t *testing.T) {
	t.Parallel()

	runtime := newAgent(t, staticProvider{}, "system")

	result, err := runtime.Run(context.Background(), nil, "broken provider")
	if err == nil || !strings.Contains(err.Error(), "nil stream") {
		t.Fatalf("Run() error = %v, want nil-stream protocol error", err)
	}
	if got, want := len(result.NewMessages), 2; got != want {
		t.Fatalf("len(NewMessages) = %d, want %d", got, want)
	}
	terminal := result.NewMessages[1].(ai.AssistantMessage)
	if terminal.StopReason != ai.StopReasonError {
		t.Fatalf("terminal StopReason = %q, want %q", terminal.StopReason, ai.StopReasonError)
	}
}

func TestTerminalEventWinsWhenReceiveAlsoReturnsCancellation(t *testing.T) {
	t.Parallel()

	message := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "settled"}},
		StopReason: ai.StopReasonStop,
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	cause := errors.New("late cancellation")
	runtime := newAgent(t, staticProvider{stream: &terminalWithErrorStream{
		event: ai.DoneEvent{Message: message},
		err:   cause,
		beforeReturn: func() {
			cancel(cause)
		},
	}}, "system")

	result, err := runtime.Run(ctx, nil, "finish this turn")
	if err != nil {
		t.Fatalf("Run() error = %v, want nil after terminal settlement", err)
	}
	want := []ai.Message{
		ai.UserMessage{Content: "finish this turn"},
		message,
	}
	if !reflect.DeepEqual(result.NewMessages, want) {
		t.Fatalf("Run() NewMessages = %#v, want %#v", result.NewMessages, want)
	}
}

func TestRunDeepClonesExplicitWorkingContext(t *testing.T) {
	t.Parallel()

	priorAssistant := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "retained answer"}},
		StopReason: ai.StopReasonStop,
	}
	replacement := []ai.Message{
		ai.UserMessage{Content: "summary of earlier work"},
		priorAssistant,
	}
	started := make(chan struct{})
	release := make(chan struct{})
	provider := &blockingProvider{started: started, release: release}
	runtime := newAgent(t, provider, "system")

	type runReturn struct {
		result agent.RunResult
		err    error
	}
	returned := make(chan runReturn, 1)
	go func() {
		result, err := runtime.Run(context.Background(), replacement, "continue the task")
		returned <- runReturn{result: result, err: err}
	}()
	<-started
	replacement[0] = ai.UserMessage{Content: "caller changed top-level slice"}
	priorAssistant.Content[0] = ai.TextContent{Text: "caller changed nested content"}
	close(release)
	got := <-returned
	if got.err != nil {
		t.Fatalf("Run() error = %v", got.err)
	}
	wantRequest := []ai.Message{
		ai.UserMessage{Content: "summary of earlier work"},
		ai.AssistantMessage{
			Content:    []ai.AssistantContent{ai.TextContent{Text: "retained answer"}},
			StopReason: ai.StopReasonStop,
		},
		ai.UserMessage{Content: "continue the task"},
	}
	requests := provider.Requests()
	if got, want := len(requests), 1; got != want {
		t.Fatalf("Provider requests = %d, want %d", got, want)
	}
	if !reflect.DeepEqual(requests[0].Messages, wantRequest) {
		t.Fatalf("Provider messages = %#v, want %#v", requests[0].Messages, wantRequest)
	}
	if got, want := got.result.NewMessages, []ai.Message{
		ai.UserMessage{Content: "continue the task"},
		provider.message(),
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Run() NewMessages = %#v, want %#v", got, want)
	}
}

func newAgent(t *testing.T, provider ai.Provider, systemPrompt string) *agent.Engine {
	t.Helper()
	runtime, err := agent.New(agent.Config{
		Provider:     provider,
		SystemPrompt: systemPrompt,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return runtime
}

func newFaux(t *testing.T, steps ...faux.Step) *faux.Provider {
	t.Helper()
	provider, err := faux.New(steps...)
	if err != nil {
		t.Fatalf("faux.New() error = %v", err)
	}
	return provider
}

func assistantStep(message ai.AssistantMessage) faux.Step {
	events := []ai.Event{ai.StartEvent{}}
	for index, content := range message.Content {
		switch content := content.(type) {
		case ai.TextContent:
			events = append(events,
				ai.TextStartEvent{ContentIndex: index},
				ai.TextDeltaEvent{ContentIndex: index, Delta: content.Text},
				ai.TextEndEvent{ContentIndex: index, Text: content.Text},
			)
		case ai.ThinkingContent:
			events = append(events,
				ai.ThinkingStartEvent{ContentIndex: index},
				ai.ThinkingDeltaEvent{ContentIndex: index, Delta: content.Thinking},
				ai.ThinkingEndEvent{ContentIndex: index, Thinking: content.Thinking},
			)
		case ai.ToolCall:
			events = append(events,
				ai.ToolCallStartEvent{ContentIndex: index, ID: content.ID, Name: content.Name},
				ai.ToolCallDeltaEvent{ContentIndex: index, Delta: string(content.Arguments)},
				ai.ToolCallEndEvent{ContentIndex: index, ToolCall: content},
			)
		default:
			panic(fmt.Sprintf("unsupported test content %T", content))
		}
	}
	events = append(events, ai.DoneEvent{Message: message})
	return faux.Step{Events: events}
}

type sliceStream struct {
	events []ai.Event
	next   int
}

func (s *sliceStream) Receive() (ai.Event, error) {
	if s.next >= len(s.events) {
		return nil, io.EOF
	}
	event := s.events[s.next]
	s.next++
	return event, nil
}

type requestMutatingProvider struct {
	message ai.AssistantMessage
}

func (p *requestMutatingProvider) Stream(_ context.Context, request ai.Request) ai.Stream {
	request.Messages[0] = ai.UserMessage{Content: "provider mutation"}
	return &sliceStream{events: []ai.Event{
		ai.StartEvent{},
		ai.DoneEvent{Message: p.message},
	}}
}

type nestedRequestMutatingProvider struct {
	responses     []ai.AssistantMessage
	previousTexts []string
	next          int
}

func (p *nestedRequestMutatingProvider) Stream(_ context.Context, request ai.Request) ai.Stream {
	if p.next > 0 {
		previous := request.Messages[1].(ai.AssistantMessage)
		p.previousTexts = append(p.previousTexts, previous.Content[0].(ai.TextContent).Text)
		previous.Content[0] = ai.TextContent{Text: "provider nested mutation"}
	}
	message := p.responses[p.next]
	p.next++
	return &sliceStream{events: []ai.Event{
		ai.StartEvent{},
		ai.DoneEvent{Message: message},
	}}
}

type settlingCancelProvider struct {
	started        chan struct{}
	cancelObserved chan struct{}
	settle         chan struct{}
	once           sync.Once
}

func (p *settlingCancelProvider) Stream(ctx context.Context, _ ai.Request) ai.Stream {
	p.once.Do(func() { close(p.started) })
	return &settlingCancelStream{
		ctx:            ctx,
		cancelObserved: p.cancelObserved,
		settle:         p.settle,
	}
}

type settlingCancelStream struct {
	ctx            context.Context
	cancelObserved chan struct{}
	settle         chan struct{}
	once           sync.Once
}

func (s *settlingCancelStream) Receive() (ai.Event, error) {
	<-s.ctx.Done()
	s.once.Do(func() { close(s.cancelObserved) })
	<-s.settle
	return nil, s.ctx.Err()
}

type staticProvider struct {
	stream ai.Stream
}

func (p staticProvider) Stream(context.Context, ai.Request) ai.Stream {
	return p.stream
}

type eofStream struct{}

func (eofStream) Receive() (ai.Event, error) {
	return nil, io.EOF
}

type errorStream struct {
	err error
}

func (s errorStream) Receive() (ai.Event, error) {
	return nil, s.err
}

type terminalWithErrorStream struct {
	event        ai.Event
	err          error
	beforeReturn func()
}

func (s *terminalWithErrorStream) Receive() (ai.Event, error) {
	s.beforeReturn()
	return s.event, s.err
}

type blockingProvider struct {
	started  chan struct{}
	release  chan struct{}
	once     sync.Once
	mu       sync.Mutex
	requests []ai.Request
}

func (p *blockingProvider) Stream(_ context.Context, request ai.Request) ai.Stream {
	p.once.Do(func() { close(p.started) })
	p.mu.Lock()
	p.requests = append(p.requests, ai.CloneRequest(request))
	p.mu.Unlock()
	return &blockingStream{release: p.release, message: p.message()}
}

func (p *blockingProvider) Requests() []ai.Request {
	p.mu.Lock()
	defer p.mu.Unlock()
	requests := make([]ai.Request, len(p.requests))
	for index, request := range p.requests {
		requests[index] = ai.CloneRequest(request)
	}
	return requests
}

func (p *blockingProvider) message() ai.AssistantMessage {
	return ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "done"}},
		StopReason: ai.StopReasonStop,
	}
}

type blockingStream struct {
	release chan struct{}
	message ai.AssistantMessage
	done    bool
}

func (s *blockingStream) Receive() (ai.Event, error) {
	if s.done {
		return nil, io.EOF
	}
	<-s.release
	s.done = true
	return ai.DoneEvent{Message: s.message}, nil
}
