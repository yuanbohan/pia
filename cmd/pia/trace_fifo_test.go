//go:build darwin || linux

package main

import (
	"path/filepath"
	"syscall"
	"testing"

	"github.com/yuanbohan/pia/internal/coding"
)

func TestWriteTraceFileRejectsExistingFIFOWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.json")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("create trace FIFO: %v", err)
	}
	if err := writeTraceFile(path, coding.Trace{}); err == nil {
		t.Fatal("write trace unexpectedly accepted FIFO")
	}
}
