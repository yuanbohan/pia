package ai

// Stream exposes one Provider response as a pull-based event sequence.
// After a terminal DoneEvent or ErrorEvent, Receive returns io.EOF forever.
type Stream interface {
	Receive() (Event, error)
}

// Event is one step in the formation of an AssistantMessage.
type Event interface {
	isEvent()
}

type StartEvent struct{}

func (StartEvent) isEvent() {}

type TextStartEvent struct {
	ContentIndex int
}

func (TextStartEvent) isEvent() {}

type TextDeltaEvent struct {
	ContentIndex int
	Delta        string
}

func (TextDeltaEvent) isEvent() {}

type TextEndEvent struct {
	ContentIndex int
	Text         string
}

func (TextEndEvent) isEvent() {}

type ThinkingStartEvent struct {
	ContentIndex int
}

func (ThinkingStartEvent) isEvent() {}

type ThinkingDeltaEvent struct {
	ContentIndex int
	Delta        string
}

func (ThinkingDeltaEvent) isEvent() {}

type ThinkingEndEvent struct {
	ContentIndex int
	Thinking     string
}

func (ThinkingEndEvent) isEvent() {}

type ToolCallStartEvent struct {
	ContentIndex int
	ID           string
	Name         string
}

func (ToolCallStartEvent) isEvent() {}

type ToolCallDeltaEvent struct {
	ContentIndex int
	Delta        string
}

func (ToolCallDeltaEvent) isEvent() {}

type ToolCallEndEvent struct {
	ContentIndex int
	ToolCall     ToolCall
}

func (ToolCallEndEvent) isEvent() {}

// DoneEvent is the successful terminal event.
type DoneEvent struct {
	Message AssistantMessage
}

func (DoneEvent) isEvent() {}

// ErrorEvent is the failed or aborted terminal event.
type ErrorEvent struct {
	Message AssistantMessage
}

func (ErrorEvent) isEvent() {}
