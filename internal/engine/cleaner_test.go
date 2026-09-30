package engine

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestScanAndPurgeOrphanArtifacts(t *testing.T) {
	tempRoot := t.TempDir()
	uploadDir := filepath.Join(tempRoot, "uploads")
	outputDir := filepath.Join(tempRoot, "outputs")
	_ = os.MkdirAll(filepath.Join(uploadDir, "temp", "abandoned-sess"), 0755)
	_ = os.MkdirAll(filepath.Join(uploadDir, "temp", "active-sess"), 0755)
	_ = os.MkdirAll(outputDir, 0755)

	abandonedPart := filepath.Join(uploadDir, "temp", "abandoned-sess", "0.part")
	activePart := filepath.Join(uploadDir, "temp", "active-sess", "0.part")
	unlinkedUpload := filepath.Join(uploadDir, "unlinked.mp4")
	trackedUpload := filepath.Join(uploadDir, "tracked.mp4")
	stalePartial := filepath.Join(outputDir, "transcoding.mkv.part")
	unlinkedOutput := filepath.Join(outputDir, "orphan_out.mp4")
	trackedOutput := filepath.Join(outputDir, "valid_out.mp4")

	_ = os.WriteFile(abandonedPart, []byte("chunk0"), 0644)
	_ = os.WriteFile(activePart, []byte("chunk_active"), 0644)
	_ = os.WriteFile(unlinkedUpload, []byte("unlinked video content"), 0644)
	_ = os.WriteFile(trackedUpload, []byte("tracked video content"), 0644)
	_ = os.WriteFile(stalePartial, []byte("partial output"), 0644)
	_ = os.WriteFile(unlinkedOutput, []byte("orphan output content"), 0644)
	_ = os.WriteFile(trackedOutput, []byte("tracked output content"), 0644)

	// Set ModTime on abandoned-sess and unlinked files to past (e.g. 2 hours ago)
	pastTime := time.Now().Add(-2 * time.Hour)
	_ = os.Chtimes(abandonedPart, pastTime, pastTime)
	_ = os.Chtimes(filepath.Join(uploadDir, "temp", "abandoned-sess"), pastTime, pastTime)
	_ = os.Chtimes(unlinkedUpload, pastTime, pastTime)
	_ = os.Chtimes(stalePartial, pastTime, pastTime)
	_ = os.Chtimes(unlinkedOutput, pastTime, pastTime)

	trackedPaths := map[string]struct{}{
		filepath.Clean(trackedUpload): {},
		filepath.Clean(trackedOutput): {},
	}

	// 1. Scan with 1 hour tempGrace and 5 minute uploadGrace
	summary, err := ScanOrphanArtifacts(uploadDir, outputDir, trackedPaths, 1*time.Hour, 5*time.Minute)
	if err != nil {
		t.Fatalf("ScanOrphanArtifacts failed: %v", err)
	}

	// Expect 4 orphan items: abandoned-sess, unlinkedUpload, stalePartial, unlinkedOutput
	// activePart is recently created (mod time = now), so it's protected by grace period
	// trackedUpload and trackedOutput are in trackedPaths
	if summary.TotalCount != 4 {
		t.Fatalf("Expected 4 orphan items, got %d: %+v", summary.TotalCount, summary.Items)
	}
	if summary.TempChunkBytes <= 0 || summary.UnlinkedUploadBytes <= 0 || summary.UnlinkedOutputBytes <= 0 || summary.StalePartialBytes <= 0 {
		t.Errorf("Expected non-zero category bytes in summary: %+v", summary)
	}

	// 2. Purge scanned orphan artifacts
	res, err := PurgeOrphanArtifacts(summary.Items)
	if err != nil {
		t.Fatalf("PurgeOrphanArtifacts failed: %v", err)
	}
	if res.DeletedCount != 4 {
		t.Errorf("Expected 4 deleted items, got %d", res.DeletedCount)
	}

	// Verify orphans are gone
	if _, err := os.Stat(abandonedPart); !os.IsNotExist(err) {
		t.Errorf("abandonedPart should be deleted")
	}
	if _, err := os.Stat(unlinkedUpload); !os.IsNotExist(err) {
		t.Errorf("unlinkedUpload should be deleted")
	}
	if _, err := os.Stat(stalePartial); !os.IsNotExist(err) {
		t.Errorf("stalePartial should be deleted")
	}
	if _, err := os.Stat(unlinkedOutput); !os.IsNotExist(err) {
		t.Errorf("unlinkedOutput should be deleted")
	}

	// Verify protected & tracked files remain intact
	if _, err := os.Stat(activePart); os.IsNotExist(err) {
		t.Errorf("activePart should remain intact")
	}
	if _, err := os.Stat(trackedUpload); os.IsNotExist(err) {
		t.Errorf("trackedUpload should remain intact")
	}
	if _, err := os.Stat(trackedOutput); os.IsNotExist(err) {
		t.Errorf("trackedOutput should remain intact")
	}
}

func TestCleanOrphanTempUploads(t *testing.T) {
	tempRoot := t.TempDir()
	uploadDir := filepath.Join(tempRoot, "uploads")
	oldDir := filepath.Join(uploadDir, "temp", "sess-old")
	newDir := filepath.Join(uploadDir, "temp", "sess-new")
	_ = os.MkdirAll(oldDir, 0755)
	_ = os.MkdirAll(newDir, 0755)

	oldFile := filepath.Join(oldDir, "0.part")
	newFile := filepath.Join(newDir, "0.part")
	_ = os.WriteFile(oldFile, []byte("old part data"), 0644)
	_ = os.WriteFile(newFile, []byte("new part data"), 0644)

	pastTime := time.Now().Add(-30 * time.Hour)
	_ = os.Chtimes(oldFile, pastTime, pastTime)
	_ = os.Chtimes(oldDir, pastTime, pastTime)

	count, bytes := CleanOrphanTempUploads(uploadDir, 24*time.Hour)
	if count != 1 || bytes != int64(len("old part data")) {
		t.Errorf("Expected 1 evicted session with %d bytes, got count=%d bytes=%d", len("old part data"), count, bytes)
	}

	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Errorf("oldDir should have been evicted")
	}
	if _, err := os.Stat(newDir); os.IsNotExist(err) {
		t.Errorf("newDir should still exist")
	}
}

