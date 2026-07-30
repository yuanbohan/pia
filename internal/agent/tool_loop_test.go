package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yuanbohan/pia/internal/agent"
	"github.com/yuanbohan/pia/internal/ai"
)

func TestNewValidatesToolRegistration(t *testing.T) {
	t.Parallel()

	var typedNil *testTool
	tests := []struct {
		name  string
		tools []agent.Tool
		want  string
	}{
		{name: "nil tool", tools: []agent.Tool{nil}, want: "tool 0 is nil"},
		{name: "typed nil tool", tools: []agent.Tool{typedNil}, want: "tool 0 is nil"},
		{
			name:  "empty name",
			tools: []agent.Tool{&testTool{definition: agent.ToolDefinition{Schema: toolSchema(" ")}}},
			want:  "name is required",
		},
		{
			name: "duplicate name",
			tools: []agent.Tool{
				&testTool{definition: agent.ToolDefinition{Schema: toolSchema("read")}},
				&testTool{definition: agent.ToolDefinition{Schema: toolSchema("read")}},
			},
			want: "duplicate tool name",
		},
		{
			name: "invalid schema",
			tools: []agent.Tool{&testTool{definition: agent.ToolDefinition{Schema: ai.ToolSchema{
				Name:       "read",
				Parameters: json.RawMessage(`{"type":`),
			}}}},
			want: "invalid parameters schema",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := agent.New(agent.Config{Provider: staticProvider{}, Tools: test.tools})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("New() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestRunFreezesToolDefinitionsAndSendsCompleteMultiTurnRequests(t *testing.T) {
	t.Parallel()

	parameters := json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)
	definitionCalls := 0
	var gotArguments map[string]string
	tool := &testTool{
		definition: agent.ToolDefinition{
			Schema: ai.ToolSchema{
				Name:        "read",
				Description: "read a file",
				Parameters:  parameters,
			},
			CanRunParallel: true,
		},
		definitionCalls: &definitionCalls,
		execute: func(_ context.Context, arguments json.RawMessage) (string, error) {
			if err := json.Unmarshal(arguments, &gotArguments); err != nil {
				return "", err
			}
			return "package main", nil
		},
	}
	first := ai.AssistantMessage{
		Content: []ai.AssistantContent{
			ai.TextContent{Text: "I will inspect the file."},
			ai.ToolCall{ID: "call-1", Name: "read", Arguments: json.RawMessage(`{"path":"main.go"}`)},
		},
		StopReason: ai.StopReasonToolUse,
	}
	final := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "inspection complete"}},
		StopReason: ai.StopReasonStop,
	}
	provider := &responseProvider{
		messages: []ai.AssistantMessage{first, final},
		mutateRequest: func(index int, request ai.Request) {
			if index == 0 {
				request.Tools[0].Parameters[1] = 'X'
			}
		},
	}
	runtime := newAgentWithTools(t, provider, "stable system", tool)
	parameters[1] = 'Y'

	result, err := runtime.Run(context.Background(), nil, "inspect main.go", emptySteeringSource{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if definitionCalls != 1 {
		t.Fatalf("Definition() calls = %d, want 1", definitionCalls)
	}
	if got, want := gotArguments["path"], "main.go"; got != want {
		t.Fatalf("tool path = %q, want %q", got, want)
	}

	wantResult := ai.ToolResultMessage{
		ToolCallID: "call-1",
		ToolName:   "read",
		Content:    "package main",
	}
	wantMessages := []ai.Message{
		ai.UserMessage{Content: "inspect main.go"},
		first,
		wantResult,
		final,
	}
	if !reflect.DeepEqual(result.NewMessages, wantMessages) {
		t.Fatalf("Run() NewMessages = %#v, want %#v", result.NewMessages, wantMessages)
	}

	requests := provider.Requests()
	if got, want := len(requests), 2; got != want {
		t.Fatalf("len(Requests()) = %d, want %d", got, want)
	}
	wantParameters := json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)
	for index, request := range requests {
		if got, want := request.SystemPrompt, "stable system"; got != want {
			t.Fatalf("request %d SystemPrompt = %q, want %q", index, got, want)
		}
		if got, want := len(request.Tools), 1; got != want {
			t.Fatalf("request %d tool count = %d, want %d", index, got, want)
		}
		if !reflect.DeepEqual(request.Tools[0].Parameters, wantParameters) {
			t.Fatalf("request %d parameters = %s, want %s", index, request.Tools[0].Parameters, wantParameters)
		}
	}
	wantSecondMessages := []ai.Message{
		ai.UserMessage{Content: "inspect main.go"},
		first,
		wantResult,
	}
	if !reflect.DeepEqual(requests[1].Messages, wantSecondMessages) {
		t.Fatalf("second request messages = %#v, want %#v", requests[1].Messages, wantSecondMessages)
	}
}

func TestRunMapsCallLocalToolFailuresAndContinues(t *testing.T) {
	t.Parallel()

	decode := &testTool{
		definition: agent.ToolDefinition{Schema: toolSchema("decode")},
		execute: func(_ context.Context, arguments json.RawMessage) (string, error) {
			var input struct {
				Value string `json:"value"`
			}
			if err := json.Unmarshal(arguments, &input); err != nil {
				return "", err
			}
			if input.Value == "" {
				return "", errors.New("value is required")
			}
			return input.Value, nil
		},
	}
	fail := &testTool{
		definition: agent.ToolDefinition{Schema: toolSchema("fail")},
		execute: func(context.Context, json.RawMessage) (string, error) {
			return "partial output", errors.New("exit status 1")
		},
	}
	provider := &responseProvider{messages: []ai.AssistantMessage{
		{
			Content: []ai.AssistantContent{
				ai.ToolCall{ID: "unknown", Name: "missing", Arguments: json.RawMessage(`{}`)},
				ai.ToolCall{ID: "json", Name: "decode", Arguments: json.RawMessage(`{`)},
				ai.ToolCall{ID: "semantic", Name: "decode", Arguments: json.RawMessage(`{}`)},
				ai.ToolCall{ID: "execute", Name: "fail", Arguments: json.RawMessage(`{}`)},
			},
			StopReason: ai.StopReasonToolUse,
		},
		{Content: []ai.AssistantContent{ai.TextContent{Text: "recovered"}}, StopReason: ai.StopReasonStop},
	}}
	runtime := newAgentWithTools(t, provider, "system", decode, fail)

	result, err := runtime.Run(context.Background(), nil, "exercise failures", emptySteeringSource{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	results := runToolResults(result.NewMessages)
	if got, want := len(results), 4; got != want {
		t.Fatalf("tool result count = %d, want %d", got, want)
	}
	wantIDs := []string{"unknown", "json", "semantic", "execute"}
	for index, result := range results {
		if result.ToolCallID != wantIDs[index] {
			t.Fatalf("result %d ID = %q, want %q", index, result.ToolCallID, wantIDs[index])
		}
		if !result.IsError {
			t.Fatalf("result %d IsError = false, want true", index)
		}
	}
	checks := []string{"unknown tool", "unexpected end of JSON", "value is required", "partial output"}
	for index, want := range checks {
		if !strings.Contains(results[index].Content, want) {
			t.Fatalf("result %d content = %q, want substring %q", index, results[index].Content, want)
		}
	}
	if !strings.Contains(results[3].Content, "exit status 1") {
		t.Fatalf("execute error content = %q, want returned content and error", results[3].Content)
	}
	if got, want := len(provider.Requests()), 2; got != want {
		t.Fatalf("Provider calls = %d, want %d", got, want)
	}
}

func TestRunDoesNotExecuteTruncatedToolCalls(t *testing.T) {
	t.Parallel()

	var executions atomic.Int32
	tool := &testTool{
		definition: agent.ToolDefinition{Schema: toolSchema("read"), CanRunParallel: true},
		execute: func(context.Context, json.RawMessage) (string, error) {
			executions.Add(1)
			return "unexpected", nil
		},
	}
	provider := &responseProvider{messages: []ai.AssistantMessage{
		{
			Content: []ai.AssistantContent{
				ai.ToolCall{ID: "call-1", Name: "read", Arguments: json.RawMessage(`{"path":`)},
				ai.ToolCall{ID: "call-2", Name: "read", Arguments: json.RawMessage(`{`)},
			},
			StopReason: ai.StopReasonLength,
		},
		{Content: []ai.AssistantContent{ai.TextContent{Text: "retried safely"}}, StopReason: ai.StopReasonStop},
	}}
	runtime := newAgentWithTools(t, provider, "system", tool)

	result, err := runtime.Run(context.Background(), nil, "inspect", emptySteeringSource{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := executions.Load(); got != 0 {
		t.Fatalf("tool executions = %d, want 0", got)
	}
	results := runToolResults(result.NewMessages)
	if got, want := len(results), 2; got != want {
		t.Fatalf("tool result count = %d, want %d", got, want)
	}
	for _, result := range results {
		if !result.IsError || !strings.Contains(result.Content, "truncated") {
			t.Fatalf("truncated result = %#v, want truncation error", result)
		}
	}
}

func TestRunRejectsMalformedToolCallProtocolBeforeAppendingProviderMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		message ai.AssistantMessage
		want    string
	}{
		{
			name:    "toolUse without call",
			message: ai.AssistantMessage{StopReason: ai.StopReasonToolUse},
			want:    "toolUse stop reason without a tool call",
		},
		{
			name: "empty call ID",
			message: ai.AssistantMessage{
				Content:    []ai.AssistantContent{ai.ToolCall{Name: "read", Arguments: json.RawMessage(`{}`)}},
				StopReason: ai.StopReasonToolUse,
			},
			want: "empty ID",
		},
		{
			name: "duplicate call ID",
			message: ai.AssistantMessage{
				Content: []ai.AssistantContent{
					ai.ToolCall{ID: "same", Name: "read", Arguments: json.RawMessage(`{}`)},
					ai.ToolCall{ID: "same", Name: "read", Arguments: json.RawMessage(`{}`)},
				},
				StopReason: ai.StopReasonToolUse,
			},
			want: "duplicate ID",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			provider := &responseProvider{messages: []ai.AssistantMessage{test.message}}
			runtime := newAgent(t, provider, "system")

			result, err := runtime.Run(context.Background(), nil, "invalid protocol", emptySteeringSource{})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Run() error = %v, want substring %q", err, test.want)
			}
			if got, want := len(result.NewMessages), 2; got != want {
				t.Fatalf("NewMessages length = %d, want %d", got, want)
			}
			terminal := result.NewMessages[1].(ai.AssistantMessage)
			if terminal.StopReason != ai.StopReasonError || !strings.Contains(terminal.ErrorMessage, "provider protocol") {
				t.Fatalf("terminal = %#v, want synthetic protocol error", terminal)
			}
			if reflect.DeepEqual(terminal, test.message) {
				t.Fatal("malformed Provider message was appended instead of a synthetic error")
			}
		})
	}
}

func TestRunTreatsEmptyToolNameAsCallLocalError(t *testing.T) {
	t.Parallel()

	provider := &responseProvider{messages: []ai.AssistantMessage{
		{
			Content:    []ai.AssistantContent{ai.ToolCall{ID: "call-1", Arguments: json.RawMessage(`{}`)}},
			StopReason: ai.StopReasonToolUse,
		},
		{StopReason: ai.StopReasonStop},
	}}
	runtime := newAgent(t, provider, "system")

	result, err := runtime.Run(context.Background(), nil, "empty tool name", emptySteeringSource{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	results := runToolResults(result.NewMessages)
	if got, want := len(results), 1; got != want {
		t.Fatalf("tool result count = %d, want %d", got, want)
	}
	if !results[0].IsError || !strings.Contains(results[0].Content, "empty name") {
		t.Fatalf("tool result = %#v, want empty-name error", results[0])
	}
}

func TestRunExecutesToolCallsWhenStopReasonIsStop(t *testing.T) {
	t.Parallel()

	var executions atomic.Int32
	tool := &testTool{
		definition: agent.ToolDefinition{Schema: toolSchema("read"), CanRunParallel: true},
		execute: func(context.Context, json.RawMessage) (string, error) {
			executions.Add(1)
			return "ok", nil
		},
	}
	provider := &responseProvider{messages: []ai.AssistantMessage{
		{
			Content:    []ai.AssistantContent{ai.ToolCall{ID: "call-1", Name: "read", Arguments: json.RawMessage(`{}`)}},
			StopReason: ai.StopReasonStop,
		},
		{StopReason: ai.StopReasonStop},
	}}
	runtime := newAgentWithTools(t, provider, "system", tool)

	if _, err := runtime.Run(context.Background(), nil, "inspect", emptySteeringSource{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := executions.Load(); got != 1 {
		t.Fatalf("tool executions = %d, want 1", got)
	}
}

func TestRunSchedulesParallelStagesAroundSerialBarriers(t *testing.T) {
	t.Parallel()

	firstReadsStarted := make(chan string, 2)
	secondReadsStarted := make(chan string, 2)
	writeStarted := make(chan struct{}, 1)
	releaseFirstReads := make(chan struct{})
	releaseWrite := make(chan struct{})
	releaseSecondReads := make(chan struct{})
	read := &testTool{
		definition: agent.ToolDefinition{Schema: toolSchema("read"), CanRunParallel: true},
		execute: func(_ context.Context, arguments json.RawMessage) (string, error) {
			label, err := argumentLabel(arguments)
			if err != nil {
				return "", err
			}
			switch label {
			case "r1", "r2":
				firstReadsStarted <- label
				<-releaseFirstReads
			case "r3", "r4":
				secondReadsStarted <- label
				<-releaseSecondReads
			default:
				return "", errors.New("unexpected read label")
			}
			return label, nil
		},
	}
	write := &testTool{
		definition: agent.ToolDefinition{Schema: toolSchema("write")},
		execute: func(_ context.Context, arguments json.RawMessage) (string, error) {
			label, err := argumentLabel(arguments)
			if err != nil {
				return "", err
			}
			writeStarted <- struct{}{}
			<-releaseWrite
			return label, nil
		},
	}
	provider := newFaux(t,
		assistantStep(toolAssistant(ai.StopReasonToolUse,
			toolCall("r1", "read", "r1"),
			toolCall("r2", "read", "r2"),
			toolCall("w", "write", "w"),
			toolCall("r3", "read", "r3"),
			toolCall("r4", "read", "r4"),
		)),
		assistantStep(ai.AssistantMessage{StopReason: ai.StopReasonStop}),
	)
	runtime := newAgentWithTools(t, provider, "system", read, write)

	returned := runInBackground(runtime, context.Background(), "schedule")
	waitLabels(t, firstReadsStarted, "r1", "r2")
	assertNoSignal(t, writeStarted, "write started before the first parallel stage settled")
	close(releaseFirstReads)
	waitSignal(t, writeStarted, "write did not start after the first stage")
	assertNoString(t, secondReadsStarted, "second read stage started before the write barrier settled")
	close(releaseWrite)
	waitLabels(t, secondReadsStarted, "r3", "r4")
	close(releaseSecondReads)

	got := waitRun(t, returned)
	if got.err != nil {
		t.Fatalf("Run() error = %v", got.err)
	}
	results := runToolResults(got.result.NewMessages)
	wantIDs := []string{"r1", "r2", "w", "r3", "r4"}
	for index, want := range wantIDs {
		if results[index].ToolCallID != want {
			t.Fatalf("result %d ID = %q, want %q", index, results[index].ToolCallID, want)
		}
	}
}

func TestParallelCompletionOrderDoesNotChangeNewMessageOrder(t *testing.T) {
	t.Parallel()

	aStarted := make(chan struct{})
	bStarted := make(chan struct{})
	bCompleted := make(chan struct{})
	releaseA := make(chan struct{})
	var completionMu sync.Mutex
	completionOrder := make([]string, 0, 2)
	read := &testTool{
		definition: agent.ToolDefinition{Schema: toolSchema("read"), CanRunParallel: true},
		execute: func(_ context.Context, arguments json.RawMessage) (string, error) {
			label, err := argumentLabel(arguments)
			if err != nil {
				return "", err
			}
			switch label {
			case "A":
				close(aStarted)
				<-releaseA
			case "B":
				close(bStarted)
				<-aStarted
			}
			completionMu.Lock()
			completionOrder = append(completionOrder, label)
			completionMu.Unlock()
			if label == "B" {
				close(bCompleted)
			}
			return "result-" + label, nil
		},
	}
	provider := newFaux(t,
		assistantStep(toolAssistant(ai.StopReasonToolUse,
			toolCall("A", "read", "A"),
			toolCall("B", "read", "B"),
		)),
		assistantStep(ai.AssistantMessage{StopReason: ai.StopReasonStop}),
	)
	runtime := newAgentWithTools(t, provider, "system", read)

	returned := runInBackground(runtime, context.Background(), "order")
	waitSignal(t, bStarted, "B did not start")
	waitSignal(t, bCompleted, "B did not complete first")
	close(releaseA)
	got := waitRun(t, returned)
	if got.err != nil {
		t.Fatalf("Run() error = %v", got.err)
	}
	completionMu.Lock()
	gotCompletion := append([]string(nil), completionOrder...)
	completionMu.Unlock()
	if want := []string{"B", "A"}; !reflect.DeepEqual(gotCompletion, want) {
		t.Fatalf("completion order = %v, want %v", gotCompletion, want)
	}
	results := runToolResults(got.result.NewMessages)
	if got, want := []string{results[0].ToolCallID, results[1].ToolCallID}, []string{"A", "B"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("NewMessages result order = %v, want %v", got, want)
	}
}

func TestToolLocalTimeoutIsCallLocalAndLaterStageContinues(t *testing.T) {
	t.Parallel()

	var writeExecutions atomic.Int32
	read := &testTool{
		definition: agent.ToolDefinition{Schema: toolSchema("read"), CanRunParallel: true},
		execute: func(ctx context.Context, arguments json.RawMessage) (string, error) {
			label, err := argumentLabel(arguments)
			if err != nil {
				return "", err
			}
			if label == "timeout" {
				child, cancel := context.WithTimeout(ctx, 15*time.Millisecond)
				defer cancel()
				<-child.Done()
				return "", child.Err()
			}
			return "fast", nil
		},
	}
	write := &testTool{
		definition: agent.ToolDefinition{Schema: toolSchema("write")},
		execute: func(context.Context, json.RawMessage) (string, error) {
			writeExecutions.Add(1)
			return "written", nil
		},
	}
	provider := newFaux(t,
		assistantStep(toolAssistant(ai.StopReasonToolUse,
			toolCall("timeout", "read", "timeout"),
			toolCall("fast", "read", "fast"),
			toolCall("write", "write", "write"),
		)),
		assistantStep(ai.AssistantMessage{StopReason: ai.StopReasonStop}),
	)
	runtime := newAgentWithTools(t, provider, "system", read, write)

	result, err := runtime.Run(context.Background(), nil, "timeout", emptySteeringSource{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := writeExecutions.Load(); got != 1 {
		t.Fatalf("write executions = %d, want 1", got)
	}
	results := runToolResults(result.NewMessages)
	if !results[0].IsError || !strings.Contains(results[0].Content, context.DeadlineExceeded.Error()) {
		t.Fatalf("timeout result = %#v, want call-local deadline error", results[0])
	}
	if results[1].IsError || results[2].IsError {
		t.Fatalf("later results = %#v, want successful fast read and write", results[1:])
	}
}

func TestRunCancellationSettlesAllCallsAndNextRunUsesPairedHistory(t *testing.T) {
	t.Parallel()

	aStarted := make(chan struct{})
	aCompleted := make(chan struct{})
	allowAComplete := make(chan struct{})
	bStarted := make(chan struct{})
	bObservedCancel := make(chan struct{})
	bSettle := make(chan struct{})
	var laterExecutions atomic.Int32
	read := &testTool{
		definition: agent.ToolDefinition{Schema: toolSchema("read"), CanRunParallel: true},
		execute: func(ctx context.Context, arguments json.RawMessage) (string, error) {
			label, err := argumentLabel(arguments)
			if err != nil {
				return "", err
			}
			switch label {
			case "A":
				close(aStarted)
				<-allowAComplete
				close(aCompleted)
				return "A actual", nil
			case "B":
				close(bStarted)
				<-ctx.Done()
				close(bObservedCancel)
				<-bSettle
				return "", context.Cause(ctx)
			case "D":
				laterExecutions.Add(1)
				return "unexpected D", nil
			default:
				return "", errors.New("unexpected label")
			}
		},
	}
	write := &testTool{
		definition: agent.ToolDefinition{Schema: toolSchema("write")},
		execute: func(context.Context, json.RawMessage) (string, error) {
			laterExecutions.Add(1)
			return "unexpected C", nil
		},
	}
	firstAssistant := toolAssistant(ai.StopReasonToolUse,
		toolCall("A", "read", "A"),
		toolCall("B", "read", "B"),
		toolCall("C", "write", "C"),
		toolCall("D", "read", "D"),
	)
	finalAssistant := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "continued"}},
		StopReason: ai.StopReasonStop,
	}
	provider := newFaux(t, assistantStep(firstAssistant), assistantStep(finalAssistant))
	runtime := newAgentWithTools(t, provider, "system", read, write)
	ctx, cancel := context.WithCancelCause(context.Background())
	cause := errors.New("operator stopped the run")

	returned := runInBackground(runtime, ctx, "first task")
	waitSignal(t, aStarted, "A did not start")
	waitSignal(t, bStarted, "B did not start")
	close(allowAComplete)
	waitSignal(t, aCompleted, "A did not complete")
	cancel(cause)
	waitSignal(t, bObservedCancel, "B did not observe cancellation")
	assertNoRunReturn(t, returned, "Run returned before the started worker settled")
	close(bSettle)
	first := waitRun(t, returned)
	if !errors.Is(first.err, cause) {
		t.Fatalf("first Run() error = %v, want cancellation cause", first.err)
	}
	if got := laterExecutions.Load(); got != 0 {
		t.Fatalf("later stage executions = %d, want 0", got)
	}
	results := runToolResults(first.result.NewMessages)
	if got, want := len(results), 4; got != want {
		t.Fatalf("tool result count = %d, want %d", got, want)
	}
	wantIDs := []string{"A", "B", "C", "D"}
	for index, want := range wantIDs {
		if results[index].ToolCallID != want {
			t.Fatalf("result %d ID = %q, want %q", index, results[index].ToolCallID, want)
		}
	}
	if results[0].IsError || results[0].Content != "A actual" {
		t.Fatalf("completed result = %#v, want actual success", results[0])
	}
	if !results[1].IsError || !strings.Contains(results[1].Content, cause.Error()) {
		t.Fatalf("executing result = %#v, want canceled error", results[1])
	}
	for _, result := range results[2:] {
		if !result.IsError || !strings.Contains(result.Content, "not executed") {
			t.Fatalf("not-started result = %#v, want not-executed error", result)
		}
	}

	second, err := runtime.Run(
		context.Background(),
		first.result.NewMessages,
		"continue after cancellation", emptySteeringSource{})

	if err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	if got, want := second.NewMessages[len(second.NewMessages)-1], ai.Message(finalAssistant); !reflect.DeepEqual(got, want) {
		t.Fatalf("second terminal = %#v, want %#v", got, want)
	}
	requests := provider.Requests()
	if got, want := len(requests), 2; got != want {
		t.Fatalf("Provider calls = %d, want %d", got, want)
	}
	wantSecondRequest := append(ai.CloneMessages(first.result.NewMessages), ai.UserMessage{Content: "continue after cancellation"})
	if !reflect.DeepEqual(requests[1].Messages, wantSecondRequest) {
		t.Fatalf("second request messages = %#v, want paired history %#v", requests[1].Messages, wantSecondRequest)
	}
}

func TestProviderAbortedTerminalClosesCompletedToolCallsWithoutExecuting(t *testing.T) {
	t.Parallel()

	var executions atomic.Int32
	tool := &testTool{
		definition: agent.ToolDefinition{Schema: toolSchema("read"), CanRunParallel: true},
		execute: func(context.Context, json.RawMessage) (string, error) {
			executions.Add(1)
			return "unexpected", nil
		},
	}
	aborted := ai.AssistantMessage{
		Content: []ai.AssistantContent{
			ai.TextContent{Text: "received before cancellation"},
			ai.ToolCall{ID: "call-1", Name: "read", Arguments: json.RawMessage(`{"path":"main.go"}`)},
		},
		StopReason:   ai.StopReasonAborted,
		ErrorMessage: "provider observed cancellation",
	}
	final := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "continued safely"}},
		StopReason: ai.StopReasonStop,
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	cause := errors.New("operator stopped the run")
	provider := &streamSequenceProvider{streams: []ai.Stream{
		&terminalWithErrorStream{
			event: ai.ErrorEvent{Message: aborted},
			err:   cause,
			beforeReturn: func() {
				cancel(cause)
			},
		},
		&sliceStream{events: []ai.Event{ai.DoneEvent{Message: final}}},
	}}
	runtime := newAgentWithTools(t, provider, "system", tool)

	first, err := runtime.Run(ctx, nil, "inspect", emptySteeringSource{})
	if !errors.Is(err, cause) {
		t.Fatalf("first Run() error = %v, want cancellation cause", err)
	}
	if got := executions.Load(); got != 0 {
		t.Fatalf("tool executions = %d, want 0", got)
	}
	if got, want := len(first.NewMessages), 3; got != want {
		t.Fatalf("first NewMessages length = %d, want %d", got, want)
	}
	if got, want := first.NewMessages[1], ai.Message(aborted); !reflect.DeepEqual(got, want) {
		t.Fatalf("aborted assistant = %#v, want exactly %#v", got, want)
	}
	result := first.NewMessages[2].(ai.ToolResultMessage)
	if result.ToolCallID != "call-1" || !result.IsError || !strings.Contains(result.Content, "not executed") {
		t.Fatalf("settlement result = %#v, want same-ID not-executed error", result)
	}

	second, err := runtime.Run(context.Background(), first.NewMessages, "continue", emptySteeringSource{})
	if err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	if got, want := second.NewMessages[len(second.NewMessages)-1], ai.Message(final); !reflect.DeepEqual(got, want) {
		t.Fatalf("second terminal = %#v, want %#v", got, want)
	}
	requests := provider.Requests()
	if got, want := len(requests), 2; got != want {
		t.Fatalf("Provider calls = %d, want %d", got, want)
	}
	wantSecondRequest := append(ai.CloneMessages(first.NewMessages), ai.UserMessage{Content: "continue"})
	if !reflect.DeepEqual(requests[1].Messages, wantSecondRequest) {
		t.Fatalf("second request messages = %#v, want paired history %#v", requests[1].Messages, wantSecondRequest)
	}
}

func TestProviderErrorTerminalClosesToolCallsWithoutExecuting(t *testing.T) {
	t.Parallel()

	var executions atomic.Int32
	tool := &testTool{
		definition: agent.ToolDefinition{Schema: toolSchema("read"), CanRunParallel: true},
		execute: func(context.Context, json.RawMessage) (string, error) {
			executions.Add(1)
			return "unexpected", nil
		},
	}
	failed := ai.AssistantMessage{
		Content: []ai.AssistantContent{
			ai.ToolCall{ID: "call-1", Name: "read", Arguments: json.RawMessage(`{"path":"main.go"}`)},
		},
		StopReason:   ai.StopReasonError,
		ErrorMessage: "upstream disconnected",
	}
	runtime := newAgentWithTools(t, staticProvider{stream: &sliceStream{events: []ai.Event{
		ai.ErrorEvent{Message: failed},
	}}}, "system", tool)

	result, err := runtime.Run(context.Background(), nil, "inspect", emptySteeringSource{})
	if err == nil || !strings.Contains(err.Error(), "upstream disconnected") {
		t.Fatalf("Run() error = %v, want Provider failure", err)
	}
	if got := executions.Load(); got != 0 {
		t.Fatalf("tool executions = %d, want 0", got)
	}
	if got, want := len(result.NewMessages), 3; got != want {
		t.Fatalf("NewMessages length = %d, want %d", got, want)
	}
	if got, want := result.NewMessages[1], ai.Message(failed); !reflect.DeepEqual(got, want) {
		t.Fatalf("error assistant = %#v, want exactly %#v", got, want)
	}
	settlement := result.NewMessages[2].(ai.ToolResultMessage)
	if settlement.ToolCallID != "call-1" || !settlement.IsError || !strings.Contains(settlement.Content, "not executed") {
		t.Fatalf("settlement result = %#v, want same-ID not-executed error", settlement)
	}
}

func TestProviderErrorTerminalWithDuplicateToolCallIDIsProtocolError(t *testing.T) {
	t.Parallel()

	malformed := ai.AssistantMessage{
		Content: []ai.AssistantContent{
			ai.ToolCall{ID: "same", Name: "read", Arguments: json.RawMessage(`{}`)},
			ai.ToolCall{ID: "same", Name: "read", Arguments: json.RawMessage(`{}`)},
		},
		StopReason:   ai.StopReasonError,
		ErrorMessage: "upstream disconnected",
	}
	runtime := newAgent(t, staticProvider{stream: &sliceStream{events: []ai.Event{
		ai.ErrorEvent{Message: malformed},
	}}}, "system")

	result, err := runtime.Run(context.Background(), nil, "inspect", emptySteeringSource{})
	if err == nil || !strings.Contains(err.Error(), "duplicate ID") {
		t.Fatalf("Run() error = %v, want duplicate-ID protocol error", err)
	}
	if got, want := len(result.NewMessages), 2; got != want {
		t.Fatalf("NewMessages length = %d, want %d", got, want)
	}
	terminal := result.NewMessages[1].(ai.AssistantMessage)
	if terminal.StopReason != ai.StopReasonError || !strings.Contains(terminal.ErrorMessage, "provider protocol") {
		t.Fatalf("terminal = %#v, want synthetic protocol error", terminal)
	}
}

type testTool struct {
	definition      agent.ToolDefinition
	definitionCalls *int
	describe        func(json.RawMessage) string
	execute         func(context.Context, json.RawMessage) (string, error)
}

func (t *testTool) Definition() agent.ToolDefinition {
	if t.definitionCalls != nil {
		*t.definitionCalls++
	}
	return t.definition
}

func (t *testTool) Execute(ctx context.Context, arguments json.RawMessage) (string, error) {
	if t.execute == nil {
		return "", nil
	}
	return t.execute(ctx, arguments)
}

func (t *testTool) DescribeInvocation(arguments json.RawMessage) string {
	if t.describe == nil {
		return t.definition.Schema.Name
	}
	return t.describe(arguments)
}

func toolSchema(name string) ai.ToolSchema {
	return ai.ToolSchema{
		Name:        name,
		Description: name + " tool",
		Parameters:  json.RawMessage(`{"type":"object"}`),
	}
}

func newAgentWithTools(t *testing.T, provider ai.Provider, systemPrompt string, tools ...agent.Tool) *agent.Engine {
	t.Helper()
	runtime, err := agent.New(agent.Config{
		Provider:     provider,
		SystemPrompt: systemPrompt,
		Tools:        tools,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return runtime
}

type responseProvider struct {
	mu            sync.Mutex
	messages      []ai.AssistantMessage
	requests      []ai.Request
	next          int
	mutateRequest func(index int, request ai.Request)
}

type streamSequenceProvider struct {
	mu       sync.Mutex
	streams  []ai.Stream
	requests []ai.Request
	next     int
}

func (p *streamSequenceProvider) Stream(_ context.Context, request ai.Request) ai.Stream {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requests = append(p.requests, ai.CloneRequest(request))
	if p.next >= len(p.streams) {
		return eofStream{}
	}
	stream := p.streams[p.next]
	p.next++
	return stream
}

func (p *streamSequenceProvider) Requests() []ai.Request {
	p.mu.Lock()
	defer p.mu.Unlock()
	requests := make([]ai.Request, len(p.requests))
	for index, request := range p.requests {
		requests[index] = ai.CloneRequest(request)
	}
	return requests
}

func (p *responseProvider) Stream(_ context.Context, request ai.Request) ai.Stream {
	p.mu.Lock()
	defer p.mu.Unlock()
	index := p.next
	p.requests = append(p.requests, ai.CloneRequest(request))
	if p.mutateRequest != nil {
		p.mutateRequest(index, request)
	}
	if index >= len(p.messages) {
		return eofStream{}
	}
	p.next++
	return &sliceStream{events: []ai.Event{ai.DoneEvent{Message: p.messages[index]}}}
}

func (p *responseProvider) Requests() []ai.Request {
	p.mu.Lock()
	defer p.mu.Unlock()
	requests := make([]ai.Request, len(p.requests))
	for index, request := range p.requests {
		requests[index] = ai.CloneRequest(request)
	}
	return requests
}

func toolCall(id, name, label string) ai.ToolCall {
	arguments, err := json.Marshal(map[string]string{"label": label})
	if err != nil {
		panic(err)
	}
	return ai.ToolCall{ID: id, Name: name, Arguments: arguments}
}

func toolAssistant(reason ai.StopReason, calls ...ai.ToolCall) ai.AssistantMessage {
	content := make([]ai.AssistantContent, len(calls))
	for index, call := range calls {
		content[index] = call
	}
	return ai.AssistantMessage{Content: content, StopReason: reason}
}

func argumentLabel(arguments json.RawMessage) (string, error) {
	var input struct {
		Label string `json:"label"`
	}
	if err := json.Unmarshal(arguments, &input); err != nil {
		return "", err
	}
	return input.Label, nil
}

func runToolResults(messages []ai.Message) []ai.ToolResultMessage {
	var results []ai.ToolResultMessage
	for _, message := range messages {
		if result, ok := message.(ai.ToolResultMessage); ok {
			results = append(results, result)
		}
	}
	return results
}

type runReturn struct {
	result agent.RunResult
	err    error
}

func runInBackground(runtime *agent.Engine, ctx context.Context, input string) <-chan runReturn {
	returned := make(chan runReturn, 1)
	go func() {
		result, err := runtime.Run(ctx, nil, input, emptySteeringSource{})
		returned <- runReturn{result: result, err: err}
	}()
	return returned
}

func waitRun(t *testing.T, returned <-chan runReturn) runReturn {
	t.Helper()
	select {
	case result := <-returned:
		return result
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Run")
		return runReturn{}
	}
}

func assertNoRunReturn(t *testing.T, returned <-chan runReturn, message string) {
	t.Helper()
	select {
	case result := <-returned:
		t.Fatalf("%s: %#v", message, result)
	case <-time.After(20 * time.Millisecond):
	}
}

func waitSignal(t *testing.T, signal <-chan struct{}, message string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatal(message)
	}
}

func assertNoSignal(t *testing.T, signal <-chan struct{}, message string) {
	t.Helper()
	select {
	case <-signal:
		t.Fatal(message)
	case <-time.After(20 * time.Millisecond):
	}
}

func assertNoString(t *testing.T, signal <-chan string, message string) {
	t.Helper()
	select {
	case got := <-signal:
		t.Fatalf("%s: %s", message, got)
	case <-time.After(20 * time.Millisecond):
	}
}

func waitLabels(t *testing.T, values <-chan string, want ...string) {
	t.Helper()
	got := make(map[string]bool, len(want))
	for range want {
		select {
		case value := <-values:
			got[value] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for labels %v; got %v", want, got)
		}
	}
	for _, value := range want {
		if !got[value] {
			t.Fatalf("started labels = %v, want %v", got, want)
		}
	}
}
