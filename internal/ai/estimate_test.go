package ai_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/yuanbohan/pia/internal/ai"
)

func TestEstimateRequestTokensUsesCompleteRequestWithoutUsage(t *testing.T) {
	t.Parallel()

	request := ai.Request{
		SystemPrompt: "12345",
		Messages: []ai.Message{
			ai.UserMessage{Content: "123456789"},
			ai.AssistantMessage{
				Content: []ai.AssistantContent{
					ai.ThinkingContent{Thinking: "1234"},
					ai.TextContent{Text: "12345"},
					ai.ToolCall{ID: "ignored", Name: "read", Arguments: json.RawMessage(`{"path":"a.go"}`)},
				},
				StopReason: ai.StopReasonToolUse,
			},
			ai.ToolResultMessage{ToolCallID: "ignored", ToolName: "read", Content: "12345678"},
		},
	}

	estimate := ai.EstimateRequestTokens(request)
	// Per-message ceil(chars/4): system 2, user 3, assistant
	// ceil((4+5+4+15)/4)=7, tool result 2.
	if got, want := estimate.Tokens, int64(14); got != want {
		t.Fatalf("EstimateRequestTokens() tokens = %d, want %d", got, want)
	}
	if estimate.UsageTokens != 0 || estimate.TrailingTokens != 14 || estimate.LastUsageIndex != -1 {
		t.Fatalf("EstimateRequestTokens() = %#v, want all-estimated result", estimate)
	}
}

func TestEstimateRequestTokensUsesLastValidUsageAndOnlyEstimatesTail(t *testing.T) {
	t.Parallel()

	request := ai.Request{
		SystemPrompt: "this prefix is already included in Provider usage",
		Messages: []ai.Message{
			ai.UserMessage{Content: "also already counted"},
			ai.AssistantMessage{
				Content:    []ai.AssistantContent{ai.TextContent{Text: "counted output"}},
				Usage:      ai.Usage{InputTokens: 100, OutputTokens: 20},
				StopReason: ai.StopReasonStop,
			},
			ai.UserMessage{Content: "12345"},
		},
	}

	estimate := ai.EstimateRequestTokens(request)
	if got, want := estimate.Tokens, int64(122); got != want {
		t.Fatalf("EstimateRequestTokens() tokens = %d, want %d", got, want)
	}
	if estimate.UsageTokens != 120 || estimate.TrailingTokens != 2 || estimate.LastUsageIndex != 1 {
		t.Fatalf("EstimateRequestTokens() = %#v, want usage plus estimated tail", estimate)
	}
}

func TestEstimateRequestTokensSkipsInvalidUsage(t *testing.T) {
	t.Parallel()

	request := ai.Request{Messages: []ai.Message{
		ai.AssistantMessage{
			Content:    []ai.AssistantContent{ai.TextContent{Text: "valid"}},
			Usage:      ai.Usage{InputTokens: 40, OutputTokens: 10},
			StopReason: ai.StopReasonStop,
		},
		ai.AssistantMessage{
			Content:    []ai.AssistantContent{ai.TextContent{Text: "1234"}},
			Usage:      ai.Usage{InputTokens: 500, OutputTokens: 100},
			StopReason: ai.StopReasonError,
		},
		ai.AssistantMessage{
			Content:    []ai.AssistantContent{ai.TextContent{Text: "1234"}},
			StopReason: ai.StopReasonStop,
		},
	}}

	estimate := ai.EstimateRequestTokens(request)
	if got, want := estimate.Tokens, int64(52); got != want {
		t.Fatalf("EstimateRequestTokens() tokens = %d, want %d", got, want)
	}
	if estimate.UsageTokens != 50 || estimate.TrailingTokens != 2 || estimate.LastUsageIndex != 0 {
		t.Fatalf("EstimateRequestTokens() = %#v, want earlier valid usage plus tail", estimate)
	}
}

func TestRequestLimitsClampOutputTokens(t *testing.T) {
	t.Parallel()

	limits := ai.RequestLimits{
		ContextCapacity: 100,
		ModelMaxOutput:  60,
		ContextSafety:   10,
	}
	if err := limits.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	tests := []struct {
		name      string
		input     int64
		requested int64
		want      int64
	}{
		{name: "requested cap", input: 10, requested: 40, want: 40},
		{name: "model cap", input: 10, requested: 90, want: 60},
		{name: "context cap", input: 70, requested: 60, want: 20},
		{name: "minimum one", input: 100, requested: 60, want: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := limits.ClampOutputTokens(test.input, test.requested); got != test.want {
				t.Fatalf("ClampOutputTokens(%d, %d) = %d, want %d", test.input, test.requested, got, test.want)
			}
		})
	}
}

func TestRequestLimitsValidateRejectsPartialConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		limits ai.RequestLimits
		want   string
	}{
		{name: "missing capacity", limits: ai.RequestLimits{ModelMaxOutput: 1}, want: "context capacity"},
		{name: "missing max output", limits: ai.RequestLimits{ContextCapacity: 1}, want: "model max output"},
		{
			name: "negative safety",
			limits: ai.RequestLimits{
				ContextCapacity: 1,
				ModelMaxOutput:  1,
				ContextSafety:   -1,
			},
			want: "context safety",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := test.limits.Validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want %q", err, test.want)
			}
		})
	}
	if err := (ai.RequestLimits{}).Validate(); err != nil {
		t.Fatalf("zero-value Validate() error = %v, want disabled limits", err)
	}
}
