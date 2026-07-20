//go:build darwin || linux

package fileutil

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func openAtCandidate(directory *os.File, name string, requireDirectory bool) (*os.File, error) {
	raw, err := directory.SyscallConn()
	if err != nil {
		return nil, fmt.Errorf("access directory handle: %w", err)
	}

	flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK
	if requireDirectory {
		flags |= unix.O_DIRECTORY
	}
	childFD := -1
	var openErr error
	controlErr := raw.Control(func(directoryFD uintptr) {
		childFD, openErr = unix.Openat(int(directoryFD), name, flags, 0)
	})
	if controlErr != nil {
		if childFD >= 0 {
			_ = unix.Close(childFD)
		}
		return nil, fmt.Errorf("use directory handle: %w", controlErr)
	}
	if openErr != nil {
		return nil, &os.PathError{Op: "openat", Path: name, Err: openErr}
	}

	file := os.NewFile(uintptr(childFD), filepath.Join(directory.Name(), name))
	if file == nil {
		_ = unix.Close(childFD)
		return nil, fmt.Errorf("wrap opened child file descriptor")
	}
	return file, nil
}
