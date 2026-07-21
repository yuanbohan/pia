package openaicompatible

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/yuanbohan/pia/internal/ai"
)

func TestBuildRequestPayloadMapsCompleteTranscript(t *testing.T) {
	t.Parallel()

	request := ai.Request{
		SystemPrompt:    "You are a coding agent.",
		MaxOutputTokens: 12345,
		Messages: []ai.Message{
			ai.UserMessage{Content: "Say hello."},
			ai.AssistantMessage{
				Content: []ai.AssistantContent{
					ai.TextContent{Text: "hello"},
					ai.TextContent{Text: " world"},
				},
				StopReason: ai.StopReasonStop,
			},
			ai.UserMessage{Content: "Inspect main.go."},
			ai.AssistantMessage{
				Content: []ai.AssistantContent{
					ai.ThinkingContent{Thinking: "inspect carefully"},
					ai.ToolCall{
						ID:        "call-1",
						Name:      "read",
						Arguments: json.RawMessage(`{"path":"main.go"}`),
					},
				},
				StopReason: ai.StopReasonToolUse,
			},
			ai.ToolResultMessage{
				ToolCallID: "call-1",
				ToolName:   "read",
				Content:    "permission denied",
				IsError:    true,
			},
			ai.AssistantMessage{
				StopReason:   ai.StopReasonError,
				ErrorMessage: "provider failed with secret-value",
			},
			ai.UserMessage{Content: "Continue."},
		},
		Tools: []ai.ToolSchema{
			{
				Name:        "read",
				Description: "Read one workspace file.",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
			},
		},
	}

	payload, err := buildRequestPayload(
		"configured-model",
		request,
		Profile{
			ReplayReasoning: true,
			StreamUsage:     true,
			Thinking:        true,
		},
		"high",
	)
	if err != nil {
		t.Fatalf("buildRequestPayload() error = %v", err)
	}

	assertJSONEqual(t, payload, `{
		"model": "configured-model",
		"messages": [
			{"role": "system", "content": "You are a coding agent."},
			{"role": "user", "content": "Say hello."},
			{"role": "assistant", "content": "hello world"},
			{"role": "user", "content": "Inspect main.go."},
			{
				"role": "assistant",
				"content": null,
				"reasoning_content": "inspect carefully",
				"tool_calls": [
					{
						"id": "call-1",
						"type": "function",
						"function": {
							"name": "read",
							"arguments": "{\"path\":\"main.go\"}"
						}
					}
				]
			},
			{"role": "tool", "content": "permission denied", "tool_call_id": "call-1"},
			{"role": "assistant", "content": ""},
			{"role": "user", "content": "Continue."}
		],
		"tools": [
			{
				"type": "function",
				"function": {
					"name": "read",
					"description": "Read one workspace file.",
					"parameters": {
						"type": "object",
						"properties": {"path": {"type": "string"}},
						"required": ["path"]
					}
				}
			}
		],
		"stream": true,
		"max_tokens": 12345,
		"stream_options": {"include_usage": true},
		"thinking": {"type": "enabled"},
		"reasoning_effort": "high"
	}`)

	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal(payload) error = %v", err)
	}
	for _, forbidden := range []string{"stop_reason", "error_message", "is_error", "secret-value"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Errorf("payload contains internal-only value %q: %s", forbidden, encoded)
		}
	}
}

func TestBuildRequestPayloadRejectsMalformedToolSchema(t *testing.T) {
	t.Parallel()

	_, err := buildRequestPayload(
		"configured-model",
		ai.Request{
			Tools: []ai.ToolSchema{
				{Name: "read", Parameters: json.RawMessage(`{"type":`)},
			},
		},
		Profile{},
		"",
	)
	if err == nil {
		t.Fatal("buildRequestPayload() error = nil, want malformed schema error")
	}
	if !strings.Contains(err.Error(), `tool "read" parameters`) {
		t.Fatalf("buildRequestPayload() error = %q, want tool context", err)
	}
}

func assertJSONEqual(t *testing.T, got any, want string) {
	t.Helper()

	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal(got) error = %v", err)
	}

	var gotValue any
	if err := json.Unmarshal(gotJSON, &gotValue); err != nil {
		t.Fatalf("json.Unmarshal(got) error = %v", err)
	}

	var wantValue any
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("json.Unmarshal(want) error = %v", err)
	}

	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("JSON mismatch\n got: %s\nwant: %s", gotJSON, want)
	}
}
