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

	if _, err := newConversation(conversationConfig{}); err == nil {
		t.Fatal("newConversation() error = nil, want missing-Core-Agent error")
	}
}

func TestConversationCompactsBetweenRunsAndUpdatesPreviousSummary(t *testing.T) {
	t.Parallel()

	first := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "first answer"}},
		Usage:      ai.Usage{InputTokens: 20, OutputTokens: 5},
		StopReason: ai.StopReasonStop,
	}
	second := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "second answer"}},
		Usage:      ai.Usage{InputTokens: 110, OutputTokens: 10},
		StopReason: ai.StopReasonStop,
	}
	third := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "third answer"}},
		Usage:      ai.Usage{InputTokens: 110, OutputTokens: 10},
		StopReason: ai.StopReasonStop,
	}
	fourth := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "fourth answer"}},
		StopReason: ai.StopReasonStop,
	}
	summaryOne := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "checkpoint one"}},
		StopReason: ai.StopReasonStop,
	}
	summaryTwo := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "checkpoint two"}},
		StopReason: ai.StopReasonStop,
	}
	provider := newConversationFaux(t,
		conversationAssistantStep(first),
		conversationAssistantStep(second),
		conversationAssistantStep(summaryOne),
		conversationAssistantStep(third),
		conversationAssistantStep(summaryTwo),
		conversationAssistantStep(fourth),
	)
	conversation := newCompactingTestConversation(t, provider)

	var history []ai.Message
	for _, input := range []string{"first question", "second question", "third question", "fourth question"} {
		var err error
		history, err = conversation.run(context.Background(), input)
		if err != nil {
			t.Fatalf("run %q error = %v", input, err)
		}
	}

	wantHistory := []ai.Message{
		ai.UserMessage{Content: "first question"}, first,
		ai.UserMessage{Content: "second question"}, second,
		ai.UserMessage{Content: "third question"}, third,
		ai.UserMessage{Content: "fourth question"}, fourth,
	}
	if !reflect.DeepEqual(history, wantHistory) {
		t.Fatalf("complete History = %#v, want original messages %#v", history, wantHistory)
	}

	requests := provider.Requests()
	if got, want := len(requests), 6; got != want {
		t.Fatalf("Provider requests = %d, want %d", got, want)
	}
	firstSummaryRequest := requests[2]
	if firstSummaryRequest.SystemPrompt != summarizationSystemPrompt || len(firstSummaryRequest.Tools) != 0 {
		t.Fatalf("first summary request = %#v, want isolated summarizer request", firstSummaryRequest)
	}
	if got, want := firstSummaryRequest.MaxOutputTokens, int64(13); got != want {
		t.Fatalf("first summary max output = %d, want %d", got, want)
	}
	firstSummaryInput := firstSummaryRequest.Messages[0].(ai.UserMessage).Content
	for _, required := range []string{
		"<conversation>",
		"[User]: first question",
		"[Assistant]: first answer",
		initialSummarizationPrompt,
	} {
		if !strings.Contains(firstSummaryInput, required) {
			t.Fatalf("first summary input missing %q\n%s", required, firstSummaryInput)
		}
	}

	firstCompactedRequest := requests[3]
	wantFirstProjection := []ai.Message{
		syntheticSummaryMessage("checkpoint one"),
		ai.UserMessage{Content: "second question"},
		withoutUsage(second),
		ai.UserMessage{Content: "third question"},
	}
	if !reflect.DeepEqual(firstCompactedRequest.Messages, wantFirstProjection) {
		t.Fatalf("first compacted messages = %#v, want %#v", firstCompactedRequest.Messages, wantFirstProjection)
	}

	secondSummaryRequest := requests[4]
	secondSummaryInput := secondSummaryRequest.Messages[0].(ai.UserMessage).Content
	for _, required := range []string{
		"[User]: second question",
		"[Assistant]: second answer",
		"<previous-summary>\ncheckpoint one\n</previous-summary>",
		updateSummarizationPrompt,
	} {
		if !strings.Contains(secondSummaryInput, required) {
			t.Fatalf("update summary input missing %q\n%s", required, secondSummaryInput)
		}
	}
	if strings.Contains(secondSummaryInput, "[User]: first question") {
		t.Fatalf("update summary reserialized already summarized history\n%s", secondSummaryInput)
	}

	wantSecondProjection := []ai.Message{
		syntheticSummaryMessage("checkpoint two"),
		ai.UserMessage{Content: "third question"},
		withoutUsage(third),
		ai.UserMessage{Content: "fourth question"},
	}
	if !reflect.DeepEqual(requests[5].Messages, wantSecondProjection) {
		t.Fatalf("second compacted messages = %#v, want %#v", requests[5].Messages, wantSecondProjection)
	}
}

func TestConversationCompactsWhenProjectedInputEqualsThreshold(t *testing.T) {
	t.Parallel()

	first := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "first answer"}},
		Usage:      ai.Usage{InputTokens: 90, OutputTokens: 8},
		StopReason: ai.StopReasonStop,
	}
	provider := newConversationFaux(t,
		conversationAssistantStep(first),
		conversationAssistantStep(ai.AssistantMessage{
			Content:    []ai.AssistantContent{ai.TextContent{Text: "turn prefix"}},
			StopReason: ai.StopReasonStop,
		}),
		conversationAssistantStep(ai.AssistantMessage{
			Content:    []ai.AssistantContent{ai.TextContent{Text: "second answer"}},
			StopReason: ai.StopReasonStop,
		}),
	)
	conversation := newCompactingTestConversation(t, provider)

	if _, err := conversation.run(context.Background(), "first question"); err != nil {
		t.Fatalf("first run error = %v", err)
	}
	// The valid usage contributes 98 tokens and the five-character input adds
	// ceil(5/4) = 2, exactly matching the test policy's threshold of 100.
	if _, err := conversation.run(context.Background(), "12345"); err != nil {
		t.Fatalf("threshold run error = %v", err)
	}

	requests := provider.Requests()
	if got, want := len(requests), 3; got != want {
		t.Fatalf("Provider requests = %d, want coding, summary, coding", got)
	}
	if requests[1].SystemPrompt != summarizationSystemPrompt {
		t.Fatalf("request at exact threshold is not a summary request: %#v", requests[1])
	}
}

func TestConversationSummaryFailureRejectsNewInputAndCanRetry(t *testing.T) {
	t.Parallel()

	first := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "first answer"}},
		Usage:      ai.Usage{InputTokens: 20, OutputTokens: 5},
		StopReason: ai.StopReasonStop,
	}
	second := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "second answer"}},
		Usage:      ai.Usage{InputTokens: 110, OutputTokens: 10},
		StopReason: ai.StopReasonStop,
	}
	recovered := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "recovered"}},
		StopReason: ai.StopReasonStop,
	}
	provider := newConversationFaux(t,
		conversationAssistantStep(first),
		conversationAssistantStep(second),
		faux.Step{Events: []ai.Event{ai.ErrorEvent{Message: ai.AssistantMessage{
			StopReason:   ai.StopReasonError,
			ErrorMessage: "summary unavailable",
		}}}},
		conversationAssistantStep(ai.AssistantMessage{
			Content:    []ai.AssistantContent{ai.TextContent{Text: "checkpoint"}},
			StopReason: ai.StopReasonStop,
		}),
		conversationAssistantStep(recovered),
	)
	conversation := newCompactingTestConversation(t, provider)

	if _, err := conversation.run(context.Background(), "first question"); err != nil {
		t.Fatalf("first run error = %v", err)
	}
	wantHistory, err := conversation.run(context.Background(), "second question")
	if err != nil {
		t.Fatalf("second run error = %v", err)
	}

	history, err := conversation.run(context.Background(), "must not be accepted")
	if err == nil || !strings.Contains(err.Error(), "summary unavailable") {
		t.Fatalf("failed compaction error = %v, want summary failure", err)
	}
	if !reflect.DeepEqual(history, wantHistory) {
		t.Fatalf("History after failed compaction = %#v, want %#v", history, wantHistory)
	}

	history, err = conversation.run(context.Background(), "accepted after retry")
	if err != nil {
		t.Fatalf("retry run error = %v", err)
	}
	wantFinal := append(ai.CloneMessages(wantHistory),
		ai.UserMessage{Content: "accepted after retry"},
		recovered,
	)
	if !reflect.DeepEqual(history, wantFinal) {
		t.Fatalf("History after retry = %#v, want %#v", history, wantFinal)
	}
	for _, message := range history {
		if user, ok := message.(ai.UserMessage); ok && user.Content == "must not be accepted" {
			t.Fatal("failed Run input entered complete History")
		}
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
	conversation, err := newConversation(conversationConfig{Core: core})
	if err != nil {
		t.Fatalf("newConversation() error = %v", err)
	}
	return conversation
}

func newCompactingTestConversation(t *testing.T, provider ai.Provider) *conversation {
	t.Helper()
	return newCompactingTestConversationWithPolicy(t, provider, testCompactionPolicy())
}

func testCompactionPolicy() compactionPolicy {
	return compactionPolicy{
		Threshold:                100,
		SoftCeiling:              70,
		RetainedRawTarget:        5,
		SummaryMaxOutput:         13,
		SplitTurnPrefixMaxOutput: 8,
	}
}

func newCompactingTestConversationWithPolicy(
	t *testing.T,
	provider ai.Provider,
	policy compactionPolicy,
) *conversation {
	t.Helper()
	limits := ai.RequestLimits{
		ContextCapacity: 1000,
		ModelMaxOutput:  400,
		ContextSafety:   10,
	}
	core, err := agent.New(agent.Config{
		Provider:      provider,
		SystemPrompt:  "system",
		RequestLimits: limits,
	})
	if err != nil {
		t.Fatalf("agent.New() error = %v", err)
	}
	conversation, err := newConversation(conversationConfig{
		Core:          core,
		Provider:      provider,
		SystemPrompt:  "system",
		RequestLimits: limits,
		Compaction:    policy,
	})
	if err != nil {
		t.Fatalf("newConversation() error = %v", err)
	}
	return conversation
}

func withoutUsage(message ai.AssistantMessage) ai.AssistantMessage {
	message.Usage = ai.Usage{}
	return message
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
