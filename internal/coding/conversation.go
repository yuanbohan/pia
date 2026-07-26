package coding

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/yuanbohan/pia/internal/agent"
	"github.com/yuanbohan/pia/internal/ai"
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

func (c *conversation) run(ctx context.Context, userInput string) (history []ai.Message, err error) {
	if history, err := c.beginRun(ctx); err != nil {
		return history, err
	}
	defer func() {
		history = c.finishRun()
	}()

	if err := c.compactBeforeRun(ctx, userInput); err != nil {
		return nil, err
	}

	result, runErr := c.core.Run(ctx, userInput)
	historyStart := c.appendRun(result.NewMessages)
	if runErr == nil || !c.compaction.enabled() {
		return nil, runErr
	}

	terminalOffset, ok := recoverableOverflowTerminal(result.NewMessages)
	if !ok {
		return nil, runErr
	}
	if err := c.compactAfterOverflow(ctx, historyStart+terminalOffset); err != nil {
		return nil, fmt.Errorf("coding: recover context overflow: %w", err)
	}

	continuation, continueErr := c.core.Continue(ctx)
	c.appendRun(continuation.NewMessages)
	if continueErr != nil {
		if _, overflowedAgain := recoverableOverflowTerminal(continuation.NewMessages); overflowedAgain {
			return nil, fmt.Errorf("coding: context overflow recovery exhausted: %w", continueErr)
		}
		return nil, fmt.Errorf("coding: continue after context overflow: %w", continueErr)
	}
	return nil, nil
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

func (c *conversation) appendRun(newMessages []ai.Message) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	start := len(c.history)
	// Core Agent already returned an ownership-independent delta, and this
	// owner never exposes it directly. Append without a redundant second clone.
	c.history = append(c.history, newMessages...)
	return start
}

func (c *conversation) finishRun() []ai.Message {
	c.mu.Lock()
	defer c.mu.Unlock()

	history := ai.CloneMessages(c.history)
	// Keep the guard active through every intermediate commit and this final
	// snapshot so another user advance cannot overtake recovery settlement.
	c.active = false
	return history
}

func (c *conversation) compactionSnapshot() ([]ai.Message, *compactionProjection) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var projection *compactionProjection
	if c.projection != nil {
		copy := *c.projection
		copy.Excluded = slices.Clone(c.projection.Excluded)
		projection = &copy
	}
	return ai.CloneMessages(c.history), projection
}

func (c *conversation) publishProjection(projection compactionProjection) {
	c.mu.Lock()
	defer c.mu.Unlock()
	copy := projection
	copy.Excluded = slices.Clone(projection.Excluded)
	c.projection = &copy
}
