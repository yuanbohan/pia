package main

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/yuanbohan/pia/internal/ai"
	"github.com/yuanbohan/pia/internal/coding"
	"github.com/yuanbohan/pia/internal/observation"
)

func TestTerminalEnterRoutesWholePendingBatchAsSteering(t *testing.T) {
	t.Parallel()

	session := &terminalTestSession{steerAccepted: true}
	model := newTerminalModel(context.Background(), session, nil)
	model.active = true
	model.pending = []string{"first"}
	model.composer.SetValue("second")

	model, _ = updateTerminal(t, model, keyPress(tea.KeyEnter, 0))

	if want := [][]string{{"first", "second"}}; !reflect.DeepEqual(
		session.steered,
		want,
	) {
		t.Fatalf("TrySteer inputs = %#v, want %#v", session.steered, want)
	}
	if len(model.pending) != 0 {
		t.Fatalf("pending = %#v, want empty after accepted Steering", model.pending)
	}
	if got := model.composer.Value(); got != "" {
		t.Fatalf("composer = %q, want empty", got)
	}
}

func TestTerminalComposerAcceptsPrintableInputImmediately(t *testing.T) {
	t.Parallel()

	model := newTerminalModel(
		context.Background(),
		&terminalTestSession{},
		nil,
	)
	model, _ = updateTerminal(
		t,
		model,
		tea.KeyPressMsg(tea.Key{Code: '你', Text: "你"}),
	)

	if got := model.composer.Value(); got != "你" {
		t.Fatalf("composer = %q, want focused printable input", got)
	}
}

func TestTerminalUnavailableSteeringRetainsWholeBatchInOrder(t *testing.T) {
	t.Parallel()

	session := &terminalTestSession{}
	model := newTerminalModel(context.Background(), session, nil)
	model.active = true
	model.pending = []string{"first"}
	model.composer.SetValue("second")

	model, _ = updateTerminal(t, model, keyPress(tea.KeyEnter, 0))

	want := []string{"first", "second"}
	if !reflect.DeepEqual(model.pending, want) {
		t.Fatalf("pending = %#v, want %#v", model.pending, want)
	}
	if got := session.steered; !reflect.DeepEqual(got, [][]string{want}) {
		t.Fatalf("TrySteer inputs = %#v, want one full batch %#v", got, want)
	}
}

func TestTerminalIdleEnterStartsWholePendingBatch(t *testing.T) {
	t.Parallel()

	session := &terminalTestSession{}
	model := newTerminalModel(context.Background(), session, nil)
	model.pending = []string{"first"}
	model.composer.SetValue("second")

	model, command := updateTerminal(t, model, keyPress(tea.KeyEnter, 0))

	if !model.active {
		t.Fatal("active = false, want true")
	}
	if len(model.pending) != 0 {
		t.Fatalf("pending = %#v, want transferred batch", model.pending)
	}
	if command == nil {
		t.Fatal("Enter command = nil, want Advance command")
	}
	message := command()
	done, ok := message.(terminalAdvanceDone)
	if !ok {
		t.Fatalf("Enter command message = %T, want terminalAdvanceDone", message)
	}
	if done.err != nil {
		t.Fatalf("Advance error = %v", done.err)
	}
	if want := [][]string{{"first", "second"}}; !reflect.DeepEqual(
		session.advanced,
		want,
	) {
		t.Fatalf("Advance inputs = %#v, want %#v", session.advanced, want)
	}
}

func TestTerminalSettlementAutoDrivesOnlyAfterNormalCompletion(t *testing.T) {
	t.Parallel()

	t.Run("normal", func(t *testing.T) {
		session := &terminalTestSession{}
		model := newTerminalModel(context.Background(), session, nil)
		model.active = true
		model.pending = []string{"next"}

		model, command := updateTerminal(t, model, terminalAdvanceDone{})

		if !model.active || command == nil {
			t.Fatalf("normal settlement = (active %t, command %v), want auto Advance", model.active, command != nil)
		}
		if len(model.pending) != 0 {
			t.Fatalf("pending = %#v, want transferred batch", model.pending)
		}
	})

	t.Run("error", func(t *testing.T) {
		session := &terminalTestSession{}
		model := newTerminalModel(context.Background(), session, nil)
		model.active = true
		model.pending = []string{"next"}

		model, command := updateTerminal(
			t,
			model,
			terminalAdvanceDone{err: errors.New("provider failed")},
		)

		if model.active || command == nil {
			t.Fatalf(
				"error settlement = (active %t, command %v), want held queue plus status output",
				model.active,
				command != nil,
			)
		}
		if want := []string{"next"}; !reflect.DeepEqual(model.pending, want) {
			t.Fatalf("pending = %#v, want %#v", model.pending, want)
		}
	})

	t.Run("cancel requested", func(t *testing.T) {
		session := &terminalTestSession{}
		model := newTerminalModel(context.Background(), session, nil)
		model.active = true
		model.cancelRequested = true
		model.pending = []string{"next"}

		model, _ = updateTerminal(t, model, terminalAdvanceDone{})

		if model.active {
			t.Fatal("active = true, want canceled settlement held")
		}
		if want := []string{"next"}; !reflect.DeepEqual(model.pending, want) {
			t.Fatalf("pending = %#v, want %#v", model.pending, want)
		}
	})
}

func TestTerminalAltUpRestoresAllPendingBeforeDraft(t *testing.T) {
	t.Parallel()

	model := newTerminalModel(
		context.Background(),
		&terminalTestSession{},
		nil,
	)
	model.pending = []string{"first", "second"}
	model.composer.SetValue("draft")

	model, _ = updateTerminal(t, model, keyPress(tea.KeyUp, tea.ModAlt))

	if len(model.pending) != 0 {
		t.Fatalf("pending = %#v, want empty", model.pending)
	}
	if got, want := model.composer.Value(), "first\n\nsecond\n\ndraft"; got != want {
		t.Fatalf("composer = %q, want %q", got, want)
	}
}

func TestTerminalEscCancelsActiveAdvanceOnlyOnce(t *testing.T) {
	t.Parallel()

	session := &terminalTestSession{}
	model := newTerminalModel(context.Background(), session, nil)
	model.active = true
	model.pending = []string{"host owned"}
	model.composer.SetValue("draft")

	model, _ = updateTerminal(t, model, keyPress(tea.KeyEscape, 0))
	model, _ = updateTerminal(t, model, keyPress(tea.KeyEscape, 0))

	if session.cancelCalls != 1 {
		t.Fatalf("Cancel calls = %d, want 1", session.cancelCalls)
	}
	if got := model.composer.Value(); got != "draft" {
		t.Fatalf("composer = %q, want unchanged", got)
	}
	if want := []string{"host owned"}; !reflect.DeepEqual(model.pending, want) {
		t.Fatalf("pending = %#v, want %#v", model.pending, want)
	}
}

func TestTerminalCtrlCAndCtrlDRemainUnsupported(t *testing.T) {
	t.Parallel()

	session := &terminalTestSession{}
	model := newTerminalModel(context.Background(), session, nil)
	model.composer.SetValue("draft")

	for _, code := range []rune{'c', 'd'} {
		model, _ = updateTerminal(t, model, keyPress(code, tea.ModCtrl))
	}

	if got := model.composer.Value(); got != "draft" {
		t.Fatalf("composer = %q, want unchanged", got)
	}
	if session.cancelCalls != 0 {
		t.Fatalf("Cancel calls = %d, want 0", session.cancelCalls)
	}
}

func TestTerminalExitClosesWithoutRunningHostQueue(t *testing.T) {
	t.Parallel()

	session := &terminalTestSession{}
	model := newTerminalModel(context.Background(), session, nil)
	model.pending = []string{"ignored"}
	model.composer.SetValue("/exit")

	model, command := updateTerminal(t, model, keyPress(tea.KeyEnter, 0))

	if !model.closing {
		t.Fatal("closing = false, want true")
	}
	if len(model.pending) != 0 {
		t.Fatalf("pending = %#v, want ignored and cleared", model.pending)
	}
	if command == nil {
		t.Fatal("/exit command = nil, want Close command")
	}
	if message := command(); reflect.TypeOf(message) != reflect.TypeOf(terminalCloseDone{}) {
		t.Fatalf("/exit command message = %T, want terminalCloseDone", message)
	}
	if session.closeCalls != 1 {
		t.Fatalf("Close calls = %d, want 1", session.closeCalls)
	}
	if len(session.advanced) != 0 {
		t.Fatalf("Advance calls = %#v, want none", session.advanced)
	}
}

func TestTerminalExitWaitsForActiveAdvanceResultAndOutput(t *testing.T) {
	session := &terminalTestSession{}
	model := newTerminalModel(t.Context(), session, nil)
	model.active = true
	model.closing = true

	updated, command := model.Update(terminalCloseDone{})
	model = updated.(terminalModel)
	if command != nil {
		t.Fatal("close completion command is non-nil while Advance is active")
	}
	if !model.closeCompleted || !model.active {
		t.Fatalf(
			"close-first state = (completed %t, active %t), want (true, true)",
			model.closeCompleted,
			model.active,
		)
	}

	history := []ai.Message{
		ai.UserMessage{Content: "task"},
		ai.AssistantMessage{
			Content:    []ai.AssistantContent{ai.TextContent{Text: "done"}},
			StopReason: ai.StopReasonStop,
		},
	}
	updated, command = model.Update(terminalAdvanceDone{
		result:    coding.AdvanceResult{History: history},
		finalText: "done",
	})
	model = updated.(terminalModel)
	if command == nil {
		t.Fatal("Advance settlement command is nil, want persistent output")
	}
	if model.active {
		t.Fatal("active = true after Advance settlement")
	}
	if got := model.latestResult.FinalText(); got != "done" {
		t.Fatalf("latest result = %q, want %q", got, "done")
	}
	if !model.outputActive {
		t.Fatal("outputActive = false, want final output before quit")
	}

	updated, command = model.Update(terminalOutputDone{})
	model = updated.(terminalModel)
	if command == nil {
		t.Fatal("output completion command is nil, want quit")
	}
	if _, ok := command().(tea.QuitMsg); !ok {
		t.Fatal("output completion command does not quit")
	}
	if model.outputActive || len(model.outputQueue) != 0 {
		t.Fatalf(
			"settled output = (active %t, queued %d), want empty",
			model.outputActive,
			len(model.outputQueue),
		)
	}
}

func TestTerminalExitStopsAfterCloseGraceExpires(t *testing.T) {
	t.Parallel()

	model := newTerminalModel(
		context.Background(),
		&terminalTestSession{},
		nil,
	)
	model.active = true
	model.closing = true

	updated, command := model.Update(terminalCloseDone{
		err:       context.DeadlineExceeded,
		forceQuit: true,
	})
	model = updated.(terminalModel)
	if command == nil {
		t.Fatal("grace-expired close command is nil, want quit")
	}
	if _, ok := command().(tea.QuitMsg); !ok {
		t.Fatal("grace-expired close command does not quit")
	}
	if !model.active || !model.closeCompleted {
		t.Fatalf(
			"grace-expired state = (active %t, completed %t), want unclean active exit",
			model.active,
			model.closeCompleted,
		)
	}
}

func TestTerminalObservationProjectionIsSemanticAndBounded(t *testing.T) {
	t.Parallel()

	tests := []struct {
		event observation.Event
		want  string
	}{
		{
			event: observation.Run{
				Phase: observation.PhaseStarted,
				Mode:  observation.RunModeInput,
			},
			want: "run input started",
		},
		{
			event: observation.Turn{
				Phase:   observation.PhaseSettled,
				Outcome: observation.OutcomeSuccess,
			},
			want: "turn settled (success)",
		},
		{
			event: observation.NewToolStarted(0, "read", "Read main.go"),
			want:  "tool started: Read main.go",
		},
		{
			event: observation.Compaction{
				Phase:  observation.PhaseStarted,
				Reason: observation.CompactionReasonOverflow,
			},
			want: "compaction overflow started",
		},
	}

	for _, test := range tests {
		if got := terminalObservationLine(test.event); got != test.want {
			t.Fatalf("terminalObservationLine(%#v) = %q, want %q", test.event, got, test.want)
		}
	}
	if got := terminalObservationLine(observation.Message{
		Role: observation.MessageRoleAssistant,
	}); got != "" {
		t.Fatalf("assistant Message projection = %q, want hidden content-free event", got)
	}
}

func TestTerminalObservationWaitStopsWhenSourceCloses(t *testing.T) {
	t.Parallel()

	events := make(chan observation.Event)
	close(events)
	command := waitForTerminalObservation(t.Context(), events)
	if command == nil {
		t.Fatal("observation wait command = nil")
	}
	if _, ok := command().(terminalObservationStopped); !ok {
		t.Fatal("closed observation source did not stop the wait loop")
	}
}

func TestFinalTextSinceUsesOnlyCurrentAdvanceHistorySuffix(t *testing.T) {
	t.Parallel()

	previous := coding.AdvanceResult{History: []ai.Message{
		ai.UserMessage{Content: "earlier"},
		ai.AssistantMessage{
			Content:    []ai.AssistantContent{ai.TextContent{Text: "old answer"}},
			StopReason: ai.StopReasonStop,
		},
	}}
	withoutTerminal := coding.AdvanceResult{
		History: append(
			append([]ai.Message(nil), previous.History...),
			ai.UserMessage{Content: "canceled input"},
		),
	}
	if got := finalTextSince(withoutTerminal, len(previous.History)); got != "" {
		t.Fatalf("canceled Advance text = %q, want no repeated prior answer", got)
	}

	currentAssistant := ai.AssistantMessage{
		Content:    []ai.AssistantContent{ai.TextContent{Text: "current answer"}},
		StopReason: ai.StopReasonStop,
	}
	withPendingSteering := coding.AdvanceResult{
		History: append(
			append([]ai.Message(nil), previous.History...),
			ai.UserMessage{Content: "current input"},
			currentAssistant,
			ai.UserMessage{Content: "accepted after terminal"},
		),
	}
	if got := finalTextSince(withPendingSteering, len(previous.History)); got != "current answer" {
		t.Fatalf("current Advance text = %q, want current terminal assistant", got)
	}
}

func TestTerminalErrorTextIsSingleLineBoundedAndValidUTF8(t *testing.T) {
	t.Parallel()

	text := terminalErrorText(errors.New(
		strings.Repeat("你", terminalErrorMaxBytes) + "\nsecret",
	))
	if len(text) > terminalErrorMaxBytes {
		t.Fatalf("terminal error bytes = %d, want <= %d", len(text), terminalErrorMaxBytes)
	}
	if !utf8.ValidString(text) {
		t.Fatalf("terminal error is invalid UTF-8: %q", text)
	}
	if strings.Contains(text, "\n") || !strings.HasSuffix(text, terminalTruncation) {
		t.Fatalf("terminal error = %q, want escaped and truncated single line", text)
	}
}

func updateTerminal(
	t *testing.T,
	model terminalModel,
	message tea.Msg,
) (terminalModel, tea.Cmd) {
	t.Helper()

	updated, command := model.Update(message)
	terminal, ok := updated.(terminalModel)
	if !ok {
		t.Fatalf("updated model = %T, want terminalModel", updated)
	}
	return terminal, command
}

func keyPress(code rune, modifier tea.KeyMod) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: code, Mod: modifier})
}

type terminalTestSession struct {
	result        coding.AdvanceResult
	advanceErr    error
	steerAccepted bool
	steerErr      error
	closeErr      error
	advanced      [][]string
	steered       [][]string
	cancelCalls   int
	closeCalls    int
}

func (*terminalTestSession) Info() coding.SessionInfo {
	return coding.SessionInfo{}
}

func (session *terminalTestSession) Advance(
	_ context.Context,
	inputs []string,
) (coding.AdvanceResult, error) {
	session.advanced = append(session.advanced, append([]string(nil), inputs...))
	return session.result, session.advanceErr
}

func (session *terminalTestSession) TrySteer(inputs []string) (bool, error) {
	session.steered = append(session.steered, append([]string(nil), inputs...))
	return session.steerAccepted, session.steerErr
}

func (session *terminalTestSession) Cancel() {
	session.cancelCalls++
}

func (session *terminalTestSession) Close(context.Context) error {
	session.closeCalls++
	return session.closeErr
}
