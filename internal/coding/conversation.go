package coding

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/yuanbohan/pi-go/internal/agent"
	"github.com/yuanbohan/pi-go/internal/ai"
)

type conversationConfig struct {
	Core          *agent.Agent
	Provider      ai.Provider
	SystemPrompt  string
	Tools         []ai.ToolSchema
	RequestLimits ai.RequestLimits
	Compaction    compactionPolicy
}

// conversation owns the complete settled history and the replaceable
// compaction projection for one Coding Agent.
type conversation struct {
	mu     sync.Mutex
	active bool

	core          *agent.Agent
	provider      ai.Provider
	systemPrompt  string
	tools         []ai.ToolSchema
	requestLimits ai.RequestLimits
	compaction    compactionPolicy

	history    []ai.Message
	projection *compactionProjection
}

func newConversation(config conversationConfig) (*conversation, error) {
	if config.Core == nil {
		return nil, errors.New("coding: core agent is required")
	}
	if err := config.Compaction.validate(); err != nil {
		return nil, fmt.Errorf("coding: compaction policy: %w", err)
	}
	if config.Compaction.enabled() {
		if config.Provider == nil {
			return nil, errors.New("coding: compaction Provider is required")
		}
		if config.RequestLimits.IsZero() {
			return nil, errors.New("coding: compaction request limits are required")
		}
		if err := config.RequestLimits.Validate(); err != nil {
			return nil, fmt.Errorf("coding: compaction request limits: %w", err)
		}
	}
	tools := ai.CloneToolSchemas(config.Tools)
	return &conversation{
		core:          config.Core,
		provider:      config.Provider,
		systemPrompt:  config.SystemPrompt,
		tools:         tools,
		requestLimits: config.RequestLimits,
		compaction:    config.Compaction,
	}, nil
}

func (c *conversation) run(ctx context.Context, userInput string) ([]ai.Message, error) {
	if history, err := c.beginRun(ctx); err != nil {
		return history, err
	}

	if err := c.compactBeforeRun(ctx, userInput); err != nil {
		return c.rejectRun(), err
	}

	result, runErr := c.core.Run(ctx, userInput)
	return c.commitRun(result.NewMessages), runErr
}

func (c *conversation) beginRun(ctx context.Context) ([]ai.Message, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.active {
		return ai.CloneMessages(c.history), agent.ErrRunActive
	}
	if cause := context.Cause(ctx); cause != nil {
		return ai.CloneMessages(c.history), cause
	}
	c.active = true
	return nil, nil
}

func (c *conversation) rejectRun() []ai.Message {
	c.mu.Lock()
	defer c.mu.Unlock()

	history := ai.CloneMessages(c.history)
	c.active = false
	return history
}

func (c *conversation) commitRun(newMessages []ai.Message) []ai.Message {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Core Agent already returned an ownership-independent delta, and this
	// owner never exposes it directly. Append without a redundant second clone.
	c.history = append(c.history, newMessages...)
	history := ai.CloneMessages(c.history)
	// Keep the guard active through both commit and snapshot so another Run
	// cannot overtake this settlement in the complete history.
	c.active = false
	return history
}

func (c *conversation) compactionSnapshot() ([]ai.Message, *compactionProjection) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var projection *compactionProjection
	if c.projection != nil {
		copy := *c.projection
		projection = &copy
	}
	return ai.CloneMessages(c.history), projection
}

func (c *conversation) publishProjection(projection compactionProjection) {
	c.mu.Lock()
	defer c.mu.Unlock()
	copy := projection
	c.projection = &copy
}
