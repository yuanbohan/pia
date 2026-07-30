// Package agent owns the run-local model and tool execution loop.
package agent

import (
	"errors"
	"fmt"

	"github.com/yuanbohan/pia/internal/ai"
	"github.com/yuanbohan/pia/internal/observation"
)

// Config contains the stable dependencies of one Agent loop.
type Config struct {
	Provider      ai.Provider
	SystemPrompt  string
	Tools         []Tool
	RequestLimits ai.RequestLimits
	Observer      observation.Observer
}

// RunResult contains the ownership-independent messages accepted by one Agent
// Loop execution.
type RunResult struct {
	NewMessages []ai.Message
}

// SteeringSource supplies user inputs accepted for one Engine invocation.
// Drain keeps admission open. DrainOrSeal returns the final available batch or
// seals admission when the invocation should stop.
type SteeringSource interface {
	Drain() []string
	DrainOrSeal() []string
}

// Engine owns immutable dependencies shared by run-local Agent Loop
// executions. Its caller owns and serializes Conversation state.
type Engine struct {
	provider      ai.Provider
	systemPrompt  string
	tools         map[string]registeredTool
	toolSchemas   []ai.ToolSchema
	requestLimits ai.RequestLimits
	observer      observation.Observer
}

// New constructs an Engine from stable Provider and tool dependencies.
func New(config Config) (*Engine, error) {
	if config.Provider == nil {
		return nil, errors.New("agent: provider is required")
	}
	if err := config.RequestLimits.Validate(); err != nil {
		return nil, fmt.Errorf("agent: request limits: %w", err)
	}
	tools, toolSchemas, err := freezeTools(config.Tools)
	if err != nil {
		return nil, err
	}
	return &Engine{
		provider:      config.Provider,
		systemPrompt:  config.SystemPrompt,
		tools:         tools,
		toolSchemas:   toolSchemas,
		requestLimits: config.RequestLimits,
		observer:      config.Observer,
	}, nil
}
