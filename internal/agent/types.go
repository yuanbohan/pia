// Package agent owns the stateful model and tool execution loop.
package agent

import (
	"errors"
	"fmt"
	"sync"

	"github.com/yuanbohan/pi-go/internal/ai"
)

// ErrRunActive means another Run already owns this Agent's Working Context.
var ErrRunActive = errors.New("agent: run already active")

// Config contains the stable dependencies of one Agent loop.
type Config struct {
	Provider      ai.Provider
	SystemPrompt  string
	Tools         []Tool
	RequestLimits ai.RequestLimits
}

// RunResult contains the ownership-independent messages accepted by one Run.
type RunResult struct {
	NewMessages []ai.Message
}

// Agent owns the replaceable Working Context used by the model and tool loop.
type Agent struct {
	mu     sync.Mutex
	active bool

	provider       ai.Provider
	systemPrompt   string
	tools          map[string]registeredTool
	toolSchemas    []ai.ToolSchema
	workingContext []ai.Message
	requestLimits  ai.RequestLimits
}

// New constructs an Agent with an empty Working Context.
func New(config Config) (*Agent, error) {
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
	return &Agent{
		provider:      config.Provider,
		systemPrompt:  config.SystemPrompt,
		tools:         tools,
		toolSchemas:   toolSchemas,
		requestLimits: config.RequestLimits,
	}, nil
}
