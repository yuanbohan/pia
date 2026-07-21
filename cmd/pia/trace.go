package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/yuanbohan/pia/internal/coding"
)

type traceFile interface {
	io.Writer
	Close() error
}

type traceFileOpener func(string, int, fs.FileMode) (traceFile, error)

func writeTraceFile(path string, trace coding.Trace) error {
	return writeTraceWithOperations(path, trace, openTraceFile, os.Remove)
}

func writeTraceWithOperations(
	path string,
	trace coding.Trace,
	open traceFileOpener,
	remove func(string) error,
) error {
	encoded, err := json.MarshalIndent(trace, "", "  ")
	if err != nil {
		return fmt.Errorf("encode trace: %w", err)
	}
	encoded = append(encoded, '\n')

	// O_EXCL is the atomic no-clobber boundary. In combination with O_CREATE it
	// rejects existing regular files, directories, FIFOs, and symlinks without
	// opening the target first or introducing a validation-to-open race.
	file, err := open(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create trace %q: %w", path, err)
	}
	_, writeErr := io.Copy(file, bytes.NewReader(encoded))
	closeErr := file.Close()
	if writeErr == nil && closeErr == nil {
		return nil
	}

	cleanupErr := remove(path)
	return errors.Join(
		wrapTraceError("write", path, writeErr),
		wrapTraceError("close", path, closeErr),
		wrapTraceError("remove partial", path, cleanupErr),
	)
}

func openTraceFile(path string, flag int, mode fs.FileMode) (traceFile, error) {
	return os.OpenFile(path, flag, mode)
}

func wrapTraceError(operation, path string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s trace %q: %w", operation, path, err)
}
