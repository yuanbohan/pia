package coding

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yuanbohan/pia/internal/agent"
	"github.com/yuanbohan/pia/internal/ai"
	"github.com/yuanbohan/pia/internal/ai/provider/faux"
	"github.com/yuanbohan/pia/internal/ai/provider/openaicompatible"
)

func TestExplicitContextOverflowClassification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		message ai.AssistantMessage
		want    bool
	}{
		{
			name: "context length exceeded",
			message: ai.AssistantMessage{
				StopReason:   ai.StopReasonError,
				ErrorMessage: `openai-compatible: HTTP status 400: {"error":{"message":"Context length exceeded"}}`,
			},
			want: true,
		},
		{
			name: "maximum model context length",
			message: ai.AssistantMessage{
				StopReason:   ai.StopReasonError,
				ErrorMessage: "Input length (265330) exceeds model's maximum context length (262144).",
			},
			want: true,
		},
		{
			name: "context window exceeded",
			message: ai.AssistantMessage{
				StopReason:   ai.StopReasonError,
				ErrorMessage: "Your input exceeds the context window of this model.",
			},
			want: true,
		},
		{
			name: "generic bad request",
			message: ai.AssistantMessage{
				StopReason:   ai.StopReasonError,
				ErrorMessage: "openai-compatible: HTTP status 400: invalid request format",
			},
		},
		{
			name: "invalid parameters",
			message: ai.AssistantMessage{
				StopReason:   ai.StopReasonError,
				ErrorMessage: "openai-compatible: HTTP status 422: invalid parameters",
			},
		},
		{
			name: "request entity too large without context evidence",
			message: ai.AssistantMessage{
				StopReason:   ai.StopReasonError,
				ErrorMessage: "openai-compatible: HTTP status 413: request entity too large",
			},
		},
		{
			name: "rate limit wins over overflow phrase",
			message: ai.AssistantMessage{
				StopReason:   ai.StopReasonError,
				ErrorMessage: "rate limit exceeded while checking context length exceeded",
			},
		},
		{
			name: "hyphenated rate limit wins over overflow phrase",
			message: ai.AssistantMessage{
				StopReason:   ai.StopReasonError,
				ErrorMessage: "rate-limit exceeded while checking context length exceeded",
			},
		},
		{
			name: "too many requests",
			message: ai.AssistantMessage{
				StopReason:   ai.StopReasonError,
				ErrorMessage: "HTTP 429 too many requests",
			},
		},
		{
			name: "429 wins over overflow phrase",
			message: ai.AssistantMessage{
				StopReason:   ai.StopReasonError,
				ErrorMessage: "HTTP status 429: context length exceeded",
			},
		},
		{
			name: "server overload",
			message: ai.AssistantMessage{
				StopReason:   ai.StopReasonError,
				ErrorMessage: "server overloaded: context window request rejected",
			},
		},
		{
			name: "internal server error",
			message: ai.AssistantMessage{
				StopReason:   ai.StopReasonError,
				ErrorMessage: "HTTP 500 internal server error",
			},
		},
		{
			name: "500 wins over overflow phrase",
			message: ai.AssistantMessage{
				StopReason:   ai.StopReasonError,
				ErrorMessage: "HTTP status 500: context length exceeded",
			},
		},
		{
			name: "service unavailable with overflow wording",
			message: ai.AssistantMessage{
				StopReason:   ai.StopReasonError,
				ErrorMessage: "HTTP 503 service unavailable while validating context length exceeded",
			},
		},
		{
			name: "length terminal",
			message: ai.AssistantMessage{
				StopReason:   ai.StopReasonLength,
				ErrorMessage: "context length exceeded",
			},
		},
		{
			name: "aborted terminal",
			message: ai.AssistantMessage{
				StopReason:   ai.StopReasonAborted,
				ErrorMessage: "context length exceeded",
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := isExplicitContextOverflow(test.message); got != test.want {
				t.Fatalf("isExplicitContextOverflow() = %t, want %t for %q", got, test.want, test.message.ErrorMessage)
			}
		})
	}
}

func TestOpenAICompatibleHTTPOverflowTerminalReachesClassifier(t *testing.T) {
	t.Parallel()

	const credential = "provider-secret"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got, want := request.Header.Get("Authorization"), "Bearer "+credential; got != want {
			t.Errorf("Authorization = %q, want %q", got, want)
		}
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write([]byte(`{"error":{"message":"Context length exceeded for provider-secret"}}`))
	}))
	defer server.Close()

	provider, err := openaicompatible.New(openaicompatible.Config{
		HTTPClient: server.Client(),
		Endpoint:   server.URL,
		APIKey:     credential,
		Model:      "fixture-model",
	})
	if err != nil {
		t.Fatalf("openaicompatible.New() error = %v", err)
	}
	event, receiveErr := provider.Stream(context.Background(), ai.Request{}).Receive()
	if receiveErr != nil {
		t.Fatalf("Receive() error = %v", receiveErr)
	}
	errorEvent, ok := event.(ai.ErrorEvent)
	if !ok {
		t.Fatalf("terminal event = %T, want ai.ErrorEvent", event)
	}
	if strings.Contains(errorEvent.Message.ErrorMessage, credential) {
		t.Fatalf("terminal leaked configured credential: %q", errorEvent.Message.ErrorMessage)
	}
	if !isExplicitContextOverflow(errorEvent.Message) {
		t.Fatalf("classifier rejected wire terminal: %#v", errorEvent.Message)
	}
}

func TestSessionRecoversOverflowWithoutRepeatingUserInput(t *testing.T) {
	t.Parallel()

	first := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "first answer"}},
		Usage:      ai.Usage{InputTokens: 20, OutputTokens: 5},
		StopReason: ai.StopReasonStop,
	}
	overflow := ai.AssistantMessage{
		StopReason:   ai.StopReasonError,
		ErrorMessage: "Input length (1001) exceeds model's maximum context length (1000).",
	}
	summary := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "earlier work checkpoint"}},
		StopReason: ai.StopReasonStop,
	}
	recovered := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "recovered answer"}},
		StopReason: ai.StopReasonStop,
	}
	provider := newCodingFaux(t,
		codingAssistantStep(first),
		faux.Step{Events: []ai.Event{ai.ErrorEvent{Message: overflow}}},
		codingAssistantStep(summary),
		codingAssistantStep(recovered),
	)
	session := newRecoveryTestSession(t, provider)

	if _, err := session.advanceHistory(context.Background(), "first question"); err != nil {
		t.Fatalf("first run error = %v", err)
	}
	history, err := session.advanceHistory(context.Background(), "continue accepted task")
	if err != nil {
		t.Fatalf("recovered run error = %v", err)
	}

	wantHistory := []ai.Message{
		ai.UserMessage{Content: "first question"},
		first,
		ai.UserMessage{Content: "continue accepted task"},
		overflow,
		recovered,
	}
	if !reflect.DeepEqual(history, wantHistory) {
		t.Fatalf("complete History = %#v, want %#v", history, wantHistory)
	}
	if got := countUserMessages(history, "continue accepted task"); got != 1 {
		t.Fatalf("accepted user input count = %d, want 1", got)
	}

	requests := provider.Requests()
	if got, want := len(requests), 4; got != want {
		t.Fatalf("Provider requests = %d, want coding, coding, summary, continuation", got)
	}
	summaryInput := requests[2].Messages[0].(ai.UserMessage).Content
	if strings.Contains(summaryInput, overflow.ErrorMessage) {
		t.Fatalf("summary input retained overflow error:\n%s", summaryInput)
	}
	if !strings.Contains(summaryInput, "first question") {
		t.Fatalf("summary input lost usable earlier context:\n%s", summaryInput)
	}
	if session.projection == nil {
		t.Fatal("recovery did not publish a projection")
	}
	if !strings.Contains(session.projection.Summary, "earlier work checkpoint") {
		t.Fatalf("published summary = %q, want generated checkpoint", session.projection.Summary)
	}

	wantContinuation := []ai.Message{
		syntheticSummaryMessage(session.projection.Summary),
		withoutUsage(first),
		ai.UserMessage{Content: "continue accepted task"},
	}
	if !reflect.DeepEqual(requests[3].Messages, wantContinuation) {
		t.Fatalf("continuation messages = %#v, want %#v", requests[3].Messages, wantContinuation)
	}

	if got, want := session.projection.Excluded, []int{3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("projection exclusions = %v, want %v", got, want)
	}
	projected, err := projectedMessages(history, session.projection)
	if err != nil {
		t.Fatalf("projectedMessages() error = %v", err)
	}
	if messagesContainAssistantError(projected, overflow.ErrorMessage) {
		t.Fatalf("published projection retained overflow error: %#v", projected)
	}
}

func TestSessionDoesNotRecoverOverflowTerminalWithCompletedToolCall(t *testing.T) {
	t.Parallel()

	overflow := ai.AssistantMessage{
		Content: []ai.AssistantContent{
			ai.ToolCall{
				ID:        "call-1",
				Name:      "read",
				Arguments: json.RawMessage(`{"path":"main.go"}`),
			},
		},
		StopReason:   ai.StopReasonError,
		ErrorMessage: "context length exceeded",
	}
	call := overflow.Content[0].(ai.ToolCall)
	provider := newCodingFaux(t, faux.Step{Events: []ai.Event{
		ai.StartEvent{},
		ai.ToolCallStartEvent{ContentIndex: 0, ID: call.ID, Name: call.Name},
		ai.ToolCallDeltaEvent{ContentIndex: 0, Delta: string(call.Arguments)},
		ai.ToolCallEndEvent{ContentIndex: 0, ToolCall: call},
		ai.ErrorEvent{Message: overflow},
	}})
	session := newRecoveryTestSession(t, provider)

	history, err := session.advanceHistory(context.Background(), "inspect")
	if err == nil || !strings.Contains(err.Error(), "context length exceeded") {
		t.Fatalf("run error = %v, want original overflow failure", err)
	}
	if got, want := len(provider.Requests()), 1; got != want {
		t.Fatalf("Provider requests = %d, want no summary or continuation", got)
	}
	if got, want := len(history), 3; got != want {
		t.Fatalf("History length = %d, want user, error assistant, not-executed result", got)
	}
	if !reflect.DeepEqual(history[1], ai.Message(overflow)) {
		t.Fatalf("History error assistant = %#v, want %#v", history[1], overflow)
	}
	settlement, ok := history[2].(ai.ToolResultMessage)
	if !ok || settlement.ToolCallID != "call-1" || !settlement.IsError ||
		!strings.Contains(settlement.Content, "not executed") {
		t.Fatalf("tool settlement = %#v, want same-ID not-executed result", history[2])
	}
	if session.projection != nil {
		t.Fatalf("ineligible overflow published projection %#v", session.projection)
	}
}

func TestSessionDoesNotRecoverOrdinaryProviderFailure(t *testing.T) {
	t.Parallel()

	failure := ai.AssistantMessage{
		StopReason:   ai.StopReasonError,
		ErrorMessage: "HTTP 503 service unavailable",
	}
	provider := newCodingFaux(t, recoveryErrorStep(failure))
	session := newRecoveryTestSession(t, provider)

	history, err := session.advanceHistory(context.Background(), "finish the task")
	if err == nil || !strings.Contains(err.Error(), failure.ErrorMessage) {
		t.Fatalf("run error = %v, want original Provider failure", err)
	}
	want := []ai.Message{
		ai.UserMessage{Content: "finish the task"},
		failure,
	}
	if !reflect.DeepEqual(history, want) {
		t.Fatalf("History = %#v, want %#v", history, want)
	}
	if got, want := len(provider.Requests()), 1; got != want {
		t.Fatalf("Provider requests = %d, want no summary or continuation", got)
	}
	if session.projection != nil {
		t.Fatalf("ordinary failure published projection %#v", session.projection)
	}
}

func TestSessionRecoveryKeepsCompletedToolResultWithoutReexecution(t *testing.T) {
	t.Parallel()

	call := ai.ToolCall{
		ID:        "call-1",
		Name:      "read",
		Arguments: json.RawMessage(`{"path":"main.go"}`),
	}
	overflow := ai.AssistantMessage{
		StopReason:   ai.StopReasonError,
		ErrorMessage: "context length exceeded",
	}
	summary := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "inspection checkpoint"}},
		StopReason: ai.StopReasonStop,
	}
	recovered := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "finished from the existing result"}},
		StopReason: ai.StopReasonStop,
	}
	provider := newCodingFaux(t,
		fauxToolStep(call),
		recoveryErrorStep(overflow),
		codingAssistantStep(summary),
		codingAssistantStep(recovered),
	)
	tool := &countingRecoveryTool{}
	session := newRecoveryTestSession(t, provider, tool)

	history, err := session.advanceHistory(context.Background(), "inspect and finish")
	if err != nil {
		t.Fatalf("recovered run error = %v", err)
	}
	if got := tool.executions.Load(); got != 1 {
		t.Fatalf("tool executions = %d, want exactly one before recovery", got)
	}
	if got, want := len(history), 5; got != want {
		t.Fatalf("History length = %d, want user, call, result, overflow, recovered", got)
	}
	result, ok := history[2].(ai.ToolResultMessage)
	if !ok || result.ToolCallID != call.ID || result.Content != "package main" {
		t.Fatalf("settled tool result = %#v, want completed call result", history[2])
	}
	if !reflect.DeepEqual(history[3], ai.Message(overflow)) || !reflect.DeepEqual(history[4], ai.Message(recovered)) {
		t.Fatalf("recovery terminals = %#v, want overflow then recovered", history[3:])
	}

	requests := provider.Requests()
	if got, want := len(requests), 4; got != want {
		t.Fatalf("Provider requests = %d, want tool turn, overflow, summary, continuation", got)
	}
	if messagesContainAssistantError(requests[2].Messages, overflow.ErrorMessage) {
		t.Fatalf("summary request retained overflow: %#v", requests[2].Messages)
	}
	if messagesContainAssistantError(requests[3].Messages, overflow.ErrorMessage) {
		t.Fatalf("continuation request retained overflow: %#v", requests[3].Messages)
	}
	if !messagesContain(requests[3].Messages, "package main") {
		t.Fatalf("continuation request lost completed tool result: %#v", requests[3].Messages)
	}
}

func TestSessionStopsAfterSecondOverflowInOneUserAdvance(t *testing.T) {
	t.Parallel()

	first := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "first answer"}},
		StopReason: ai.StopReasonStop,
	}
	overflowOne := ai.AssistantMessage{
		StopReason:   ai.StopReasonError,
		ErrorMessage: "context length exceeded on initial execution",
	}
	overflowTwo := ai.AssistantMessage{
		StopReason:   ai.StopReasonError,
		ErrorMessage: "context length exceeded on continuation",
	}
	summary := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "checkpoint"}},
		StopReason: ai.StopReasonStop,
	}
	provider := newCodingFaux(t,
		codingAssistantStep(first),
		recoveryErrorStep(overflowOne),
		codingAssistantStep(summary),
		recoveryErrorStep(overflowTwo),
	)
	session := newRecoveryTestSession(t, provider)
	if _, err := session.advanceHistory(context.Background(), "first question"); err != nil {
		t.Fatalf("first run error = %v", err)
	}

	history, err := session.advanceHistory(context.Background(), "finish once")
	if err == nil || !strings.Contains(err.Error(), "overflow recovery exhausted") ||
		!strings.Contains(err.Error(), overflowTwo.ErrorMessage) {
		t.Fatalf("second run error = %v, want exhausted recovery preserving second cause", err)
	}
	if got, want := len(provider.Requests()), 4; got != want {
		t.Fatalf("Provider requests = %d, want exactly one summary and one continuation", got)
	}
	if got := countUserMessages(history, "finish once"); got != 1 {
		t.Fatalf("accepted user input count = %d, want 1", got)
	}
	if !messagesContainAssistantError(history, overflowOne.ErrorMessage) ||
		!messagesContainAssistantError(history, overflowTwo.ErrorMessage) {
		t.Fatalf("complete History lost overflow facts: %#v", history)
	}
	if session.projection == nil || !reflect.DeepEqual(session.projection.Excluded, []int{3}) {
		t.Fatalf("projection = %#v, want only initial overflow excluded", session.projection)
	}
}

func TestSessionSummaryFailureKeepsOverflowContextAndProjectionUnchanged(t *testing.T) {
	t.Parallel()

	first := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "first answer"}},
		StopReason: ai.StopReasonStop,
	}
	overflow := ai.AssistantMessage{
		StopReason:   ai.StopReasonError,
		ErrorMessage: "context window exceeded",
	}
	summaryFailure := ai.AssistantMessage{
		StopReason:   ai.StopReasonError,
		ErrorMessage: "summary service unavailable",
	}
	provider := newCodingFaux(t,
		codingAssistantStep(first),
		recoveryErrorStep(overflow),
		recoveryErrorStep(summaryFailure),
	)
	session := newRecoveryTestSession(t, provider)
	if _, err := session.advanceHistory(context.Background(), "first question"); err != nil {
		t.Fatalf("first run error = %v", err)
	}

	history, err := session.advanceHistory(context.Background(), "accepted before overflow")
	if err == nil || !strings.Contains(err.Error(), "recover context overflow") ||
		!strings.Contains(err.Error(), summaryFailure.ErrorMessage) {
		t.Fatalf("recovery error = %v, want wrapped summary cause", err)
	}
	if !messagesContainAssistantError(history, overflow.ErrorMessage) {
		t.Fatalf("complete History lost overflow: %#v", history)
	}
	if session.projection != nil {
		t.Fatalf("failed summary published projection %#v", session.projection)
	}
	if got, want := len(provider.Requests()), 3; got != want {
		t.Fatalf("Provider requests = %d, want no continuation", got)
	}
}

func TestLaterRecoveryFailurePreservesPreviouslyCommittedProjection(t *testing.T) {
	t.Parallel()

	first := textAssistant("first answer")
	overflowOne := ai.AssistantMessage{
		StopReason:   ai.StopReasonError,
		ErrorMessage: "context length exceeded: first marker",
	}
	recoveredOne := textAssistant("first recovered answer")
	overflowTwo := ai.AssistantMessage{
		StopReason:   ai.StopReasonError,
		ErrorMessage: "context length exceeded: second marker",
	}
	summaryFailure := ai.AssistantMessage{
		StopReason:   ai.StopReasonError,
		ErrorMessage: "summary unavailable",
	}
	provider := newCodingFaux(t,
		codingAssistantStep(first),
		recoveryErrorStep(overflowOne),
		codingAssistantStep(textAssistant("checkpoint one")),
		codingAssistantStep(recoveredOne),
		recoveryErrorStep(overflowTwo),
		recoveryErrorStep(summaryFailure),
	)
	session := newRecoveryTestSession(t, provider)
	if _, err := session.advanceHistory(context.Background(), "first question"); err != nil {
		t.Fatalf("first run error = %v", err)
	}
	if _, err := session.advanceHistory(context.Background(), "first recovery task"); err != nil {
		t.Fatalf("first recovery run error = %v", err)
	}
	_, committed := session.compactionSnapshot()
	if committed == nil {
		t.Fatal("first recovery did not publish a projection")
	}

	history, err := session.advanceHistory(context.Background(), "second recovery task")
	if err == nil || !strings.Contains(err.Error(), summaryFailure.ErrorMessage) {
		t.Fatalf("second recovery error = %v, want summary failure", err)
	}
	_, afterFailure := session.compactionSnapshot()
	if !reflect.DeepEqual(afterFailure, committed) {
		t.Fatalf("projection after failure = %#v, want prior committed projection %#v", afterFailure, committed)
	}
	if !messagesContainAssistantError(history, overflowOne.ErrorMessage) ||
		!messagesContainAssistantError(history, overflowTwo.ErrorMessage) {
		t.Fatalf("complete History lost overflow facts: %#v", history)
	}
	projected, projectErr := projectedMessages(history, afterFailure)
	if projectErr != nil {
		t.Fatalf("projectedMessages() error = %v", projectErr)
	}
	if messagesContainAssistantError(projected, overflowOne.ErrorMessage) {
		t.Fatalf("old committed exclusion was lost: %#v", projected)
	}
	if !messagesContainAssistantError(projected, overflowTwo.ErrorMessage) {
		t.Fatalf("failed candidate unexpectedly excluded new overflow: %#v", projected)
	}
}

func TestSessionRejectsOversizedRecoveryCandidateWithoutPublishing(t *testing.T) {
	t.Parallel()

	first := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "first answer"}},
		StopReason: ai.StopReasonStop,
	}
	overflow := ai.AssistantMessage{
		StopReason:   ai.StopReasonError,
		ErrorMessage: "maximum context length exceeded",
	}
	oversizedSummary := ai.AssistantMessage{
		Content: []ai.AssistantContent{
			ai.TextContent{Text: strings.Repeat("oversized checkpoint ", 400)},
		},
		StopReason: ai.StopReasonStop,
	}
	provider := newCodingFaux(t,
		codingAssistantStep(first),
		recoveryErrorStep(overflow),
		codingAssistantStep(oversizedSummary),
	)
	session := newRecoveryTestSession(t, provider)
	if _, err := session.advanceHistory(context.Background(), "first question"); err != nil {
		t.Fatalf("first run error = %v", err)
	}

	history, err := session.advanceHistory(context.Background(), "accepted before overflow")
	if err == nil || !strings.Contains(err.Error(), "still at or above threshold") {
		t.Fatalf("recovery error = %v, want oversized candidate rejection", err)
	}
	if !messagesContainAssistantError(history, overflow.ErrorMessage) {
		t.Fatalf("complete History lost overflow: %#v", history)
	}
	if session.projection != nil {
		t.Fatalf("oversized candidate published projection %#v", session.projection)
	}
	if got, want := len(provider.Requests()), 3; got != want {
		t.Fatalf("Provider requests = %d, want no continuation", got)
	}
}

func TestSessionCancellationDuringRecoverySummaryKeepsCandidateUnpublished(t *testing.T) {
	t.Parallel()

	cancelErr := errors.New("operator canceled recovery summary")
	first := textAssistant("first answer")
	overflow := ai.AssistantMessage{
		StopReason:   ai.StopReasonError,
		ErrorMessage: "context length exceeded",
	}
	summaryStarted := make(chan struct{})
	provider := newScriptedRecoveryProvider(
		eventStreamFactory(ai.DoneEvent{Message: first}),
		eventStreamFactory(ai.ErrorEvent{Message: overflow}),
		func(ctx context.Context) ai.Stream {
			return &cancelBlockingStream{ctx: ctx, started: summaryStarted}
		},
	)
	session := newRecoveryTestSession(t, provider)
	wantBeforeRecovery, err := session.advanceHistory(context.Background(), "first question")
	if err != nil {
		t.Fatalf("first run error = %v", err)
	}

	ctx, cancel := context.WithCancelCause(context.Background())
	type runReturn struct {
		history []ai.Message
		err     error
	}
	returned := make(chan runReturn, 1)
	go func() {
		history, runErr := session.advanceHistory(ctx, "accepted before cancellation")
		returned <- runReturn{history: history, err: runErr}
	}()
	select {
	case <-summaryStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("recovery summary did not start")
	}

	concurrentHistory, concurrentErr := session.advanceHistory(context.Background(), "must be rejected")
	if !errors.Is(concurrentErr, ErrSessionBusy) {
		t.Fatalf("concurrent Advance error = %v, want ErrSessionBusy", concurrentErr)
	}
	wantInProgress := append(ai.CloneMessages(wantBeforeRecovery),
		ai.UserMessage{Content: "accepted before cancellation"},
		overflow,
	)
	if !reflect.DeepEqual(concurrentHistory, wantInProgress) {
		t.Fatalf("concurrent History = %#v, want committed overflow %#v", concurrentHistory, wantInProgress)
	}

	cancel(cancelErr)
	select {
	case result := <-returned:
		if !errors.Is(result.err, cancelErr) {
			t.Fatalf("canceled recovery error = %v, want context cause", result.err)
		}
		if !reflect.DeepEqual(result.history, wantInProgress) {
			t.Fatalf("canceled recovery History = %#v, want %#v", result.history, wantInProgress)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("canceled recovery did not settle")
	}
	if session.projection != nil {
		t.Fatalf("summary cancellation published projection %#v", session.projection)
	}
}

func TestSessionCancellationDuringRecoveryContinuationKeepsCommittedProjection(t *testing.T) {
	t.Parallel()

	cancelErr := errors.New("operator canceled continuation")
	first := textAssistant("first answer")
	overflow := ai.AssistantMessage{
		StopReason:   ai.StopReasonError,
		ErrorMessage: "context length exceeded",
	}
	summary := textAssistant("checkpoint")
	continuationStarted := make(chan struct{})
	provider := newScriptedRecoveryProvider(
		eventStreamFactory(ai.DoneEvent{Message: first}),
		eventStreamFactory(ai.ErrorEvent{Message: overflow}),
		eventStreamFactory(ai.DoneEvent{Message: summary}),
		func(ctx context.Context) ai.Stream {
			return &cancelBlockingStream{ctx: ctx, started: continuationStarted}
		},
	)
	session := newRecoveryTestSession(t, provider)
	if _, err := session.advanceHistory(context.Background(), "first question"); err != nil {
		t.Fatalf("first run error = %v", err)
	}

	ctx, cancel := context.WithCancelCause(context.Background())
	type runReturn struct {
		history []ai.Message
		err     error
	}
	returned := make(chan runReturn, 1)
	go func() {
		history, runErr := session.advanceHistory(ctx, "accepted before cancellation")
		returned <- runReturn{history: history, err: runErr}
	}()
	select {
	case <-continuationStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("recovery continuation did not start")
	}

	if session.projection == nil || !reflect.DeepEqual(session.projection.Excluded, []int{3}) {
		t.Fatalf("projection before continuation settlement = %#v, want committed exclusion", session.projection)
	}
	if _, concurrentErr := session.advanceHistory(context.Background(), "must be rejected"); !errors.Is(concurrentErr, ErrSessionBusy) {
		t.Fatalf("concurrent Advance error = %v, want ErrSessionBusy", concurrentErr)
	}

	cancel(cancelErr)
	select {
	case result := <-returned:
		if !errors.Is(result.err, cancelErr) {
			t.Fatalf("canceled continuation error = %v, want context cause", result.err)
		}
		if got := countUserMessages(result.history, "accepted before cancellation"); got != 1 {
			t.Fatalf("accepted user count = %d, want 1", got)
		}
		if len(result.history) == 0 {
			t.Fatal("canceled continuation returned empty History")
		}
		terminal, ok := result.history[len(result.history)-1].(ai.AssistantMessage)
		if !ok || terminal.StopReason != ai.StopReasonAborted ||
			!strings.Contains(terminal.ErrorMessage, cancelErr.Error()) {
			t.Fatalf("continuation terminal = %#v, want aborted context cause", result.history[len(result.history)-1])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("canceled continuation did not settle")
	}
	if session.projection == nil || !reflect.DeepEqual(session.projection.Excluded, []int{3}) {
		t.Fatalf("cancellation rolled back committed projection %#v", session.projection)
	}
}

func TestExcludedOverflowStaysOutAcrossThresholdCompactionAndLaterRecovery(t *testing.T) {
	t.Parallel()

	first := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "first answer"}},
		StopReason: ai.StopReasonStop,
	}
	overflowOne := ai.AssistantMessage{
		StopReason:   ai.StopReasonError,
		ErrorMessage: "context length exceeded: first recovery marker",
	}
	recoveredOne := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "first recovered answer"}},
		Usage:      ai.Usage{InputTokens: 110, OutputTokens: 10},
		StopReason: ai.StopReasonStop,
	}
	ordinary := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "ordinary answer"}},
		Usage:      ai.Usage{InputTokens: 20, OutputTokens: 5},
		StopReason: ai.StopReasonStop,
	}
	overflowTwo := ai.AssistantMessage{
		StopReason:   ai.StopReasonError,
		ErrorMessage: "context length exceeded: second recovery marker",
	}
	recoveredTwo := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "second recovered answer"}},
		StopReason: ai.StopReasonStop,
	}
	provider := newCodingFaux(t,
		codingAssistantStep(first),                   // 0: first coding Run
		recoveryErrorStep(overflowOne),               // 1: first overflow
		codingAssistantStep(textAssistant("r1")),     // 2: first recovery summary
		codingAssistantStep(recoveredOne),            // 3: first continuation
		codingAssistantStep(textAssistant("normal")), // 4: threshold history summary
		codingAssistantStep(textAssistant("prefix")), // 5: threshold turn-prefix summary
		codingAssistantStep(ordinary),                // 6: ordinary coding Run
		recoveryErrorStep(overflowTwo),               // 7: second overflow
		codingAssistantStep(textAssistant("r2")),     // 8: second recovery summary
		codingAssistantStep(recoveredTwo),            // 9: second continuation
	)
	session := newRecoveryTestSession(t, provider)
	if _, err := session.advanceHistory(context.Background(), "first question"); err != nil {
		t.Fatalf("first run error = %v", err)
	}
	if _, err := session.advanceHistory(context.Background(), "first recovery task"); err != nil {
		t.Fatalf("first recovery run error = %v", err)
	}

	session.compaction = testCompactionPolicy()
	session.compaction.RetainedRawTarget = 1
	if _, err := session.advanceHistory(context.Background(), "ordinary task"); err != nil {
		t.Fatalf("threshold-compacted run error = %v", err)
	}
	history, err := session.advanceHistory(context.Background(), "second recovery task")
	if err != nil {
		t.Fatalf("second recovery run error = %v", err)
	}

	requests := provider.Requests()
	if got, want := len(requests), 10; got != want {
		t.Fatalf("Provider requests = %d, want %d", got, want)
	}
	for _, index := range []int{4, 5, 6, 7, 8, 9} {
		if messagesContainAssistantError(requests[index].Messages, overflowOne.ErrorMessage) {
			t.Fatalf("request %d reintroduced first overflow: %#v", index, requests[index].Messages)
		}
	}
	for _, index := range []int{8, 9} {
		if messagesContainAssistantError(requests[index].Messages, overflowTwo.ErrorMessage) {
			t.Fatalf("request %d retained current overflow: %#v", index, requests[index].Messages)
		}
	}
	if !messagesContainAssistantError(history, overflowOne.ErrorMessage) ||
		!messagesContainAssistantError(history, overflowTwo.ErrorMessage) {
		t.Fatalf("complete History lost overflow facts: %#v", history)
	}
	if got := countUserMessages(history, "first recovery task"); got != 1 {
		t.Fatalf("first recovery user count = %d, want 1", got)
	}
	if got := countUserMessages(history, "second recovery task"); got != 1 {
		t.Fatalf("second recovery user count = %d, want 1", got)
	}
	if session.projection == nil || !reflect.DeepEqual(session.projection.Excluded, []int{8}) {
		t.Fatalf("final projection = %#v, want only second absolute overflow position", session.projection)
	}
}

func TestProjectionExclusionsAreOwnershipIndependent(t *testing.T) {
	t.Parallel()

	provider := newCodingFaux(t)
	session := newHistoryTestSession(t, provider)
	session.history = []ai.Message{
		ai.UserMessage{Content: "question"},
		ai.AssistantMessage{StopReason: ai.StopReasonError, ErrorMessage: "context length exceeded"},
	}
	projection := compactionProjection{
		Summary:        "checkpoint",
		FirstKept:      0,
		UsageValidFrom: 2,
		Excluded:       []int{1},
	}
	session.publishProjection(projection)
	projection.Excluded[0] = 0

	_, snapshot := session.compactionSnapshot()
	if snapshot == nil || !reflect.DeepEqual(snapshot.Excluded, []int{1}) {
		t.Fatalf("projection snapshot = %#v, want independent exclusion [1]", snapshot)
	}
	snapshot.Excluded[0] = 0
	if !reflect.DeepEqual(session.projection.Excluded, []int{1}) {
		t.Fatalf("snapshot mutation changed owner projection: %#v", session.projection)
	}
}

func TestCompactionModelSourceFiltersExclusionsAndPreservesAbsolutePositions(t *testing.T) {
	t.Parallel()

	retainedAssistant := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "usable answer"}},
		Usage:      ai.Usage{InputTokens: 30, OutputTokens: 5},
		StopReason: ai.StopReasonStop,
	}
	history := []ai.Message{
		ai.UserMessage{Content: "already summarized"},
		retainedAssistant,
		ai.AssistantMessage{StopReason: ai.StopReasonError, ErrorMessage: "old overflow"},
		ai.UserMessage{Content: "current accepted input"},
		ai.AssistantMessage{StopReason: ai.StopReasonError, ErrorMessage: "new overflow"},
	}
	projection := &compactionProjection{
		Summary:        "previous checkpoint",
		FirstKept:      1,
		UsageValidFrom: 3,
		Excluded:       []int{2},
	}

	source, err := buildCompactionModelSource(history, projection, []int{4})
	if err != nil {
		t.Fatalf("buildCompactionModelSource() error = %v", err)
	}
	if got, want := source.Positions, []int{1, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("source absolute positions = %v, want %v", got, want)
	}
	if got, want := source.Excluded, []int{2, 4}; !reflect.DeepEqual(got, want) {
		t.Fatalf("source exclusions = %v, want %v", got, want)
	}
	wantMessages := []ai.Message{
		withoutUsage(retainedAssistant),
		ai.UserMessage{Content: "current accepted input"},
	}
	if !reflect.DeepEqual(source.Messages, wantMessages) {
		t.Fatalf("source messages = %#v, want filtered usable messages %#v", source.Messages, wantMessages)
	}
	wantCurrent := append(
		[]ai.Message{syntheticSummaryMessage("previous checkpoint")},
		wantMessages...,
	)
	if got := source.currentMessages(); !reflect.DeepEqual(got, wantCurrent) {
		t.Fatalf("current projected messages = %#v, want %#v", got, wantCurrent)
	}
}

func TestSessionWithProviderProjectsRecoveredFinalTextAndCompleteTrace(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	if err := os.WriteFile(workspace+"/main.go", []byte("package main\n"), 0o600); err != nil {
		t.Fatalf("write workspace fixture: %v", err)
	}
	call := ai.ToolCall{
		ID:        "read-1",
		Name:      "read",
		Arguments: json.RawMessage(`{"path":"main.go"}`),
	}
	overflow := ai.AssistantMessage{
		StopReason:   ai.StopReasonError,
		ErrorMessage: "context length exceeded",
	}
	recovered := textAssistant("recovered final answer")
	provider := newCodingFaux(t,
		fauxToolStep(call),
		recoveryErrorStep(overflow),
		codingAssistantStep(textAssistant("checkpoint")),
		codingAssistantStep(recovered),
	)

	result, err := advanceTestSessionWithProvider(context.Background(), testSessionAdvance{
		WorkspacePath: workspace,
		Input:         "read main.go and finish",
	}, provider)
	if err != nil {
		t.Fatalf("advance test Session error = %v", err)
	}
	if got, want := result.FinalText(), "recovered final answer"; got != want {
		t.Fatalf("FinalText() = %q, want %q", got, want)
	}
	if !messagesContainAssistantError(result.History, overflow.ErrorMessage) {
		t.Fatalf("complete product History lost overflow: %#v", result.History)
	}
	if got := countUserMessages(result.History, "read main.go and finish"); got != 1 {
		t.Fatalf("product task count = %d, want 1", got)
	}
	trace, err := BuildTrace(result.SessionInfo, result.AdvanceResult, nil)
	if err != nil {
		t.Fatalf("BuildTrace() error = %v", err)
	}
	var tracedOverflow bool
	for _, message := range trace.Transcript {
		if strings.Contains(message.ErrorMessage, overflow.ErrorMessage) {
			tracedOverflow = true
		}
	}
	if !tracedOverflow {
		t.Fatalf("trace lost initial overflow: %#v", trace.Transcript)
	}
}

func newRecoveryTestSession(
	t *testing.T,
	provider ai.Provider,
	tools ...agent.Tool,
) *Session {
	t.Helper()

	limits := ai.RequestLimits{
		ContextCapacity: 10_000,
		ModelMaxOutput:  2_000,
		ContextSafety:   100,
	}
	schemas := toolSchemas(tools)
	engine, err := agent.New(agent.Config{
		Provider:      provider,
		SystemPrompt:  "system",
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
			SystemPrompt: "system",
			Tools:        schemas,
		},
		Compaction: compactionPolicy{
			Threshold:                1_000,
			SoftCeiling:              500,
			RetainedRawTarget:        8,
			SummaryMaxOutput:         64,
			SplitTurnPrefixMaxOutput: 32,
		},
	})
	if err != nil {
		t.Fatalf("newSession() error = %v", err)
	}
	return session
}

func countUserMessages(messages []ai.Message, content string) int {
	var count int
	for _, message := range messages {
		user, ok := message.(ai.UserMessage)
		if ok && user.Content == content {
			count++
		}
	}
	return count
}

func messagesContainAssistantError(messages []ai.Message, fragment string) bool {
	for _, message := range messages {
		switch message := message.(type) {
		case ai.AssistantMessage:
			if strings.Contains(message.ErrorMessage, fragment) {
				return true
			}
		case *ai.AssistantMessage:
			if message != nil && strings.Contains(message.ErrorMessage, fragment) {
				return true
			}
		}
	}
	return false
}

func recoveryErrorStep(message ai.AssistantMessage) faux.Step {
	return faux.Step{Events: []ai.Event{ai.ErrorEvent{Message: message}}}
}

func textAssistant(text string) ai.AssistantMessage {
	return ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: text}},
		StopReason: ai.StopReasonStop,
	}
}

type countingRecoveryTool struct {
	executions atomic.Int32
}

func (tool *countingRecoveryTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Schema: ai.ToolSchema{
			Name:       "read",
			Parameters: json.RawMessage(`{"type":"object"}`),
		},
		CanRunParallel: true,
	}
}

func (tool *countingRecoveryTool) Execute(context.Context, json.RawMessage) (string, error) {
	tool.executions.Add(1)
	return "package main", nil
}

func (tool *countingRecoveryTool) DescribeInvocation(json.RawMessage) string {
	return "Read"
}

var _ agent.Tool = (*countingRecoveryTool)(nil)

type recoveryStreamFactory func(context.Context) ai.Stream

type scriptedRecoveryProvider struct {
	mu        sync.Mutex
	next      int
	factories []recoveryStreamFactory
	requests  []ai.Request
}

func newScriptedRecoveryProvider(factories ...recoveryStreamFactory) *scriptedRecoveryProvider {
	return &scriptedRecoveryProvider{factories: factories}
}

func (provider *scriptedRecoveryProvider) Stream(ctx context.Context, request ai.Request) ai.Stream {
	provider.mu.Lock()
	index := provider.next
	provider.next++
	provider.requests = append(provider.requests, ai.CloneRequest(request))
	var factory recoveryStreamFactory
	if index < len(provider.factories) {
		factory = provider.factories[index]
	}
	provider.mu.Unlock()

	if factory == nil {
		return &oneEventStream{event: ai.ErrorEvent{Message: ai.AssistantMessage{
			StopReason:   ai.StopReasonError,
			ErrorMessage: "unexpected scripted Provider request",
		}}}
	}
	return factory(ctx)
}

func (provider *scriptedRecoveryProvider) Requests() []ai.Request {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	requests := make([]ai.Request, len(provider.requests))
	for index, request := range provider.requests {
		requests[index] = ai.CloneRequest(request)
	}
	return requests
}

func eventStreamFactory(event ai.Event) recoveryStreamFactory {
	return func(context.Context) ai.Stream {
		return &oneEventStream{event: event}
	}
}

type gatedRecoveryStream struct {
	release <-chan struct{}
	event   ai.Event
	done    bool
}

func (stream *gatedRecoveryStream) Receive() (ai.Event, error) {
	if stream.done {
		return nil, nil
	}
	<-stream.release
	stream.done = true
	return stream.event, nil
}

var _ ai.Provider = (*scriptedRecoveryProvider)(nil)
var _ ai.Stream = (*gatedRecoveryStream)(nil)
