package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	tea "charm.land/bubbletea/v2"

	"github.com/yuanbohan/pia/internal/coding"
	"github.com/yuanbohan/pia/internal/observation"
)

const terminalObservationBuffer = 256

func executeInteractive(
	ctx context.Context,
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
	deps dependencies,
) error {
	runtimeConfig, err := loadRuntimeConfiguration(deps)
	if err != nil {
		return err
	}

	observationCtx, stopObservations := context.WithCancel(ctx)
	events := make(chan observation.Event, terminalObservationBuffer)
	observer := observation.Observer(func(event observation.Event) {
		select {
		case events <- event:
		case <-observationCtx.Done():
		}
	})

	session, err := deps.newSession(coding.SessionConfig{
		WorkspacePath:  runtimeConfig.workspacePath,
		DeepSeekAPIKey: runtimeConfig.apiKey,
		Observer:       observer,
	})
	if err != nil {
		stopObservations()
		return err
	}
	info := session.Info()
	if err := writeSkillDiagnostics(stderr, info.SkillDiagnostics); err != nil {
		stopObservations()
		closeCtx, cancel := context.WithTimeout(
			context.WithoutCancel(ctx),
			terminalCloseGrace,
		)
		defer cancel()
		return errors.Join(err, session.Close(closeCtx))
	}

	model := newTerminalModel(ctx, session, events)
	model.observationCtx = observationCtx
	program := tea.NewProgram(
		model,
		tea.WithContext(ctx),
		tea.WithInput(stdin),
		tea.WithOutput(stdout),
		tea.WithoutSignalHandler(),
	)
	finalModel, programErr := program.Run()
	stopObservations()

	final, ok := finalModel.(terminalModel)
	if !ok {
		final = model
		if programErr == nil {
			programErr = fmt.Errorf(
				"interactive terminal returned model %T",
				finalModel,
			)
		}
	}

	closeErr := final.closeErr
	if !final.closeCompleted {
		closeCtx, cancel := context.WithTimeout(
			context.WithoutCancel(ctx),
			terminalCloseGrace,
		)
		closeErr = session.Close(closeCtx)
		cancel()
	}

	evidenceErrs := append([]error(nil), final.advanceErrs...)
	evidenceErrs = append(evidenceErrs, programErr, closeErr)
	settlementErr := errors.Join(evidenceErrs...)
	var traceErr error
	if runtimeConfig.tracePath != "" {
		trace, buildErr := deps.buildTrace(
			info,
			final.latestResult,
			settlementErr,
		)
		if buildErr != nil {
			traceErr = fmt.Errorf("build requested trace: %w", buildErr)
		} else if writeErr := deps.writeTrace(
			runtimeConfig.tracePath,
			trace,
		); writeErr != nil {
			traceErr = fmt.Errorf("write requested trace: %w", writeErr)
		}
	}

	// Advance failures that the operator recovered from remain trace evidence,
	// but do not make a later clean /exit fail.
	return errors.Join(programErr, closeErr, traceErr)
}

func writeSkillDiagnostics(
	writer io.Writer,
	diagnostics []coding.SkillDiagnostic,
) error {
	for _, diagnostic := range diagnostics {
		var err error
		if diagnostic.Path == "" {
			_, err = fmt.Fprintf(
				writer,
				"pia: warning: %q\n",
				diagnostic.Message,
			)
		} else {
			_, err = fmt.Fprintf(
				writer,
				"pia: warning: %q: %q\n",
				diagnostic.Path,
				diagnostic.Message,
			)
		}
		if err != nil {
			return fmt.Errorf("write Skill diagnostic: %w", err)
		}
	}
	return nil
}
