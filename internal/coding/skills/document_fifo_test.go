//go:build darwin || linux

package skills

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestLoadCurrentBodyRejectsFIFOWithoutWaitingForAWriter(t *testing.T) {
	directory := t.TempDir()
	skillDirectory := filepath.Join(directory, piaSkillsDirectory, "events")
	if err := os.MkdirAll(skillDirectory, 0o755); err != nil {
		t.Fatalf("create Skill directory: %v", err)
	}
	if err := syscall.Mkfifo(filepath.Join(skillDirectory, piaSkillFilename), 0o600); err != nil {
		t.Fatalf("create Skill FIFO: %v", err)
	}
	root := openTestRoot(t, directory)
	entry := Entry{Name: "events", Directory: "events", Location: ".pia/skills/events/SKILL.md"}

	returned := make(chan error, 1)
	go func() {
		_, err := LoadCurrentBody(context.Background(), root, entry, 1024)
		returned <- err
	}()
	select {
	case err := <-returned:
		if err == nil || !strings.Contains(err.Error(), "regular") {
			t.Fatalf("LoadCurrentBody(FIFO) error = %v, want regular-file failure", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("LoadCurrentBody blocked opening a FIFO")
	}
}
