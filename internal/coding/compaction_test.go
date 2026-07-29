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

	"github.com/yuanbohan/pia/internal/ai"
	"github.com/yuanbohan/pia/internal/ai/provider/faux"
)

func TestChooseCompactionPlanKeepsToolProtocolWhole(t *testing.T) {
	t.Parallel()

	call := ai.ToolCall{ID: "call-1", Name: "read", Arguments: json.RawMessage(`{"path":"main.go"}`)}
	history := []ai.Message{
		ai.UserMessage{Content: "inspect main.go"},
		ai.AssistantMessage{Content: []ai.AssistantContent{call}, StopReason: ai.StopReasonToolUse},
		ai.ToolResultMessage{ToolCallID: "call-1", ToolName: "read", Content: "12345678"},
		ai.AssistantMessage{Content: []ai.AssistantContent{ai.TextContent{Text: "done"}}, StopReason: ai.StopReasonStop},
	}
	policy := compactionPolicy{
		Threshold:                100,
		SoftCeiling:              90,
		RetainedRawTarget:        6,
		SummaryMaxOutput:         5,
		SplitTurnPrefixMaxOutput: 2,
	}

	plan, err := chooseCompactionPlan(history, 0, "continue", "system", nil, policy)
	if err != nil {
		t.Fatalf("chooseCompactionPlan() error = %v", err)
	}
	if plan.FirstKept != 1 || !plan.SplitTurn {
		t.Fatalf("plan = %#v, want split at tool-calling assistant", plan)
	}
	if got, want := plan.TurnPrefixMessages, []ai.Message{history[0]}; !reflect.DeepEqual(got, want) {
		t.Fatalf("turn prefix = %#v, want %#v", got, want)
	}
	candidate := compactedMessages("summary", history, plan.FirstKept)
	if err := validateWorkingContext(candidate); err != nil {
		t.Fatalf("validateWorkingContext() error = %v", err)
	}
	if _, ok := candidate[1].(ai.AssistantMessage); !ok {
		t.Fatalf("first retained raw message = %T, want assistant", candidate[1])
	}
	if _, ok := candidate[2].(ai.ToolResultMessage); !ok {
		t.Fatalf("second retained raw message = %T, want matching tool result", candidate[2])
	}
}

func TestChooseCompactionPlanSkipsToolResultCutPoint(t *testing.T) {
	t.Parallel()

	history := []ai.Message{
		ai.UserMessage{Content: "inspect"},
		ai.AssistantMessage{
			Content: []ai.AssistantContent{
				ai.ToolCall{ID: "call-1", Name: "read", Arguments: json.RawMessage(`{"path":"main.go"}`)},
			},
			StopReason: ai.StopReasonToolUse,
		},
		ai.ToolResultMessage{ToolCallID: "call-1", ToolName: "read", Content: "12345678"},
		ai.AssistantMessage{Content: []ai.AssistantContent{ai.TextContent{Text: "done"}}, StopReason: ai.StopReasonStop},
	}
	policy := compactionPolicy{
		Threshold:                100,
		SoftCeiling:              90,
		RetainedRawTarget:        2,
		SummaryMaxOutput:         5,
		SplitTurnPrefixMaxOutput: 2,
	}

	plan, err := chooseCompactionPlan(history, 0, "continue", "system", nil, policy)
	if err != nil {
		t.Fatalf("chooseCompactionPlan() error = %v", err)
	}
	if plan.FirstKept != 3 {
		t.Fatalf("FirstKept = %d, want 3 so call and result are summarized together", plan.FirstKept)
	}
	if _, ok := history[plan.FirstKept].(ai.ToolResultMessage); ok {
		t.Fatal("compaction plan begins at an orphaned tool result")
	}
}

func TestChooseCompactionPlanReducesRetainedSuffixToFitSoftCeiling(t *testing.T) {
	t.Parallel()

	history := []ai.Message{
		ai.UserMessage{Content: "1111111111111111"},
		ai.AssistantMessage{Content: []ai.AssistantContent{ai.TextContent{Text: "2222222222222222"}}, StopReason: ai.StopReasonStop},
		ai.UserMessage{Content: "3333333333333333"},
		ai.AssistantMessage{Content: []ai.AssistantContent{ai.TextContent{Text: "4444444444444444"}}, StopReason: ai.StopReasonStop},
		ai.UserMessage{Content: "5555555555555555"},
		ai.AssistantMessage{Content: []ai.AssistantContent{ai.TextContent{Text: "6666666666666666"}}, StopReason: ai.StopReasonStop},
		ai.UserMessage{Content: "7777777777777777"},
		ai.AssistantMessage{Content: []ai.AssistantContent{ai.TextContent{Text: "8888888888888888"}}, StopReason: ai.StopReasonStop},
	}
	policy := compactionPolicy{
		Threshold:                100,
		SoftCeiling:              40,
		RetainedRawTarget:        20,
		SummaryMaxOutput:         1,
		SplitTurnPrefixMaxOutput: 1,
	}

	plan, err := chooseCompactionPlan(history, 0, "next", "sys", nil, policy)
	if err != nil {
		t.Fatalf("chooseCompactionPlan() error = %v", err)
	}
	if got, want := plan.FirstKept, 6; got != want {
		t.Fatalf("FirstKept = %d, want %d after soft-ceiling adjustment", got, want)
	}
	operations := formatFileOperations(extractFileOperations(history[:plan.FirstKept]))
	retainedTokens := estimateMessagesTokens(history[plan.FirstKept:])
	fixedTokens := ai.EstimateRequestTokens(ai.Request{SystemPrompt: "sys"}).Tokens +
		ai.EstimateMessageTokens(ai.UserMessage{Content: "next"})
	forecast := forecastCandidateTokens(plan, retainedTokens, fixedTokens, operations, policy)
	if forecast > policy.SoftCeiling {
		t.Fatalf("forecast = %d, want <= soft ceiling %d", forecast, policy.SoftCeiling)
	}
}

func TestSummarySerializationAndFileOperationsUseCompactionFormat(t *testing.T) {
	t.Parallel()

	longResult := strings.Repeat("界", toolResultSummaryMaxCharacters+1)
	messages := []ai.Message{
		ai.UserMessage{Content: "inspect"},
		ai.AssistantMessage{Content: []ai.AssistantContent{
			ai.ThinkingContent{Thinking: "think"},
			ai.TextContent{Text: "reading"},
			ai.ToolCall{ID: "read-1", Name: "read", Arguments: json.RawMessage(`{"path":"b.go"}`)},
			ai.ToolCall{ID: "edit-1", Name: "edit", Arguments: json.RawMessage(`{"path":"a.go","edits":[]}`)},
			ai.ToolCall{ID: "read-2", Name: "read", Arguments: json.RawMessage(`{"path":"a.go"}`)},
		}, StopReason: ai.StopReasonToolUse},
		ai.ToolResultMessage{ToolCallID: "read-1", ToolName: "read", Content: longResult},
	}

	serialized := serializeConversation(messages)
	for _, required := range []string{
		"[User]: inspect",
		"[Assistant thinking]: think",
		"[Assistant]: reading",
		`[Assistant tool calls]: read(path="b.go"); edit(path="a.go", edits=[]); read(path="a.go")`,
		"[... 1 more characters truncated]",
	} {
		if !strings.Contains(serialized, required) {
			t.Fatalf("serialized conversation missing %q\n%s", required, serialized)
		}
	}
	operations := formatFileOperations(extractFileOperations(messages))
	if got, want := operations, "\n\n<read-files>\nb.go\n</read-files>\n\n<modified-files>\na.go\n</modified-files>"; got != want {
		t.Fatalf("file operations = %q, want %q", got, want)
	}
}

func TestSummaryTextRejectsInvalidTerminalContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		message ai.AssistantMessage
		want    string
	}{
		{
			name:    "empty text",
			message: ai.AssistantMessage{StopReason: ai.StopReasonStop},
			want:    "empty text",
		},
		{
			name: "tool call",
			message: ai.AssistantMessage{
				Content:    []ai.AssistantContent{ai.ToolCall{ID: "call", Name: "read", Arguments: json.RawMessage(`{}`)}},
				StopReason: ai.StopReasonToolUse,
			},
			want: "unexpected tool call",
		},
		{
			name: "unexpected reason",
			message: ai.AssistantMessage{
				Content:    []ai.AssistantContent{ai.TextContent{Text: "summary"}},
				StopReason: ai.StopReasonError,
			},
			want: "unexpected stop reason",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := summaryText(test.message)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("summaryText() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestSessionRejectsCandidateThatCannotFitBelowThreshold(t *testing.T) {
	t.Parallel()

	answer := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "first answer"}},
		Usage:      ai.Usage{InputTokens: 110, OutputTokens: 10},
		StopReason: ai.StopReasonStop,
	}
	provider := newCodingFaux(t,
		codingAssistantStep(answer),
		codingAssistantStep(ai.AssistantMessage{
			Content:    []ai.AssistantContent{ai.TextContent{Text: "turn prefix"}},
			StopReason: ai.StopReasonStop,
		}),
	)
	session := newCompactingTestSession(t, provider)
	wantHistory, err := session.advanceHistory(context.Background(), "first question")
	if err != nil {
		t.Fatalf("first run error = %v", err)
	}

	history, err := session.advanceHistory(context.Background(), strings.Repeat("x", 400))
	if err == nil || !strings.Contains(err.Error(), "still at or above threshold") {
		t.Fatalf("oversized candidate error = %v, want threshold failure", err)
	}
	if !reflect.DeepEqual(history, wantHistory) {
		t.Fatalf("History after oversized candidate = %#v, want %#v", history, wantHistory)
	}
	requests := provider.Requests()
	if got, want := len(requests), 2; got != want {
		t.Fatalf("Provider requests = %d, want one coding plus one summary", got)
	}
	if got, want := requests[1].MaxOutputTokens, int64(8); got != want {
		t.Fatalf("split-prefix summary max output = %d, want %d", got, want)
	}
	prompt := requests[1].Messages[0].(ai.UserMessage).Content
	if !strings.Contains(prompt, turnPrefixSummarizationPrompt) {
		t.Fatalf("split-prefix summary request missing Pi prompt\n%s", prompt)
	}
}

func TestSessionAllowsCandidateAboveSoftCeilingBelowThreshold(t *testing.T) {
	t.Parallel()

	first := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "first answer"}},
		Usage:      ai.Usage{InputTokens: 110, OutputTokens: 10},
		StopReason: ai.StopReasonStop,
	}
	second := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "second answer"}},
		StopReason: ai.StopReasonStop,
	}
	provider := newCodingFaux(t,
		codingAssistantStep(first),
		codingAssistantStep(ai.AssistantMessage{
			Content:    []ai.AssistantContent{ai.TextContent{Text: "prefix"}},
			StopReason: ai.StopReasonStop,
		}),
		codingAssistantStep(second),
	)
	policy := testCompactionPolicy()
	policy.SoftCeiling = 30
	session := newCompactingTestSessionWithPolicy(t, provider, policy)

	if _, err := session.advanceHistory(context.Background(), "first question"); err != nil {
		t.Fatalf("first run error = %v", err)
	}
	if _, err := session.advanceHistory(context.Background(), "next"); err != nil {
		t.Fatalf("second run error = %v", err)
	}

	requests := provider.Requests()
	if got, want := len(requests), 3; got != want {
		t.Fatalf("Provider requests = %d, want %d", got, want)
	}
	actual := ai.EstimateRequestTokens(requests[2]).Tokens
	if actual <= policy.SoftCeiling || actual >= policy.Threshold {
		t.Fatalf(
			"compacted request estimate = %d, want soft ceiling %d < estimate < threshold %d",
			actual,
			policy.SoftCeiling,
			policy.Threshold,
		)
	}
}

func TestSessionInvalidSummaryLeavesStateUnchanged(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		step faux.Step
		want string
	}{
		{
			name: "empty summary",
			step: codingAssistantStep(ai.AssistantMessage{StopReason: ai.StopReasonStop}),
			want: "empty text",
		},
		{
			name: "tool call",
			step: fauxToolStep(ai.ToolCall{ID: "call", Name: "read", Arguments: json.RawMessage(`{}`)}),
			want: "unexpected tool call",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			answer := ai.AssistantMessage{
				Content:    []ai.AssistantContent{ai.TextContent{Text: "first answer"}},
				Usage:      ai.Usage{InputTokens: 110, OutputTokens: 10},
				StopReason: ai.StopReasonStop,
			}
			provider := newCodingFaux(t, codingAssistantStep(answer), test.step)
			session := newCompactingTestSession(t, provider)
			wantHistory, err := session.advanceHistory(context.Background(), "first question")
			if err != nil {
				t.Fatalf("first run error = %v", err)
			}

			history, err := session.advanceHistory(context.Background(), "not accepted")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("invalid summary error = %v, want %q", err, test.want)
			}
			if !reflect.DeepEqual(history, wantHistory) {
				t.Fatalf("History after invalid summary = %#v, want %#v", history, wantHistory)
			}
		})
	}
}

func TestSessionCancellationDuringSummaryKeepsStateAndGuard(t *testing.T) {
	t.Parallel()

	cancelErr := errors.New("operator canceled compaction")
	provider := &blockingSummaryProvider{
		summaryStarted: make(chan struct{}),
		first: ai.AssistantMessage{
			Content:    []ai.AssistantContent{ai.TextContent{Text: "first answer"}},
			Usage:      ai.Usage{InputTokens: 110, OutputTokens: 10},
			StopReason: ai.StopReasonStop,
		},
		recovered: ai.AssistantMessage{
			Content:    []ai.AssistantContent{ai.TextContent{Text: "recovered"}},
			StopReason: ai.StopReasonStop,
		},
	}
	session := newCompactingTestSession(t, provider)
	wantHistory, err := session.advanceHistory(context.Background(), "first question")
	if err != nil {
		t.Fatalf("first run error = %v", err)
	}

	ctx, cancel := context.WithCancelCause(context.Background())
	type runResult struct {
		history []ai.Message
		err     error
	}
	returned := make(chan runResult, 1)
	go func() {
		history, runErr := session.advanceHistory(ctx, "canceled input")
		returned <- runResult{history: history, err: runErr}
	}()
	select {
	case <-provider.summaryStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("summary request did not start")
	}

	concurrentHistory, concurrentErr := session.advanceHistory(context.Background(), "concurrent input")
	if !errors.Is(concurrentErr, ErrSessionBusy) {
		t.Fatalf("concurrent Advance error = %v, want ErrSessionBusy", concurrentErr)
	}
	if !reflect.DeepEqual(concurrentHistory, wantHistory) {
		t.Fatalf("concurrent History = %#v, want %#v", concurrentHistory, wantHistory)
	}

	cancel(cancelErr)
	select {
	case result := <-returned:
		if !errors.Is(result.err, cancelErr) {
			t.Fatalf("canceled run error = %v, want cancellation cause", result.err)
		}
		if !reflect.DeepEqual(result.history, wantHistory) {
			t.Fatalf("History after cancellation = %#v, want %#v", result.history, wantHistory)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("canceled compaction did not return")
	}

	history, err := session.advanceHistory(context.Background(), "accepted after cancel")
	if err != nil {
		t.Fatalf("retry run error = %v", err)
	}
	wantFinal := append(ai.CloneMessages(wantHistory),
		ai.UserMessage{Content: "accepted after cancel"},
		provider.recovered,
	)
	if !reflect.DeepEqual(history, wantFinal) {
		t.Fatalf("History after retry = %#v, want %#v", history, wantFinal)
	}
}

type blockingSummaryProvider struct {
	mu             sync.Mutex
	next           int
	requests       []ai.Request
	summaryStarted chan struct{}
	first          ai.AssistantMessage
	recovered      ai.AssistantMessage
}

func (provider *blockingSummaryProvider) Stream(ctx context.Context, request ai.Request) ai.Stream {
	provider.mu.Lock()
	call := provider.next
	provider.next++
	provider.requests = append(provider.requests, ai.CloneRequest(request))
	provider.mu.Unlock()

	switch call {
	case 0:
		return &oneEventStream{event: ai.DoneEvent{Message: provider.first}}
	case 1:
		return &cancelBlockingStream{ctx: ctx, started: provider.summaryStarted}
	case 2:
		return &oneEventStream{event: ai.DoneEvent{Message: ai.AssistantMessage{
			Content:    []ai.AssistantContent{ai.TextContent{Text: "checkpoint"}},
			StopReason: ai.StopReasonStop,
		}}}
	default:
		return &oneEventStream{event: ai.DoneEvent{Message: provider.recovered}}
	}
}

type oneEventStream struct {
	event ai.Event
	done  bool
}

func (stream *oneEventStream) Receive() (ai.Event, error) {
	if stream.done {
		return nil, io.EOF
	}
	stream.done = true
	return stream.event, nil
}

type cancelBlockingStream struct {
	ctx     context.Context
	started chan struct{}
	once    sync.Once
	done    bool
}

func (stream *cancelBlockingStream) Receive() (ai.Event, error) {
	if stream.done {
		return nil, io.EOF
	}
	stream.once.Do(func() { close(stream.started) })
	<-stream.ctx.Done()
	stream.done = true
	return ai.ErrorEvent{Message: ai.AssistantMessage{
		StopReason:   ai.StopReasonAborted,
		ErrorMessage: context.Cause(stream.ctx).Error(),
	}}, nil
}

var _ ai.Provider = (*blockingSummaryProvider)(nil)
var _ ai.Stream = (*oneEventStream)(nil)
var _ ai.Stream = (*cancelBlockingStream)(nil)
