//go:build darwin || linux

package main

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/yuanbohan/pia/internal/coding"
)

func TestProcessMainCancelsOnSignalsAndWritesTraceAfterSettlement(t *testing.T) {
	tests := []struct {
		name   string
		signal os.Signal
	}{
		{name: "SIGINT", signal: os.Interrupt},
		{name: "SIGTERM", signal: syscall.SIGTERM},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			commandContext, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()

			directory := t.TempDir()
			readyPath := filepath.Join(directory, "ready")
			tracePath := filepath.Join(directory, "trace")
			command := exec.CommandContext(commandContext, os.Args[0], "-test.run=^TestSignalHelperProcess$")
			command.Env = append(os.Environ(),
				"PIA_SIGNAL_HELPER=1",
				"PIA_SIGNAL_READY="+readyPath,
				"PIA_SIGNAL_WORKSPACE="+directory,
				deepSeekAPIKeyEnv+"=key",
				tracePathEnv+"="+tracePath,
			)
			if err := command.Start(); err != nil {
				t.Fatalf("start helper: %v", err)
			}
			waitForPath(t, readyPath)
			if err := command.Process.Signal(test.signal); err != nil {
				t.Fatalf("send %s: %v", test.name, err)
			}
			err := command.Wait()
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.Success() {
				t.Fatalf("helper wait error = %v, want nonzero process exit", err)
			}
			content, readErr := os.ReadFile(tracePath)
			if readErr != nil {
				t.Fatalf("read cancellation trace: %v", readErr)
			}
			if !strings.Contains(string(content), "signal received") {
				t.Fatalf("trace = %q, want cancellation evidence", content)
			}
		})
	}
}

func TestSignalHelperProcess(t *testing.T) {
	if os.Getenv("PIA_SIGNAL_HELPER") != "1" {
		return
	}
	readyPath := os.Getenv("PIA_SIGNAL_READY")
	workspacePath := os.Getenv("PIA_SIGNAL_WORKSPACE")
	deps := dependencies{
		lookupEnv: os.LookupEnv,
		getwd:     func() (string, error) { return workspacePath, nil },
		newSession: func(coding.SessionConfig) (codingSession, error) {
			return signalSession{readyPath: readyPath}, nil
		},
		buildTrace: coding.BuildTrace,
		writeTrace: func(path string, trace coding.Trace) error {
			return os.WriteFile(path, []byte(trace.SettlementError), 0o600)
		},
	}
	os.Exit(processMain([]string{"task"}, io.Discard, io.Discard, deps))
}

type signalSession struct {
	readyPath string
}

func (signalSession) Info() coding.SessionInfo {
	return coding.SessionInfo{}
}

func (s signalSession) Advance(ctx context.Context, _ string) (coding.AdvanceResult, error) {
	if err := os.WriteFile(s.readyPath, []byte("ready"), 0o600); err != nil {
		return coding.AdvanceResult{}, err
	}
	<-ctx.Done()
	return coding.AdvanceResult{}, context.Cause(ctx)
}

func (signalSession) Close(context.Context) error {
	return nil
}

func waitForPath(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}
