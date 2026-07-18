//go:build darwin || linux

package fileutil

import (
	"os"
	"syscall"
)

func openCandidate(root *os.Root, path string) (*os.File, error) {
	// The build tag limits this open policy to Darwin and Linux, where the
	// nonblocking FIFO behavior is explicitly tested. Other targets use the
	// portable fallback until their semantics are verified. A read-only FIFO can
	// block before callers can inspect its handle; O_NONBLOCK lets read and edit
	// reject the actual non-regular object and has no effect on regular files.
	return root.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}
