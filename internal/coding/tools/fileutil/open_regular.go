package fileutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	return validateRegularFile(file)
}

// OpenRegularFileAt opens one direct child of directory without following a
// final symlink, then validates the returned handle as a regular file.
func OpenRegularFileAt(directory *os.File, name string) (*os.File, error) {
	if directory == nil {
		return nil, fmt.Errorf("open regular file at directory: directory is required")
	}
	if err := validateDirectChildName(name); err != nil {
		return nil, fmt.Errorf("open regular file at directory: %w", err)
	}
	file, err := openAtCandidate(directory, name, false)
	if err != nil {
		return nil, err
	}
	return validateRegularFile(file)
}

// OpenDirectory opens path beneath root and validates the object through the
// returned handle. On supported platforms it shares the nonblocking open policy
// used for regular files so a FIFO cannot stall validation before discovery.
func OpenDirectory(root *os.Root, path string) (*os.File, error) {
	if root == nil {
		return nil, fmt.Errorf("open directory: root is required")
	}
	file, err := openCandidate(root, path)
	if err != nil {
		return nil, err
	}
	return validateDirectory(file)
}

// OpenDirectoryAt opens one direct child of directory without following a
// final symlink, then validates the returned handle as a directory.
func OpenDirectoryAt(directory *os.File, name string) (*os.File, error) {
	if directory == nil {
		return nil, fmt.Errorf("open directory at directory: directory is required")
	}
	if err := validateDirectChildName(name); err != nil {
		return nil, fmt.Errorf("open directory at directory: %w", err)
	}
	file, err := openAtCandidate(directory, name, true)
	if err != nil {
		return nil, err
	}
	return validateDirectory(file)
}

func validateDirectory(file *os.File) (*os.File, error) {
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("inspect opened directory: %w", err)
	}
	if !info.IsDir() {
		_ = file.Close()
		return nil, fmt.Errorf("target is not a directory")
	}
	return file, nil
}

func validateDirectChildName(name string) error {
	if name == "" || name == "." || name == ".." || strings.Contains(name, "/") {
		return fmt.Errorf("name must be one direct child")
	}
	return nil
}

// OpenRegularHostFile opens an absolute host path and validates the object
// through the returned handle. Callers own the policy that permits host access.
func OpenRegularHostFile(path string) (*os.File, error) {
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("open regular host file: absolute path is required")
	}
	file, err := openHostCandidate(path)
	if err != nil {
		return nil, err
	}
	return validateRegularFile(file)
}

func validateRegularFile(file *os.File) (*os.File, error) {
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
