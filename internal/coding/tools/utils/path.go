package utils

import (
	"fmt"
	"path/filepath"
	"strings"
)

const maxWorkspacePathBytes = 4 << 10

// NormalizeWorkspacePath validates a root-relative path and returns both the
// OS-native path used with os.Root and the slash-normalized model-visible path.
func NormalizeWorkspacePath(path string) (rootPath string, displayPath string, err error) {
	// Concrete tools include this path in model-visible results and errors. A
	// fixed bound prevents an invalid path from bypassing their output limits;
	// 4096 bytes also covers the supported macOS/Linux filesystem path range.
	if len(path) > maxWorkspacePathBytes {
		return "", "", fmt.Errorf("path exceeds the %d-byte limit", maxWorkspacePathBytes)
	}
	if strings.TrimSpace(path) == "" {
		return "", "", fmt.Errorf("path is required")
	}
	if !filepath.IsLocal(path) {
		return "", "", fmt.Errorf("path %q must be workspace-relative and must not escape it", path)
	}
	// Cleaning a parent component before calling os.Root can change which file
	// is addressed when an earlier component is a symlink. Reject it so the
	// normalized path shown to the model is the same path given to os.Root.
	for _, component := range strings.Split(filepath.ToSlash(path), "/") {
		if component == ".." {
			return "", "", fmt.Errorf("path must not contain .. components")
		}
	}
	cleaned := filepath.Clean(path)
	return cleaned, filepath.ToSlash(cleaned), nil
}
