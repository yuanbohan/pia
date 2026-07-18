//go:build darwin || linux

package bash

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExecuteDoesNotStartAfterCancellation(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	tool := newTestTool(t, workspace)
	cause := errors.New("stop before start")
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(cause)

	_, err := tool.Execute(ctx, argumentsJSON(t, `touch should-not-exist`, nil))
	if !errors.Is(err, cause) {
		t.Fatalf("Execute() error = %v, want cancellation cause", err)
	}
	if _, statErr := os.Stat(filepath.Join(workspace, "should-not-exist")); !os.IsNotExist(statErr) {
		t.Fatalf("Stat(side effect) error = %v, want file not to exist", statErr)
	}
}

func TestExecuteTimeoutKillsOriginalProcessGroup(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	tool := newTestTool(t, workspace)
	timeout := 0.5
	result, err := tool.Execute(context.Background(), argumentsJSON(t,
		`sh -c 'sleep 30 & grandchild=$!; printf "%s" "$grandchild" > grandchild.pid; wait' & child=$!; printf '%s' "$child" > child.pid; wait`, &timeout))
	if err == nil || !strings.Contains(err.Error(), "timed out after 0.5 seconds") {
		t.Fatalf("Execute() error = %v, want timeout", err)
	}
	if result != "" {
		t.Fatalf("Execute() result = %q, want no command output", result)
	}

	childPID := parsePID(t, waitForFile(t, filepath.Join(workspace, "child.pid")))
	childNeedsCleanup := true
	t.Cleanup(func() {
		if childNeedsCleanup {
			killAndWaitForProcess(t, childPID)
		}
	})
	grandchildPID := parsePID(t, waitForFile(t, filepath.Join(workspace, "grandchild.pid")))
	grandchildNeedsCleanup := true
	t.Cleanup(func() {
		if grandchildNeedsCleanup {
			killAndWaitForProcess(t, grandchildPID)
		}
	})
	waitForProcessExit(t, childPID)
	childNeedsCleanup = false
	waitForProcessExit(t, grandchildPID)
	grandchildNeedsCleanup = false
}

func TestExecuteCancellationKillsOriginalProcessGroupAndPreservesCause(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	tool := newTestTool(t, workspace)
	cause := errors.New("operator canceled")
	ctx, cancel := context.WithCancelCause(context.Background())
	type outcome struct {
		result string
		err    error
	}
	done := make(chan outcome, 1)
	input := argumentsJSON(t,
		`printf 'started\n'; sleep 30 >/dev/null 2>&1 & child=$!; printf '%s' "$child" > child.pid; wait`, nil)
	go func() {
		result, err := tool.Execute(ctx, input)
		done <- outcome{result: result, err: err}
	}()

	childPID := parsePID(t, waitForFile(t, filepath.Join(workspace, "child.pid")))
	childNeedsCleanup := true
	t.Cleanup(func() {
		if childNeedsCleanup {
			killAndWaitForProcess(t, childPID)
		}
	})
	cancel(cause)

	select {
	case got := <-done:
		if !errors.Is(got.err, cause) {
			t.Fatalf("Execute() error = %v, want cancellation cause", got.err)
		}
		if got.result != "started\n" {
			t.Fatalf("Execute() result = %q, want output produced before cancellation", got.result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Execute() did not settle after cancellation")
	}
	waitForProcessExit(t, childPID)
	childNeedsCleanup = false
}

func TestExecuteLeavesQuietBackgroundProcessRunningAfterNormalShellExit(t *testing.T) {
	t.Parallel()

	tool := newTestTool(t, t.TempDir())
	started := time.Now()
	result, err := tool.Execute(context.Background(), argumentsJSON(t,
		`sleep 30 & printf '%s\n' "$!"`, nil))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("Execute() elapsed = %s, want quiet-pipe idle release", elapsed)
	}

	pid := parsePID(t, result)
	if !processExists(pid) {
		t.Fatalf("background process %d exited, want it to survive normal shell exit", pid)
	}
	t.Cleanup(func() { killAndWaitForProcess(t, pid) })
}

func TestExecuteDrainsActiveDescendantOutputAfterShellExit(t *testing.T) {
	t.Parallel()

	tool := newTestTool(t, t.TempDir())
	result, err := tool.Execute(context.Background(), argumentsJSON(t,
		`(i=1; while [ "$i" -le 5 ]; do printf 'line-%s\n' "$i"; i=$((i + 1)); sleep 0.03; done) &`, nil))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result, "line-1\n") || !strings.Contains(result, "line-5\n") {
		t.Fatalf("Execute() result = %q, want complete descendant output", result)
	}
}
