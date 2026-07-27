// Package observation defines the bounded, ephemeral semantic facts emitted
// while one coding operation is running.
package observation

import (
	"strings"
	"unicode/utf8"

	"github.com/yuanbohan/pia/internal/ai"
)

const (
	maxToolNameBytes    = 64
	maxToolSummaryBytes = 512
	truncationMarker    = "..."
)

// Event is one immutable semantic observation. Its concrete values carry only
// fixed enums, scalars, and independently owned bounded strings.
type Event interface {
	isEvent()
}

// Observer receives events synchronously in producer-defined order.
type Observer func(Event)

// Observe delivers event when an observer is installed.
func (observer Observer) Observe(event Event) {
	if observer != nil {
		observer(event)
	}
}

// Phase distinguishes the start and settlement facts of one operation.
type Phase string

const (
	PhaseStarted Phase = "started"
	PhaseSettled Phase = "settled"
)

// Outcome classifies a settled operation without copying its error.
type Outcome string

const (
	OutcomeSuccess Outcome = "success"
	OutcomeError   Outcome = "error"
)

// OutcomeFromError maps the current binary settlement policy.
func OutcomeFromError(err error) Outcome {
	if err != nil {
		return OutcomeError
	}
	return OutcomeSuccess
}

// RunMode distinguishes an input-started execution from a continuation.
type RunMode string

const (
	RunModeInput        RunMode = "input"
	RunModeContinuation RunMode = "continuation"
)

// CompactionReason explains why a real compaction attempt began.
type CompactionReason string

const (
	CompactionReasonThreshold CompactionReason = "threshold"
	CompactionReasonOverflow  CompactionReason = "overflow"
)

// MessageRole identifies which complete protocol message was accepted.
type MessageRole string

const (
	MessageRoleUser       MessageRole = "user"
	MessageRoleAssistant  MessageRole = "assistant"
	MessageRoleToolResult MessageRole = "tool_result"
)

// Advance observes the complete outer user operation.
type Advance struct {
	Phase   Phase
	Outcome Outcome
}

func (Advance) isEvent() {}

// Compaction observes one threshold or overflow compaction attempt.
type Compaction struct {
	Phase   Phase
	Reason  CompactionReason
	Outcome Outcome
}

func (Compaction) isEvent() {}

// Run observes one accepted Core Agent execution.
type Run struct {
	Phase   Phase
	Mode    RunMode
	Outcome Outcome
}

func (Run) isEvent() {}

// Turn observes one Provider response cycle and its tool settlements.
type Turn struct {
	Phase   Phase
	Outcome Outcome
}

func (Turn) isEvent() {}

// Message observes one complete message accepted into Working Context.
type Message struct {
	Role       MessageRole
	StopReason ai.StopReason
	IsError    bool
}

func (Message) isEvent() {}

// Tool observes one model-requested tool execution.
type Tool struct {
	Phase   Phase
	Index   int
	Name    string
	Summary string
	Outcome Outcome
}

func (Tool) isEvent() {}

// NewToolStarted builds a bounded tool-start observation.
func NewToolStarted(index int, name, summary string) Tool {
	return Tool{
		Phase:   PhaseStarted,
		Index:   index,
		Name:    boundedText(name, maxToolNameBytes),
		Summary: boundedText(summary, maxToolSummaryBytes),
	}
}

// NewToolSettled builds a bounded tool-settlement observation.
func NewToolSettled(index int, name, summary string, outcome Outcome) Tool {
	return Tool{
		Phase:   PhaseSettled,
		Index:   index,
		Name:    boundedText(name, maxToolNameBytes),
		Summary: boundedText(summary, maxToolSummaryBytes),
		Outcome: outcome,
	}
}

func boundedText(value string, limit int) string {
	value = strings.ToValidUTF8(value, "\uFFFD")
	if len(value) <= limit {
		return strings.Clone(value)
	}

	end := limit - len(truncationMarker)
	for end > 0 && !utf8.ValidString(value[:end]) {
		end--
	}
	return value[:end] + truncationMarker
}
