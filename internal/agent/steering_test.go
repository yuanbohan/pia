package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/yuanbohan/pia/internal/agent"
	"github.com/yuanbohan/pia/internal/ai"
)

func TestRunRequiresSteeringSource(t *testing.T) {
	t.Parallel()

	runtime := newAgent(t, newFaux(t), "system")
	if _, err := runtime.Run(context.Background(), nil, "input", nil); err == nil {
		t.Fatal("Run() error = nil, want missing-steering-source error")
	}
	if _, err := runtime.Continue(
		context.Background(),
		[]ai.Message{ai.UserMessage{Content: "continue"}},
		nil,
	); err == nil {
		t.Fatal("Continue() error = nil, want missing-steering-source error")
	}
}

func TestRunDrainsSteeringAtStartAndBeforeStopping(t *testing.T) {
	t.Parallel()

	firstAssistant := steeringTextAssistant("first")
	finalAssistant := steeringTextAssistant("done")
	provider := &responseProvider{
		messages: []ai.AssistantMessage{firstAssistant, finalAssistant},
	}
	source := &scriptedSteeringSource{
		drains: []steeringBatch{
			{messages: []string{"start one", "start two"}},
		},
		finalDrains: []steeringBatch{
			{messages: []string{"before stop"}},
			{},
		},
	}
	runtime := newAgent(t, provider, "system")

	result, err := runtime.Run(
		context.Background(),
		nil,
		"initial",
		source,
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	requests := provider.Requests()
	if len(requests) != 2 {
		t.Fatalf("Provider request count = %d, want 2", len(requests))
	}
	wantFirstRequest := []ai.Message{
		ai.UserMessage{Content: "initial"},
		ai.UserMessage{Content: "start one"},
		ai.UserMessage{Content: "start two"},
	}
	if !reflect.DeepEqual(requests[0].Messages, wantFirstRequest) {
		t.Fatalf(
			"first Provider messages = %#v, want %#v",
			requests[0].Messages,
			wantFirstRequest,
		)
	}
	wantSecondRequest := append(
		append([]ai.Message{}, wantFirstRequest...),
		firstAssistant,
		ai.UserMessage{Content: "before stop"},
	)
	if !reflect.DeepEqual(requests[1].Messages, wantSecondRequest) {
		t.Fatalf(
			"second Provider messages = %#v, want %#v",
			requests[1].Messages,
			wantSecondRequest,
		)
	}
	wantDelta := append(
		append([]ai.Message{}, wantSecondRequest...),
		finalAssistant,
	)
	if !reflect.DeepEqual(result.NewMessages, wantDelta) {
		t.Fatalf("Run() NewMessages = %#v, want %#v", result.NewMessages, wantDelta)
	}
	if got, want := source.callSnapshot(), []string{
		"drain",
		"drain_or_seal",
		"drain_or_seal",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Steering calls = %#v, want %#v", got, want)
	}
}

func TestRunDrainsSteeringAfterCompleteToolBatch(t *testing.T) {
	t.Parallel()

	toolTerminal := toolAssistant(
		ai.StopReasonToolUse,
		toolCall("first-call", "inspect", "first"),
		toolCall("second-call", "inspect", "second"),
	)
	finalAssistant := steeringTextAssistant("done")
	provider := &responseProvider{
		messages: []ai.AssistantMessage{toolTerminal, finalAssistant},
	}
	tool := &testTool{
		definition: agent.ToolDefinition{Schema: toolSchema("inspect")},
		execute: func(_ context.Context, arguments json.RawMessage) (string, error) {
			label, err := argumentLabel(arguments)
			if err != nil {
				return "", err
			}
			return "result " + label, nil
		},
	}
	source := &scriptedSteeringSource{
		drains: []steeringBatch{
			{},
			{messages: []string{"after both tools"}},
		},
		finalDrains: []steeringBatch{{}},
	}
	runtime := newAgentWithTools(t, provider, "system", tool)

	result, err := runtime.Run(
		context.Background(),
		nil,
		"inspect both",
		source,
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	requests := provider.Requests()
	if len(requests) != 2 {
		t.Fatalf("Provider request count = %d, want 2", len(requests))
	}
	wantSecondRequest := []ai.Message{
		ai.UserMessage{Content: "inspect both"},
		toolTerminal,
		ai.ToolResultMessage{
			ToolCallID: "first-call",
			ToolName:   "inspect",
			Content:    "result first",
		},
		ai.ToolResultMessage{
			ToolCallID: "second-call",
			ToolName:   "inspect",
			Content:    "result second",
		},
		ai.UserMessage{Content: "after both tools"},
	}
	if !reflect.DeepEqual(requests[1].Messages, wantSecondRequest) {
		t.Fatalf(
			"second Provider messages = %#v, want %#v",
			requests[1].Messages,
			wantSecondRequest,
		)
	}
	if !reflect.DeepEqual(
		result.NewMessages[len(result.NewMessages)-1],
		finalAssistant,
	) {
		t.Fatalf("Run() final message = %#v, want %#v", result.NewMessages, finalAssistant)
	}
	if got, want := source.callSnapshot(), []string{
		"drain",
		"drain",
		"drain_or_seal",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Steering calls = %#v, want %#v", got, want)
	}
}

func TestContinueDrainsTransferredSteeringBeforeFirstProvider(t *testing.T) {
	t.Parallel()

	finalAssistant := steeringTextAssistant("recovered")
	provider := &responseProvider{messages: []ai.AssistantMessage{finalAssistant}}
	source := &scriptedSteeringSource{
		drains:      []steeringBatch{{messages: []string{"recovery correction"}}},
		finalDrains: []steeringBatch{{}},
	}
	runtime := newAgent(t, provider, "system")
	initial := []ai.Message{
		ai.UserMessage{Content: "continue accepted work"},
	}

	result, err := runtime.Continue(context.Background(), initial, source)
	if err != nil {
		t.Fatalf("Continue() error = %v", err)
	}

	requests := provider.Requests()
	if len(requests) != 1 {
		t.Fatalf("Provider request count = %d, want 1", len(requests))
	}
	wantRequest := append(
		append([]ai.Message{}, initial...),
		ai.UserMessage{Content: "recovery correction"},
	)
	if !reflect.DeepEqual(requests[0].Messages, wantRequest) {
		t.Fatalf(
			"Provider messages = %#v, want %#v",
			requests[0].Messages,
			wantRequest,
		)
	}
	wantDelta := []ai.Message{
		ai.UserMessage{Content: "recovery correction"},
		finalAssistant,
	}
	if !reflect.DeepEqual(result.NewMessages, wantDelta) {
		t.Fatalf("Continue() NewMessages = %#v, want %#v", result.NewMessages, wantDelta)
	}
}

func TestRunDoesNotDrainSteeringAfterProviderFailure(t *testing.T) {
	t.Parallel()

	providerErr := errors.New("stream failed")
	provider := &streamSequenceProvider{
		streams: []ai.Stream{errorStream{err: providerErr}},
	}
	source := &scriptedSteeringSource{
		drains:      []steeringBatch{{}},
		finalDrains: []steeringBatch{{messages: []string{"must remain pending"}}},
	}
	runtime := newAgent(t, provider, "system")

	if _, err := runtime.Run(
		context.Background(),
		nil,
		"initial",
		source,
	); !errors.Is(err, providerErr) {
		t.Fatalf("Run() error = %v, want %v", err, providerErr)
	}
	if got, want := source.callSnapshot(), []string{"drain"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Steering calls = %#v, want %#v", got, want)
	}
}

func TestRunDoesNotDrainSteeringAfterToolStageCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	provider := &responseProvider{
		messages: []ai.AssistantMessage{
			toolAssistant(
				ai.StopReasonToolUse,
				toolCall("cancel-call", "cancel", "now"),
			),
		},
	}
	tool := &testTool{
		definition: agent.ToolDefinition{Schema: toolSchema("cancel")},
		execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			cancel()
			return "canceled", nil
		},
	}
	source := &scriptedSteeringSource{
		drains: []steeringBatch{
			{},
			{messages: []string{"must remain pending"}},
		},
	}
	runtime := newAgentWithTools(t, provider, "system", tool)

	result, err := runtime.Run(ctx, nil, "cancel during tool", source)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if got, want := source.callSnapshot(), []string{"drain"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Steering calls = %#v, want %#v", got, want)
	}
	if got, want := len(result.NewMessages), 3; got != want {
		t.Fatalf("Run() NewMessages length = %d, want %d", got, want)
	}
	if resultMessage, ok := result.NewMessages[2].(ai.ToolResultMessage); !ok ||
		resultMessage.Content != "canceled" {
		t.Fatalf("Run() tool result = %#v, want completed canceled tool", result.NewMessages[2])
	}
}

func TestRunAppendsEntireDrainedBatchBeforeReturningCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	source := &scriptedSteeringSource{
		drains: []steeringBatch{{
			messages: []string{"first correction", "second correction"},
			onDrain:  cancel,
		}},
	}
	provider := &responseProvider{}
	runtime := newAgent(t, provider, "system")

	result, err := runtime.Run(ctx, nil, "initial", source)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	wantDelta := []ai.Message{
		ai.UserMessage{Content: "initial"},
		ai.UserMessage{Content: "first correction"},
		ai.UserMessage{Content: "second correction"},
	}
	if !reflect.DeepEqual(result.NewMessages, wantDelta) {
		t.Fatalf("Run() NewMessages = %#v, want %#v", result.NewMessages, wantDelta)
	}
	if requests := provider.Requests(); len(requests) != 0 {
		t.Fatalf("Provider request count = %d, want 0", len(requests))
	}
}

type steeringBatch struct {
	messages []string
	onDrain  func()
}

type scriptedSteeringSource struct {
	mu          sync.Mutex
	drains      []steeringBatch
	finalDrains []steeringBatch
	calls       []string
}

func (s *scriptedSteeringSource) Drain() []string {
	return s.next("drain", &s.drains)
}

func (s *scriptedSteeringSource) DrainOrSeal() []string {
	return s.next("drain_or_seal", &s.finalDrains)
}

func (s *scriptedSteeringSource) next(
	call string,
	batches *[]steeringBatch,
) []string {
	s.mu.Lock()
	s.calls = append(s.calls, call)
	var batch steeringBatch
	if len(*batches) > 0 {
		batch = (*batches)[0]
		*batches = (*batches)[1:]
	}
	s.mu.Unlock()

	if batch.onDrain != nil {
		batch.onDrain()
	}
	return append([]string(nil), batch.messages...)
}

func (s *scriptedSteeringSource) callSnapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.calls...)
}

type emptySteeringSource struct{}

func (emptySteeringSource) Drain() []string {
	return nil
}

func (emptySteeringSource) DrainOrSeal() []string {
	return nil
}

func steeringTextAssistant(text string) ai.AssistantMessage {
	return ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: text}},
		StopReason: ai.StopReasonStop,
	}
}
