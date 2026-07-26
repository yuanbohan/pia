package coding

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/yuanbohan/pia/internal/agent"
	"github.com/yuanbohan/pia/internal/ai"
	"github.com/yuanbohan/pia/internal/ai/provider/deepseek"
	skillcatalog "github.com/yuanbohan/pia/internal/coding/skills"
	bashtool "github.com/yuanbohan/pia/internal/coding/tools/bash"
	edittool "github.com/yuanbohan/pia/internal/coding/tools/edit"
	readtool "github.com/yuanbohan/pia/internal/coding/tools/read"
	skilltool "github.com/yuanbohan/pia/internal/coding/tools/skill"
	writetool "github.com/yuanbohan/pia/internal/coding/tools/write"
)

const (
	productProviderName   = "deepseek"
	productModelName      = "deepseek-v4-pro"
	productReasoningLevel = "high"

	productContextCapacity = 1_000_000
	productModelMaxOutput  = 384_000
	productContextSafety   = 4_096

	productCompactionThreshold   = 192_000
	productCompactionSoftCeiling = 64_000
	productRetainedRawTarget     = 20_000
	productSummaryMaxOutput      = 13_107
	productSplitPrefixMaxOutput  = 8_192
)

// RunInput contains the one-shot coding application inputs supplied by its
// process host. APIKey is used only to construct the Provider and is never
// copied into RunResult or trace metadata.
type RunInput struct {
	WorkspacePath string
	Task          string
	APIKey        string
}

// ModelInfo is the non-credential product profile used for one coding Run.
type ModelInfo struct {
	Provider        string
	Name            string
	Thinking        bool
	ReasoningEffort string
}

// RunResult is the settled coding context retained for final projection and
// optional diagnostics. Transcript is the complete Conversation History; the
// separate Go error remains the Run outcome.
type RunResult struct {
	WorkspacePath    string
	SystemPrompt     string
	Model            ModelInfo
	Tools            []ai.ToolSchema
	SkillDiagnostics []SkillDiagnostic
	Transcript       []ai.Message
}

// Run executes the fixed Phase 1 DeepSeek coding profile.
func Run(ctx context.Context, input RunInput) (RunResult, error) {
	provider, err := deepseek.New(productDeepSeekConfig(input.APIKey))
	if err != nil {
		return RunResult{Model: productModelInfo()}, fmt.Errorf("coding: create DeepSeek Provider: %w", err)
	}
	return runWithProvider(ctx, input, provider)
}

func runWithProvider(
	ctx context.Context,
	input RunInput,
	provider ai.Provider,
) (RunResult, error) {
	return runWithWorkspaceOperations(
		ctx,
		input,
		provider,
		OpenWorkspace,
		(*Workspace).Close,
	)
}

func runWithWorkspaceOperations(
	ctx context.Context,
	input RunInput,
	provider ai.Provider,
	openWorkspace func(string) (*Workspace, error),
	closeWorkspace func(*Workspace) error,
) (result RunResult, err error) {
	result.Model = productModelInfo()
	if strings.TrimSpace(input.Task) == "" {
		return result, fmt.Errorf("coding: task is required")
	}
	if provider == nil {
		return result, fmt.Errorf("coding: Provider is required")
	}

	workspace, err := openWorkspace(input.WorkspacePath)
	if err != nil {
		return result, fmt.Errorf("coding: open workspace: %w", err)
	}
	// The workspace root is borrowed by every file tool. Close it only after
	// the Conversation advance has settled all Provider and tool work, and
	// preserve both the primary failure and any descriptor-cleanup failure for
	// diagnosis.
	defer func() {
		if closeErr := closeWorkspace(workspace); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("coding: close workspace: %w", closeErr))
		}
	}()
	result.WorkspacePath = workspace.Path()

	skillDiscovery, err := discoverPiaSkills(workspace)
	if err != nil {
		return result, err
	}
	result.SkillDiagnostics = append([]SkillDiagnostic(nil), skillDiscovery.Diagnostics...)
	tools, err := newCodingTools(workspace, skillDiscovery.Entries)
	if err != nil {
		return result, err
	}
	result.Tools = toolSchemas(tools)
	prompt, err := buildSystemPrompt(workspace, tools, skillDiscovery.Catalog)
	if err != nil {
		return result, err
	}
	result.SystemPrompt = prompt

	requestLimits := productRequestLimits()
	core, err := agent.New(agent.Config{
		Provider:      provider,
		SystemPrompt:  prompt,
		Tools:         tools,
		RequestLimits: requestLimits,
	})
	if err != nil {
		return result, fmt.Errorf("coding: create Agent: %w", err)
	}
	conversation, err := newConversation(conversationConfig{
		Core:          core,
		Provider:      provider,
		SystemPrompt:  prompt,
		Tools:         result.Tools,
		RequestLimits: requestLimits,
		Compaction:    productCompactionPolicy(),
	})
	if err != nil {
		return result, err
	}
	history, runErr := conversation.run(ctx, input.Task)
	result.Transcript = history
	if runErr != nil {
		return result, fmt.Errorf("coding: run Agent: %w", runErr)
	}
	return result, nil
}

func productRequestLimits() ai.RequestLimits {
	return ai.RequestLimits{
		ContextCapacity: productContextCapacity,
		ModelMaxOutput:  productModelMaxOutput,
		ContextSafety:   productContextSafety,
	}
}

func productCompactionPolicy() compactionPolicy {
	return compactionPolicy{
		Threshold:                productCompactionThreshold,
		SoftCeiling:              productCompactionSoftCeiling,
		RetainedRawTarget:        productRetainedRawTarget,
		SummaryMaxOutput:         productSummaryMaxOutput,
		SplitTurnPrefixMaxOutput: productSplitPrefixMaxOutput,
	}
}

func newCodingTools(workspace *Workspace, skillEntries []skillcatalog.Entry) ([]agent.Tool, error) {
	read, err := readtool.New(workspace.Root())
	if err != nil {
		return nil, fmt.Errorf("coding: create read tool: %w", err)
	}
	bash, err := bashtool.New(bashtool.Config{WorkingDirectory: workspace.Path()})
	if err != nil {
		return nil, fmt.Errorf("coding: create bash tool: %w", err)
	}
	edit, err := edittool.New(workspace.Root())
	if err != nil {
		return nil, fmt.Errorf("coding: create edit tool: %w", err)
	}
	write, err := writetool.New(workspace.Root())
	if err != nil {
		return nil, fmt.Errorf("coding: create write tool: %w", err)
	}
	// Preserve the frozen Pi tools' relative order. A project-local activation
	// tool is appended only when the same snapshot disclosed at least one Skill.
	tools := []agent.Tool{read, bash, edit, write}
	if len(skillEntries) == 0 {
		return tools, nil
	}
	skill, err := skilltool.New(workspace.Root(), skillEntries)
	if err != nil {
		return nil, fmt.Errorf("coding: create skill tool: %w", err)
	}
	return append(tools, skill), nil
}

func toolSchemas(tools []agent.Tool) []ai.ToolSchema {
	schemas := make([]ai.ToolSchema, len(tools))
	for index, tool := range tools {
		schema := tool.Definition().Schema
		schema.Parameters = bytes.Clone(schema.Parameters)
		schemas[index] = schema
	}
	return schemas
}

func productDeepSeekConfig(apiKey string) deepseek.Config {
	// The temporary one-shot command deliberately has one product profile. Keep
	// model selection out of the CLI until a real configuration consumer exists.
	return deepseek.Config{
		APIKey:          apiKey,
		Model:           productModelName,
		ReasoningEffort: productReasoningLevel,
	}
}

func productModelInfo() ModelInfo {
	return ModelInfo{
		Provider:        productProviderName,
		Name:            productModelName,
		Thinking:        true,
		ReasoningEffort: productReasoningLevel,
	}
}

// FinalText concatenates text blocks from the last assistant message without
// exposing reasoning, tool calls, tool results, or earlier assistant text.
func (r RunResult) FinalText() string {
	for index := len(r.Transcript) - 1; index >= 0; index-- {
		var message ai.AssistantMessage
		switch candidate := r.Transcript[index].(type) {
		case ai.AssistantMessage:
			message = candidate
		case *ai.AssistantMessage:
			if candidate == nil {
				continue
			}
			message = *candidate
		default:
			continue
		}

		var final strings.Builder
		for _, content := range message.Content {
			switch content := content.(type) {
			case ai.TextContent:
				final.WriteString(content.Text)
			case *ai.TextContent:
				if content != nil {
					final.WriteString(content.Text)
				}
			}
		}
		return final.String()
	}
	return ""
}
