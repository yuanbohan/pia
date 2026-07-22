package skills

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/yuanbohan/pia/internal/coding/tools/fileutil"
)

func openPiaSkillsDirectory(root *os.Root) (*os.File, error) {
	entry, err := root.Lstat(piaSkillsDirectory)
	if err != nil {
		return nil, err
	}
	if entry.Mode()&fs.ModeSymlink != 0 {
		return nil, fmt.Errorf("project Skills source is a symlink")
	}
	if !entry.IsDir() {
		return nil, fmt.Errorf("project Skills source is not a directory")
	}

	directory, err := fileutil.OpenDirectory(root, piaSkillsDirectory)
	if err != nil {
		return nil, err
	}
	opened, openStatErr := directory.Stat()
	current, currentStatErr := root.Lstat(piaSkillsDirectory)
	if joined := errors.Join(openStatErr, currentStatErr); joined != nil {
		return nil, errors.Join(fmt.Errorf("verify opened project Skills source: %w", joined), directory.Close())
	}
	// Root.OpenFile follows safe in-workspace symlinks. Rechecking the final
	// entry and its identity prevents a source swapped after the first Lstat
	// from turning that behavior into implicit symlink discovery.
	if current.Mode()&fs.ModeSymlink != 0 || !current.IsDir() || !os.SameFile(opened, current) {
		return nil, errors.Join(
			fmt.Errorf("project Skills source changed while it was being opened"),
			directory.Close(),
		)
	}
	return directory, nil
}
