package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/yuanbohan/pi-go/internal/ai"
	"github.com/yuanbohan/pi-go/internal/coding"
)

func TestExecuteValidatesArgumentsBeforeReadingConfiguration(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "missing", args: nil},
		{name: "too many", args: []string{"one", "two"}},
		{name: "blank", args: []string{" \t\n"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deps := dependencies{
				lookupEnv: func(string) (string, bool) {
					t.Fatal("configuration was read before arguments were accepted")
					return "", false
				},
			}
			var stdout bytes.Buffer
			err := execute(context.Background(), test.args, &stdout, io.Discard, deps)
			if err == nil || !strings.Contains(err.Error(), "exactly one") {
				t.Fatalf("execute() error = %v, want argument error", err)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}
		})
	}
}

func TestExecuteRequiresInheritedKeyBeforeReadingWorkspace(t *testing.T) {
	deps := dependencies{
		lookupEnv: func(name string) (string, bool) {
			if name != deepSeekAPIKeyEnv {
				t.Fatalf("first environment lookup = %q, want %q", name, deepSeekAPIKeyEnv)
			}
			return "  ", true
		},
		getwd: func() (string, error) {
			t.Fatal("workspace was read without a usable API key")
			return "", nil
		},
	}

	err := execute(context.Background(), []string{"task"}, io.Discard, io.Discard, deps)
	if err == nil || !strings.Contains(err.Error(), deepSeekAPIKeyEnv) {
		t.Fatalf("execute() error = %v, want missing-key error", err)
	}
}

func TestExecuteReturnsWorkingDirectoryFailureBeforeRun(t *testing.T) {
	getwdErr := errors.New("cwd unavailable")
	deps := successfulDependencies()
	deps.getwd = func() (string, error) { return "", getwdErr }
	deps.run = func(context.Context, coding.RunInput) (coding.RunResult, error) {
		t.Fatal("Run started without a working directory")
		return coding.RunResult{}, nil
	}

	err := execute(context.Background(), []string{"task"}, io.Discard, io.Discard, deps)
	if !errors.Is(err, getwdErr) {
		t.Fatalf("execute() error = %v, want getwd error", err)
	}
}

func TestExecutePassesRawTaskAndPrintsOnlyFinalText(t *testing.T) {
	const task = "-fix the project exactly"
	wantInput := coding.RunInput{WorkspacePath: "/workspace", Task: task, APIKey: "key"}
	var gotInput coding.RunInput
	deps := successfulDependencies()
	deps.run = func(_ context.Context, input coding.RunInput) (coding.RunResult, error) {
		gotInput = input
		return coding.RunResult{Transcript: []ai.Message{
			ai.UserMessage{Content: task},
			ai.AssistantMessage{Content: []ai.AssistantContent{
				ai.ThinkingContent{Thinking: "hidden reasoning"},
				ai.ToolCall{ID: "call", Name: "read"},
			}, StopReason: ai.StopReasonToolUse},
			ai.ToolResultMessage{ToolCallID: "call", ToolName: "read", Content: "hidden tool result"},
			ai.AssistantMessage{Content: []ai.AssistantContent{
				ai.ThinkingContent{Thinking: "more hidden reasoning"},
				ai.TextContent{Text: "finished"},
			}, StopReason: ai.StopReasonLength},
		}}, nil
	}

	var stdout bytes.Buffer
	if err := execute(context.Background(), []string{task}, &stdout, io.Discard, deps); err != nil {
		t.Fatalf("execute() error = %v", err)
	}
	if !reflect.DeepEqual(gotInput, wantInput) {
		t.Fatalf("Run input = %#v, want %#v", gotInput, wantInput)
	}
	if got, want := stdout.String(), "finished\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestExecuteDoesNotAddBlankLineToFinalTextWithNewline(t *testing.T) {
	deps := successfulDependencies()
	deps.run = func(context.Context, coding.RunInput) (coding.RunResult, error) {
		return coding.RunResult{Transcript: []ai.Message{
			ai.AssistantMessage{
				Content:    []ai.AssistantContent{ai.TextContent{Text: "finished\n"}},
				StopReason: ai.StopReasonStop,
			},
		}}, nil
	}

	var stdout bytes.Buffer
	if err := execute(context.Background(), []string{"task"}, &stdout, io.Discard, deps); err != nil {
		t.Fatalf("execute() error = %v", err)
	}
	if got, want := stdout.String(), "finished\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestExecuteAllowsEmptyFinalText(t *testing.T) {
	deps := successfulDependencies()
	deps.run = func(context.Context, coding.RunInput) (coding.RunResult, error) {
		return coding.RunResult{Transcript: []ai.Message{
			ai.AssistantMessage{StopReason: ai.StopReasonStop},
		}}, nil
	}
	var stdout bytes.Buffer
	if err := execute(context.Background(), []string{"task"}, &stdout, io.Discard, deps); err != nil {
		t.Fatalf("execute() error = %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}

func TestExecuteDoesNotBuildTraceWhenPathIsUnsetOrEmpty(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]string
	}{
		{name: "unset", values: map[string]string{deepSeekAPIKeyEnv: "key"}},
		{name: "empty", values: map[string]string{deepSeekAPIKeyEnv: "key", tracePathEnv: ""}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deps := successfulDependencies()
			deps.lookupEnv = environment(test.values)
			deps.buildTrace = func(coding.RunResult, error) (coding.Trace, error) {
				t.Fatal("trace was built without a non-empty trace path")
				return coding.Trace{}, nil
			}
			deps.writeTrace = func(string, coding.Trace) error {
				t.Fatal("trace was written without a non-empty trace path")
				return nil
			}
			if err := execute(context.Background(), []string{"task"}, io.Discard, io.Discard, deps); err != nil {
				t.Fatalf("execute() error = %v", err)
			}
		})
	}
}

func TestExecuteWritesRequestedTraceAfterFailedRunAndSuppressesFinal(t *testing.T) {
	runErr := errors.New("Provider failed")
	result := coding.RunResult{Transcript: []ai.Message{
		ai.AssistantMessage{Content: []ai.AssistantContent{ai.TextContent{Text: "partial"}}, StopReason: ai.StopReasonError},
	}}
	deps := successfulDependencies()
	deps.lookupEnv = environment(map[string]string{
		deepSeekAPIKeyEnv: "key",
		tracePathEnv:      "evidence/trace.json",
	})
	deps.run = func(context.Context, coding.RunInput) (coding.RunResult, error) {
		return result, runErr
	}
	traceBuilt := false
	deps.buildTrace = func(gotResult coding.RunResult, gotErr error) (coding.Trace, error) {
		traceBuilt = true
		if !reflect.DeepEqual(gotResult, result) || !errors.Is(gotErr, runErr) {
			t.Fatalf("trace input = (%#v, %v), want settled result and Run error", gotResult, gotErr)
		}
		return coding.Trace{RunError: gotErr.Error()}, nil
	}
	traceWritten := false
	deps.writeTrace = func(path string, trace coding.Trace) error {
		traceWritten = true
		if got, want := path, "/workspace/evidence/trace.json"; got != want {
			t.Fatalf("trace path = %q, want %q", got, want)
		}
		if trace.RunError != runErr.Error() {
			t.Fatalf("trace RunError = %q, want %q", trace.RunError, runErr)
		}
		return nil
	}

	var stdout bytes.Buffer
	err := execute(context.Background(), []string{"task"}, &stdout, io.Discard, deps)
	if !errors.Is(err, runErr) {
		t.Fatalf("execute() error = %v, want Run error", err)
	}
	if !traceBuilt || !traceWritten {
		t.Fatal("requested trace was not built and written after failed Run")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}

func TestExecuteWritesRequestedTraceAfterSuccessfulRunAndPrintsFinal(t *testing.T) {
	result := coding.RunResult{Transcript: []ai.Message{
		ai.AssistantMessage{Content: []ai.AssistantContent{ai.TextContent{Text: "finished"}}, StopReason: ai.StopReasonStop},
	}}
	deps := successfulDependencies()
	deps.lookupEnv = environment(map[string]string{
		deepSeekAPIKeyEnv: "key",
		tracePathEnv:      "evidence/trace.json",
	})
	deps.run = func(context.Context, coding.RunInput) (coding.RunResult, error) {
		return result, nil
	}
	deps.buildTrace = func(gotResult coding.RunResult, gotErr error) (coding.Trace, error) {
		if !reflect.DeepEqual(gotResult, result) || gotErr != nil {
			t.Fatalf("trace input = (%#v, %v), want successful settled result", gotResult, gotErr)
		}
		return coding.Trace{Workspace: "/workspace"}, nil
	}
	traceWritten := false
	deps.writeTrace = func(path string, trace coding.Trace) error {
		traceWritten = true
		if got, want := path, "/workspace/evidence/trace.json"; got != want {
			t.Fatalf("trace path = %q, want %q", got, want)
		}
		if trace.Workspace != "/workspace" {
			t.Fatalf("trace Workspace = %q, want /workspace", trace.Workspace)
		}
		return nil
	}

	var stdout bytes.Buffer
	if err := execute(context.Background(), []string{"task"}, &stdout, io.Discard, deps); err != nil {
		t.Fatalf("execute() error = %v", err)
	}
	if !traceWritten {
		t.Fatal("requested trace was not written after successful Run")
	}
	if got, want := stdout.String(), "finished\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestExecuteJoinsRunAndTraceErrors(t *testing.T) {
	runErr := errors.New("Run failed")
	traceErr := errors.New("trace failed")
	deps := successfulDependencies()
	deps.lookupEnv = environment(map[string]string{
		deepSeekAPIKeyEnv: "key",
		tracePathEnv:      "/trace.json",
	})
	deps.run = func(context.Context, coding.RunInput) (coding.RunResult, error) {
		return coding.RunResult{}, runErr
	}
	deps.writeTrace = func(string, coding.Trace) error { return traceErr }

	err := execute(context.Background(), []string{"task"}, io.Discard, io.Discard, deps)
	if !errors.Is(err, runErr) || !errors.Is(err, traceErr) {
		t.Fatalf("execute() error = %v, want joined Run and trace errors", err)
	}
}

func TestExecuteTraceFailureSuppressesSuccessfulFinalText(t *testing.T) {
	traceErr := errors.New("trace failed")
	deps := successfulDependencies()
	deps.lookupEnv = environment(map[string]string{
		deepSeekAPIKeyEnv: "key",
		tracePathEnv:      "/trace.json",
	})
	deps.writeTrace = func(string, coding.Trace) error { return traceErr }
	var stdout bytes.Buffer

	err := execute(context.Background(), []string{"task"}, &stdout, io.Discard, deps)
	if !errors.Is(err, traceErr) {
		t.Fatalf("execute() error = %v, want trace error", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want final text suppressed", stdout.String())
	}
}

func TestExecuteTraceBuildFailureDoesNotCallWriter(t *testing.T) {
	buildErr := errors.New("trace conversion failed")
	deps := successfulDependencies()
	deps.lookupEnv = environment(map[string]string{
		deepSeekAPIKeyEnv: "key",
		tracePathEnv:      "/trace.json",
	})
	deps.buildTrace = func(coding.RunResult, error) (coding.Trace, error) {
		return coding.Trace{}, buildErr
	}
	deps.writeTrace = func(string, coding.Trace) error {
		t.Fatal("trace writer was called after conversion failure")
		return nil
	}

	err := execute(context.Background(), []string{"task"}, io.Discard, io.Discard, deps)
	if !errors.Is(err, buildErr) {
		t.Fatalf("execute() error = %v, want trace-build error", err)
	}
}

func TestExecuteReturnsStdoutFailure(t *testing.T) {
	writeErr := errors.New("stdout unavailable")
	deps := successfulDependencies()
	err := execute(context.Background(), []string{"task"}, errorWriter{err: writeErr}, io.Discard, deps)
	if !errors.Is(err, writeErr) {
		t.Fatalf("execute() error = %v, want stdout error", err)
	}
}

func TestRunProcessPrintsSkillDiagnosticsOnSuccessfulRun(t *testing.T) {
	deps := successfulDependencies()
	deps.run = func(context.Context, coding.RunInput) (coding.RunResult, error) {
		return coding.RunResult{
			SkillDiagnostics: []coding.SkillDiagnostic{{
				Path:    ".pia/skills/broken/SKILL.md",
				Message: "required name is missing",
			}},
			Transcript: []ai.Message{ai.AssistantMessage{
				Content:    []ai.AssistantContent{ai.TextContent{Text: "done"}},
				StopReason: ai.StopReasonStop,
			}},
		}, nil
	}
	var stdout, stderr bytes.Buffer
	if got := runProcess(context.Background(), []string{"task"}, &stdout, &stderr, deps); got != 0 {
		t.Fatalf("runProcess() code = %d, want 0; stderr=%q", got, stderr.String())
	}
	if got, want := stdout.String(), "done\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if got, want := stderr.String(), "pia: warning: .pia/skills/broken/SKILL.md: required name is missing\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}

func TestRunProcessDoesNotPrintSkillDiagnosticsWhenRunFails(t *testing.T) {
	runErr := errors.New("run failed")
	deps := successfulDependencies()
	deps.run = func(context.Context, coding.RunInput) (coding.RunResult, error) {
		return coding.RunResult{SkillDiagnostics: []coding.SkillDiagnostic{{
			Path:    ".pia/skills/broken/SKILL.md",
			Message: "required name is missing",
		}}}, runErr
	}
	var stderr bytes.Buffer
	if got := runProcess(context.Background(), []string{"task"}, io.Discard, &stderr, deps); got != 1 {
		t.Fatalf("runProcess() code = %d, want 1", got)
	}
	if got, want := stderr.String(), "pia: run failed\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}

func TestExecuteWithDiagnosticsReturnsStderrFailure(t *testing.T) {
	writeErr := errors.New("stderr unavailable")
	deps := successfulDependencies()
	deps.run = func(context.Context, coding.RunInput) (coding.RunResult, error) {
		return coding.RunResult{SkillDiagnostics: []coding.SkillDiagnostic{{
			Path:    ".pia/skills/broken/SKILL.md",
			Message: "required name is missing",
		}}}, nil
	}
	err := execute(context.Background(), []string{"task"}, io.Discard, errorWriter{err: writeErr}, deps)
	if !errors.Is(err, writeErr) {
		t.Fatalf("execute() error = %v, want stderr failure", err)
	}
}

func TestRunProcessReportsErrorsOnlyToStderr(t *testing.T) {
	deps := successfulDependencies()
	runErr := errors.New("broken")
	deps.run = func(context.Context, coding.RunInput) (coding.RunResult, error) {
		return coding.RunResult{}, runErr
	}
	var stdout, stderr bytes.Buffer

	code := runProcess(context.Background(), []string{"task"}, &stdout, &stderr, deps)
	if code == 0 {
		t.Fatal("runProcess() code = 0, want nonzero")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if got := stderr.String(); !strings.Contains(got, "pia:") || !strings.Contains(got, runErr.Error()) {
		t.Fatalf("stderr = %q, want temporary command context and error", got)
	}
}

func successfulDependencies() dependencies {
	return dependencies{
		lookupEnv: environment(map[string]string{deepSeekAPIKeyEnv: "key"}),
		getwd:     func() (string, error) { return "/workspace", nil },
		run: func(context.Context, coding.RunInput) (coding.RunResult, error) {
			return coding.RunResult{Transcript: []ai.Message{
				ai.AssistantMessage{Content: []ai.AssistantContent{ai.TextContent{Text: "done"}}, StopReason: ai.StopReasonStop},
			}}, nil
		},
		buildTrace: coding.BuildTrace,
		writeTrace: func(string, coding.Trace) error { return nil },
	}
}

func environment(values map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}

type errorWriter struct{ err error }

func (w errorWriter) Write([]byte) (int, error) { return 0, w.err }
