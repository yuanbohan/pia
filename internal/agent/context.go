package agent

import "github.com/yuanbohan/pia/internal/ai"

type execution struct {
	engine         *Engine
	workingContext []ai.Message
}

func newExecution(engine *Engine, workingContext []ai.Message) *execution {
	return &execution{
		engine:         engine,
		workingContext: ai.CloneMessages(workingContext),
	}
}

func (e *execution) requestSnapshot() ai.Request {
	request := ai.Request{
		SystemPrompt: e.engine.systemPrompt,
		Messages:     e.workingContext,
		Tools:        e.engine.toolSchemas,
	}
	if !e.engine.requestLimits.IsZero() {
		projectedInput := ai.EstimateRequestTokens(request).Tokens
		request.MaxOutputTokens = e.engine.requestLimits.ClampOutputTokens(
			projectedInput,
			e.engine.requestLimits.ModelMaxOutput,
		)
	}
	return ai.CloneRequest(request)
}

func (e *execution) appendAssistant(message ai.AssistantMessage) {
	e.workingContext = append(e.workingContext, ai.CloneAssistantMessage(message))
}

func (e *execution) appendToolResults(results []ai.ToolResultMessage) {
	for _, result := range results {
		e.workingContext = append(e.workingContext, result)
	}
}

func (e *execution) snapshotRun(start int) RunResult {
	return RunResult{NewMessages: ai.CloneMessages(e.workingContext[start:])}
}
