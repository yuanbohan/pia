//go:build darwin || linux

package bash

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func newTestTool(t *testing.T, workingDirectory string) *Tool {
	t.Helper()
	tool, err := New(Config{WorkingDirectory: workingDirectory, ShellPath: "/bin/sh"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return tool
}

func argumentsJSON(t *testing.T, command string, timeout *float64) json.RawMessage {
	t.Helper()
	input := map[string]any{"command": command}
	if timeout != nil {
		input["timeout"] = *timeout
	}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Marshal(arguments) error = %v", err)
	}
	return raw
}

func parsePID(t *testing.T, value string) int {
	t.Helper()
	pid, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		t.Fatalf("parse PID %q: %v", value, err)
	}
	return pid
}

func processExists(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

func killAndWaitForProcess(t *testing.T, pid int) {
	t.Helper()
	if processExists(pid) {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
	waitForProcessExit(t, pid)
}

func waitForProcessExit(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for processExists(pid) {
		if time.Now().After(deadline) {
			t.Fatalf("process %d is still alive", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitForFile(t *testing.T, path string) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		content, err := os.ReadFile(path)
		if err == nil && len(content) > 0 {
			return string(content)
		}
		if err != nil && !os.IsNotExist(err) {
			t.Fatalf("ReadFile(%q) error = %v", path, err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("file %q was not populated", path)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
