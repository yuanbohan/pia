package coding

import (
	"strings"

	"github.com/yuanbohan/pia/internal/ai"
)

// TODO: Re-evaluate this coding-owned text classifier when a Provider exposes
// a stable structured overflow code, a second Provider needs the same
// classification, or generic retry becomes another consumer. D81 keeps it
// beside its only current policy consumer instead of claiming an ai-level
// contract that the current DeepSeek evidence does not support.
func isExplicitContextOverflow(message ai.AssistantMessage) bool {
	if message.StopReason != ai.StopReasonError {
		return false
	}

	text := strings.ToLower(strings.TrimSpace(message.ErrorMessage))
	if text == "" {
		return false
	}
	normalized := strings.NewReplacer("_", " ", "-", " ").Replace(text)
	if hasNonOverflowErrorEvidence(normalized) {
		return false
	}

	if strings.Contains(normalized, "context length exceeded") {
		return true
	}
	if strings.Contains(normalized, "context window") && strings.Contains(normalized, "exceed") {
		return true
	}
	return strings.Contains(normalized, "maximum context length") &&
		(strings.Contains(normalized, "exceed") ||
			strings.Contains(normalized, "too long") ||
			strings.Contains(normalized, "longer"))
}

func hasNonOverflowErrorEvidence(text string) bool {
	for _, fragment := range [...]string{
		"rate limit",
		"too many requests",
		"throttl",
		"http status 429",
		"http 429",
		"http status 5",
		"http 5",
		"internal server error",
		"bad gateway",
		"service unavailable",
		"gateway timeout",
		"server overload",
		"server is overload",
	} {
		if strings.Contains(text, fragment) {
			return true
		}
	}
	return false
}

func recoverableOverflowTerminal(messages []ai.Message) (int, bool) {
	if len(messages) == 0 {
		return 0, false
	}
	index := len(messages) - 1
	terminal, ok := messages[index].(ai.AssistantMessage)
	if !ok || !isExplicitContextOverflow(terminal) {
		return 0, false
	}
	for _, content := range terminal.Content {
		if _, ok := compactionToolCall(content); ok {
			return 0, false
		}
	}
	return index, true
}
