package fileutil

import (
	"fmt"
	"os"
)

// OpenRegularFile opens path beneath root and validates the object through the
// returned handle. Read and edit share this primitive so neither can validate
// one path object and then consume a different object after a path swap.
func OpenRegularFile(root *os.Root, path string) (*os.File, error) {
	if root == nil {
		return nil, fmt.Errorf("open regular file: root is required")
	}
	file, err := openCandidate(root, path)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("inspect opened file: %w", err)
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, fmt.Errorf("target is not a regular file")
	}
	return file, nil
}
