package coding

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yuanbohan/pi-go/internal/ai"
	"github.com/yuanbohan/pi-go/internal/ai/provider/faux"
)

func TestRunWithProviderComposesStableMultiTurnCodingContext(t *testing.T) {
	directory := t.TempDir()
	writePromptFile(t, directory, "AGENTS.md", []byte("original project guidance"))
	writePromptFile(t, directory, "input.txt", []byte("old value\n"))

	provider := newRuntimeFaux(t,
		fauxToolStep(ai.ToolCall{ID: "read-1", Name: "read", Arguments: json.RawMessage(`{"path":"input.txt"}`)}),
		fauxToolStep(ai.ToolCall{ID: "edit-1", Name: "edit", Arguments: json.RawMessage(`{"path":"input.txt","edits":[{"oldText":"old value","newText":"edited value"}]}`)}),
		fauxToolStep(ai.ToolCall{ID: "write-1", Name: "write", Arguments: json.RawMessage(`{"path":"AGENTS.md","content":"changed project guidance"}`)}),
		fauxToolStep(ai.ToolCall{ID: "bash-1", Name: "bash", Arguments: json.RawMessage(`{"command":"test \"$(cat input.txt)\" = \"edited value\" && printf 'from bash\\n' > output.txt"}`)}),
		fauxFinalStep("checked changes", "done", " and verified"),
	)

	const fakeKey = "fake-key-must-not-enter-runtime-data"
	result, err := runWithProvider(
		context.Background(),
		RunInput{WorkspacePath: directory, Task: "fix the project", APIKey: fakeKey},
		provider,
	)
	if err != nil {
		t.Fatalf("run with provider: %v", err)
	}

	if got, want := result.Model, productModelInfo(); !reflect.DeepEqual(got, want) {
		t.Fatalf("model info = %#v, want %#v", got, want)
	}
	if got, want := result.FinalText(), "done and verified"; got != want {
		t.Fatalf("FinalText() = %q, want %q", got, want)
	}
	if !strings.Contains(result.SystemPrompt, "original project guidance") || strings.Contains(result.SystemPrompt, "changed project guidance") {
		t.Fatalf("system prompt is not the pre-Run snapshot\n%s", result.SystemPrompt)
	}
	if got, want := toolSchemaNames(result.Tools), []string{"read", "bash", "edit", "write"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("result tools = %v, want %v", got, want)
	}

	inputContent, err := os.ReadFile(filepath.Join(directory, "input.txt"))
	if err != nil {
		t.Fatalf("read edited input: %v", err)
	}
	if got, want := string(inputContent), "edited value\n"; got != want {
		t.Fatalf("input.txt = %q, want %q", got, want)
	}
	outputContent, err := os.ReadFile(filepath.Join(directory, "output.txt"))
	if err != nil {
		t.Fatalf("read bash output: %v", err)
	}
	if got, want := string(outputContent), "from bash\n"; got != want {
		t.Fatalf("output.txt = %q, want %q", got, want)
	}

	requests := provider.Requests()
	if got, want := len(requests), 5; got != want {
		t.Fatalf("provider request count = %d, want %d", got, want)
	}
	for index, request := range requests {
		if request.SystemPrompt != result.SystemPrompt {
			t.Fatalf("request %d prompt changed between turns", index)
		}
		if !reflect.DeepEqual(request.Tools, result.Tools) {
			t.Fatalf("request %d tool schemas differ from settled result", index)
		}
		if got, want := len(request.Messages), 1+2*index; got != want {
			t.Fatalf("request %d message count = %d, want %d", index, got, want)
		}
		if got, want := request.MaxOutputTokens, int64(productModelMaxOutput); got != want {
			t.Fatalf("request %d MaxOutputTokens = %d, want %d", index, got, want)
		}
	}

	trace, err := BuildTrace(result, nil)
	if err != nil {
		t.Fatalf("build trace: %v", err)
	}
	encoded, err := json.Marshal(trace)
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}
	if strings.Contains(string(encoded), fakeKey) {
		t.Fatal("runtime result or trace contains the provider credential")
	}
}

func TestRunWithProviderDoesNotSendUnselectedFileOrEnvironmentContent(t *testing.T) {
	directory := t.TempDir()
	const sentinel = "sentinel-only-visible-after-read"
	writePromptFile(t, directory, "private.txt", []byte(sentinel))
	t.Setenv("PIA_CONTEXT_SENTINEL", sentinel)
	provider := newRuntimeFaux(t,
		fauxToolStep(ai.ToolCall{ID: "read-1", Name: "read", Arguments: json.RawMessage(`{"path":"private.txt"}`)}),
		fauxFinalStep("", "done"),
	)

	_, err := runWithProvider(
		context.Background(),
		RunInput{WorkspacePath: directory, Task: "inspect private.txt"},
		provider,
	)
	if err != nil {
		t.Fatalf("run with provider: %v", err)
	}
	requests := provider.Requests()
	if got, want := len(requests), 2; got != want {
		t.Fatalf("provider request count = %d, want %d", got, want)
	}
	if strings.Contains(requests[0].SystemPrompt, sentinel) {
		t.Fatal("initial system prompt contains unselected file or environment content")
	}
	if got, ok := requests[0].Messages[0].(ai.UserMessage); !ok || strings.Contains(got.Content, sentinel) {
		t.Fatalf("initial user message = %#v, want no sentinel", requests[0].Messages[0])
	}
	for _, schema := range requests[0].Tools {
		if strings.Contains(schema.Name+schema.Description+string(schema.Parameters), sentinel) {
			t.Fatal("initial tool schemas contain unselected sentinel")
		}
	}
	toolResult, ok := requests[1].Messages[len(requests[1].Messages)-1].(ai.ToolResultMessage)
	if !ok || !strings.Contains(toolResult.Content, sentinel) {
		t.Fatalf("second request last message = %#v, want explicit read result with sentinel", requests[1].Messages[len(requests[1].Messages)-1])
	}
}

func TestRunWithProviderDisclosesThenReadsPiaSkill(t *testing.T) {
	directory := t.TempDir()
	writePiaSkill(t, directory, "review-go", `name: review-go
description: Review Go changes.
`, "SKILL_INSTRUCTIONS_SENTINEL")
	referencePath := filepath.Join(directory, piaSkillsDirectory, "review-go", "references")
	if err := os.MkdirAll(referencePath, 0o755); err != nil {
		t.Fatalf("create unsupported reference directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(referencePath, "notes.md"), []byte("REFERENCE_SENTINEL"), 0o600); err != nil {
		t.Fatalf("write unsupported reference: %v", err)
	}

	provider := newRuntimeFaux(t,
		fauxToolStep(ai.ToolCall{
			ID:        "read-skill",
			Name:      "read",
			Arguments: json.RawMessage(`{"path":".pia/skills/review-go/SKILL.md"}`),
		}),
		fauxFinalStep("", "used the project skill"),
	)
	result, err := runWithProvider(
		context.Background(),
		RunInput{WorkspacePath: directory, Task: "review this Go change"},
		provider,
	)
	if err != nil {
		t.Fatalf("run with provider: %v", err)
	}
	if len(result.SkillDiagnostics) != 0 {
		t.Fatalf("Skill diagnostics = %#v, want none", result.SkillDiagnostics)
	}
	for _, fragment := range []string{
		"<name>review-go</name>",
		"<description>Review Go changes.</description>",
		"<location>.pia/skills/review-go/SKILL.md</location>",
	} {
		if !strings.Contains(result.SystemPrompt, fragment) {
			t.Errorf("system prompt does not contain %q\n%s", fragment, result.SystemPrompt)
		}
	}
	for _, forbidden := range []string{"SKILL_INSTRUCTIONS_SENTINEL", "REFERENCE_SENTINEL"} {
		if strings.Contains(result.SystemPrompt, forbidden) {
			t.Fatalf("initial system prompt contains undisclosed content %q", forbidden)
		}
	}

	requests := provider.Requests()
	if got, want := len(requests), 2; got != want {
		t.Fatalf("provider request count = %d, want %d", got, want)
	}
	toolResult, ok := requests[1].Messages[len(requests[1].Messages)-1].(ai.ToolResultMessage)
	if !ok || !strings.Contains(toolResult.Content, "SKILL_INSTRUCTIONS_SENTINEL") {
		t.Fatalf("second request last message = %#v, want on-demand SKILL.md read", requests[1].Messages[len(requests[1].Messages)-1])
	}
	if strings.Contains(toolResult.Content, "REFERENCE_SENTINEL") {
		t.Fatalf("Skill read implicitly included an unsupported reference: %q", toolResult.Content)
	}
}

func TestRunWithProviderReturnsSkillDiagnosticsWithoutBlockingTask(t *testing.T) {
	directory := t.TempDir()
	writePiaSkill(t, directory, "broken", `description: Missing name.
`, "BROKEN_BODY_SENTINEL")
	provider := newRuntimeFaux(t, fauxFinalStep("", "ordinary task completed"))

	result, err := runWithProvider(
		context.Background(),
		RunInput{WorkspacePath: directory, Task: "complete an ordinary task"},
		provider,
	)
	if err != nil {
		t.Fatalf("run with provider: %v", err)
	}
	if got, want := result.FinalText(), "ordinary task completed"; got != want {
		t.Fatalf("FinalText() = %q, want %q", got, want)
	}
	if !skillDiagnosticsContain(result.SkillDiagnostics, "required name") {
		t.Fatalf("Skill diagnostics = %#v, want missing-name warning", result.SkillDiagnostics)
	}
	if strings.Contains(result.SystemPrompt, "BROKEN_BODY_SENTINEL") || strings.Contains(result.SystemPrompt, "<available_skills>") {
		t.Fatalf("invalid Skill entered system prompt\n%s", result.SystemPrompt)
	}
}

func TestRunWithProviderReturnsTranscriptAndJoinsCloseError(t *testing.T) {
	provider := newRuntimeFaux(t, faux.Step{Events: []ai.Event{
		ai.ErrorEvent{Message: ai.AssistantMessage{
			StopReason:   ai.StopReasonError,
			ErrorMessage: "upstream unavailable",
		}},
	}})
	closeErr := errors.New("close sentinel")
	var opened *Workspace
	result, err := runWithWorkspaceOperations(
		context.Background(),
		RunInput{WorkspacePath: t.TempDir(), Task: "try once"},
		provider,
		func(path string) (*Workspace, error) {
			workspace, openErr := OpenWorkspace(path)
			opened = workspace
			return workspace, openErr
		},
		func(workspace *Workspace) error {
			if _, statErr := workspace.Root().Stat("."); statErr != nil {
				t.Errorf("workspace was closed before Run settlement: %v", statErr)
			}
			return errors.Join(workspace.Close(), closeErr)
		},
	)
	if err == nil || !strings.Contains(err.Error(), "upstream unavailable") {
		t.Fatalf("run error = %v, want Provider failure", err)
	}
	if !errors.Is(err, closeErr) {
		t.Fatalf("run error = %v, want joined close error", err)
	}
	if got, want := len(result.Transcript), 2; got != want {
		t.Fatalf("transcript length = %d, want %d", got, want)
	}
	if opened == nil {
		t.Fatal("workspace was not opened")
	}
	if _, statErr := opened.Root().Stat("."); statErr == nil {
		t.Fatal("workspace root remains usable after Run returned")
	}
}

func TestRunWithProviderCancellationSettlesBeforeClosingWorkspace(t *testing.T) {
	provider := &cancelAwareProvider{started: make(chan struct{})}
	ctx, cancel := context.WithCancelCause(context.Background())
	cancelErr := errors.New("operator canceled")
	closed := make(chan struct{})

	type outcome struct {
		result RunResult
		err    error
	}
	finished := make(chan outcome, 1)
	go func() {
		result, err := runWithWorkspaceOperations(
			ctx,
			RunInput{WorkspacePath: t.TempDir(), Task: "wait"},
			provider,
			OpenWorkspace,
			func(workspace *Workspace) error {
				defer close(closed)
				return workspace.Close()
			},
		)
		finished <- outcome{result: result, err: err}
	}()

	select {
	case <-provider.started:
	case <-time.After(5 * time.Second):
		t.Fatal("Provider did not start")
	}
	select {
	case <-closed:
		t.Fatal("workspace closed while Provider was still active")
	default:
	}
	cancel(cancelErr)

	select {
	case got := <-finished:
		if !errors.Is(got.err, cancelErr) {
			t.Fatalf("run error = %v, want cancellation cause", got.err)
		}
		if gotLen, want := len(got.result.Transcript), 2; gotLen != want {
			t.Fatalf("transcript length = %d, want %d", gotLen, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceled Run did not settle")
	}
	select {
	case <-closed:
	default:
		t.Fatal("workspace was not closed after cancellation settlement")
	}
}

func TestRunWithProviderClosesWorkspaceAfterPromptFailure(t *testing.T) {
	directory := t.TempDir()
	if err := os.Mkdir(filepath.Join(directory, "AGENTS.md"), 0o755); err != nil {
		t.Fatalf("create invalid project instructions: %v", err)
	}
	provider := newRuntimeFaux(t, fauxFinalStep("", "unused"))
	closed := false
	var opened *Workspace

	result, err := runWithWorkspaceOperations(
		context.Background(),
		RunInput{WorkspacePath: directory, Task: "unused"},
		provider,
		func(path string) (*Workspace, error) {
			workspace, openErr := OpenWorkspace(path)
			opened = workspace
			return workspace, openErr
		},
		func(workspace *Workspace) error {
			closed = true
			return workspace.Close()
		},
	)
	if err == nil || !strings.Contains(err.Error(), "project instructions") {
		t.Fatalf("run error = %v, want project-instructions failure", err)
	}
	if !closed || opened == nil {
		t.Fatal("workspace was not closed after prompt failure")
	}
	if got := len(provider.Requests()); got != 0 {
		t.Fatalf("Provider requests = %d, want 0", got)
	}
	if got, want := toolSchemaNames(result.Tools), []string{"read", "bash", "edit", "write"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("diagnostic tools after prompt failure = %v, want %v", got, want)
	}
}

func TestProductDeepSeekConfigIsFixed(t *testing.T) {
	config := productDeepSeekConfig("key")
	if got, want := config.Model, "deepseek-v4-pro"; got != want {
		t.Fatalf("model = %q, want %q", got, want)
	}
	if got, want := config.ReasoningEffort, "high"; got != want {
		t.Fatalf("reasoning effort = %q, want %q", got, want)
	}
	if got, want := config.APIKey, "key"; got != want {
		t.Fatalf("API key = %q, want %q", got, want)
	}
	info := productModelInfo()
	if !info.Thinking || info.Provider != "deepseek" || info.Name != config.Model || info.ReasoningEffort != config.ReasoningEffort {
		t.Fatalf("model info = %#v, want fixed DeepSeek thinking profile", info)
	}
	if got, want := productRequestLimits(), (ai.RequestLimits{
		ContextCapacity: 1_000_000,
		ModelMaxOutput:  384_000,
		ContextSafety:   4_096,
	}); got != want {
		t.Fatalf("request limits = %#v, want %#v", got, want)
	}
	if got, want := productCompactionPolicy(), (compactionPolicy{
		Threshold:                192_000,
		SoftCeiling:              64_000,
		RetainedRawTarget:        20_000,
		SummaryMaxOutput:         13_107,
		SplitTurnPrefixMaxOutput: 8_192,
	}); got != want {
		t.Fatalf("compaction policy = %#v, want %#v", got, want)
	}
}

func TestRunRejectsMissingAPIKeyBeforeOpeningWorkspace(t *testing.T) {
	result, err := Run(context.Background(), RunInput{
		WorkspacePath: t.TempDir(),
		Task:          "do not contact the network",
		APIKey:        "  ",
	})
	if err == nil || !strings.Contains(err.Error(), "API key") {
		t.Fatalf("Run() error = %v, want missing-key error", err)
	}
	if len(result.Transcript) != 0 {
		t.Fatalf("Run() transcript = %#v, want empty", result.Transcript)
	}
}

func TestRunResultFinalTextUsesOnlyLastAssistantTextBlocks(t *testing.T) {
	result := RunResult{Transcript: []ai.Message{
		ai.AssistantMessage{Content: []ai.AssistantContent{ai.TextContent{Text: "earlier"}}, StopReason: ai.StopReasonStop},
		ai.ToolResultMessage{ToolCallID: "call", ToolName: "read", Content: "tool output"},
		ai.AssistantMessage{Content: []ai.AssistantContent{
			ai.ThinkingContent{Thinking: "private reasoning"},
			ai.TextContent{Text: "final "},
			ai.ToolCall{ID: "ignored", Name: "read", Arguments: json.RawMessage(`{}`)},
			ai.TextContent{Text: "answer"},
		}, StopReason: ai.StopReasonStop},
	}}
	if got, want := result.FinalText(), "final answer"; got != want {
		t.Fatalf("FinalText() = %q, want %q", got, want)
	}
	if got := (RunResult{Transcript: []ai.Message{ai.AssistantMessage{StopReason: ai.StopReasonStop}}}).FinalText(); got != "" {
		t.Fatalf("empty FinalText() = %q, want empty", got)
	}
}

func newRuntimeFaux(t *testing.T, steps ...faux.Step) *faux.Provider {
	t.Helper()
	provider, err := faux.New(steps...)
	if err != nil {
		t.Fatalf("create Faux Provider: %v", err)
	}
	return provider
}

func fauxToolStep(call ai.ToolCall) faux.Step {
	message := ai.AssistantMessage{
		Content:    []ai.AssistantContent{call},
		StopReason: ai.StopReasonToolUse,
	}
	return faux.Step{Events: []ai.Event{
		ai.StartEvent{},
		ai.ToolCallStartEvent{ContentIndex: 0, ID: call.ID, Name: call.Name},
		ai.ToolCallDeltaEvent{ContentIndex: 0, Delta: string(call.Arguments)},
		ai.ToolCallEndEvent{ContentIndex: 0, ToolCall: call},
		ai.DoneEvent{Message: message},
	}}
}

func fauxFinalStep(thinking string, texts ...string) faux.Step {
	message := ai.AssistantMessage{StopReason: ai.StopReasonStop}
	events := []ai.Event{ai.StartEvent{}}
	contentIndex := 0
	if thinking != "" {
		message.Content = append(message.Content, ai.ThinkingContent{Thinking: thinking})
		events = append(events,
			ai.ThinkingStartEvent{ContentIndex: contentIndex},
			ai.ThinkingDeltaEvent{ContentIndex: contentIndex, Delta: thinking},
			ai.ThinkingEndEvent{ContentIndex: contentIndex, Thinking: thinking},
		)
		contentIndex++
	}
	for _, text := range texts {
		message.Content = append(message.Content, ai.TextContent{Text: text})
		events = append(events,
			ai.TextStartEvent{ContentIndex: contentIndex},
			ai.TextDeltaEvent{ContentIndex: contentIndex, Delta: text},
			ai.TextEndEvent{ContentIndex: contentIndex, Text: text},
		)
		contentIndex++
	}
	events = append(events, ai.DoneEvent{Message: message})
	return faux.Step{Events: events}
}

func toolSchemaNames(schemas []ai.ToolSchema) []string {
	names := make([]string, len(schemas))
	for index, schema := range schemas {
		names[index] = schema.Name
	}
	return names
}

type cancelAwareProvider struct {
	started chan struct{}
	once    sync.Once
}

func (p *cancelAwareProvider) Stream(ctx context.Context, _ ai.Request) ai.Stream {
	return &cancelAwareStream{ctx: ctx, start: func() { p.once.Do(func() { close(p.started) }) }}
}

type cancelAwareStream struct {
	ctx   context.Context
	start func()
}

func (s *cancelAwareStream) Receive() (ai.Event, error) {
	s.start()
	<-s.ctx.Done()
	return ai.ErrorEvent{Message: ai.AssistantMessage{
		StopReason:   ai.StopReasonAborted,
		ErrorMessage: context.Cause(s.ctx).Error(),
	}}, nil
}

var _ ai.Provider = (*cancelAwareProvider)(nil)
