// Package utils contains small contracts shared by concrete coding tools.
package utils

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// DecodeArguments decodes exactly one tool-argument object and rejects fields
// the concrete argument type does not declare.
func DecodeArguments[T any](arguments json.RawMessage) (T, error) {
	var value T
	trimmed := bytes.TrimSpace(arguments)
	if len(trimmed) > 0 && trimmed[0] != '{' {
		return value, errors.New("arguments must be a JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(arguments))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, err
	}

	var trailing struct{}
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return value, errors.New("arguments must contain exactly one JSON object")
	}
	return value, nil
}
