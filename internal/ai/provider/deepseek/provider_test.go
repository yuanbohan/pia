package deepseek

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/yuanbohan/pi-go/internal/ai"
)

func TestNewAppliesDeepSeekDefaultsAndCompatibilityProfile(t *testing.T) {
	t.Parallel()

	const apiKey = "deepseek-test-key"
	var captured *http.Request
	var capturedBody []byte
	provider, err := New(Config{
		HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			captured = request.Clone(request.Context())
			var readErr error
			capturedBody, readErr = io.ReadAll(request.Body)
			if readErr != nil {
				t.Fatalf("io.ReadAll(request.Body) error = %v", readErr)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"text/event-stream"},
				},
				Body: io.NopCloser(strings.NewReader(`data: {"choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":null}]}

data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: {"choices":[],"usage":{"prompt_tokens":9,"completion_tokens":2}}

data: [DONE]

`)),
			}, nil
		})},
		APIKey:          apiKey,
		Model:           "configured-deepseek-model",
		ReasoningEffort: "high",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	stream := provider.Stream(context.Background(), ai.Request{
		SystemPrompt: "system",
		Messages: []ai.Message{
			ai.AssistantMessage{
				Content: []ai.AssistantContent{
					ai.ThinkingContent{Thinking: "replay this reasoning"},
					ai.ToolCall{ID: "call-1", Name: "read", Arguments: json.RawMessage(`{}`)},
				},
				StopReason: ai.StopReasonToolUse,
			},
			ai.ToolResultMessage{ToolCallID: "call-1", ToolName: "read", Content: "result"},
		},
	})
	var done ai.DoneEvent
	for {
		event, receiveErr := stream.Receive()
		if receiveErr == io.EOF {
			break
		}
		if receiveErr != nil {
			t.Fatalf("Receive() error = %v", receiveErr)
		}
		if terminal, ok := event.(ai.DoneEvent); ok {
			done = terminal
		}
	}
	if done.Message.StopReason != ai.StopReasonStop {
		t.Fatalf("terminal = %#v, want successful stop", done.Message)
	}
	if captured == nil {
		t.Fatal("HTTP request was not sent")
	}
	if got := captured.URL.String(); got != DefaultEndpoint {
		t.Errorf("endpoint = %q, want %q", got, DefaultEndpoint)
	}
	if got := captured.Header.Get("Authorization"); got != "Bearer "+apiKey {
		t.Errorf("Authorization = %q, want bearer API key", got)
	}
	if strings.Contains(string(capturedBody), apiKey) {
		t.Fatalf("request body contains API key: %s", capturedBody)
	}

	var payload map[string]any
	if err := json.Unmarshal(capturedBody, &payload); err != nil {
		t.Fatalf("json.Unmarshal(request body) error = %v", err)
	}
	if payload["model"] != "configured-deepseek-model" {
		t.Errorf("model = %#v, want configured model", payload["model"])
	}
	if payload["reasoning_effort"] != "high" {
		t.Errorf("reasoning_effort = %#v, want configured value", payload["reasoning_effort"])
	}
	streamOptions, ok := payload["stream_options"].(map[string]any)
	if !ok || streamOptions["include_usage"] != true {
		t.Errorf("stream_options = %#v, want include_usage", payload["stream_options"])
	}
	thinking, ok := payload["thinking"].(map[string]any)
	if !ok || thinking["type"] != "enabled" {
		t.Errorf("thinking = %#v, want enabled", payload["thinking"])
	}
	messages, ok := payload["messages"].([]any)
	if !ok || len(messages) < 2 {
		t.Fatalf("messages = %#v, want replayed assistant", payload["messages"])
	}
	assistant, ok := messages[1].(map[string]any)
	if !ok || assistant["reasoning_content"] != "replay this reasoning" {
		t.Errorf("assistant message = %#v, want reasoning_content replay", messages[1])
	}
}

func TestNewEnforcesEndpointSecurityPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		endpoint       string
		allowLocalHTTP bool
		wantError      bool
	}{
		{name: "default HTTPS endpoint"},
		{name: "remote HTTPS", endpoint: "https://gateway.example.test/chat/completions"},
		{name: "remote HTTP", endpoint: "http://gateway.example.test/chat/completions", wantError: true},
		{name: "remote HTTP despite override", endpoint: "http://gateway.example.test/chat/completions", allowLocalHTTP: true, wantError: true},
		{name: "localhost HTTP without override", endpoint: "http://localhost:8080/chat/completions", wantError: true},
		{name: "localhost lookalike", endpoint: "http://localhost.example.test/chat/completions", allowLocalHTTP: true, wantError: true},
		{name: "localhost HTTP with override", endpoint: "http://localhost:8080/chat/completions", allowLocalHTTP: true},
		{name: "IPv4 loopback HTTP with override", endpoint: "http://127.0.0.1:8080/chat/completions", allowLocalHTTP: true},
		{name: "IPv6 loopback HTTP with override", endpoint: "http://[::1]:8080/chat/completions", allowLocalHTTP: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := New(Config{
				Endpoint:               test.endpoint,
				APIKey:                 "key",
				Model:                  "model",
				AllowInsecureLocalhost: test.allowLocalHTTP,
			})
			if test.wantError && err == nil {
				t.Fatal("New() error = nil, want endpoint policy error")
			}
			if !test.wantError && err != nil {
				t.Fatalf("New() error = %v, want success", err)
			}
		})
	}
}

func TestNewRequiresDeepSeekCredentialsAndModel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config Config
	}{
		{name: "missing API key", config: Config{Model: "model"}},
		{name: "blank API key", config: Config{APIKey: " \t", Model: "model"}},
		{name: "missing model", config: Config{APIKey: "key"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := New(test.config); err == nil {
				t.Fatal("New() error = nil, want required configuration error")
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
