package coding

import (
	"bytes"
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

// ModelInfo is the non-credential product profile used by one Session.
type ModelInfo struct {
	Provider        string
	Name            string
	Thinking        bool
	ReasoningEffort string
}

// NewSession constructs the fixed DeepSeek coding profile for one Workspace.
func NewSession(config SessionConfig) (*Session, error) {
	if strings.TrimSpace(config.WorkspacePath) == "" {
		return nil, errors.New("coding: workspace path is required")
	}
	provider, err := deepseek.New(productDeepSeekConfig(config.DeepSeekAPIKey))
	if err != nil {
		return nil, fmt.Errorf("coding: create DeepSeek Provider: %w", err)
	}
	return newSessionWithWorkspaceOperations(
		config,
		provider,
		OpenWorkspace,
		(*Workspace).Close,
	)
}

func newSessionWithWorkspaceOperations(
	config SessionConfig,
	provider ai.Provider,
	openWorkspace func(string) (*Workspace, error),
	closeWorkspace func(*Workspace) error,
) (session *Session, err error) {
	if provider == nil {
		return nil, errors.New("coding: Provider is required")
	}

	workspace, err := openWorkspace(config.WorkspacePath)
	if err != nil {
		return nil, fmt.Errorf("coding: open workspace: %w", err)
	}
	owned := true
	defer func() {
		if !owned {
			return
		}
		if closeErr := closeWorkspace(workspace); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("coding: close workspace: %w", closeErr))
		}
	}()

	skillDiscovery, err := discoverPiaSkills(workspace)
	if err != nil {
		return nil, err
	}
	tools, err := newCodingTools(workspace, skillDiscovery.Entries)
	if err != nil {
		return nil, err
	}
	schemas := toolSchemas(tools)
	prompt, err := buildSystemPrompt(workspace, tools, skillDiscovery.Catalog)
	if err != nil {
		return nil, err
	}

	requestLimits := productRequestLimits()
	engine, err := agent.New(agent.Config{
		Provider:      provider,
		SystemPrompt:  prompt,
		Tools:         tools,
		RequestLimits: requestLimits,
		Observer:      config.Observer,
	})
	if err != nil {
		return nil, fmt.Errorf("coding: create Agent execution engine: %w", err)
	}
	session, err = newSession(sessionDependencies{
		Engine:        engine,
		Provider:      provider,
		RequestLimits: requestLimits,
		Compaction:    productCompactionPolicy(),
		Observer:      config.Observer,
		Info: SessionInfo{
			WorkspacePath:    workspace.Path(),
			SystemPrompt:     prompt,
			Model:            productModelInfo(),
			Tools:            schemas,
			SkillDiagnostics: skillDiscovery.Diagnostics,
		},
		CloseWorkspace: func() error {
			return closeWorkspace(workspace)
		},
	})
	if err != nil {
		return nil, err
	}
	owned = false
	return session, nil
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
