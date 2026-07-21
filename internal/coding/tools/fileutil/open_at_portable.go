//go:build !darwin && !linux

package fileutil

import (
	"fmt"
	"os"
	"runtime"
)

func openAtCandidate(_ *os.File, _ string, _ bool) (*os.File, error) {
	return nil, fmt.Errorf("directory-relative safe open is unsupported on %s", runtime.GOOS)
}
