package fileutil

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// ReplaceRegularFile writes source to a temporary regular file in an already
// pinned parent directory and renames it over name. The caller owns parent.
func ReplaceRegularFile(ctx context.Context, parent *os.Root, name string, source io.Reader) error {
	if parent == nil {
		return fmt.Errorf("replace regular file: parent root is required")
	}
	if source == nil {
		return fmt.Errorf("replace regular file: content source is required")
	}
	if name == "" || name == "." || filepath.Base(name) != name {
		return fmt.Errorf("replace regular file: target must be one name in the pinned parent")
	}
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}

	existingPermissions, err := inspectReplacementTarget(parent, name)
	if err != nil {
		return err
	}
	temporary, temporaryName, err := createReplacementFile(parent, existingPermissions)
	if err != nil {
		return err
	}
	closed := false
	committed := false
	defer func() {
		if !closed {
			_ = temporary.Close()
		}
		if !committed {
			_ = parent.Remove(temporaryName)
		}
	}()

	if existingPermissions != nil {
		// Creation applies the process umask. Reset the temporary file through
		// its opened handle so replacing a file preserves its permission bits
		// without a second path lookup that could target a symlink.
		if err := temporary.Chmod(*existingPermissions); err != nil {
			return fmt.Errorf("set temporary file permissions: %w", err)
		}
	}
	if _, err := io.Copy(temporary, contextReader{ctx: ctx, source: source}); err != nil {
		return fmt.Errorf("write temporary file: %w", err)
	}
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	closeErr := temporary.Close()
	closed = true
	if closeErr != nil {
		return fmt.Errorf("close temporary file: %w", closeErr)
	}
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}

	// Rename is the visibility commit point: readers observe either the old
	// file or the complete replacement. This does not promise crash durability,
	// because Phase 1 deliberately does not fsync the file and parent directory.
	if err := parent.Rename(temporaryName, name); err != nil {
		return fmt.Errorf("commit replacement: %w", err)
	}
	committed = true
	// Cancellation after the rename must not report a failure for a side effect
	// that already committed and invite the model to retry it.
	return nil
}

func inspectReplacementTarget(parent *os.Root, name string) (*os.FileMode, error) {
	info, err := parent.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect target: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("target is a symbolic link; file replacement only accepts regular files")
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("target is not a regular file")
	}
	permissions := info.Mode().Perm()
	return &permissions, nil
}

func createReplacementFile(parent *os.Root, existingPermissions *os.FileMode) (*os.File, string, error) {
	permissions := os.FileMode(0o666)
	if existingPermissions != nil {
		permissions = *existingPermissions
	}
	for range 10 {
		name := ".pi-go-replace-" + rand.Text() + ".tmp"
		file, err := parent.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, permissions)
		if err == nil {
			return file, name, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, "", fmt.Errorf("create temporary file: %w", err)
		}
	}
	return nil, "", fmt.Errorf("create temporary file: too many name collisions")
}

type contextReader struct {
	ctx    context.Context
	source io.Reader
}

func (r contextReader) Read(buffer []byte) (int, error) {
	if cause := context.Cause(r.ctx); cause != nil {
		return 0, cause
	}
	read, err := r.source.Read(buffer)
	// Recheck after a potentially blocking read. Returning both completed bytes
	// and the cause lets io.Copy stop after writing only to the uncommitted temp.
	if cause := context.Cause(r.ctx); cause != nil {
		return read, cause
	}
	return read, err
}
