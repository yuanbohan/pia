package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"

	"github.com/yuanbohan/pia/internal/coding"
	"github.com/yuanbohan/pia/internal/observation"
)

const (
	terminalCloseGrace    = 5 * time.Second
	terminalErrorMaxBytes = 2 * 1024
	terminalTruncation    = "..."
)

type terminalModel struct {
	ctx            context.Context
	observationCtx context.Context
	session        codingSession
	observations   <-chan observation.Event
	composer       textarea.Model

	pending         []string
	active          bool
	cancelRequested bool
	closing         bool
	closeCompleted  bool

	historySize  int
	latestResult coding.AdvanceResult
	advanceErrs  []error
	closeErr     error

	outputQueue  []string
	outputActive bool
}

type terminalObservation struct {
	event observation.Event
}

type terminalObservationStopped struct{}

type terminalAdvanceDone struct {
	result    coding.AdvanceResult
	err       error
	finalText string
}

type terminalCloseDone struct {
	err       error
	forceQuit bool
}

type terminalOutputDone struct{}

func newTerminalModel(
	ctx context.Context,
	session codingSession,
	observations <-chan observation.Event,
) terminalModel {
	composer := textarea.New()
	composer.Placeholder = "Message Pia (/exit to close)"
	composer.Prompt = "> "
	composer.ShowLineNumbers = false
	composer.DynamicHeight = true
	composer.MinHeight = 1
	composer.MaxHeight = 6
	composer.SetHeight(1)
	composer.SetWidth(80)
	_ = composer.Focus()

	return terminalModel{
		ctx:            ctx,
		observationCtx: ctx,
		session:        session,
		observations:   observations,
		composer:       composer,
	}
}

func (model terminalModel) Init() tea.Cmd {
	return tea.Batch(
		model.composer.Focus(),
		waitForTerminalObservation(model.observationCtx, model.observations),
	)
}

func (model terminalModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.KeyPressMsg:
		return model.handleKey(message)
	case tea.WindowSizeMsg:
		width := message.Width - 2
		if width < 1 {
			width = 1
		}
		model.composer.SetWidth(width)
		return model, nil
	case terminalObservation:
		line := terminalObservationLine(message.event)
		next := waitForTerminalObservation(
			model.observationCtx,
			model.observations,
		)
		if line == "" {
			return model, next
		}
		var output tea.Cmd
		model, output = model.enqueueOutput("pia: " + escapeLine(line))
		return model, tea.Batch(output, next)
	case terminalObservationStopped:
		return model, nil
	case terminalAdvanceDone:
		return model.settleAdvance(message)
	case terminalCloseDone:
		model.closeCompleted = true
		model.closeErr = message.err
		if message.forceQuit {
			return model, tea.Quit
		}
		return model.maybeQuit()
	case terminalOutputDone:
		model.outputActive = false
		var output tea.Cmd
		model, output = model.startNextOutput()
		if output != nil {
			return model, output
		}
		return model.maybeQuit()
	default:
		var command tea.Cmd
		model.composer, command = model.composer.Update(message)
		return model, command
	}
}

func (model terminalModel) View() tea.View {
	state := "idle"
	switch {
	case model.closing:
		state = "closing"
	case model.active && model.cancelRequested:
		state = "canceling"
	case model.active:
		state = "running"
	}
	content := fmt.Sprintf(
		"pia [%s, queued:%d]\n%s\nEnter send | Alt+Up edit queued | Esc cancel | /exit close",
		state,
		len(model.pending),
		model.composer.View(),
	)
	return tea.NewView(content)
}

func (model terminalModel) handleKey(
	message tea.KeyPressMsg,
) (tea.Model, tea.Cmd) {
	if model.closing {
		return model, nil
	}

	switch message.Keystroke() {
	case "ctrl+c", "ctrl+d":
		return model, nil
	case "esc":
		if model.active && !model.cancelRequested {
			model.cancelRequested = true
			model.session.Cancel()
			return model.enqueueOutput("pia: cancel requested")
		}
		return model, nil
	case "alt+up":
		return model.restorePending()
	case "enter":
		return model.submit()
	default:
		var command tea.Cmd
		model.composer, command = model.composer.Update(message)
		return model, command
	}
}

func (model terminalModel) submit() (tea.Model, tea.Cmd) {
	draft := model.composer.Value()
	if strings.TrimSpace(draft) == "/exit" {
		model.composer.SetValue("")
		model.pending = nil
		model.closing = true
		return model, model.closeCommand()
	}

	if strings.TrimSpace(draft) != "" {
		model.pending = append(model.pending, strings.Clone(draft))
		model.composer.SetValue("")
	}
	if len(model.pending) == 0 {
		return model, nil
	}
	if model.active {
		return model.routePendingSteering()
	}
	return model.startPendingAdvance()
}

func (model terminalModel) routePendingSteering() (tea.Model, tea.Cmd) {
	accepted, err := model.session.TrySteer(model.pending)
	switch {
	case err != nil:
		return model.enqueueOutput(fmt.Sprintf(
			"pia: steering failed; %d submission(s) remain queued: %s",
			len(model.pending),
			terminalErrorText(err),
		))
	case !accepted:
		return model.enqueueOutput(fmt.Sprintf(
			"pia: steering unavailable; %d submission(s) queued",
			len(model.pending),
		))
	default:
		count := len(model.pending)
		model.pending = nil
		return model.enqueueOutput(fmt.Sprintf(
			"pia: steering accepted: %d submission(s)",
			count,
		))
	}
}

func (model terminalModel) startPendingAdvance() (terminalModel, tea.Cmd) {
	batch := model.pending
	model.pending = nil
	model.active = true
	model.cancelRequested = false
	historyStart := model.historySize
	session := model.session
	ctx := model.ctx

	return model, func() tea.Msg {
		result, err := session.Advance(ctx, batch)
		return terminalAdvanceDone{
			result:    result,
			err:       err,
			finalText: finalTextSince(result, historyStart),
		}
	}
}

func (model terminalModel) restorePending() (tea.Model, tea.Cmd) {
	if len(model.pending) == 0 {
		return model, nil
	}

	restored := strings.Join(model.pending, "\n\n")
	if draft := model.composer.Value(); draft != "" {
		restored += "\n\n" + draft
	}
	model.composer.SetValue(restored)
	model.pending = nil
	return model.enqueueOutput("pia: queued submissions restored to composer")
}

func (model terminalModel) settleAdvance(
	message terminalAdvanceDone,
) (tea.Model, tea.Cmd) {
	wasCanceled := model.cancelRequested
	model.active = false
	model.cancelRequested = false
	model.latestResult = message.result
	model.historySize = len(message.result.History)

	var line string
	switch {
	case message.err != nil:
		model.advanceErrs = append(model.advanceErrs, message.err)
		line = fmt.Sprintf(
			"pia: advance failed: %s",
			terminalErrorText(message.err),
		)
	case message.finalText != "":
		line = "assistant:\n" + safeTerminalText(message.finalText)
	default:
		line = "pia: advance settled"
	}
	var output tea.Cmd
	model, output = model.enqueueOutput(line)

	if model.closing {
		var quit tea.Cmd
		model, quit = model.maybeQuit()
		return model, tea.Batch(output, quit)
	}
	if message.err != nil || wasCanceled || len(model.pending) == 0 {
		return model, output
	}

	var advance tea.Cmd
	model, advance = model.startPendingAdvance()
	return model, tea.Batch(output, advance)
}

func (model terminalModel) enqueueOutput(line string) (terminalModel, tea.Cmd) {
	model.outputQueue = append(model.outputQueue, line)
	if model.outputActive {
		return model, nil
	}
	return model.startNextOutput()
}

func (model terminalModel) startNextOutput() (terminalModel, tea.Cmd) {
	if len(model.outputQueue) == 0 {
		return model, nil
	}
	line := model.outputQueue[0]
	if len(model.outputQueue) == 1 {
		model.outputQueue = nil
	} else {
		model.outputQueue = model.outputQueue[1:]
	}
	model.outputActive = true
	return model, tea.Sequence(
		tea.Println(line),
		func() tea.Msg { return terminalOutputDone{} },
	)
}

func (model terminalModel) maybeQuit() (terminalModel, tea.Cmd) {
	if model.closing &&
		model.closeCompleted &&
		!model.active &&
		!model.outputActive &&
		len(model.outputQueue) == 0 {
		return model, tea.Quit
	}
	return model, nil
}

func (model terminalModel) closeCommand() tea.Cmd {
	session := model.session
	parent := context.WithoutCancel(model.ctx)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(
			parent,
			terminalCloseGrace,
		)
		defer cancel()
		err := session.Close(ctx)
		cause := context.Cause(ctx)
		return terminalCloseDone{
			err:       err,
			forceQuit: cause != nil && errors.Is(err, cause),
		}
	}
}

func waitForTerminalObservation(
	ctx context.Context,
	events <-chan observation.Event,
) tea.Cmd {
	if events == nil {
		return nil
	}
	return func() tea.Msg {
		select {
		case event, ok := <-events:
			if !ok {
				return terminalObservationStopped{}
			}
			return terminalObservation{event: event}
		case <-ctx.Done():
			return terminalObservationStopped{}
		}
	}
}

func terminalObservationLine(event observation.Event) string {
	switch event := event.(type) {
	case observation.Advance:
		if event.Phase == observation.PhaseStarted {
			return "advance started"
		}
		return fmt.Sprintf("advance settled (%s)", event.Outcome)
	case observation.Run:
		if event.Phase == observation.PhaseStarted {
			return fmt.Sprintf("run %s started", event.Mode)
		}
		return fmt.Sprintf("run settled (%s)", event.Outcome)
	case observation.Turn:
		if event.Phase == observation.PhaseStarted {
			return "turn started"
		}
		return fmt.Sprintf("turn settled (%s)", event.Outcome)
	case observation.Tool:
		summary := event.Summary
		if summary == "" {
			summary = event.Name
		}
		if event.Phase == observation.PhaseStarted {
			return "tool started: " + summary
		}
		return fmt.Sprintf("tool settled (%s): %s", event.Outcome, summary)
	case observation.Compaction:
		if event.Phase == observation.PhaseStarted {
			return fmt.Sprintf("compaction %s started", event.Reason)
		}
		return fmt.Sprintf(
			"compaction %s settled (%s)",
			event.Reason,
			event.Outcome,
		)
	default:
		return ""
	}
}

func finalTextSince(result coding.AdvanceResult, historyStart int) string {
	if historyStart < 0 || historyStart > len(result.History) {
		historyStart = 0
	}
	return coding.AdvanceResult{
		History: result.History[historyStart:],
	}.FinalText()
}

func safeTerminalText(value string) string {
	lines := strings.Split(value, "\n")
	for index := range lines {
		lines[index] = escapeLine(lines[index])
	}
	return strings.Join(lines, "\n")
}

func terminalErrorText(err error) string {
	if err == nil {
		return ""
	}
	value := escapeLine(err.Error())
	if len(value) <= terminalErrorMaxBytes {
		return value
	}
	end := terminalErrorMaxBytes - len(terminalTruncation)
	for end > 0 && !utf8.ValidString(value[:end]) {
		end--
	}
	return value[:end] + terminalTruncation
}
