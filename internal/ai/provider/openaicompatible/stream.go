package openaicompatible

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/yuanbohan/pi-go/internal/ai"
)

const maxSSEEventBytes = 1 << 20

var _ ai.Stream = (*responseStream)(nil)

func (stream *responseStream) Receive() (ai.Event, error) {
	if stream.terminal {
		return nil, io.EOF
	}
	if !stream.started {
		stream.started = true
		return stream.start()
	}
	if len(stream.queue) > 0 {
		return stream.dequeue(), nil
	}
	return stream.receiveSSE()
}

func (stream *responseStream) receiveSSE() (ai.Event, error) {
	if stream.sse == nil {
		stream.sse = newSSEReader(stream.response.Body)
	}

	for {
		if cause := context.Cause(stream.ctx); cause != nil {
			return stream.fail(cause)
		}

		data, err := stream.sse.readData()
		if err != nil {
			if errors.Is(err, io.EOF) {
				if stream.hasFinishReason() {
					return stream.fail(fmt.Errorf("openai-compatible: stream ended before [DONE]"))
				}
				return stream.fail(fmt.Errorf("openai-compatible: stream ended before finish_reason and [DONE]"))
			}
			return stream.fail(fmt.Errorf("openai-compatible: read SSE event: %w", err))
		}

		if strings.TrimSpace(data) == "[DONE]" {
			if !stream.hasFinishReason() {
				return stream.fail(fmt.Errorf("openai-compatible: received [DONE] before finish_reason"))
			}
			return stream.complete()
		}

		if err := stream.applySSEData(data); err != nil {
			return stream.fail(err)
		}
		if len(stream.queue) > 0 {
			return stream.dequeue(), nil
		}
	}
}

func (stream *responseStream) applySSEData(data string) error {
	var chunk responseChunk
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return fmt.Errorf("openai-compatible: decode SSE data: %w", err)
	}
	if chunk.Error != nil {
		message := strings.TrimSpace(chunk.Error.Message)
		if message == "" {
			message = "unspecified provider error"
		}
		return fmt.Errorf("openai-compatible: provider stream error: %s", message)
	}
	if chunk.Usage != nil {
		if chunk.Usage.PromptTokens < 0 || chunk.Usage.CompletionTokens < 0 {
			return fmt.Errorf("openai-compatible: usage token counts must not be negative")
		}
		stream.usage = ai.Usage{
			InputTokens:  chunk.Usage.PromptTokens,
			OutputTokens: chunk.Usage.CompletionTokens,
		}
	}

	if len(chunk.Choices) == 0 {
		return nil
	}
	if len(chunk.Choices) != 1 {
		return fmt.Errorf("openai-compatible: SSE chunk contains %d choices, want 1", len(chunk.Choices))
	}
	if stream.hasFinishReason() {
		return fmt.Errorf("openai-compatible: received another choice after finish_reason")
	}

	choice := chunk.Choices[0]
	if choice.Index == nil || *choice.Index != 0 {
		return fmt.Errorf("openai-compatible: choice index is missing or is not 0")
	}
	if choice.Delta.Content != nil {
		stream.appendText(contentText, *choice.Delta.Content)
	}
	if choice.Delta.ReasoningContent != nil {
		stream.appendText(contentThinking, *choice.Delta.ReasoningContent)
	}
	for _, call := range choice.Delta.ToolCalls {
		if err := stream.appendToolCall(call); err != nil {
			return err
		}
	}
	if choice.FinishReason != nil {
		if err := stream.setFinishReason(*choice.FinishReason); err != nil {
			return err
		}
	}

	return nil
}

func (stream *responseStream) appendText(kind contentKind, delta string) {
	// DeepSeek uses empty content/reasoning deltas in role-only and finish chunks.
	// They carry no model content, so creating a block would pollute the transcript
	// and shift the content indexes of later reasoning or tool calls.
	if delta == "" {
		return
	}

	content := stream.findContent(kind)
	if content == nil {
		content = &contentState{
			kind:  kind,
			index: len(stream.content),
		}
		stream.content = append(stream.content, content)
		switch kind {
		case contentText:
			stream.queue = append(stream.queue, ai.TextStartEvent{ContentIndex: content.index})
		case contentThinking:
			stream.queue = append(stream.queue, ai.ThinkingStartEvent{ContentIndex: content.index})
		}
	}

	content.value.WriteString(delta)
	switch kind {
	case contentText:
		stream.queue = append(stream.queue, ai.TextDeltaEvent{
			ContentIndex: content.index,
			Delta:        delta,
		})
	case contentThinking:
		stream.queue = append(stream.queue, ai.ThinkingDeltaEvent{
			ContentIndex: content.index,
			Delta:        delta,
		})
	}
}

func (stream *responseStream) findContent(kind contentKind) *contentState {
	for _, content := range stream.content {
		if content.kind == kind {
			return content
		}
	}
	return nil
}

func (stream *responseStream) appendToolCall(delta responseToolCallDelta) error {
	if delta.Index == nil || *delta.Index < 0 {
		return fmt.Errorf("openai-compatible: tool call index is missing or negative")
	}
	if delta.Type != "" && delta.Type != "function" {
		return fmt.Errorf("openai-compatible: tool call %d has unsupported type %q", *delta.Index, delta.Type)
	}

	if stream.tools == nil {
		stream.tools = make(map[int]*toolCallState)
	}
	call := stream.tools[*delta.Index]
	if call == nil {
		content := &contentState{
			kind:  contentToolCall,
			index: len(stream.content),
		}
		call = &toolCallState{
			wireIndex: *delta.Index,
			content:   content,
		}
		content.toolCall = call
		stream.tools[*delta.Index] = call
		stream.content = append(stream.content, content)
	}

	if delta.ID != "" {
		if call.id != "" && call.id != delta.ID {
			return fmt.Errorf(
				"openai-compatible: tool call %d changed ID from %q to %q",
				call.wireIndex,
				call.id,
				delta.ID,
			)
		}
		call.id = delta.ID
	}
	if delta.Function.Name != "" {
		if call.name != "" && call.name != delta.Function.Name {
			return fmt.Errorf(
				"openai-compatible: tool call %d changed name from %q to %q",
				call.wireIndex,
				call.name,
				delta.Function.Name,
			)
		}
		call.name = delta.Function.Name
	}
	call.arguments.WriteString(delta.Function.Arguments)

	if !call.started && call.id != "" && call.name != "" {
		call.started = true
		stream.queue = append(stream.queue, ai.ToolCallStartEvent{
			ContentIndex: call.content.index,
			ID:           call.id,
			Name:         call.name,
		})
		if call.arguments.Len() > 0 {
			stream.queue = append(stream.queue, ai.ToolCallDeltaEvent{
				ContentIndex: call.content.index,
				Delta:        call.arguments.String(),
			})
		}
	} else if call.started && delta.Function.Arguments != "" {
		stream.queue = append(stream.queue, ai.ToolCallDeltaEvent{
			ContentIndex: call.content.index,
			Delta:        delta.Function.Arguments,
		})
	}

	return nil
}

func (stream *responseStream) setFinishReason(reason string) error {
	var stopReason ai.StopReason
	switch reason {
	case "stop":
		stopReason = ai.StopReasonStop
	case "length":
		stopReason = ai.StopReasonLength
	case "tool_calls":
		stopReason = ai.StopReasonToolUse
	default:
		return fmt.Errorf("openai-compatible: unsupported finish_reason %q", reason)
	}

	for _, content := range stream.content {
		if content.kind != contentToolCall {
			continue
		}
		call := content.toolCall
		if strings.TrimSpace(call.id) == "" || strings.TrimSpace(call.name) == "" {
			return fmt.Errorf(
				"openai-compatible: tool call %d is missing ID or name at finish_reason",
				call.wireIndex,
			)
		}
	}

	stream.finishReason = stopReason
	return nil
}

func (stream *responseStream) hasFinishReason() bool {
	return stream.finishReason != ""
}

func (stream *responseStream) complete() (ai.Event, error) {
	stream.closeResponse()
	for _, content := range stream.content {
		switch content.kind {
		case contentText:
			stream.queue = append(stream.queue, ai.TextEndEvent{
				ContentIndex: content.index,
				Text:         content.value.String(),
			})
		case contentThinking:
			stream.queue = append(stream.queue, ai.ThinkingEndEvent{
				ContentIndex: content.index,
				Thinking:     content.value.String(),
			})
		case contentToolCall:
			stream.queue = append(stream.queue, ai.ToolCallEndEvent{
				ContentIndex: content.index,
				ToolCall:     content.toolCall.value(),
			})
		}
	}
	stream.queue = append(stream.queue, ai.DoneEvent{
		Message: stream.terminalMessage(stream.finishReason, true),
	})
	return stream.dequeue(), nil
}

func (stream *responseStream) dequeue() ai.Event {
	event := stream.queue[0]
	stream.queue[0] = nil
	stream.queue = stream.queue[1:]
	if _, ok := event.(ai.DoneEvent); ok {
		stream.terminal = true
	}
	return event
}

func (stream *responseStream) terminalMessage(stopReason ai.StopReason, includeToolCalls bool) ai.AssistantMessage {
	var content []ai.AssistantContent
	for _, state := range stream.content {
		switch state.kind {
		case contentText:
			content = append(content, ai.TextContent{Text: state.value.String()})
		case contentThinking:
			content = append(content, ai.ThinkingContent{Thinking: state.value.String()})
		case contentToolCall:
			if includeToolCalls {
				content = append(content, state.toolCall.value())
			}
		}
	}
	return ai.AssistantMessage{
		Content:    content,
		Usage:      stream.usage,
		StopReason: stopReason,
	}
}

type contentKind uint8

const (
	contentText contentKind = iota
	contentThinking
	contentToolCall
)

type contentState struct {
	kind     contentKind
	index    int
	value    strings.Builder
	toolCall *toolCallState
}

type toolCallState struct {
	wireIndex int
	content   *contentState
	id        string
	name      string
	arguments strings.Builder
	started   bool
}

func (call *toolCallState) value() ai.ToolCall {
	return ai.ToolCall{
		ID:        call.id,
		Name:      call.name,
		Arguments: json.RawMessage(call.arguments.String()),
	}
}

type responseChunk struct {
	Choices []responseChoice `json:"choices"`
	Usage   *responseUsage   `json:"usage"`
	Error   *responseError   `json:"error"`
}

type responseChoice struct {
	Index        *int          `json:"index"`
	Delta        responseDelta `json:"delta"`
	FinishReason *string       `json:"finish_reason"`
}

type responseDelta struct {
	Content          *string                 `json:"content"`
	ReasoningContent *string                 `json:"reasoning_content"`
	ToolCalls        []responseToolCallDelta `json:"tool_calls"`
}

type responseToolCallDelta struct {
	Index    *int                 `json:"index"`
	ID       string               `json:"id"`
	Type     string               `json:"type"`
	Function responseToolFunction `json:"function"`
}

type responseToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type responseUsage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
}

type responseError struct {
	Message string `json:"message"`
}

type sseReader struct {
	reader    *bufio.Reader
	firstLine bool
}

func newSSEReader(reader io.Reader) *sseReader {
	return &sseReader{
		reader:    bufio.NewReaderSize(reader, 64<<10),
		firstLine: true,
	}
}

func (reader *sseReader) readData() (string, error) {
	for {
		var data []string
		eventBytes := 0

		for {
			line, eof, err := reader.readLine(maxSSEEventBytes - eventBytes)
			if err != nil {
				return "", err
			}
			eventBytes += len(line) + 1
			if eventBytes > maxSSEEventBytes {
				return "", fmt.Errorf("SSE event exceeds %d bytes", maxSSEEventBytes)
			}

			if reader.firstLine {
				line = bytes.TrimPrefix(line, []byte{0xef, 0xbb, 0xbf})
				reader.firstLine = false
			}

			if len(line) == 0 {
				if len(data) > 0 {
					return strings.Join(data, "\n"), nil
				}
				if eof {
					return "", io.EOF
				}
				break
			}

			if line[0] != ':' {
				field, value, found := strings.Cut(string(line), ":")
				if !found {
					value = ""
				}
				if strings.HasPrefix(value, " ") {
					value = value[1:]
				}
				if field == "data" {
					data = append(data, value)
				}
			}

			if eof {
				if len(data) > 0 {
					return strings.Join(data, "\n"), nil
				}
				return "", io.EOF
			}
		}
	}
}

func (reader *sseReader) readLine(remaining int) ([]byte, bool, error) {
	if remaining <= 0 {
		return nil, false, fmt.Errorf("SSE event exceeds %d bytes", maxSSEEventBytes)
	}

	line := make([]byte, 0)
	for {
		fragment, prefix, err := reader.reader.ReadLine()
		if len(line)+len(fragment) > remaining {
			return nil, false, fmt.Errorf("SSE event exceeds %d bytes", maxSSEEventBytes)
		}
		line = append(line, fragment...)
		if err != nil {
			if errors.Is(err, io.EOF) {
				if len(line) == 0 {
					return nil, true, io.EOF
				}
				return line, true, nil
			}
			return nil, false, err
		}
		if !prefix {
			return line, false, nil
		}
	}
}
