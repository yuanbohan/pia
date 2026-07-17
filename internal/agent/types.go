// Package agent owns the stateful model and tool execution loop.
package agent

import (
	"errors"
	"sync"

	"github.com/yuanbohan/pi-go/internal/ai"
)

// ErrRunActive means another Run already owns this Agent's transcript.
var ErrRunActive = errors.New("agent: run already active")

// Config contains the stable dependencies of one Agent conversation.
type Config struct {
	Provider     ai.Provider
	SystemPrompt string
	Tools        []Tool
}

// RunResult is an ownership-independent snapshot taken when Run returns.
type RunResult struct {
	Transcript []ai.Message
}

// Agent owns one conversation's ordered in-memory transcript.
type Agent struct {
	mu     sync.Mutex
	active bool

	provider     ai.Provider
	systemPrompt string
	tools        map[string]registeredTool
	toolSchemas  []ai.ToolSchema
	transcript   []ai.Message
}

// New constructs an empty Agent conversation.
func New(config Config) (*Agent, error) {
	if config.Provider == nil {
		return nil, errors.New("agent: provider is required")
	}
	tools, toolSchemas, err := freezeTools(config.Tools)
	if err != nil {
		return nil, err
	}
	return &Agent{
		provider:     config.Provider,
		systemPrompt: config.SystemPrompt,
		tools:        tools,
		toolSchemas:  toolSchemas,
	}, nil
}
