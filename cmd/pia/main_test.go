package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/yuanbohan/pia/internal/ai"
	"github.com/yuanbohan/pia/internal/coding"
	"github.com/yuanbohan/pia/internal/observation"
)

func TestExecuteValidatesArgumentsBeforeReadingConfiguration(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "missing"},
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

func TestExecuteReturnsWorkingDirectoryFailureBeforeConstructingSession(t *testing.T) {
	getwdErr := errors.New("cwd unavailable")
	deps, _ := successfulDependencies()
	deps.getwd = func() (string, error) { return "", getwdErr }
	deps.newSession = func(coding.SessionConfig) (codingSession, error) {
		t.Fatal("Session constructed without a working directory")
		return nil, nil
	}

	err := execute(context.Background(), []string{"task"}, io.Discard, io.Discard, deps)
	if !errors.Is(err, getwdErr) {
		t.Fatalf("execute() error = %v, want getwd error", err)
	}
}

func TestExecuteConstructsAdvancesClosesAndPrintsOnlyFinalText(t *testing.T) {
	const task = "-fix the project exactly"
	deps, session := successfulDependencies()
	var gotConfig coding.SessionConfig
	deps.newSession = func(config coding.SessionConfig) (codingSession, error) {
		gotConfig = config
		session.observer = config.Observer
		return session, nil
	}
	session.result = coding.AdvanceResult{History: []ai.Message{
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
	}}

	var stdout bytes.Buffer
	if err := execute(context.Background(), []string{task}, &stdout, io.Discard, deps); err != nil {
		t.Fatalf("execute() error = %v", err)
	}
	if gotConfig.WorkspacePath != "/workspace" || gotConfig.DeepSeekAPIKey != "key" {
		t.Fatalf("Session config = %#v, want workspace and key", gotConfig)
	}
	if gotConfig.Observer == nil {
		t.Fatal("Session config observer = nil, want live line observer")
	}
	if session.input != task {
		t.Fatalf("Advance input = %q, want raw task %q", session.input, task)
	}
	if session.closeCalls != 1 {
		t.Fatalf("Close calls = %d, want 1", session.closeCalls)
	}
	if got, want := stdout.String(), "finished\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestExecuteProjectsLiveEventsAndReturnsObserverFailureAfterFinalText(t *testing.T) {
	writeErr := errors.New("live stderr unavailable")
	deps, session := successfulDependencies()
	session.onAdvance = func() {
		session.observer.Observe(observation.NewToolStarted(0, "read", "Read main.go"))
	}

	var stdout bytes.Buffer
	err := execute(context.Background(), []string{"task"}, &stdout, errorWriter{err: writeErr}, deps)
	if !errors.Is(err, writeErr) {
		t.Fatalf("execute() error = %v, want observer write error", err)
	}
	if got, want := stdout.String(), "done\n"; got != want {
		t.Fatalf("stdout = %q, want successful final text", got)
	}
}

func TestExecuteJoinsSettlementAndObserverFailures(t *testing.T) {
	advanceErr := errors.New("advance failed")
	writeErr := errors.New("live stderr unavailable")
	deps, session := successfulDependencies()
	session.advanceErr = advanceErr
	session.onAdvance = func() {
		session.observer.Observe(observation.NewToolStarted(0, "read", "Read main.go"))
	}

	err := execute(
		context.Background(),
		[]string{"task"},
		io.Discard,
		errorWriter{err: writeErr},
		deps,
	)
	if !errors.Is(err, advanceErr) || !errors.Is(err, writeErr) {
		t.Fatalf("execute() error = %v, want Advance and observer errors", err)
	}
}

func TestExecuteFinalTextFormatting(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "adds newline", text: "finished", want: "finished\n"},
		{name: "keeps newline", text: "finished\n", want: "finished\n"},
		{name: "allows empty", text: "", want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deps, session := successfulDependencies()
			session.result = coding.AdvanceResult{History: []ai.Message{
				ai.AssistantMessage{
					Content:    []ai.AssistantContent{ai.TextContent{Text: test.text}},
					StopReason: ai.StopReasonStop,
				},
			}}
			var stdout bytes.Buffer
			if err := execute(context.Background(), []string{"task"}, &stdout, io.Discard, deps); err != nil {
				t.Fatalf("execute() error = %v", err)
			}
			if got := stdout.String(); got != test.want {
				t.Fatalf("stdout = %q, want %q", got, test.want)
			}
		})
	}
}

func TestExecuteReturnsConstructorFailureWithoutTraceOrClose(t *testing.T) {
	constructorErr := errors.New("construct failed")
	deps, _ := successfulDependencies()
	deps.lookupEnv = environment(map[string]string{
		deepSeekAPIKeyEnv: "key",
		tracePathEnv:      "/trace.json",
	})
	deps.newSession = func(coding.SessionConfig) (codingSession, error) {
		return nil, constructorErr
	}
	deps.buildTrace = func(coding.SessionInfo, coding.AdvanceResult, error) (coding.Trace, error) {
		t.Fatal("trace built without a constructed Session")
		return coding.Trace{}, nil
	}

	err := execute(context.Background(), []string{"task"}, io.Discard, io.Discard, deps)
	if !errors.Is(err, constructorErr) {
		t.Fatalf("execute() error = %v, want constructor error", err)
	}
}

func TestExecuteJoinsAdvanceAndCloseFailuresAndTracesSettlement(t *testing.T) {
	advanceErr := errors.New("advance failed")
	closeErr := errors.New("close failed")
	deps, session := successfulDependencies()
	session.advanceErr = advanceErr
	session.closeErr = closeErr
	deps.lookupEnv = environment(map[string]string{
		deepSeekAPIKeyEnv: "key",
		tracePathEnv:      "evidence/trace.json",
	})

	var tracedInfo coding.SessionInfo
	var tracedResult coding.AdvanceResult
	var tracedErr error
	deps.buildTrace = func(
		info coding.SessionInfo,
		result coding.AdvanceResult,
		err error,
	) (coding.Trace, error) {
		tracedInfo = info
		tracedResult = result
		tracedErr = err
		return coding.Trace{SettlementError: err.Error()}, nil
	}
	var tracePath string
	deps.writeTrace = func(path string, _ coding.Trace) error {
		tracePath = path
		return nil
	}

	var stdout bytes.Buffer
	err := execute(context.Background(), []string{"task"}, &stdout, io.Discard, deps)
	if !errors.Is(err, advanceErr) || !errors.Is(err, closeErr) {
		t.Fatalf("execute() error = %v, want Advance and Close errors", err)
	}
	if !reflect.DeepEqual(tracedInfo, session.info) ||
		!reflect.DeepEqual(tracedResult, session.result) ||
		!errors.Is(tracedErr, advanceErr) ||
		!errors.Is(tracedErr, closeErr) {
		t.Fatalf("trace input = (%#v, %#v, %v), want complete Session settlement", tracedInfo, tracedResult, tracedErr)
	}
	if tracePath != "/workspace/evidence/trace.json" {
		t.Fatalf("trace path = %q, want workspace-relative path", tracePath)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want failure to suppress final text", stdout.String())
	}
}

func TestExecuteDoesNotBuildTraceWhenPathIsUnset(t *testing.T) {
	deps, _ := successfulDependencies()
	deps.buildTrace = func(coding.SessionInfo, coding.AdvanceResult, error) (coding.Trace, error) {
		t.Fatal("trace built without a configured path")
		return coding.Trace{}, nil
	}
	if err := execute(context.Background(), []string{"task"}, io.Discard, io.Discard, deps); err != nil {
		t.Fatalf("execute() error = %v", err)
	}
}

func TestExecuteTraceFailuresSuppressFinalText(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*dependencies, error)
	}{
		{
			name: "build",
			configure: func(deps *dependencies, want error) {
				deps.buildTrace = func(coding.SessionInfo, coding.AdvanceResult, error) (coding.Trace, error) {
					return coding.Trace{}, want
				}
				deps.writeTrace = func(string, coding.Trace) error {
					t.Fatal("trace writer called after build failure")
					return nil
				}
			},
		},
		{
			name: "write",
			configure: func(deps *dependencies, want error) {
				deps.writeTrace = func(string, coding.Trace) error { return want }
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			want := errors.New("trace failed")
			deps, _ := successfulDependencies()
			deps.lookupEnv = environment(map[string]string{
				deepSeekAPIKeyEnv: "key",
				tracePathEnv:      "/trace.json",
			})
			test.configure(&deps, want)

			var stdout bytes.Buffer
			err := execute(context.Background(), []string{"task"}, &stdout, io.Discard, deps)
			if !errors.Is(err, want) {
				t.Fatalf("execute() error = %v, want trace error", err)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want trace failure to suppress final text", stdout.String())
			}
		})
	}
}

func TestRunProcessPrintsEscapedSkillDiagnosticsAfterSuccess(t *testing.T) {
	deps, session := successfulDependencies()
	session.info.SkillDiagnostics = []coding.SkillDiagnostic{{
		Path:    ".pia/skills/bad\npia: forged\r\x1b[31m/SKILL.md",
		Message: "invalid\npia: forged\r\x1b[31m Skill",
	}}

	var stdout, stderr bytes.Buffer
	if got := runProcess(context.Background(), []string{"task"}, &stdout, &stderr, deps); got != 0 {
		t.Fatalf("runProcess() code = %d, want 0; stderr=%q", got, stderr.String())
	}
	if got, want := stdout.String(), "done\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	wantWarning := "pia: warning: \".pia/skills/bad\\npia: forged\\r\\x1b[31m/SKILL.md\": \"invalid\\npia: forged\\r\\x1b[31m Skill\"\n"
	if got := stderr.String(); got != wantWarning {
		t.Fatalf("stderr = %q, want %q", got, wantWarning)
	}
}

func TestRunProcessDoesNotPrintSkillDiagnosticsAfterFailure(t *testing.T) {
	deps, session := successfulDependencies()
	session.info.SkillDiagnostics = []coding.SkillDiagnostic{{
		Path:    ".pia/skills/broken/SKILL.md",
		Message: "must not be printed",
	}}
	session.advanceErr = errors.New("advance failed")

	var stderr bytes.Buffer
	if got := runProcess(context.Background(), []string{"task"}, io.Discard, &stderr, deps); got != 1 {
		t.Fatalf("runProcess() code = %d, want 1", got)
	}
	if got, want := stderr.String(), "pia: advance failed\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}

func TestExecuteReturnsSkillDiagnosticWriterFailure(t *testing.T) {
	writeErr := errors.New("stderr unavailable")
	deps, session := successfulDependencies()
	session.info.SkillDiagnostics = []coding.SkillDiagnostic{{
		Path:    ".pia/skills/broken/SKILL.md",
		Message: "required name is missing",
	}}

	err := execute(context.Background(), []string{"task"}, io.Discard, errorWriter{err: writeErr}, deps)
	if !errors.Is(err, writeErr) {
		t.Fatalf("execute() error = %v, want diagnostic writer failure", err)
	}
}

func TestRunProcessReportsSettlementErrorsOnlyToStderr(t *testing.T) {
	deps, session := successfulDependencies()
	session.advanceErr = errors.New("broken")
	var stdout, stderr bytes.Buffer

	code := runProcess(context.Background(), []string{"task"}, &stdout, &stderr, deps)
	if code == 0 {
		t.Fatal("runProcess() code = 0, want nonzero")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if got := stderr.String(); !strings.Contains(got, "pia:") || !strings.Contains(got, "broken") {
		t.Fatalf("stderr = %q, want command context and settlement error", got)
	}
}

func successfulDependencies() (dependencies, *fakeSession) {
	session := &fakeSession{
		info: coding.SessionInfo{WorkspacePath: "/workspace"},
		result: coding.AdvanceResult{History: []ai.Message{
			ai.AssistantMessage{
				Content:    []ai.AssistantContent{ai.TextContent{Text: "done"}},
				StopReason: ai.StopReasonStop,
			},
		}},
	}
	deps := dependencies{
		lookupEnv: environment(map[string]string{deepSeekAPIKeyEnv: "key"}),
		getwd:     func() (string, error) { return "/workspace", nil },
		newSession: func(config coding.SessionConfig) (codingSession, error) {
			session.observer = config.Observer
			return session, nil
		},
		buildTrace: coding.BuildTrace,
		writeTrace: func(string, coding.Trace) error { return nil },
	}
	return deps, session
}

type fakeSession struct {
	info       coding.SessionInfo
	result     coding.AdvanceResult
	advanceErr error
	closeErr   error
	observer   observation.Observer
	onAdvance  func()
	input      string
	closeCalls int
}

func (s *fakeSession) Info() coding.SessionInfo {
	return s.info
}

func (s *fakeSession) Advance(_ context.Context, input string) (coding.AdvanceResult, error) {
	s.input = input
	if s.onAdvance != nil {
		s.onAdvance()
	}
	return s.result, s.advanceErr
}

func (s *fakeSession) Close(context.Context) error {
	s.closeCalls++
	return s.closeErr
}

func environment(values map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}

type errorWriter struct{ err error }

func (w errorWriter) Write([]byte) (int, error) { return 0, w.err }
