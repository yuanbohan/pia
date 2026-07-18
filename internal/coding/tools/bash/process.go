package bash

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"time"
)

const exitOutputIdleGrace = 100 * time.Millisecond

type commandConfig struct {
	shell            string
	workingDirectory string
	command          string
	timeout          *time.Duration
}

type exitStatusError struct {
	code  int
	cause error
}

func (e *exitStatusError) Error() string {
	if e.code >= 0 {
		return fmt.Sprintf("bash: command exited with code %d", e.code)
	}
	return fmt.Sprintf("bash: command terminated: %v", e.cause)
}

func (e *exitStatusError) Unwrap() error {
	return e.cause
}

type streamEvent struct {
	data []byte
	done bool
	err  error
}

func runCommand(ctx context.Context, config commandConfig) (outputSnapshot, error) {
	output := newOutputAccumulator()
	if cause := context.Cause(ctx); cause != nil {
		snapshot, _ := output.finish()
		return snapshot, cause
	}

	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		snapshot, _ := output.finish()
		return snapshot, fmt.Errorf("bash: create stdout pipe: %w", err)
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		_ = stdoutReader.Close()
		_ = stdoutWriter.Close()
		snapshot, _ := output.finish()
		return snapshot, fmt.Errorf("bash: create stderr pipe: %w", err)
	}

	// CommandContext covers the direct shell as a final fallback. The explicit
	// process-group kill below is still required because CommandContext alone
	// does not terminate grandchildren started by shell pipelines or scripts.
	command := exec.CommandContext(ctx, config.shell, "-c", config.command)
	command.Dir = config.workingDirectory
	// Cmd.Environ preserves the complete parent environment and, because Dir is
	// already set, updates PWD for the workspace. No credential filtering is
	// implied by this trusted local-CLI contract.
	command.Env = command.Environ()
	command.Stdin = nil
	command.Stdout = stdoutWriter
	command.Stderr = stderrWriter
	if err := configureProcessGroup(command); err != nil {
		closePipeSet(stdoutReader, stdoutWriter, stderrReader, stderrWriter)
		snapshot, _ := output.finish()
		return snapshot, fmt.Errorf("bash: configure process group: %w", err)
	}
	if err := command.Start(); err != nil {
		closePipeSet(stdoutReader, stdoutWriter, stderrReader, stderrWriter)
		snapshot, _ := output.finish()
		if cause := context.Cause(ctx); cause != nil {
			return snapshot, cause
		}
		return snapshot, fmt.Errorf("bash: start shell %q: %w", config.shell, err)
	}
	// The parent must close its writer copies immediately; otherwise EOF would
	// never identify that the shell and all pipe-inheriting descendants closed.
	_ = stdoutWriter.Close()
	_ = stderrWriter.Close()

	events := make(chan streamEvent, 16)
	go drainStream(stdoutReader, events)
	go drainStream(stderrReader, events)
	wait := make(chan error, 1)
	go func() { wait <- command.Wait() }()

	var timeoutTimer *time.Timer
	var timeout <-chan time.Time
	if config.timeout != nil {
		timeoutTimer = time.NewTimer(*config.timeout)
		timeout = timeoutTimer.C
	}
	defer func() {
		if timeoutTimer != nil {
			timeoutTimer.Stop()
		}
	}()

	var idleTimer *time.Timer
	var idle <-chan time.Time
	defer func() {
		if idleTimer != nil {
			idleTimer.Stop()
		}
	}()

	contextDone := ctx.Done()
	readersRemaining := 2
	var waitErr error
	shellExited := false
	readersClosed := false
	var terminalErr error
	acceptingOutput := true

	armIdle := func() {
		if idleTimer == nil {
			idleTimer = time.NewTimer(exitOutputIdleGrace)
		} else {
			if !idleTimer.Stop() {
				select {
				case <-idleTimer.C:
				default:
				}
			}
			idleTimer.Reset(exitOutputIdleGrace)
		}
		idle = idleTimer.C
	}
	closeReaders := func() {
		if readersClosed {
			return
		}
		readersClosed = true
		_ = stdoutReader.Close()
		_ = stderrReader.Close()
	}
	killFor := func(reason error) {
		if terminalErr != nil {
			return
		}
		terminalErr = reason
		if err := killProcessGroup(command); err != nil {
			terminalErr = errors.Join(terminalErr, fmt.Errorf("bash: kill process group: %w", err))
		}
	}

	for !shellExited || readersRemaining > 0 {
		select {
		case event := <-events:
			if event.done {
				readersRemaining--
				if event.err != nil {
					killFor(fmt.Errorf("bash: read command output: %w", event.err))
				}
				if readersRemaining == 0 {
					idle = nil
				}
				continue
			}
			if acceptingOutput {
				if err := output.append(event.data); err != nil {
					acceptingOutput = false
					killFor(err)
				}
			}
			if shellExited && readersRemaining > 0 {
				// Match frozen Pi's output-idle grace: quiet inherited handles
				// release the call, while every new chunk re-arms the grace so
				// actively arriving tail output is not cut off.
				armIdle()
			}
		case waitErr = <-wait:
			shellExited = true
			if readersRemaining > 0 {
				armIdle()
			}
		case <-idle:
			idle = nil
			closeReaders()
		case <-contextDone:
			cause := context.Cause(ctx)
			killFor(fmt.Errorf("bash: command canceled: %w", cause))
			contextDone = nil
		case <-timeout:
			killFor(fmt.Errorf(
				"bash: command timed out after %s seconds",
				strconv.FormatFloat(config.timeout.Seconds(), 'f', -1, 64),
			))
			timeout = nil
		}
	}
	closeReaders()

	snapshot, finishErr := output.finish()
	if cause := context.Cause(ctx); cause != nil && terminalErr == nil {
		// CommandContext may have already stopped the direct shell before this
		// loop observes ctx.Done. Kill the group here as well so a descendant
		// that redirected both output pipes cannot escape Run cancellation.
		killFor(fmt.Errorf("bash: command canceled: %w", cause))
	}
	if terminalErr != nil {
		return snapshot, errors.Join(terminalErr, finishErr)
	}
	if finishErr != nil {
		return snapshot, finishErr
	}
	if waitErr == nil {
		return snapshot, nil
	}
	var processExit *exec.ExitError
	if errors.As(waitErr, &processExit) {
		return snapshot, &exitStatusError{code: processExit.ExitCode(), cause: waitErr}
	}
	return snapshot, fmt.Errorf("bash: wait for shell: %w", waitErr)
}

func drainStream(reader *os.File, events chan<- streamEvent) {
	buffer := make([]byte, 32<<10)
	for {
		count, err := reader.Read(buffer)
		if count > 0 {
			chunk := make([]byte, count)
			copy(chunk, buffer[:count])
			events <- streamEvent{data: chunk}
		}
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) {
				err = nil
			}
			events <- streamEvent{done: true, err: err}
			return
		}
	}
}

func closePipeSet(files ...*os.File) {
	for _, file := range files {
		if file != nil {
			_ = file.Close()
		}
	}
}
