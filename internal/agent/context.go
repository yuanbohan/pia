package agent

import "github.com/yuanbohan/pia/internal/ai"

// ReplaceWorkingContext atomically installs the messages used by the next Run
// or Continue execution.
func (a *Agent) ReplaceWorkingContext(messages []ai.Message) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Replacing context between Turns would make one execution observe two
	// unrelated histories. The caller must coordinate replacement after the
	// active Run or Continue settles.
	if a.active {
		return ErrRunActive
	}
	a.workingContext = ai.CloneMessages(messages)
	return nil
}

func (a *Agent) requestSnapshot() ai.Request {
	a.mu.Lock()
	defer a.mu.Unlock()
	request := ai.Request{
		SystemPrompt: a.systemPrompt,
		Messages:     a.workingContext,
		Tools:        a.toolSchemas,
	}
	if !a.requestLimits.IsZero() {
		projectedInput := ai.EstimateRequestTokens(request).Tokens
		request.MaxOutputTokens = a.requestLimits.ClampOutputTokens(
			projectedInput,
			a.requestLimits.ModelMaxOutput,
		)
	}
	return ai.CloneRequest(request)
}

func (a *Agent) appendAssistant(message ai.AssistantMessage) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.workingContext = append(a.workingContext, ai.CloneAssistantMessage(message))
}

func (a *Agent) appendToolResults(results []ai.ToolResultMessage) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, result := range results {
		a.workingContext = append(a.workingContext, result)
	}
}

func (a *Agent) snapshotRun(start int) RunResult {
	a.mu.Lock()
	defer a.mu.Unlock()
	return RunResult{NewMessages: ai.CloneMessages(a.workingContext[start:])}
}
