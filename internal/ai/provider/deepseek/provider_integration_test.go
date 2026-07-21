//go:build integration

package deepseek

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/yuanbohan/pia/internal/ai"
)

const smokeOptInEnvironment = "PIA_RUN_DEEPSEEK_SMOKE"

func TestDeepSeekSmoke(t *testing.T) {
	if os.Getenv(smokeOptInEnvironment) != "1" {
		t.Skipf("set %s=1 and run with -tags=integration to enable paid DeepSeek smoke tests", smokeOptInEnvironment)
	}

	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	model := os.Getenv("DEEPSEEK_MODEL")
	endpoint := os.Getenv("DEEPSEEK_ENDPOINT")
	if strings.TrimSpace(apiKey) == "" {
		t.Fatal("DEEPSEEK_API_KEY is required after smoke opt-in")
	}
	if strings.TrimSpace(model) == "" {
		t.Fatal("DEEPSEEK_MODEL is required after smoke opt-in")
	}
	loggedEndpoint := endpoint
	if strings.TrimSpace(loggedEndpoint) == "" {
		loggedEndpoint = DefaultEndpoint
	}
	t.Logf("DeepSeek smoke model=%q endpoint=%q", model, loggedEndpoint)

	provider, err := New(Config{
		Endpoint: endpoint,
		APIKey:   apiKey,
		Model:    model,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Run("text", func(t *testing.T) {
		message := smokeTurn(t, provider, ai.Request{
			SystemPrompt: "Follow the user's response-format instruction exactly.",
			Messages: []ai.Message{
				ai.UserMessage{Content: "Reply with exactly: pia-smoke-ok"},
			},
		})
		var text strings.Builder
		for _, content := range message.Content {
			if block, ok := content.(ai.TextContent); ok {
				text.WriteString(block.Text)
			}
		}
		if !strings.Contains(text.String(), "pia-smoke-ok") {
			t.Fatalf("response text = %q, want smoke marker", text.String())
		}
	})

	t.Run("tool call", func(t *testing.T) {
		message := smokeTurn(t, provider, ai.Request{
			SystemPrompt: "Use the provided tool when the user explicitly requests it.",
			Messages: []ai.Message{
				ai.UserMessage{Content: `Call echo exactly once with {"value":"pia-tool-smoke"}.`},
			},
			Tools: []ai.ToolSchema{
				{
					Name:        "echo",
					Description: "Echo one value.",
					Parameters: json.RawMessage(`{
						"type":"object",
						"properties":{"value":{"type":"string"}},
						"required":["value"],
						"additionalProperties":false
					}`),
				},
			},
		})
		var call *ai.ToolCall
		for _, content := range message.Content {
			if block, ok := content.(ai.ToolCall); ok {
				copy := block
				call = &copy
				break
			}
		}
		if call == nil {
			t.Fatalf("terminal content = %#v, want echo tool call", message.Content)
		}
		if call.Name != "echo" || !json.Valid(call.Arguments) {
			t.Fatalf("tool call = %#v, want valid echo arguments", *call)
		}
	})

	t.Run("explicitly settled aborted history", func(t *testing.T) {
		message := smokeTurn(t, provider, ai.Request{
			SystemPrompt: "Continue from the complete ordered transcript.",
			Messages: []ai.Message{
				ai.UserMessage{Content: "Read main.go."},
				ai.AssistantMessage{
					Content: []ai.AssistantContent{
						ai.ThinkingContent{Thinking: "I should inspect the file."},
						ai.ToolCall{ID: "settled-call-1", Name: "read", Arguments: json.RawMessage(`{"path":"main.go"}`)},
					},
					StopReason:   ai.StopReasonAborted,
					ErrorMessage: "local cancellation",
				},
				ai.ToolResultMessage{
					ToolCallID: "settled-call-1",
					ToolName:   "read",
					Content:    "Not executed because the provider turn was aborted.",
					IsError:    true,
				},
				ai.UserMessage{Content: "The prior call is settled. Reply with: continued-ok"},
			},
			Tools: []ai.ToolSchema{
				{
					Name:        "read",
					Description: "Read one file.",
					Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
				},
			},
		})
		if message.StopReason == ai.StopReasonError || message.StopReason == ai.StopReasonAborted {
			t.Fatalf("continued terminal = %#v, want accepted settled history", message)
		}
	})
}

func smokeTurn(t *testing.T, provider ai.Provider, request ai.Request) ai.AssistantMessage {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	stream := provider.Stream(ctx, request)
	for {
		event, err := stream.Receive()
		if err != nil {
			t.Fatalf("Receive() error = %v", err)
		}
		switch terminal := event.(type) {
		case ai.DoneEvent:
			return terminal.Message
		case ai.ErrorEvent:
			t.Fatalf("provider terminal error: %s", terminal.Message.ErrorMessage)
		default:
			if cause := context.Cause(ctx); cause != nil {
				t.Fatalf("DeepSeek smoke context ended: %v", cause)
			}
		}
	}
}
