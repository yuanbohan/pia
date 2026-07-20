//go:build !darwin && !linux

package fileutil

import "os"

func openCandidate(root *os.Root, path string) (*os.File, error) {
	// FIFO nonblocking-open semantics are not yet verified on this target, so use
	// the portable fallback to preserve buildability. This does not expand Phase 1
	// support beyond Darwin and Linux: a FIFO with no writer has no nonblocking
	// guarantee here, although regular-file validation still runs after open.
	return root.Open(path)
}

func openHostCandidate(path string) (*os.File, error) {
	return os.Open(path)
}
