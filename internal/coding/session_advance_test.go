package coding

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yuanbohan/pia/internal/agent"
	"github.com/yuanbohan/pia/internal/ai"
	"github.com/yuanbohan/pia/internal/ai/provider/faux"
	skilltool "github.com/yuanbohan/pia/internal/coding/tools/skill"
)

func TestNewSessionRequiresExecutionEngine(t *testing.T) {
	t.Parallel()

	if _, err := newSession(sessionDependencies{}); err == nil {
		t.Fatal("newSession() error = nil, want missing execution-engine error")
	}
}

func TestSessionCompactsBetweenRunsAndUpdatesPreviousSummary(t *testing.T) {
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
	provider := newCodingFaux(t,
		codingAssistantStep(first),
		codingAssistantStep(second),
		codingAssistantStep(summaryOne),
		codingAssistantStep(third),
		codingAssistantStep(summaryTwo),
		codingAssistantStep(fourth),
	)
	session := newCompactingTestSession(t, provider)

	var history []ai.Message
	for _, input := range []string{"first question", "second question", "third question", "fourth question"} {
		var err error
		history, err = session.advanceHistory(context.Background(), input)
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

func TestSessionCompactsSkillResultWithoutProtectedProjection(t *testing.T) {
	directory := t.TempDir()
	writePiaSkill(t, directory, "review-go", `name: review-go
description: Review Go changes.
`, "SKILL_BODY_MUST_NOT_BE_PROTECTED")
	workspace := openPromptWorkspace(t, directory)
	discovery, err := discoverPiaSkills(workspace)
	if err != nil {
		t.Fatalf("discover Pia skills: %v", err)
	}
	activation, err := skilltool.New(workspace.Root(), discovery.Entries)
	if err != nil {
		t.Fatalf("create skill tool: %v", err)
	}
	tools := []agent.Tool{activation}
	schemas := toolSchemas(tools)
	systemPrompt := "system\n" + discovery.Catalog
	const (
		firstInput  = "first task"
		secondInput = "next task"
	)
	initialTokens := ai.EstimateRequestTokens(ai.Request{
		SystemPrompt: systemPrompt,
		Messages:     []ai.Message{ai.UserMessage{Content: firstInput}},
		Tools:        schemas,
	}).Tokens
	threshold := initialTokens + 2_000
	firstFinal := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "first run done"}},
		Usage:      ai.Usage{InputTokens: threshold + 1},
		StopReason: ai.StopReasonStop,
	}
	provider := newCodingFaux(t,
		fauxToolStep(ai.ToolCall{ID: "load-skill", Name: "skill", Arguments: json.RawMessage(`{"name":"review-go"}`)}),
		codingAssistantStep(firstFinal),
		codingAssistantStep(ai.AssistantMessage{
			Content:    []ai.AssistantContent{ai.TextContent{Text: "The review Skill was used in the prior run."}},
			StopReason: ai.StopReasonStop,
		}),
		codingAssistantStep(ai.AssistantMessage{
			Content:    []ai.AssistantContent{ai.TextContent{Text: "continued"}},
			StopReason: ai.StopReasonStop,
		}),
	)
	limits := ai.RequestLimits{ContextCapacity: 100_000, ModelMaxOutput: 400, ContextSafety: 10}
	engine, err := agent.New(agent.Config{
		Provider:      provider,
		SystemPrompt:  systemPrompt,
		Tools:         tools,
		RequestLimits: limits,
	})
	if err != nil {
		t.Fatalf("agent.New() error = %v", err)
	}
	session, err := newSession(sessionDependencies{
		Engine:         engine,
		Provider:       provider,
		RequestLimits:  limits,
		CloseWorkspace: func() error { return nil },
		Info: SessionInfo{
			SystemPrompt: systemPrompt,
			Tools:        schemas,
		},
		Compaction: compactionPolicy{
			Threshold:                threshold,
			SoftCeiling:              threshold - 100,
			RetainedRawTarget:        1,
			SummaryMaxOutput:         13,
			SplitTurnPrefixMaxOutput: 8,
		},
	})
	if err != nil {
		t.Fatalf("newSession() error = %v", err)
	}

	if _, err := session.advanceHistory(context.Background(), firstInput); err != nil {
		t.Fatalf("first run error = %v", err)
	}
	history, err := session.advanceHistory(context.Background(), secondInput)
	if err != nil {
		t.Fatalf("second run error = %v", err)
	}
	if !messagesContain(history, "SKILL_BODY_MUST_NOT_BE_PROTECTED") {
		t.Fatal("complete Conversation History lost the original Skill result")
	}

	requests := provider.Requests()
	if got, want := len(requests), 4; got != want {
		t.Fatalf("Provider requests = %d, want coding, coding, summary, coding", got)
	}
	if !messagesContain(requests[2].Messages, "SKILL_BODY_MUST_NOT_BE_PROTECTED") {
		t.Fatal("ordinary summary input did not receive the compacted Skill result")
	}
	if messagesContain(requests[3].Messages, "SKILL_BODY_MUST_NOT_BE_PROTECTED") {
		t.Fatalf("compacted coding request retained a protected Skill body: %#v", requests[3].Messages)
	}
	if strings.Contains(requests[3].SystemPrompt, "SKILL_BODY_MUST_NOT_BE_PROTECTED") {
		t.Fatal("stable system prompt gained a protected Skill body")
	}
	for _, forbidden := range []string{"dormant_skill", "active_skill", "activation receipt"} {
		if messagesContain(requests[3].Messages, forbidden) {
			t.Fatalf("compacted coding request contains Skill-specific projection %q", forbidden)
		}
	}
}

func TestSessionCompactsWhenProjectedInputEqualsThreshold(t *testing.T) {
	t.Parallel()

	first := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "first answer"}},
		Usage:      ai.Usage{InputTokens: 90, OutputTokens: 8},
		StopReason: ai.StopReasonStop,
	}
	provider := newCodingFaux(t,
		codingAssistantStep(first),
		codingAssistantStep(ai.AssistantMessage{
			Content:    []ai.AssistantContent{ai.TextContent{Text: "turn prefix"}},
			StopReason: ai.StopReasonStop,
		}),
		codingAssistantStep(ai.AssistantMessage{
			Content:    []ai.AssistantContent{ai.TextContent{Text: "second answer"}},
			StopReason: ai.StopReasonStop,
		}),
	)
	session := newCompactingTestSession(t, provider)

	if _, err := session.advanceHistory(context.Background(), "first question"); err != nil {
		t.Fatalf("first run error = %v", err)
	}
	// The valid usage contributes 98 tokens and the five-character input adds
	// ceil(5/4) = 2, exactly matching the test policy's threshold of 100.
	if _, err := session.advanceHistory(context.Background(), "12345"); err != nil {
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

func TestSessionSummaryFailureRejectsNewInputAndCanRetry(t *testing.T) {
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
	provider := newCodingFaux(t,
		codingAssistantStep(first),
		codingAssistantStep(second),
		faux.Step{Events: []ai.Event{ai.ErrorEvent{Message: ai.AssistantMessage{
			StopReason:   ai.StopReasonError,
			ErrorMessage: "summary unavailable",
		}}}},
		codingAssistantStep(ai.AssistantMessage{
			Content:    []ai.AssistantContent{ai.TextContent{Text: "checkpoint"}},
			StopReason: ai.StopReasonStop,
		}),
		codingAssistantStep(recovered),
	)
	session := newCompactingTestSession(t, provider)

	if _, err := session.advanceHistory(context.Background(), "first question"); err != nil {
		t.Fatalf("first run error = %v", err)
	}
	wantHistory, err := session.advanceHistory(context.Background(), "second question")
	if err != nil {
		t.Fatalf("second run error = %v", err)
	}

	history, err := session.advanceHistory(context.Background(), "must not be accepted")
	if err == nil || !strings.Contains(err.Error(), "summary unavailable") {
		t.Fatalf("failed compaction error = %v, want summary failure", err)
	}
	if !reflect.DeepEqual(history, wantHistory) {
		t.Fatalf("History after failed compaction = %#v, want %#v", history, wantHistory)
	}

	history, err = session.advanceHistory(context.Background(), "accepted after retry")
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

func TestSessionSequentialRunsReturnCompleteIndependentHistory(t *testing.T) {
	t.Parallel()

	first := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "first answer"}},
		StopReason: ai.StopReasonStop,
	}
	second := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "second answer"}},
		StopReason: ai.StopReasonStop,
	}
	provider := newCodingFaux(t, codingAssistantStep(first), codingAssistantStep(second))
	session := newHistoryTestSession(t, provider)

	firstHistory, err := session.advanceHistory(context.Background(), "first question")
	if err != nil {
		t.Fatalf("first run error = %v", err)
	}
	returnedAssistant := firstHistory[1].(ai.AssistantMessage)
	returnedAssistant.Content[0] = ai.TextContent{Text: "caller mutation"}

	secondHistory, err := session.advanceHistory(context.Background(), "second question")
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

func TestSessionCommitsAcceptedRunDeltaBeforeReturningError(t *testing.T) {
	t.Parallel()

	failed := ai.AssistantMessage{
		StopReason:   ai.StopReasonError,
		ErrorMessage: "upstream unavailable",
	}
	provider := newCodingFaux(t, faux.Step{Events: []ai.Event{
		ai.ErrorEvent{Message: failed},
	}})
	session := newHistoryTestSession(t, provider)

	history, err := session.advanceHistory(context.Background(), "try once")
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

func TestSessionPreCanceledRunReturnsUnchangedHistory(t *testing.T) {
	t.Parallel()

	answer := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "done"}},
		StopReason: ai.StopReasonStop,
	}
	provider := newCodingFaux(t, codingAssistantStep(answer))
	session := newHistoryTestSession(t, provider)
	wantHistory, err := session.advanceHistory(context.Background(), "accepted")
	if err != nil {
		t.Fatalf("first run error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	history, err := session.advanceHistory(ctx, "not accepted")
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

func TestSessionRejectsConcurrentRunWithoutQueueing(t *testing.T) {
	t.Parallel()

	provider := &blockingCodingProvider{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	session := newHistoryTestSession(t, provider)
	type runReturn struct {
		history []ai.Message
		err     error
	}
	firstReturned := make(chan runReturn, 1)
	go func() {
		history, err := session.advanceHistory(context.Background(), "first input")
		firstReturned <- runReturn{history: history, err: err}
	}()
	<-provider.started

	secondReturned := make(chan runReturn, 1)
	go func() {
		history, err := session.advanceHistory(context.Background(), "second input")
		secondReturned <- runReturn{history: history, err: err}
	}()
	select {
	case second := <-secondReturned:
		if !errors.Is(second.err, ErrSessionBusy) {
			t.Fatalf("second Advance error = %v, want ErrSessionBusy", second.err)
		}
		if len(second.history) != 0 {
			t.Fatalf("second history = %#v, want settled empty history", second.history)
		}
	case <-time.After(time.Second):
		t.Fatal("second Advance blocked instead of returning ErrSessionBusy")
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

func newHistoryTestSession(t *testing.T, provider ai.Provider) *Session {
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

func newCompactingTestSession(t *testing.T, provider ai.Provider) *Session {
	t.Helper()
	return newCompactingTestSessionWithPolicy(t, provider, testCompactionPolicy())
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

func newCompactingTestSessionWithPolicy(
	t *testing.T,
	provider ai.Provider,
	policy compactionPolicy,
) *Session {
	t.Helper()
	limits := ai.RequestLimits{
		ContextCapacity: 1000,
		ModelMaxOutput:  400,
		ContextSafety:   10,
	}
	engine, err := agent.New(agent.Config{
		Provider:      provider,
		SystemPrompt:  "system",
		RequestLimits: limits,
	})
	if err != nil {
		t.Fatalf("agent.New() error = %v", err)
	}
	session, err := newSession(sessionDependencies{
		Engine:         engine,
		Provider:       provider,
		RequestLimits:  limits,
		Compaction:     policy,
		CloseWorkspace: func() error { return nil },
		Info:           SessionInfo{SystemPrompt: "system"},
	})
	if err != nil {
		t.Fatalf("newSession() error = %v", err)
	}
	return session
}

func withoutUsage(message ai.AssistantMessage) ai.AssistantMessage {
	message.Usage = ai.Usage{}
	return message
}

func messagesContain(messages []ai.Message, fragment string) bool {
	return strings.Contains(serializeConversation(messages), fragment)
}

func newCodingFaux(t *testing.T, steps ...faux.Step) *faux.Provider {
	t.Helper()
	provider, err := faux.New(steps...)
	if err != nil {
		t.Fatalf("faux.New() error = %v", err)
	}
	return provider
}

func codingAssistantStep(message ai.AssistantMessage) faux.Step {
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

type blockingCodingProvider struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (p *blockingCodingProvider) Stream(context.Context, ai.Request) ai.Stream {
	p.once.Do(func() { close(p.started) })
	return &blockingCodingStream{release: p.release, message: p.message()}
}

func (p *blockingCodingProvider) message() ai.AssistantMessage {
	return ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "done"}},
		StopReason: ai.StopReasonStop,
	}
}

type blockingCodingStream struct {
	release chan struct{}
	message ai.AssistantMessage
	done    bool
}

func (s *blockingCodingStream) Receive() (ai.Event, error) {
	if s.done {
		return nil, io.EOF
	}
	<-s.release
	s.done = true
	return ai.DoneEvent{Message: s.message}, nil
}
