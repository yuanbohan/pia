package coding

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/yuanbohan/pi-go/internal/ai"
)

const summarizationSystemPrompt = `You are a context summarization assistant. Your task is to read a conversation between a user and an AI assistant, then produce a structured summary following the exact format specified.

Do NOT continue the conversation. Do NOT respond to any questions in the conversation. ONLY output the structured summary.`

const initialSummarizationPrompt = `The messages above are a conversation to summarize. Create a structured context checkpoint summary that another LLM will use to continue the work.

Use this EXACT format:

## Goal
[What is the user trying to accomplish? Can be multiple items if the session covers different tasks.]

## Constraints & Preferences
- [Any constraints, preferences, or requirements mentioned by user]
- [Or "(none)" if none were mentioned]

## Progress
### Done
- [x] [Completed tasks/changes]

### In Progress
- [ ] [Current work]

### Blocked
- [Issues preventing progress, if any]

## Key Decisions
- **[Decision]**: [Brief rationale]

## Next Steps
1. [Ordered list of what should happen next]

## Critical Context
- [Any data, examples, or references needed to continue]
- [Or "(none)" if not applicable]

Keep each section concise. Preserve exact file paths, function names, and error messages.`

const updateSummarizationPrompt = `The messages above are NEW conversation messages to incorporate into the existing summary provided in <previous-summary> tags.

Update the existing structured summary with new information. RULES:
- PRESERVE all existing information from the previous summary
- ADD new progress, decisions, and context from the new messages
- UPDATE the Progress section: move items from "In Progress" to "Done" when completed
- UPDATE "Next Steps" based on what was accomplished
- PRESERVE exact file paths, function names, and error messages
- If something is no longer relevant, you may remove it

Use this EXACT format:

## Goal
[Preserve existing goals, add new ones if the task expanded]

## Constraints & Preferences
- [Preserve existing, add new ones discovered]

## Progress
### Done
- [x] [Include previously done items AND newly completed items]

### In Progress
- [ ] [Current work - update based on progress]

### Blocked
- [Current blockers - remove if resolved]

## Key Decisions
- **[Decision]**: [Brief rationale] (preserve all previous, add new)

## Next Steps
1. [Update based on current state]

## Critical Context
- [Preserve important context, add new if needed]

Keep each section concise. Preserve exact file paths, function names, and error messages.`

const turnPrefixSummarizationPrompt = `This is the PREFIX of a turn that was too large to keep. The SUFFIX (recent work) is retained.

Summarize the prefix to provide context for the retained suffix:

## Original Request
[What did the user ask for in this turn?]

## Early Progress
- [Key decisions and work done in the prefix]

## Context for Suffix
- [Information needed to understand the retained recent work]

Be concise. Focus on what's needed to understand the kept suffix.`

const compactionSummaryPrefix = `The conversation history before this point was compacted into the following summary:

<summary>
`

const compactionSummarySuffix = `
</summary>`

const toolResultSummaryMaxCharacters = 2000

func buildSummarizationPrompt(messages []ai.Message, previousSummary string) string {
	var prompt strings.Builder
	prompt.WriteString("<conversation>\n")
	prompt.WriteString(serializeConversation(messages))
	prompt.WriteString("\n</conversation>\n\n")
	if previousSummary != "" {
		prompt.WriteString("<previous-summary>\n")
		prompt.WriteString(previousSummary)
		prompt.WriteString("\n</previous-summary>\n\n")
		prompt.WriteString(updateSummarizationPrompt)
	} else {
		prompt.WriteString(initialSummarizationPrompt)
	}
	return prompt.String()
}

func buildTurnPrefixPrompt(messages []ai.Message) string {
	return "<conversation>\n" + serializeConversation(messages) +
		"\n</conversation>\n\n" + turnPrefixSummarizationPrompt
}

func syntheticSummaryMessage(summary string) ai.UserMessage {
	return ai.UserMessage{Content: compactionSummaryPrefix + summary + compactionSummarySuffix}
}

func serializeConversation(messages []ai.Message) string {
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		switch message := message.(type) {
		case ai.UserMessage:
			if message.Content != "" {
				parts = append(parts, "[User]: "+message.Content)
			}
		case ai.AssistantMessage:
			parts = append(parts, serializeAssistant(message)...)
		case ai.ToolResultMessage:
			if message.Content != "" {
				parts = append(parts, "[Tool result]: "+truncateSummaryText(message.Content))
			}
		case *ai.UserMessage:
			if message != nil && message.Content != "" {
				parts = append(parts, "[User]: "+message.Content)
			}
		case *ai.AssistantMessage:
			if message != nil {
				parts = append(parts, serializeAssistant(*message)...)
			}
		case *ai.ToolResultMessage:
			if message != nil && message.Content != "" {
				parts = append(parts, "[Tool result]: "+truncateSummaryText(message.Content))
			}
		}
	}
	return strings.Join(parts, "\n\n")
}

func serializeAssistant(message ai.AssistantMessage) []string {
	var textParts []string
	var thinkingParts []string
	var toolCalls []string
	for _, content := range message.Content {
		switch content := content.(type) {
		case ai.TextContent:
			textParts = append(textParts, content.Text)
		case *ai.TextContent:
			if content != nil {
				textParts = append(textParts, content.Text)
			}
		case ai.ThinkingContent:
			thinkingParts = append(thinkingParts, content.Thinking)
		case *ai.ThinkingContent:
			if content != nil {
				thinkingParts = append(thinkingParts, content.Thinking)
			}
		case ai.ToolCall:
			toolCalls = append(toolCalls, formatToolCall(content))
		case *ai.ToolCall:
			if content != nil {
				toolCalls = append(toolCalls, formatToolCall(*content))
			}
		}
	}
	parts := make([]string, 0, 3)
	if len(thinkingParts) > 0 {
		parts = append(parts, "[Assistant thinking]: "+strings.Join(thinkingParts, "\n"))
	}
	if len(textParts) > 0 {
		parts = append(parts, "[Assistant]: "+strings.Join(textParts, "\n"))
	}
	if len(toolCalls) > 0 {
		parts = append(parts, "[Assistant tool calls]: "+strings.Join(toolCalls, "; "))
	}
	return parts
}

func formatToolCall(call ai.ToolCall) string {
	return call.Name + "(" + formatToolArguments(call.Arguments) + ")"
}

func formatToolArguments(arguments json.RawMessage) string {
	decoder := json.NewDecoder(bytes.NewReader(arguments))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return strings.TrimSpace(string(arguments))
	}
	var parts []string
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return strings.TrimSpace(string(arguments))
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return strings.TrimSpace(string(arguments))
		}
		parts = append(parts, fmt.Sprintf("%s=%s", key, strings.TrimSpace(string(value))))
	}
	if _, err := decoder.Token(); err != nil {
		return strings.TrimSpace(string(arguments))
	}
	return strings.Join(parts, ", ")
}

func truncateSummaryText(text string) string {
	characters := []rune(text)
	if len(characters) <= toolResultSummaryMaxCharacters {
		return text
	}
	truncated := len(characters) - toolResultSummaryMaxCharacters
	return string(characters[:toolResultSummaryMaxCharacters]) +
		fmt.Sprintf("\n\n[... %d more characters truncated]", truncated)
}

type fileOperations struct {
	read    map[string]struct{}
	written map[string]struct{}
	edited  map[string]struct{}
}

func extractFileOperations(messages []ai.Message) fileOperations {
	operations := fileOperations{
		read:    make(map[string]struct{}),
		written: make(map[string]struct{}),
		edited:  make(map[string]struct{}),
	}
	operations.add(messages)
	return operations
}

func (operations fileOperations) add(messages []ai.Message) {
	for _, message := range messages {
		assistant, ok := message.(ai.AssistantMessage)
		if !ok {
			if pointer, pointerOK := message.(*ai.AssistantMessage); pointerOK && pointer != nil {
				assistant = *pointer
				ok = true
			}
		}
		if !ok {
			continue
		}
		for _, content := range assistant.Content {
			call, ok := compactionToolCall(content)
			if !ok {
				continue
			}
			var arguments struct {
				Path string `json:"path"`
			}
			if json.Unmarshal(call.Arguments, &arguments) != nil || arguments.Path == "" {
				continue
			}
			switch call.Name {
			case "read":
				operations.read[arguments.Path] = struct{}{}
			case "write":
				operations.written[arguments.Path] = struct{}{}
			case "edit":
				operations.edited[arguments.Path] = struct{}{}
			}
		}
	}
}

func formatFileOperations(operations fileOperations) string {
	modified := make(map[string]struct{}, len(operations.written)+len(operations.edited))
	for path := range operations.written {
		modified[path] = struct{}{}
	}
	for path := range operations.edited {
		modified[path] = struct{}{}
	}
	readOnly := make([]string, 0, len(operations.read))
	for path := range operations.read {
		if _, changed := modified[path]; !changed {
			readOnly = append(readOnly, path)
		}
	}
	modifiedFiles := make([]string, 0, len(modified))
	for path := range modified {
		modifiedFiles = append(modifiedFiles, path)
	}
	sort.Strings(readOnly)
	sort.Strings(modifiedFiles)

	var sections []string
	if len(readOnly) > 0 {
		sections = append(sections, "<read-files>\n"+strings.Join(readOnly, "\n")+"\n</read-files>")
	}
	if len(modifiedFiles) > 0 {
		sections = append(sections, "<modified-files>\n"+strings.Join(modifiedFiles, "\n")+"\n</modified-files>")
	}
	if len(sections) == 0 {
		return ""
	}
	return "\n\n" + strings.Join(sections, "\n\n")
}
