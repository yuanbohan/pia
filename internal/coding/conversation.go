package coding

import (
	"context"
	"errors"
	"sync"

	"github.com/yuanbohan/pi-go/internal/agent"
	"github.com/yuanbohan/pi-go/internal/ai"
)

// conversation owns the complete settled history for one Coding Agent.
type conversation struct {
	mu     sync.Mutex
	active bool

	core    *agent.Agent
	history []ai.Message
}

func newConversation(core *agent.Agent) (*conversation, error) {
	if core == nil {
		return nil, errors.New("coding: core agent is required")
	}
	return &conversation{core: core}, nil
}

func (c *conversation) run(ctx context.Context, userInput string) ([]ai.Message, error) {
	if history, err := c.beginRun(ctx); err != nil {
		return history, err
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
