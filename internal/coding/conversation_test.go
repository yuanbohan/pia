package coding

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yuanbohan/pi-go/internal/agent"
	"github.com/yuanbohan/pi-go/internal/ai"
	"github.com/yuanbohan/pi-go/internal/ai/provider/faux"
)

func TestNewConversationRequiresCoreAgent(t *testing.T) {
	t.Parallel()

	if _, err := newConversation(nil); err == nil {
		t.Fatal("newConversation() error = nil, want missing-Core-Agent error")
	}
}

func TestConversationSequentialRunsReturnCompleteIndependentHistory(t *testing.T) {
	t.Parallel()

	first := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "first answer"}},
		StopReason: ai.StopReasonStop,
	}
	second := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "second answer"}},
		StopReason: ai.StopReasonStop,
	}
	provider := newConversationFaux(t, conversationAssistantStep(first), conversationAssistantStep(second))
	conversation := newTestConversation(t, provider)

	firstHistory, err := conversation.run(context.Background(), "first question")
	if err != nil {
		t.Fatalf("first run error = %v", err)
	}
	returnedAssistant := firstHistory[1].(ai.AssistantMessage)
	returnedAssistant.Content[0] = ai.TextContent{Text: "caller mutation"}

	secondHistory, err := conversation.run(context.Background(), "second question")
	if err != nil {
		t.Fatalf("second run error = %v", err)
	}
	wantHistory := []ai.Message{
		ai.UserMessage{Content: "first question"},
		first,
		ai.UserMessage{Content: "second question"},
		second,
	}
	if !reflect.DeepEqual(secondHistory, wantHistory) {
		t.Fatalf("second history = %#v, want %#v", secondHistory, wantHistory)
	}

	requests := provider.Requests()
	if got, want := len(requests), 2; got != want {
		t.Fatalf("Provider requests = %d, want %d", got, want)
	}
	wantSecondRequest := wantHistory[:3]
	if !reflect.DeepEqual(requests[1].Messages, wantSecondRequest) {
		t.Fatalf("second Provider messages = %#v, want %#v", requests[1].Messages, wantSecondRequest)
	}
}

func TestConversationCommitsAcceptedRunDeltaBeforeReturningError(t *testing.T) {
	t.Parallel()

	failed := ai.AssistantMessage{
		StopReason:   ai.StopReasonError,
		ErrorMessage: "upstream unavailable",
	}
	provider := newConversationFaux(t, faux.Step{Events: []ai.Event{
		ai.ErrorEvent{Message: failed},
	}})
	conversation := newTestConversation(t, provider)

	history, err := conversation.run(context.Background(), "try once")
	if err == nil || !strings.Contains(err.Error(), "upstream unavailable") {
		t.Fatalf("run error = %v, want Provider failure", err)
	}
	want := []ai.Message{
		ai.UserMessage{Content: "try once"},
		failed,
	}
	if !reflect.DeepEqual(history, want) {
		t.Fatalf("history = %#v, want committed failure %#v", history, want)
	}
}

func TestConversationPreCanceledRunReturnsUnchangedHistory(t *testing.T) {
	t.Parallel()

	answer := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "done"}},
		StopReason: ai.StopReasonStop,
	}
	provider := newConversationFaux(t, conversationAssistantStep(answer))
	conversation := newTestConversation(t, provider)
	wantHistory, err := conversation.run(context.Background(), "accepted")
	if err != nil {
		t.Fatalf("first run error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	history, err := conversation.run(ctx, "not accepted")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled run error = %v, want context.Canceled", err)
	}
	if !reflect.DeepEqual(history, wantHistory) {
		t.Fatalf("history after pre-cancel = %#v, want unchanged %#v", history, wantHistory)
	}
	if got, want := len(provider.Requests()), 1; got != want {
		t.Fatalf("Provider requests = %d, want %d", got, want)
	}
}

func TestConversationRejectsConcurrentRunWithoutQueueing(t *testing.T) {
	t.Parallel()

	provider := &conversationBlockingProvider{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	conversation := newTestConversation(t, provider)
	type runReturn struct {
		history []ai.Message
		err     error
	}
	firstReturned := make(chan runReturn, 1)
	go func() {
		history, err := conversation.run(context.Background(), "first input")
		firstReturned <- runReturn{history: history, err: err}
	}()
	<-provider.started

	secondReturned := make(chan runReturn, 1)
	go func() {
		history, err := conversation.run(context.Background(), "second input")
		secondReturned <- runReturn{history: history, err: err}
	}()
	select {
	case second := <-secondReturned:
		if !errors.Is(second.err, agent.ErrRunActive) {
			t.Fatalf("second run error = %v, want ErrRunActive", second.err)
		}
		if len(second.history) != 0 {
			t.Fatalf("second history = %#v, want settled empty history", second.history)
		}
	case <-time.After(time.Second):
		t.Fatal("second run blocked instead of returning ErrRunActive")
	}

	close(provider.release)
	first := <-firstReturned
	if first.err != nil {
		t.Fatalf("first run error = %v", first.err)
	}
	want := []ai.Message{
		ai.UserMessage{Content: "first input"},
		provider.message(),
	}
	if !reflect.DeepEqual(first.history, want) {
		t.Fatalf("first history = %#v, want %#v", first.history, want)
	}
}

func newTestConversation(t *testing.T, provider ai.Provider) *conversation {
	t.Helper()
	core, err := agent.New(agent.Config{Provider: provider, SystemPrompt: "system"})
	if err != nil {
		t.Fatalf("agent.New() error = %v", err)
	}
	conversation, err := newConversation(core)
	if err != nil {
		t.Fatalf("newConversation() error = %v", err)
	}
	return conversation
}

func newConversationFaux(t *testing.T, steps ...faux.Step) *faux.Provider {
	t.Helper()
	provider, err := faux.New(steps...)
	if err != nil {
		t.Fatalf("faux.New() error = %v", err)
	}
	return provider
}

func conversationAssistantStep(message ai.AssistantMessage) faux.Step {
	events := []ai.Event{ai.StartEvent{}}
	for index, content := range message.Content {
		text := content.(ai.TextContent)
		events = append(events,
			ai.TextStartEvent{ContentIndex: index},
			ai.TextDeltaEvent{ContentIndex: index, Delta: text.Text},
			ai.TextEndEvent{ContentIndex: index, Text: text.Text},
		)
	}
	events = append(events, ai.DoneEvent{Message: message})
	return faux.Step{Events: events}
}

type conversationBlockingProvider struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (p *conversationBlockingProvider) Stream(context.Context, ai.Request) ai.Stream {
	p.once.Do(func() { close(p.started) })
	return &conversationBlockingStream{release: p.release, message: p.message()}
}

func (p *conversationBlockingProvider) message() ai.AssistantMessage {
	return ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "done"}},
		StopReason: ai.StopReasonStop,
	}
}

type conversationBlockingStream struct {
	release chan struct{}
	message ai.AssistantMessage
	done    bool
}

func (s *conversationBlockingStream) Receive() (ai.Event, error) {
	if s.done {
		return nil, io.EOF
	}
	<-s.release
	s.done = true
	return ai.DoneEvent{Message: s.message}, nil
}
