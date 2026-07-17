package tools

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestReadPageStopsOnceSelectedLineExceedsByteLimit(t *testing.T) {
	t.Parallel()

	source := &unboundedLineReader{}
	_, err := readPage(context.Background(), source, 1, maxReadLines)
	if err == nil || !strings.Contains(err.Error(), "line 1 exceeds the 51200-byte limit") {
		t.Fatalf("readPage() error = %v, want oversized-line error", err)
	}
	if source.reads > 2 {
		t.Fatalf("readPage() source reads = %d, want at most 2", source.reads)
	}
}

func TestReadPageOffsetCannotBypassOversizedLineLimit(t *testing.T) {
	t.Parallel()

	source := &unboundedLineReader{}
	_, err := readPage(context.Background(), source, 2, maxReadLines)
	if err == nil || !strings.Contains(err.Error(), "line 1 exceeds the 51200-byte limit") {
		t.Fatalf("readPage() error = %v, want oversized skipped-line error", err)
	}
	if source.reads > 2 {
		t.Fatalf("readPage() source reads = %d, want at most 2", source.reads)
	}
}

type unboundedLineReader struct {
	reads int
}

func (r *unboundedLineReader) Read(buffer []byte) (int, error) {
	r.reads++
	if r.reads > 2 {
		return 0, errors.New("read continued after the selected line exceeded its limit")
	}
	for index := range buffer {
		buffer[index] = 'x'
	}
	return len(buffer), nil
}
