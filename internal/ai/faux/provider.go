// Package faux provides a deterministic, scripted implementation of ai.Provider.
package faux

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/yuanbohan/pi-go/internal/ai"
)

const (
	queueExhaustedMessage = "faux response queue exhausted"
	abortedMessage        = "request was aborted"
)

// Step is the complete event script for one Provider call.
type Step struct {
	Events []ai.Event
}

// Provider consumes one validated Step for each call to Stream.
type Provider struct {
	mu       sync.Mutex
	steps    []Step
	nextStep int
	requests []ai.Request
}

// New constructs a Provider and rejects malformed scripts before they run.
func New(steps ...Step) (*Provider, error) {
	clonedSteps := make([]Step, len(steps))
	for index, step := range steps {
		events, err := cloneEvents(step.Events)
		if err != nil {
			return nil, fmt.Errorf("faux step %d: %w", index, err)
		}
		clonedSteps[index] = Step{Events: events}
		if err := validateStep(clonedSteps[index]); err != nil {
			return nil, fmt.Errorf("faux step %d: %w", index, err)
		}
	}

	return &Provider{steps: clonedSteps}, nil
}

// Stream records a request snapshot and consumes the next scripted Step.
func (p *Provider) Stream(ctx context.Context, request ai.Request) ai.Stream {
	p.mu.Lock()
	p.requests = append(p.requests, cloneRequest(request))

	var events []ai.Event
	if p.nextStep >= len(p.steps) {
		events = []ai.Event{providerErrorEvent(queueExhaustedMessage)}
	} else {
		events = p.steps[p.nextStep].Events
		p.nextStep++
	}
	p.mu.Unlock()

	return newStream(ctx, events)
}

// Requests returns independent copies of all request snapshots in call order.
func (p *Provider) Requests() []ai.Request {
	p.mu.Lock()
	defer p.mu.Unlock()

	requests := make([]ai.Request, len(p.requests))
	for index, request := range p.requests {
		requests[index] = cloneRequest(request)
	}
	return requests
}

type stream struct {
	ctx       context.Context
	events    []ai.Event
	nextEvent int
	terminal  bool
	partial   partialCollector
}

func newStream(ctx context.Context, events []ai.Event) *stream {
	return &stream{
		ctx:    ctx,
		events: events,
		partial: partialCollector{
			blocks: make(map[int]*blockState),
		},
	}
}

func (s *stream) Receive() (ai.Event, error) {
	if s.terminal {
		return nil, io.EOF
	}

	if s.ctx.Err() != nil {
		s.terminal = true
		return ai.ErrorEvent{Message: s.partial.abortedMessage()}, nil
	}

	if s.nextEvent >= len(s.events) {
		s.terminal = true
		return providerErrorEvent("faux stream exhausted before terminal"), nil
	}

	event := s.events[s.nextEvent]
	s.nextEvent++
	s.partial.observe(event)
	switch event.(type) {
	case ai.DoneEvent, ai.ErrorEvent:
		s.terminal = true
	}

	cloned, err := cloneEvent(event)
	if err != nil {
		s.terminal = true
		return nil, err
	}
	return cloned, nil
}

type blockKind uint8

const (
	blockKindText blockKind = iota + 1
	blockKindThinking
	blockKindToolCall
)

type blockState struct {
	kind      blockKind
	text      strings.Builder
	id        string
	name      string
	arguments strings.Builder
	ended     bool
	toolCall  ai.ToolCall
}

type partialCollector struct {
	blocks map[int]*blockState
}

func (c *partialCollector) observe(event ai.Event) {
	switch event := event.(type) {
	case ai.TextStartEvent:
		c.blocks[event.ContentIndex] = &blockState{kind: blockKindText}
	case ai.TextDeltaEvent:
		c.blocks[event.ContentIndex].text.WriteString(event.Delta)
	case ai.TextEndEvent:
		c.blocks[event.ContentIndex].ended = true
	case ai.ThinkingStartEvent:
		c.blocks[event.ContentIndex] = &blockState{kind: blockKindThinking}
	case ai.ThinkingDeltaEvent:
		c.blocks[event.ContentIndex].text.WriteString(event.Delta)
	case ai.ThinkingEndEvent:
		c.blocks[event.ContentIndex].ended = true
	case ai.ToolCallStartEvent:
		c.blocks[event.ContentIndex] = &blockState{kind: blockKindToolCall}
	case ai.ToolCallEndEvent:
		block := c.blocks[event.ContentIndex]
		block.ended = true
		block.toolCall = cloneToolCall(event.ToolCall)
	}
}

func (c *partialCollector) abortedMessage() ai.AssistantMessage {
	return ai.AssistantMessage{
		Content:      assembledContent(c.blocks),
		StopReason:   ai.StopReasonAborted,
		ErrorMessage: abortedMessage,
	}
}

func validateStep(step Step) error {
	if len(step.Events) == 0 {
		return fmt.Errorf("missing terminal event")
	}

	started := false
	terminal := false
	blocks := make(map[int]*blockState)

	for index, event := range step.Events {
		if terminal {
			return fmt.Errorf("event %d appears after terminal event", index)
		}

		switch event := event.(type) {
		case ai.StartEvent:
			if started || index != 0 {
				return fmt.Errorf("start event must appear once at the beginning")
			}
			started = true
		case ai.TextStartEvent:
			if err := beginBlock(started, blocks, event.ContentIndex, blockKindText); err != nil {
				return fmt.Errorf("text start event %d: %w", index, err)
			}
		case ai.TextDeltaEvent:
			block, err := activeBlock(blocks, event.ContentIndex, blockKindText)
			if err != nil {
				return fmt.Errorf("text delta event %d: %w", index, err)
			}
			block.text.WriteString(event.Delta)
		case ai.TextEndEvent:
			block, err := activeBlock(blocks, event.ContentIndex, blockKindText)
			if err != nil {
				return fmt.Errorf("text end event %d: %w", index, err)
			}
			if got := block.text.String(); got != event.Text {
				return fmt.Errorf("text end event %d content %q does not match deltas %q", index, event.Text, got)
			}
			block.ended = true
		case ai.ThinkingStartEvent:
			if err := beginBlock(started, blocks, event.ContentIndex, blockKindThinking); err != nil {
				return fmt.Errorf("thinking start event %d: %w", index, err)
			}
		case ai.ThinkingDeltaEvent:
			block, err := activeBlock(blocks, event.ContentIndex, blockKindThinking)
			if err != nil {
				return fmt.Errorf("thinking delta event %d: %w", index, err)
			}
			block.text.WriteString(event.Delta)
		case ai.ThinkingEndEvent:
			block, err := activeBlock(blocks, event.ContentIndex, blockKindThinking)
			if err != nil {
				return fmt.Errorf("thinking end event %d: %w", index, err)
			}
			if got := block.text.String(); got != event.Thinking {
				return fmt.Errorf("thinking end event %d content %q does not match deltas %q", index, event.Thinking, got)
			}
			block.ended = true
		case ai.ToolCallStartEvent:
			if event.ID == "" || event.Name == "" {
				return fmt.Errorf("tool-call start event %d requires id and name", index)
			}
			if err := beginBlock(started, blocks, event.ContentIndex, blockKindToolCall); err != nil {
				return fmt.Errorf("tool-call start event %d: %w", index, err)
			}
			blocks[event.ContentIndex].id = event.ID
			blocks[event.ContentIndex].name = event.Name
		case ai.ToolCallDeltaEvent:
			block, err := activeBlock(blocks, event.ContentIndex, blockKindToolCall)
			if err != nil {
				return fmt.Errorf("tool-call delta event %d: %w", index, err)
			}
			block.arguments.WriteString(event.Delta)
		case ai.ToolCallEndEvent:
			block, err := activeBlock(blocks, event.ContentIndex, blockKindToolCall)
			if err != nil {
				return fmt.Errorf("tool-call end event %d: %w", index, err)
			}
			if event.ToolCall.ID != block.id || event.ToolCall.Name != block.name {
				return fmt.Errorf("tool-call end event %d id or name does not match its start", index)
			}
			if !json.Valid(event.ToolCall.Arguments) {
				return fmt.Errorf("tool-call end event %d arguments are not valid JSON", index)
			}
			if !bytes.Equal(bytes.TrimSpace(event.ToolCall.Arguments), bytes.TrimSpace([]byte(block.arguments.String()))) {
				return fmt.Errorf("tool-call end event %d arguments do not match deltas", index)
			}
			block.ended = true
			block.toolCall = cloneToolCall(event.ToolCall)
		case ai.DoneEvent:
			if !started {
				return fmt.Errorf("done event requires a start event")
			}
			if hasActiveBlock(blocks) {
				return fmt.Errorf("done event cannot interrupt an active content block")
			}
			if !isDoneReason(event.Message.StopReason) {
				return fmt.Errorf("done event has invalid stop reason %q", event.Message.StopReason)
			}
			if event.Message.ErrorMessage != "" {
				return fmt.Errorf("done event cannot carry an error message")
			}
			if err := validateTerminalContent(event.Message.Content, blocks); err != nil {
				return fmt.Errorf("done event: %w", err)
			}
			terminal = true
		case ai.ErrorEvent:
			if !isErrorReason(event.Message.StopReason) {
				return fmt.Errorf("error event has invalid stop reason %q", event.Message.StopReason)
			}
			if event.Message.ErrorMessage == "" {
				return fmt.Errorf("error event requires an error message")
			}
			if err := validateTerminalContent(event.Message.Content, blocks); err != nil {
				return fmt.Errorf("error event: %w", err)
			}
			terminal = true
		default:
			return fmt.Errorf("unsupported event %T", event)
		}
	}

	if !terminal {
		return fmt.Errorf("missing terminal event")
	}
	return nil
}

func beginBlock(started bool, blocks map[int]*blockState, contentIndex int, kind blockKind) error {
	if !started {
		return fmt.Errorf("content block requires a start event")
	}
	if contentIndex < 0 {
		return fmt.Errorf("content index must be non-negative")
	}
	if _, exists := blocks[contentIndex]; exists {
		return fmt.Errorf("content index %d already exists", contentIndex)
	}
	blocks[contentIndex] = &blockState{kind: kind}
	return nil
}

func activeBlock(blocks map[int]*blockState, contentIndex int, kind blockKind) (*blockState, error) {
	block, exists := blocks[contentIndex]
	if !exists || block.kind != kind || block.ended {
		return nil, fmt.Errorf("content index %d has no active matching block", contentIndex)
	}
	return block, nil
}

func hasActiveBlock(blocks map[int]*blockState) bool {
	for _, block := range blocks {
		if !block.ended {
			return true
		}
	}
	return false
}

func validateTerminalContent(got []ai.AssistantContent, blocks map[int]*blockState) error {
	indices := orderedBlockIndices(blocks)
	for position, index := range indices {
		if index != position {
			return fmt.Errorf("content index %d does not map to final content position %d", index, position)
		}
	}

	want := assembledContent(blocks)
	if !reflect.DeepEqual(got, want) {
		return fmt.Errorf("message content %#v does not match streamed content %#v", got, want)
	}
	return nil
}

func assembledContent(blocks map[int]*blockState) []ai.AssistantContent {
	indices := orderedBlockIndices(blocks)

	var content []ai.AssistantContent
	for _, index := range indices {
		block := blocks[index]
		switch block.kind {
		case blockKindText:
			content = append(content, ai.TextContent{Text: block.text.String()})
		case blockKindThinking:
			content = append(content, ai.ThinkingContent{Thinking: block.text.String()})
		case blockKindToolCall:
			if block.ended {
				content = append(content, cloneToolCall(block.toolCall))
			}
		}
	}
	return content
}

func orderedBlockIndices(blocks map[int]*blockState) []int {
	indices := make([]int, 0, len(blocks))
	for index := range blocks {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	return indices
}

func isDoneReason(reason ai.StopReason) bool {
	switch reason {
	case ai.StopReasonStop, ai.StopReasonLength, ai.StopReasonToolUse:
		return true
	default:
		return false
	}
}

func isErrorReason(reason ai.StopReason) bool {
	return reason == ai.StopReasonError || reason == ai.StopReasonAborted
}

func providerErrorEvent(message string) ai.ErrorEvent {
	return ai.ErrorEvent{Message: ai.AssistantMessage{
		StopReason:   ai.StopReasonError,
		ErrorMessage: message,
	}}
}

func cloneEvents(events []ai.Event) ([]ai.Event, error) {
	cloned := make([]ai.Event, len(events))
	for index, event := range events {
		copy, err := cloneEvent(event)
		if err != nil {
			return nil, fmt.Errorf("event %d: %w", index, err)
		}
		cloned[index] = copy
	}
	return cloned, nil
}

func cloneEvent(event ai.Event) (ai.Event, error) {
	if event == nil {
		return nil, fmt.Errorf("event is nil")
	}
	value := reflect.ValueOf(event)
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil, fmt.Errorf("event %T is nil", event)
		}
		event = value.Elem().Interface().(ai.Event)
	}

	switch event := event.(type) {
	case ai.StartEvent,
		ai.TextStartEvent,
		ai.TextDeltaEvent,
		ai.TextEndEvent,
		ai.ThinkingStartEvent,
		ai.ThinkingDeltaEvent,
		ai.ThinkingEndEvent,
		ai.ToolCallStartEvent,
		ai.ToolCallDeltaEvent:
		return event, nil
	case ai.ToolCallEndEvent:
		event.ToolCall = cloneToolCall(event.ToolCall)
		return event, nil
	case ai.DoneEvent:
		event.Message = cloneAssistantMessage(event.Message)
		return event, nil
	case ai.ErrorEvent:
		event.Message = cloneAssistantMessage(event.Message)
		return event, nil
	default:
		return nil, fmt.Errorf("unsupported event %T", event)
	}
}

func cloneRequest(request ai.Request) ai.Request {
	cloned := ai.Request{SystemPrompt: request.SystemPrompt}
	if request.Messages != nil {
		cloned.Messages = make([]ai.Message, len(request.Messages))
		for index, message := range request.Messages {
			cloned.Messages[index] = cloneMessage(message)
		}
	}
	if request.Tools != nil {
		cloned.Tools = make([]ai.ToolSchema, len(request.Tools))
		for index, tool := range request.Tools {
			cloned.Tools[index] = tool
			cloned.Tools[index].Parameters = bytes.Clone(tool.Parameters)
		}
	}
	return cloned
}

func cloneMessage(message ai.Message) ai.Message {
	switch message := message.(type) {
	case ai.UserMessage:
		return message
	case *ai.UserMessage:
		if message == nil {
			return message
		}
		return *message
	case ai.AssistantMessage:
		return cloneAssistantMessage(message)
	case *ai.AssistantMessage:
		if message == nil {
			return message
		}
		return cloneAssistantMessage(*message)
	case ai.ToolResultMessage:
		return message
	case *ai.ToolResultMessage:
		if message == nil {
			return message
		}
		return *message
	default:
		return message
	}
}

func cloneAssistantMessage(message ai.AssistantMessage) ai.AssistantMessage {
	cloned := message
	if message.Content != nil {
		cloned.Content = make([]ai.AssistantContent, len(message.Content))
		for index, content := range message.Content {
			cloned.Content[index] = cloneAssistantContent(content)
		}
	}
	return cloned
}

func cloneAssistantContent(content ai.AssistantContent) ai.AssistantContent {
	switch content := content.(type) {
	case ai.TextContent:
		return content
	case *ai.TextContent:
		if content == nil {
			return content
		}
		return *content
	case ai.ThinkingContent:
		return content
	case *ai.ThinkingContent:
		if content == nil {
			return content
		}
		return *content
	case ai.ToolCall:
		return cloneToolCall(content)
	case *ai.ToolCall:
		if content == nil {
			return content
		}
		return cloneToolCall(*content)
	default:
		return content
	}
}

func cloneToolCall(toolCall ai.ToolCall) ai.ToolCall {
	toolCall.Arguments = bytes.Clone(toolCall.Arguments)
	return toolCall
}
