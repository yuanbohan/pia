//go:build darwin || linux

package coding

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestDiscoverPiaSkillsRejectsFIFOWithoutWaitingForWriter(t *testing.T) {
	directory := t.TempDir()
	skillDirectory := filepath.Join(directory, piaSkillsDirectory, "fifo")
	if err := os.MkdirAll(skillDirectory, 0o755); err != nil {
		t.Fatalf("create Skill directory: %v", err)
	}
	if err := syscall.Mkfifo(filepath.Join(skillDirectory, piaSkillFilename), 0o600); err != nil {
		t.Fatalf("create Skill FIFO: %v", err)
	}
	workspace := openPromptWorkspace(t, directory)

	type outcome struct {
		discovery piaSkillDiscovery
		err       error
	}
	finished := make(chan outcome, 1)
	go func() {
		discovery, err := discoverPiaSkills(workspace)
		finished <- outcome{discovery: discovery, err: err}
	}()

	select {
	case got := <-finished:
		if got.err != nil {
			t.Fatalf("discover Pia skills: %v", got.err)
		}
		if got.discovery.Catalog != "" {
			t.Fatalf("FIFO entered Skill catalog\n%s", got.discovery.Catalog)
		}
		if !skillDiagnosticsContain(got.discovery.Diagnostics, "regular") {
			t.Fatalf("diagnostics = %#v, want regular-file warning", got.discovery.Diagnostics)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Skill discovery blocked opening a FIFO with no writer")
	}
}

func TestDiscoverPiaSkillsRejectsFIFOSourceWithoutWaitingForWriter(t *testing.T) {
	directory := t.TempDir()
	piaDirectory := filepath.Join(directory, ".pia")
	if err := os.MkdirAll(piaDirectory, 0o755); err != nil {
		t.Fatalf("create .pia directory: %v", err)
	}
	if err := syscall.Mkfifo(filepath.Join(piaDirectory, "skills"), 0o600); err != nil {
		t.Fatalf("create Skill source FIFO: %v", err)
	}
	workspace := openPromptWorkspace(t, directory)

	type outcome struct {
		discovery piaSkillDiscovery
		err       error
	}
	finished := make(chan outcome, 1)
	go func() {
		discovery, err := discoverPiaSkills(workspace)
		finished <- outcome{discovery: discovery, err: err}
	}()

	select {
	case got := <-finished:
		if got.err != nil {
			t.Fatalf("discover Pia skills: %v", got.err)
		}
		if got.discovery.Catalog != "" {
			t.Fatalf("FIFO source entered Skill catalog\n%s", got.discovery.Catalog)
		}
		if !skillDiagnosticsContain(got.discovery.Diagnostics, "directory") {
			t.Fatalf("diagnostics = %#v, want directory warning", got.discovery.Diagnostics)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Skill discovery blocked opening a source FIFO with no writer")
	}
}
