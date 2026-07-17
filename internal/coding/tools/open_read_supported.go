//go:build darwin || linux

package tools

import (
	"os"
	"syscall"
)

func openRegularFileCandidate(root *os.Root, path string) (*os.File, error) {
	// The build tag limits this open policy to Darwin and Linux, where the
	// nonblocking FIFO behavior is explicitly tested. Other targets use the
	// portable fallback in open_read_other.go until their semantics are verified.
	// A read-only FIFO can block before we can inspect its handle. O_NONBLOCK
	// lets Execute open and reject the actual non-regular object without relying
	// on a separate, racy path stat; it has no effect on accepted regular files.
	return root.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}
