package main

import (
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/yuanbohan/pia/internal/observation"
)

type lineObserver struct {
	writer   io.Writer
	firstErr error
}

func newLineObserver(writer io.Writer) *lineObserver {
	return &lineObserver{writer: writer}
}

func (observer *lineObserver) Observe(event observation.Event) {
	if observer.firstErr != nil {
		return
	}
	line := projectedLine(event)
	if line == "" {
		return
	}

	output := "pia: " + escapeLine(line) + "\n"
	written, err := io.WriteString(observer.writer, output)
	if err == nil && written != len(output) {
		err = io.ErrShortWrite
	}
	if err != nil {
		observer.firstErr = fmt.Errorf("write live observation: %w", err)
	}
}

func (observer *lineObserver) Err() error {
	return observer.firstErr
}

func projectedLine(event observation.Event) string {
	switch event := event.(type) {
	case observation.Tool:
		summary := event.Summary
		if summary == "" {
			summary = event.Name
		}
		switch {
		case event.Phase == observation.PhaseStarted:
			return summary
		case event.Phase == observation.PhaseSettled &&
			event.Outcome == observation.OutcomeError:
			return summary + " failed"
		}
	case observation.Compaction:
		action := fmt.Sprintf("Compact context (%s)", event.Reason)
		switch {
		case event.Phase == observation.PhaseStarted:
			return action
		case event.Phase == observation.PhaseSettled &&
			event.Outcome == observation.OutcomeError:
			return action + " failed"
		}
	}
	return ""
}

func escapeLine(value string) string {
	var escaped strings.Builder
	escaped.Grow(len(value))
	for _, character := range value {
		switch character {
		case '\n':
			escaped.WriteString(`\n`)
		case '\r':
			escaped.WriteString(`\r`)
		case '\t':
			escaped.WriteString(`\t`)
		default:
			if unicode.IsControl(character) {
				_, _ = fmt.Fprintf(&escaped, `\u%04x`, character)
				continue
			}
			escaped.WriteRune(character)
		}
	}
	return escaped.String()
}
