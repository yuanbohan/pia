package openaicompatible

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/yuanbohan/pi-go/internal/ai"
)

func TestProviderStreamDefersHTTPUntilReceiveAndSnapshotsRequest(t *testing.T) {
	t.Parallel()

	const apiKey = "top-secret-api-key"
	var calls int
	var captured *http.Request
	var capturedBody []byte
	body := &trackingReadCloser{Reader: strings.NewReader("")}
	client := &http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			calls++
			captured = request.Clone(request.Context())
			var err error
			capturedBody, err = io.ReadAll(request.Body)
			if err != nil {
				t.Fatalf("io.ReadAll(request.Body) error = %v", err)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"text/event-stream; charset=utf-8"},
				},
				Body: body,
			}, nil
		}),
	}

	provider, err := New(Config{
		HTTPClient: client,
		Endpoint:   "https://example.test/chat/completions",
		APIKey:     apiKey,
		Model:      "configured-model",
		Profile: Profile{
			ReplayReasoning: true,
			StreamUsage:     true,
			Thinking:        true,
		},
		ReasoningEffort: "high",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	request := ai.Request{
		SystemPrompt: "original system",
		Messages: []ai.Message{
			ai.UserMessage{Content: "original user"},
		},
		Tools: []ai.ToolSchema{
			{
				Name:       "read",
				Parameters: json.RawMessage(`{"type":"object"}`),
			},
		},
	}
	stream := provider.Stream(context.Background(), request)
	if calls != 0 {
		t.Fatalf("HTTP calls after Stream() = %d, want 0", calls)
	}

	request.SystemPrompt = "mutated system"
	request.Messages[0] = ai.UserMessage{Content: "mutated user"}
	request.Tools[0].Parameters[0] = '['

	event, err := stream.Receive()
	if err != nil {
		t.Fatalf("Receive() error = %v", err)
	}
	if _, ok := event.(ai.StartEvent); !ok {
		t.Fatalf("Receive() event = %T, want ai.StartEvent", event)
	}
	if calls != 1 {
		t.Fatalf("HTTP calls after Receive() = %d, want 1", calls)
	}

	if captured.Method != http.MethodPost {
		t.Errorf("method = %q, want POST", captured.Method)
	}
	if got := captured.URL.String(); got != "https://example.test/chat/completions" {
		t.Errorf("URL = %q, want configured endpoint", got)
	}
	if got := captured.Header.Get("Authorization"); got != "Bearer "+apiKey {
		t.Errorf("Authorization = %q, want bearer credential", got)
	}
	if got := captured.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	if got := captured.Header.Get("Accept"); got != "text/event-stream" {
		t.Errorf("Accept = %q, want text/event-stream", got)
	}
	if strings.Contains(string(capturedBody), apiKey) {
		t.Fatalf("request body contains API key: %s", capturedBody)
	}
	if !strings.Contains(string(capturedBody), "original system") ||
		!strings.Contains(string(capturedBody), "original user") ||
		!strings.Contains(string(capturedBody), `"parameters":{"type":"object"}`) {
		t.Fatalf("request body does not contain frozen request: %s", capturedBody)
	}
	if strings.Contains(string(capturedBody), "mutated") {
		t.Fatalf("request body contains post-Stream mutation: %s", capturedBody)
	}

	concrete, ok := stream.(*responseStream)
	if !ok {
		t.Fatalf("Stream() = %T, want *responseStream", stream)
	}
	concrete.closeResponse()
	if !body.closed {
		t.Fatal("closeResponse() did not close response body")
	}
}

func TestNewRejectsInvalidStableConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config Config
	}{
		{name: "missing endpoint", config: Config{Model: "model"}},
		{name: "relative endpoint", config: Config{Endpoint: "/chat/completions", Model: "model"}},
		{name: "unsupported scheme", config: Config{Endpoint: "ftp://example.test/chat/completions", Model: "model"}},
		{name: "endpoint user info", config: Config{Endpoint: "https://user:pass@example.test/chat/completions", Model: "model"}},
		{name: "missing model", config: Config{Endpoint: "https://example.test/chat/completions"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := New(test.config); err == nil {
				t.Fatal("New() error = nil, want configuration error")
			}
		})
	}
}

func TestProviderReportsRequestConversionFailureWithoutHTTP(t *testing.T) {
	t.Parallel()

	var calls int
	provider, err := New(Config{
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return nil, nil
			}),
		},
		Endpoint: "https://example.test/chat/completions",
		Model:    "model",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	stream := provider.Stream(context.Background(), ai.Request{
		Tools: []ai.ToolSchema{{Name: "bad", Parameters: json.RawMessage(`{`)}},
	})
	event, err := stream.Receive()
	if err != nil {
		t.Fatalf("first Receive() error = %v", err)
	}
	errorEvent, ok := event.(ai.ErrorEvent)
	if !ok {
		t.Fatalf("first Receive() event = %T, want ai.ErrorEvent", event)
	}
	if errorEvent.Message.StopReason != ai.StopReasonError {
		t.Errorf("stop reason = %q, want %q", errorEvent.Message.StopReason, ai.StopReasonError)
	}
	if !strings.Contains(errorEvent.Message.ErrorMessage, `tool "bad" parameters`) {
		t.Errorf("error message = %q, want conversion context", errorEvent.Message.ErrorMessage)
	}
	if calls != 0 {
		t.Fatalf("HTTP calls = %d, want 0", calls)
	}

	for attempt := 0; attempt < 2; attempt++ {
		event, err = stream.Receive()
		if err != io.EOF || event != nil {
			t.Fatalf("Receive() after terminal = (%T, %v), want (nil, io.EOF)", event, err)
		}
	}
}

func TestProviderPreCanceledContextDoesNotSendHTTP(t *testing.T) {
	t.Parallel()

	var calls int
	provider, err := New(Config{
		HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			calls++
			return nil, errors.New("unexpected request")
		})},
		Endpoint: "https://example.test/chat/completions",
		Model:    "model",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	cause := errors.New("canceled before receive")
	cancel(cause)

	event, err := provider.Stream(ctx, ai.Request{}).Receive()
	if err != nil {
		t.Fatalf("Receive() error = %v", err)
	}
	errorEvent, ok := event.(ai.ErrorEvent)
	if !ok {
		t.Fatalf("Receive() event = %T, want ai.ErrorEvent", event)
	}
	if errorEvent.Message.StopReason != ai.StopReasonAborted || errorEvent.Message.ErrorMessage != cause.Error() {
		t.Fatalf("terminal = %#v, want context cause", errorEvent.Message)
	}
	if calls != 0 {
		t.Fatalf("HTTP calls = %d, want 0", calls)
	}
}

func TestProviderRedactsCredentialFromTransportErrorWithoutRetry(t *testing.T) {
	t.Parallel()

	const apiKey = "transport-secret"
	var calls int
	provider, err := New(Config{
		HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			calls++
			return nil, errors.New("transport failure mentioning " + apiKey)
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
	if strings.Contains(errorEvent.Message.ErrorMessage, apiKey) ||
		!strings.Contains(errorEvent.Message.ErrorMessage, "[REDACTED]") {
		t.Fatalf("error was not credential-redacted: %q", errorEvent.Message.ErrorMessage)
	}
	if calls != 1 {
		t.Fatalf("HTTP calls = %d, want exactly one attempt", calls)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

type trackingReadCloser struct {
	io.Reader
	closed bool
}

func (body *trackingReadCloser) Close() error {
	body.closed = true
	return nil
}
