package openaicompatible

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/yuanbohan/pia/internal/ai"
)

func TestStreamParsesSSEInSourceOrderAndWaitsForDone(t *testing.T) {
	t.Parallel()

	body := &countingReadCloser{Reader: strings.NewReader(`data: {"choices":[{"index":0,"delta":{"content":"","reasoning_content":"","role":"assistant"},"finish_reason":null}]}

data: {"choices":[{"index":0,"delta":{"reasoning_content":"plan "},"finish_reason":null}]}

data: {"choices":[{"index":0,"delta":{"reasoning_content":"carefully"},"finish_reason":null}]}

data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call-1","type":"function","function":{"name":"read","arguments":"{\"path\":\""}}]},"finish_reason":null}]}

data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"main.go\"}"}}]},"finish_reason":null}]}

data: {"choices":[{"index":0,"delta":{"content":"","reasoning_content":""},"finish_reason":"tool_calls"}]}

data: {"choices":[],"usage":{"prompt_tokens":11,"completion_tokens":7,"completion_tokens_details":{"reasoning_tokens":3}}}

data: [DONE]

`)}
	stream := streamForBody(t, context.Background(), body)

	start, err := stream.Receive()
	if err != nil {
		t.Fatalf("first Receive() error = %v", err)
	}
	if _, ok := start.(ai.StartEvent); !ok {
		t.Fatalf("first Receive() event = %T, want ai.StartEvent", start)
	}
	if body.reads != 0 {
		t.Fatalf("response-body reads after StartEvent = %d, want 0", body.reads)
	}

	events := append([]ai.Event{start}, receiveToEOF(t, stream)...)
	want := []ai.Event{
		ai.StartEvent{},
		ai.ThinkingStartEvent{ContentIndex: 0},
		ai.ThinkingDeltaEvent{ContentIndex: 0, Delta: "plan "},
		ai.ThinkingDeltaEvent{ContentIndex: 0, Delta: "carefully"},
		ai.ToolCallStartEvent{ContentIndex: 1, ID: "call-1", Name: "read"},
		ai.ToolCallDeltaEvent{ContentIndex: 1, Delta: `{"path":"`},
		ai.ToolCallDeltaEvent{ContentIndex: 1, Delta: `main.go"}`},
		ai.ThinkingEndEvent{ContentIndex: 0, Thinking: "plan carefully"},
		ai.ToolCallEndEvent{
			ContentIndex: 1,
			ToolCall: ai.ToolCall{
				ID:        "call-1",
				Name:      "read",
				Arguments: json.RawMessage(`{"path":"main.go"}`),
			},
		},
		ai.DoneEvent{
			Message: ai.AssistantMessage{
				Content: []ai.AssistantContent{
					ai.ThinkingContent{Thinking: "plan carefully"},
					ai.ToolCall{
						ID:        "call-1",
						Name:      "read",
						Arguments: json.RawMessage(`{"path":"main.go"}`),
					},
				},
				Usage: ai.Usage{
					InputTokens:  11,
					OutputTokens: 7,
				},
				StopReason: ai.StopReasonToolUse,
			},
		},
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events mismatch\n got: %#v\nwant: %#v", events, want)
	}
	if !body.closed {
		t.Fatal("response body was not closed after [DONE]")
	}
}

func TestStreamKeepsMalformedToolArgumentsAsCallLocalData(t *testing.T) {
	t.Parallel()

	stream := streamForBody(t, context.Background(), io.NopCloser(strings.NewReader(`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call-1","type":"function","function":{"name":"read","arguments":"{not-json"}}]},"finish_reason":null}]}

data: {"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}

data: [DONE]

`)))
	events := receiveToEOF(t, stream)
	done := terminalDone(t, events)
	call, ok := done.Message.Content[0].(ai.ToolCall)
	if !ok {
		t.Fatalf("terminal content = %T, want ai.ToolCall", done.Message.Content[0])
	}
	if got := string(call.Arguments); got != "{not-json" {
		t.Fatalf("tool arguments = %q, want malformed raw data preserved", got)
	}
}

func TestStreamAggregatesInterleavedToolCallsByWireIndex(t *testing.T) {
	t.Parallel()

	stream := streamForBody(t, context.Background(), io.NopCloser(strings.NewReader(`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"call-b","type":"function","function":{"name":"second","arguments":"{\"value\":\""}}]},"finish_reason":null}]}

data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call-a","type":"function","function":{"name":"first","arguments":"{\"value\":\""}}]},"finish_reason":null}]}

data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"function":{"arguments":"b\"}"}}]},"finish_reason":null}]}

data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"a\"}"}}]},"finish_reason":null}]}

data: {"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}

data: [DONE]

`)))
	done := terminalDone(t, receiveToEOF(t, stream))
	want := []ai.AssistantContent{
		ai.ToolCall{
			ID:        "call-b",
			Name:      "second",
			Arguments: json.RawMessage(`{"value":"b"}`),
		},
		ai.ToolCall{
			ID:        "call-a",
			Name:      "first",
			Arguments: json.RawMessage(`{"value":"a"}`),
		},
	}
	if !reflect.DeepEqual(done.Message.Content, want) {
		t.Fatalf("terminal content = %#v, want source-ordered calls %#v", done.Message.Content, want)
	}
}

func TestStreamRequiresFinishReasonAndDone(t *testing.T) {
	t.Parallel()

	t.Run("done before finish omits uncompleted calls", func(t *testing.T) {
		t.Parallel()
		stream := streamForBody(t, context.Background(), io.NopCloser(strings.NewReader(`data: {"choices":[{"index":0,"delta":{"content":"partial"},"finish_reason":null}]}

data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call-1","type":"function","function":{"name":"read","arguments":"{}"}}]},"finish_reason":null}]}

data: [DONE]

`)))
		errorEvent := terminalError(t, receiveToEOF(t, stream))
		if !strings.Contains(errorEvent.Message.ErrorMessage, "before finish_reason") {
			t.Fatalf("error = %q, want missing finish_reason context", errorEvent.Message.ErrorMessage)
		}
		wantContent := []ai.AssistantContent{ai.TextContent{Text: "partial"}}
		if !reflect.DeepEqual(errorEvent.Message.Content, wantContent) {
			t.Fatalf("terminal content = %#v, want text without uncompleted call", errorEvent.Message.Content)
		}
	})

	t.Run("EOF after finish retains completed calls", func(t *testing.T) {
		t.Parallel()
		stream := streamForBody(t, context.Background(), io.NopCloser(strings.NewReader(`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call-1","type":"function","function":{"name":"read","arguments":"{}"}}]},"finish_reason":null}]}

data: {"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}

`)))
		errorEvent := terminalError(t, receiveToEOF(t, stream))
		if !strings.Contains(errorEvent.Message.ErrorMessage, "before [DONE]") {
			t.Fatalf("error = %q, want missing [DONE] context", errorEvent.Message.ErrorMessage)
		}
		if len(errorEvent.Message.Content) != 1 {
			t.Fatalf("terminal content length = %d, want completed call", len(errorEvent.Message.Content))
		}
		if _, ok := errorEvent.Message.Content[0].(ai.ToolCall); !ok {
			t.Fatalf("terminal content = %T, want ai.ToolCall", errorEvent.Message.Content[0])
		}
	})
}

func TestStreamRejectsMalformedSSEProtocol(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		body        string
		wantMessage string
	}{
		{
			name:        "malformed JSON",
			body:        "data: {not-json\n\n",
			wantMessage: "decode SSE data",
		},
		{
			name: "missing tool index",
			body: `data: {"choices":[{"index":0,"delta":{"tool_calls":[{"id":"call-1","type":"function","function":{"name":"read","arguments":"{}"}}]},"finish_reason":null}]}

`,
			wantMessage: "tool call index",
		},
		{
			name: "missing tool metadata at finish",
			body: `data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{}"}}]},"finish_reason":null}]}

data: {"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}

`,
			wantMessage: "missing ID or name",
		},
		{
			name: "unsupported finish reason",
			body: `data: {"choices":[{"index":0,"delta":{},"finish_reason":"content_filter"}]}

`,
			wantMessage: "unsupported finish_reason",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			stream := streamForBody(t, context.Background(), io.NopCloser(strings.NewReader(test.body)))
			errorEvent := terminalError(t, receiveToEOF(t, stream))
			if !strings.Contains(errorEvent.Message.ErrorMessage, test.wantMessage) {
				t.Fatalf("error = %q, want %q", errorEvent.Message.ErrorMessage, test.wantMessage)
			}
		})
	}
}

func TestStreamRejectsOversizedSSEEvent(t *testing.T) {
	t.Parallel()

	body := "data: " + strings.Repeat("x", (1<<20)+1) + "\n\n"
	stream := streamForBody(t, context.Background(), io.NopCloser(strings.NewReader(body)))
	errorEvent := terminalError(t, receiveToEOF(t, stream))
	if !strings.Contains(errorEvent.Message.ErrorMessage, "SSE event exceeds 1048576 bytes") {
		t.Fatalf("error = %q, want bounded-event context", errorEvent.Message.ErrorMessage)
	}
}

func TestStreamCancellationSettlementDependsOnFinishReason(t *testing.T) {
	t.Parallel()

	t.Run("before finish omits partial calls", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancelCause(context.Background())
		stream := streamForBody(t, ctx, io.NopCloser(strings.NewReader(`data: {"choices":[{"index":0,"delta":{"content":"formed text"},"finish_reason":null}]}

data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call-1","type":"function","function":{"name":"read","arguments":"{}"}}]},"finish_reason":null}]}

data: {"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}

data: [DONE]

`)))

		receiveTypes(t, stream,
			ai.StartEvent{},
			ai.TextStartEvent{},
			ai.TextDeltaEvent{},
			ai.ToolCallStartEvent{},
			ai.ToolCallDeltaEvent{},
		)
		cause := errors.New("learner canceled")
		cancel(cause)

		event, err := stream.Receive()
		if err != nil {
			t.Fatalf("Receive() error = %v", err)
		}
		errorEvent, ok := event.(ai.ErrorEvent)
		if !ok {
			t.Fatalf("Receive() event = %T, want ai.ErrorEvent", event)
		}
		if errorEvent.Message.StopReason != ai.StopReasonAborted || errorEvent.Message.ErrorMessage != cause.Error() {
			t.Fatalf("terminal = %#v, want aborted context cause", errorEvent.Message)
		}
		wantContent := []ai.AssistantContent{ai.TextContent{Text: "formed text"}}
		if !reflect.DeepEqual(errorEvent.Message.Content, wantContent) {
			t.Fatalf("terminal content = %#v, want formed text without partial call", errorEvent.Message.Content)
		}
	})

	t.Run("after finish retains completed calls", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancelCause(context.Background())
		cause := errors.New("cancel after finish")
		body := &scriptedReadCloser{
			chunks: [][]byte{
				[]byte("data: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-1\",\"type\":\"function\",\"function\":{\"name\":\"read\",\"arguments\":\"{}\"}}]},\"finish_reason\":null}]}\n\n"),
				[]byte("data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n"),
				[]byte("data: [DONE]\n\n"),
			},
			afterRead: func(index int) {
				if index == 1 {
					cancel(cause)
				}
			},
		}
		stream := streamForBody(t, ctx, body)
		receiveTypes(t, stream,
			ai.StartEvent{},
			ai.ToolCallStartEvent{},
			ai.ToolCallDeltaEvent{},
		)

		event, err := stream.Receive()
		if err != nil {
			t.Fatalf("Receive() error = %v", err)
		}
		errorEvent, ok := event.(ai.ErrorEvent)
		if !ok {
			t.Fatalf("Receive() event = %T, want ai.ErrorEvent", event)
		}
		if errorEvent.Message.StopReason != ai.StopReasonAborted || errorEvent.Message.ErrorMessage != cause.Error() {
			t.Fatalf("terminal = %#v, want aborted context cause", errorEvent.Message)
		}
		if len(errorEvent.Message.Content) != 1 {
			t.Fatalf("terminal content length = %d, want completed call", len(errorEvent.Message.Content))
		}
		if _, ok := errorEvent.Message.Content[0].(ai.ToolCall); !ok {
			t.Fatalf("terminal content = %T, want ai.ToolCall", errorEvent.Message.Content[0])
		}
		if body.next != 2 {
			t.Fatalf("body chunks read = %d, want cancellation before [DONE] read", body.next)
		}
	})
}

func TestHTTPErrorIsBoundedAndRedactsCredential(t *testing.T) {
	t.Parallel()

	const apiKey = "credential-that-must-not-leak"
	body := &trackingReadCloser{Reader: strings.NewReader(apiKey + strings.Repeat("x", (64<<10)+100))}
	provider, err := New(Config{
		HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Header:     make(http.Header),
				Body:       body,
			}, nil
		})},
		Endpoint: "https://example.test/chat/completions",
		APIKey:   apiKey,
		Model:    "model",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	event, err := provider.Stream(context.Background(), ai.Request{}).Receive()
	if err != nil {
		t.Fatalf("Receive() error = %v", err)
	}
	errorEvent, ok := event.(ai.ErrorEvent)
	if !ok {
		t.Fatalf("Receive() event = %T, want ai.ErrorEvent", event)
	}
	if strings.Contains(errorEvent.Message.ErrorMessage, apiKey) {
		t.Fatalf("error leaked API key: %q", errorEvent.Message.ErrorMessage)
	}
	if !strings.Contains(errorEvent.Message.ErrorMessage, "[truncated]") {
		t.Fatalf("error = %q, want truncation marker", errorEvent.Message.ErrorMessage)
	}
	if len(errorEvent.Message.ErrorMessage) > (64<<10)+256 {
		t.Fatalf("error length = %d, want bounded body", len(errorEvent.Message.ErrorMessage))
	}
	if !body.closed {
		t.Fatal("HTTP error response body was not closed")
	}
}

func streamForBody(t *testing.T, ctx context.Context, body io.ReadCloser) ai.Stream {
	t.Helper()

	provider, err := New(Config{
		HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"text/event-stream"},
				},
				Body: body,
			}, nil
		})},
		Endpoint: "https://example.test/chat/completions",
		Model:    "model",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return provider.Stream(ctx, ai.Request{})
}

func receiveToEOF(t *testing.T, stream ai.Stream) []ai.Event {
	t.Helper()

	var events []ai.Event
	for count := 0; count < 100; count++ {
		event, err := stream.Receive()
		if errors.Is(err, io.EOF) {
			return events
		}
		if err != nil {
			t.Fatalf("Receive() error = %v", err)
		}
		events = append(events, event)
	}
	t.Fatal("stream did not reach io.EOF within 100 events")
	return nil
}

func terminalDone(t *testing.T, events []ai.Event) ai.DoneEvent {
	t.Helper()
	if len(events) == 0 {
		t.Fatal("events are empty")
	}
	done, ok := events[len(events)-1].(ai.DoneEvent)
	if !ok {
		t.Fatalf("terminal event = %T, want ai.DoneEvent", events[len(events)-1])
	}
	return done
}

func terminalError(t *testing.T, events []ai.Event) ai.ErrorEvent {
	t.Helper()
	if len(events) == 0 {
		t.Fatal("events are empty")
	}
	errorEvent, ok := events[len(events)-1].(ai.ErrorEvent)
	if !ok {
		t.Fatalf("terminal event = %T, want ai.ErrorEvent", events[len(events)-1])
	}
	return errorEvent
}

func receiveTypes(t *testing.T, stream ai.Stream, want ...ai.Event) {
	t.Helper()
	for index, wantEvent := range want {
		got, err := stream.Receive()
		if err != nil {
			t.Fatalf("Receive() %d error = %v", index, err)
		}
		if reflect.TypeOf(got) != reflect.TypeOf(wantEvent) {
			t.Fatalf("Receive() %d event = %T, want %T", index, got, wantEvent)
		}
	}
}

type countingReadCloser struct {
	io.Reader
	reads  int
	closed bool
}

func (body *countingReadCloser) Read(buffer []byte) (int, error) {
	body.reads++
	return body.Reader.Read(buffer)
}

func (body *countingReadCloser) Close() error {
	body.closed = true
	return nil
}

type scriptedReadCloser struct {
	chunks    [][]byte
	next      int
	afterRead func(int)
	closed    bool
}

func (body *scriptedReadCloser) Read(buffer []byte) (int, error) {
	if body.next >= len(body.chunks) {
		return 0, io.EOF
	}
	index := body.next
	chunk := body.chunks[index]
	if len(chunk) > len(buffer) {
		panic("scripted chunk exceeds read buffer")
	}
	n := copy(buffer, chunk)
	body.next++
	if body.afterRead != nil {
		body.afterRead(index)
	}
	return n, nil
}

func (body *scriptedReadCloser) Close() error {
	body.closed = true
	return nil
}
