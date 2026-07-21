package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yuanbohan/pia/internal/coding"
)

func TestWriteTraceFileCreatesPrivateJSONWithoutClobbering(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.json")
	trace := coding.Trace{Workspace: "/workspace", RunError: "failed"}
	if err := writeTraceFile(path, trace); err != nil {
		t.Fatalf("write trace: %v", err)
	}

	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("inspect trace: %v", err)
	}
	if got := info.Mode().Perm(); got&0o077 != 0 {
		t.Fatalf("trace permissions = %04o, want no group or other access", got)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}
	var decoded coding.Trace
	if err := json.Unmarshal(content, &decoded); err != nil {
		t.Fatalf("decode trace: %v\n%s", err, content)
	}
	if decoded.Workspace != trace.Workspace || decoded.RunError != trace.RunError {
		t.Fatalf("decoded trace = %#v, want %#v", decoded, trace)
	}

	if err := writeTraceFile(path, coding.Trace{Workspace: "replacement"}); err == nil {
		t.Fatal("second trace write unexpectedly replaced existing file")
	}
	contentAfter, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read retained trace: %v", err)
	}
	if string(contentAfter) != string(content) {
		t.Fatal("existing trace content changed after no-clobber failure")
	}
}

func TestWriteTraceFileRejectsExistingNonRegularTargets(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, path string)
	}{
		{
			name: "directory",
			setup: func(t *testing.T, path string) {
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatalf("create directory: %v", err)
				}
			},
		},
		{
			name: "symlink",
			setup: func(t *testing.T, path string) {
				target := filepath.Join(filepath.Dir(path), "target")
				if err := os.WriteFile(target, []byte("target content"), 0o600); err != nil {
					t.Fatalf("write symlink target: %v", err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatalf("create symlink: %v", err)
				}
			},
		},
		{
			name: "dangling symlink",
			setup: func(t *testing.T, path string) {
				if err := os.Symlink("missing", path); err != nil {
					t.Fatalf("create dangling symlink: %v", err)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "trace.json")
			test.setup(t, path)
			before, err := os.Lstat(path)
			if err != nil {
				t.Fatalf("inspect target before write: %v", err)
			}

			if err := writeTraceFile(path, coding.Trace{}); err == nil {
				t.Fatal("write trace unexpectedly accepted existing target")
			}
			after, err := os.Lstat(path)
			if err != nil {
				t.Fatalf("inspect target after write: %v", err)
			}
			if before.Mode().Type() != after.Mode().Type() {
				t.Fatalf("target type changed from %v to %v", before.Mode(), after.Mode())
			}
		})
	}
}

func TestWriteTraceFileDoesNotFollowExistingSymlink(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target")
	path := filepath.Join(directory, "trace.json")
	if err := os.WriteFile(target, []byte("target content"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	if err := writeTraceFile(path, coding.Trace{Workspace: "replacement"}); err == nil {
		t.Fatal("write trace unexpectedly followed existing symlink")
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read symlink target: %v", err)
	}
	if got, want := string(content), "target content"; got != want {
		t.Fatalf("symlink target = %q, want %q", got, want)
	}
}

func TestWriteTraceFileEncodeFailureCreatesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.json")
	trace := coding.Trace{Tools: []coding.TraceTool{{Parameters: json.RawMessage(`{`)}}}
	if err := writeTraceFile(path, trace); err == nil {
		t.Fatal("write trace unexpectedly accepted invalid JSON payload")
	}
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("trace Lstat() error = %v, want not-exist", err)
	}
}

func TestWriteTraceFileDoesNotCreateParentDirectories(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "missing", "nested")
	path := filepath.Join(parent, "trace.json")
	if err := writeTraceFile(path, coding.Trace{}); err == nil {
		t.Fatal("write trace unexpectedly created missing parents")
	}
	if _, err := os.Stat(parent); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("parent Stat() error = %v, want not-exist", err)
	}
}

func TestWriteTraceWithOperationsCleansPartialFileAndJoinsErrors(t *testing.T) {
	writeErr := errors.New("write failed")
	closeErr := errors.New("close failed")
	removeErr := errors.New("remove failed")
	file := &failingTraceFile{writeErr: writeErr, closeErr: closeErr}
	removed := false

	err := writeTraceWithOperations(
		"trace.json",
		coding.Trace{},
		func(string, int, fs.FileMode) (traceFile, error) { return file, nil },
		func(string) error {
			removed = true
			return removeErr
		},
	)
	for _, want := range []error{writeErr, closeErr, removeErr} {
		if !errors.Is(err, want) {
			t.Errorf("write error = %v, want joined %v", err, want)
		}
	}
	if !removed {
		t.Fatal("partial trace was not removed")
	}
}

func TestWriteTraceWithOperationsCleansFileAfterCloseFailure(t *testing.T) {
	closeErr := errors.New("close failed")
	file := &closeFailingTraceFile{closeErr: closeErr}
	removed := false

	err := writeTraceWithOperations(
		"trace.json",
		coding.Trace{},
		func(string, int, fs.FileMode) (traceFile, error) { return file, nil },
		func(string) error {
			removed = true
			return nil
		},
	)
	if !errors.Is(err, closeErr) {
		t.Fatalf("write error = %v, want close error", err)
	}
	if !removed {
		t.Fatal("trace was not removed after close failure")
	}
}

func TestWriteTraceWithOperationsDoesNotRemoveAfterOpenFailure(t *testing.T) {
	openErr := errors.New("open failed")
	removed := false
	err := writeTraceWithOperations(
		"trace.json",
		coding.Trace{},
		func(string, int, fs.FileMode) (traceFile, error) { return nil, openErr },
		func(string) error {
			removed = true
			return nil
		},
	)
	if !errors.Is(err, openErr) {
		t.Fatalf("write error = %v, want open error", err)
	}
	if removed {
		t.Fatal("open failure removed a path this call did not create")
	}
}

type failingTraceFile struct {
	writeErr error
	closeErr error
}

func (f *failingTraceFile) Write(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, f.writeErr
	}
	return len(buffer) / 2, f.writeErr
}

func (f *failingTraceFile) Close() error { return f.closeErr }

type closeFailingTraceFile struct {
	bytes.Buffer
	closeErr error
}

func (f *closeFailingTraceFile) Close() error { return f.closeErr }

func TestResolveTracePathUsesLaunchWorkspace(t *testing.T) {
	if got, want := resolveTracePath("/workspace", "evidence/trace.json"), "/workspace/evidence/trace.json"; got != want {
		t.Fatalf("relative trace path = %q, want %q", got, want)
	}
	absolute := filepath.Join(string(filepath.Separator), "tmp", "trace.json")
	if got := resolveTracePath("/workspace", absolute); got != absolute {
		t.Fatalf("absolute trace path = %q, want %q", got, absolute)
	}
	if got := resolveTracePath("/workspace", "   "); !strings.Contains(got, "   ") {
		t.Fatalf("whitespace trace path = %q, want raw non-empty path", got)
	}
}
