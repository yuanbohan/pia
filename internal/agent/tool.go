package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/yuanbohan/pia/internal/ai"
	"github.com/yuanbohan/pia/internal/observation"
)

// ToolDefinition is the stable model schema and scheduling capability exposed
// by one Tool when an Agent is constructed.
type ToolDefinition struct {
	Schema ai.ToolSchema
	// CanRunParallel promises that concurrent Execute calls on this Tool
	// instance are safe. It defaults to false.
	CanRunParallel bool
}

// Tool describes, validates, and executes one raw model-requested invocation.
// DescribeInvocation must be side-effect-free and return only a concise,
// operator-safe summary; it runs synchronously on the coordinator before
// Execute only when an observer is installed. Call-local Execute failures
// become model-visible tool results and do not fail the surrounding Run.
// Execute must honor ctx and, when CanRunParallel is true, support concurrent
// calls on the same instance.
type Tool interface {
	Definition() ToolDefinition
	DescribeInvocation(arguments json.RawMessage) string
	Execute(ctx context.Context, arguments json.RawMessage) (string, error)
}

type registeredTool struct {
	tool           Tool
	canRunParallel bool
}

func freezeTools(tools []Tool) (map[string]registeredTool, []ai.ToolSchema, error) {
	registered := make(map[string]registeredTool, len(tools))
	var schemas []ai.ToolSchema
	if len(tools) > 0 {
		schemas = make([]ai.ToolSchema, 0, len(tools))
	}
	for index, tool := range tools {
		if isNilTool(tool) {
			return nil, nil, fmt.Errorf("agent: tool %d is nil", index)
		}

		definition := tool.Definition()
		name := definition.Schema.Name
		if strings.TrimSpace(name) == "" {
			return nil, nil, fmt.Errorf("agent: tool %d name is required", index)
		}
		if _, exists := registered[name]; exists {
			return nil, nil, fmt.Errorf("agent: duplicate tool name %q", name)
		}
		if !json.Valid(definition.Schema.Parameters) {
			return nil, nil, fmt.Errorf("agent: tool %q has invalid parameters schema", name)
		}

		schema := definition.Schema
		schema.Parameters = bytes.Clone(definition.Schema.Parameters)
		registered[name] = registeredTool{
			tool:           tool,
			canRunParallel: definition.CanRunParallel,
		}
		schemas = append(schemas, schema)
	}
	return registered, schemas, nil
}

func isNilTool(tool Tool) bool {
	if tool == nil {
		return true
	}
	value := reflect.ValueOf(tool)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

type toolStage struct {
	start    int
	end      int
	parallel bool
}

type toolOutcome struct {
	result   ai.ToolResultMessage
	assigned bool
}

type toolDisplay struct {
	name    string
	summary string
}

type completedTool struct {
	index  int
	result ai.ToolResultMessage
}

func (e *Engine) executeToolBatch(ctx context.Context, calls []ai.ToolCall) ([]ai.ToolResultMessage, error) {
	stages := e.toolStages(calls)
	outcomes := make([]toolOutcome, len(calls))
	displays := make([]toolDisplay, len(calls))

	for _, stage := range stages {
		if cause := context.Cause(ctx); cause != nil {
			settleNotExecuted(calls, outcomes, cause)
			return orderedToolResults(outcomes), cause
		}

		if stage.parallel {
			completed := make(chan completedTool, stage.end-stage.start)
			for index := stage.start; index < stage.end; index++ {
				displays[index] = e.observeToolStarted(index, calls[index])
				index := index
				go func() {
					completed <- completedTool{
						index:  index,
						result: e.executeToolCall(ctx, calls[index]),
					}
				}()
			}
			for range stage.end - stage.start {
				settled := <-completed
				outcomes[settled.index] = toolOutcome{
					result:   settled.result,
					assigned: true,
				}
				e.observeToolSettled(
					settled.index,
					displays[settled.index],
					settled.result,
				)
			}
		} else {
			index := stage.start
			displays[index] = e.observeToolStarted(index, calls[index])
			outcomes[index] = toolOutcome{
				result:   e.executeToolCall(ctx, calls[index]),
				assigned: true,
			}
			e.observeToolSettled(index, displays[index], outcomes[index].result)
		}

		if cause := context.Cause(ctx); cause != nil {
			settleNotExecuted(calls, outcomes, cause)
			return orderedToolResults(outcomes), cause
		}
	}

	return orderedToolResults(outcomes), nil
}

func (e *Engine) observeToolStarted(index int, call ai.ToolCall) toolDisplay {
	if e.observer == nil {
		return toolDisplay{}
	}

	display := toolDisplay{name: call.Name, summary: call.Name}
	registered, exists := e.tools[call.Name]
	if exists {
		if summary := registered.tool.DescribeInvocation(bytes.Clone(call.Arguments)); summary != "" {
			display.summary = summary
		}
	}
	e.observer.Observe(observation.NewToolStarted(
		index,
		display.name,
		display.summary,
	))
	return display
}

func (e *Engine) observeToolSettled(
	index int,
	display toolDisplay,
	result ai.ToolResultMessage,
) {
	if e.observer == nil {
		return
	}

	outcome := observation.OutcomeSuccess
	if result.IsError {
		outcome = observation.OutcomeError
	}
	e.observer.Observe(observation.NewToolSettled(
		index,
		display.name,
		display.summary,
		outcome,
	))
}

func (e *Engine) toolStages(calls []ai.ToolCall) []toolStage {
	stages := make([]toolStage, 0, len(calls))
	for index := 0; index < len(calls); {
		registered, exists := e.tools[calls[index].Name]
		if !exists || !registered.canRunParallel {
			stages = append(stages, toolStage{start: index, end: index + 1})
			index++
			continue
		}

		end := index + 1
		for end < len(calls) {
			next, exists := e.tools[calls[end].Name]
			if !exists || !next.canRunParallel {
				break
			}
			end++
		}
		stages = append(stages, toolStage{start: index, end: end, parallel: true})
		index = end
	}
	return stages
}

func (e *Engine) executeToolCall(ctx context.Context, call ai.ToolCall) ai.ToolResultMessage {
	if strings.TrimSpace(call.Name) == "" {
		return toolErrorResult(call, fmt.Errorf("agent: tool call %q has an empty name", call.ID))
	}
	registered, exists := e.tools[call.Name]
	if !exists {
		return toolErrorResult(call, fmt.Errorf("agent: unknown tool %q", call.Name))
	}

	content, err := registered.tool.Execute(ctx, call.Arguments)
	if err != nil {
		return ai.ToolResultMessage{
			ToolCallID: call.ID,
			ToolName:   call.Name,
			Content:    toolFailureContent(content, err),
			IsError:    true,
		}
	}
	return ai.ToolResultMessage{
		ToolCallID: call.ID,
		ToolName:   call.Name,
		Content:    content,
	}
}

func toolErrorResult(call ai.ToolCall, err error) ai.ToolResultMessage {
	return ai.ToolResultMessage{
		ToolCallID: call.ID,
		ToolName:   call.Name,
		Content:    err.Error(),
		IsError:    true,
	}
}

func toolFailureContent(content string, err error) string {
	if content == "" {
		return err.Error()
	}
	return content + "\n\nError: " + err.Error()
}

func settleNotExecuted(calls []ai.ToolCall, outcomes []toolOutcome, cause error) {
	for index, outcome := range outcomes {
		if outcome.assigned {
			continue
		}
		outcomes[index] = toolOutcome{
			result: notExecutedToolResult(
				calls[index],
				fmt.Sprintf("the Run was canceled before it started: %v", cause),
			),
			assigned: true,
		}
	}
}

func failedTurnToolResults(calls []ai.ToolCall, reason ai.StopReason) []ai.ToolResultMessage {
	results := make([]ai.ToolResultMessage, len(calls))
	for index, call := range calls {
		results[index] = notExecutedToolResult(
			call,
			fmt.Sprintf("the Provider turn ended with stop reason %q before tool execution", reason),
		)
	}
	return results
}

func notExecutedToolResult(call ai.ToolCall, reason string) ai.ToolResultMessage {
	return ai.ToolResultMessage{
		ToolCallID: call.ID,
		ToolName:   call.Name,
		Content:    "tool call was not executed because " + reason,
		IsError:    true,
	}
}

func orderedToolResults(outcomes []toolOutcome) []ai.ToolResultMessage {
	results := make([]ai.ToolResultMessage, len(outcomes))
	for index, outcome := range outcomes {
		results[index] = outcome.result
	}
	return results
}

func truncatedToolResults(calls []ai.ToolCall) []ai.ToolResultMessage {
	results := make([]ai.ToolResultMessage, len(calls))
	for index, call := range calls {
		results[index] = ai.ToolResultMessage{
			ToolCallID: call.ID,
			ToolName:   call.Name,
			Content:    "tool call was not executed because the assistant response was truncated",
			IsError:    true,
		}
	}
	return results
}
