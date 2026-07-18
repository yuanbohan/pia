// Package toolargs contains strict JSON argument decoding shared by coding
// tools. It stays in the coding domain until a non-coding tool proves the same
// contract is shared more broadly.
package toolargs

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"
)

const maxDiagnosticBytes = 512

// Decode decodes exactly one tool-argument object and rejects fields the
// concrete argument type does not declare.
func Decode[T any](arguments json.RawMessage) (T, error) {
	var value T
	trimmed := bytes.TrimSpace(arguments)
	if len(trimmed) > 0 && trimmed[0] != '{' {
		return value, errors.New("arguments must be a JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(arguments))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, boundDiagnostic(err)
	}

	var trailing struct{}
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return value, errors.New("arguments must contain exactly one JSON object")
	}
	return value, nil
}

func boundDiagnostic(err error) error {
	message := err.Error()
	if len(message) <= maxDiagnosticBytes {
		return err
	}
	prefix := message[:maxDiagnosticBytes]
	for !utf8.ValidString(prefix) {
		prefix = prefix[:len(prefix)-1]
	}
	// JSON decoder errors can quote a model-controlled field name. Truncate the
	// diagnostic so invalid tool arguments cannot bypass output bounds even when
	// a concrete tool legitimately accepts a large content field.
	return errors.New(prefix + "... [truncated]")
}
