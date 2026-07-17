// Package coding owns the workspace and composition boundary for a local
// coding Agent.
package coding

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Workspace owns one canonical path and the os.Root shared by its file tools.
// The composition layer must keep it open until all tool calls have settled.
type Workspace struct {
	path string
	root *os.Root
}

// OpenWorkspace canonicalizes the operator-selected directory and opens the
// root used by all file tools.
func OpenWorkspace(path string) (*Workspace, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("coding: workspace path is required")
	}

	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("coding: open workspace %q: %w", path, err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, fmt.Errorf("coding: open workspace %q: %w", path, err)
	}
	root, err := os.OpenRoot(canonical)
	if err != nil {
		return nil, fmt.Errorf("coding: open workspace %q: %w", path, err)
	}
	return &Workspace{path: canonical, root: root}, nil
}

// Path returns the canonical host path selected by the operator.
func (w *Workspace) Path() string {
	return w.path
}

// Root returns a borrowed root. Callers must not close it; Workspace owns its
// lifetime so every file tool observes the same pinned directory.
func (w *Workspace) Root() *os.Root {
	return w.root
}

// Close releases the root after all calls using this workspace have settled.
func (w *Workspace) Close() error {
	return w.root.Close()
}
