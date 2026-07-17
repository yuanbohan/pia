package ai

import (
	"encoding/json"
)

// Request is the complete, model-visible snapshot for one Provider call.
type Request struct {
	SystemPrompt string
	Messages     []Message
	Tools        []ToolSchema
}

// ToolSchema describes a tool to the model. Runtime validation belongs to the
// concrete Agent tool, not to this model-visible JSON Schema.
type ToolSchema struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}
