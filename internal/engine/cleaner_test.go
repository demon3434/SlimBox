package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanupPartialFile(t *testing.T) {
	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "output.mkv")
	partPath := outputPath + ".part"

	// Create a dummy .part file simulating an interrupted transcode
	if err := os.WriteFile(partPath, []byte("incomplete data"), 0644); err != nil {
		t.Fatalf("Failed to create test part file: %v", err)
	}

	if _, err := os.Stat(partPath); os.IsNotExist(err) {
		t.Fatalf("Part file was not created")
	}

	// Trigger cleaner
	CleanupPartialFile(outputPath)

	// Verify part file was completely removed (ADR-0011)
	if _, err := os.Stat(partPath); !os.IsNotExist(err) {
		t.Errorf("Expected part file %s to be deleted, but it still exists", partPath)
	}
}

func TestCleanOrphanPartials(t *testing.T) {
	tempDir := t.TempDir()
	file1 := filepath.Join(tempDir, "movie1.mkv.part")
	file2 := filepath.Join(tempDir, "movie2.mkv.part")
	normalFile := filepath.Join(tempDir, "finished.mkv")

	_ = os.WriteFile(file1, []byte("data"), 0644)
	_ = os.WriteFile(file2, []byte("data"), 0644)
	_ = os.WriteFile(normalFile, []byte("data"), 0644)

	CleanOrphanPartials(tempDir)

	if _, err := os.Stat(file1); !os.IsNotExist(err) {
		t.Errorf("Orphan file1 was not cleaned")
	}
	if _, err := os.Stat(file2); !os.IsNotExist(err) {
		t.Errorf("Orphan file2 was not cleaned")
	}
	if _, err := os.Stat(normalFile); os.IsNotExist(err) {
		t.Errorf("Finished normal file should not be cleaned")
	}
}
