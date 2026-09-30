package engine

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFindExecutable_LocalBundlePrecedence(t *testing.T) {
	tempDir := t.TempDir()
	binDir := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("Failed to create temp bin dir: %v", err)
	}

	dummyBin := "dummy_tool"
	if runtime.GOOS == "windows" {
		dummyBin += ".exe"
	}
	dummyPath := filepath.Join(binDir, dummyBin)
	if err := os.WriteFile(dummyPath, []byte("#!/bin/sh\necho ok\n"), 0755); err != nil {
		t.Fatalf("Failed to write dummy binary: %v", err)
	}

	// Change working directory to tempDir during test
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current wd: %v", err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to chdir: %v", err)
	}

	resolved := FindExecutable("dummy_tool")
	expectedRel := filepath.Join(".", "bin", dummyBin)
	if filepath.Clean(resolved) != filepath.Clean(expectedRel) && filepath.Clean(resolved) != filepath.Clean(dummyPath) {
		t.Errorf("Expected FindExecutable to discover %s, got: %s", expectedRel, resolved)
	}
}

func TestFindFFmpeg_Fallback(t *testing.T) {
	// Should at least return "ffmpeg" or a valid path on system
	p := FindFFmpeg()
	if p == "" {
		t.Errorf("Expected non-empty path for FindFFmpeg")
	}
}
