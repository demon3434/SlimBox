package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"slimbox/internal/domain"
	"slimbox/internal/engine"
	"slimbox/internal/repository"
)

func TestStorageHandler_ScanAndClean(t *testing.T) {
	tempRoot := t.TempDir()
	uploadDir := filepath.Join(tempRoot, "uploads")
	outputDir := filepath.Join(tempRoot, "outputs")
	dbPath := filepath.Join(tempRoot, "test.db")
	_ = os.MkdirAll(filepath.Join(uploadDir, "temp", "sess-abandoned"), 0755)
	_ = os.MkdirAll(outputDir, 0755)

	db, err := repository.NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to init DB: %v", err)
	}
	defer db.Close()
	taskRepo := repository.NewTaskRepository(db)

	trackedFile := filepath.Join(uploadDir, "tracked.mp4")
	unlinkedFile := filepath.Join(uploadDir, "orphan.mp4")
	chunkFile := filepath.Join(uploadDir, "temp", "sess-abandoned", "0.part")

	_ = os.WriteFile(trackedFile, []byte("tracked video"), 0644)
	_ = os.WriteFile(unlinkedFile, []byte("orphan video"), 0644)
	_ = os.WriteFile(chunkFile, []byte("chunk part"), 0644)

	// Backdate orphan files so they exceed grace periods
	pastTime := time.Now().Add(-2 * time.Hour)
	_ = os.Chtimes(chunkFile, pastTime, pastTime)
	_ = os.Chtimes(filepath.Join(uploadDir, "temp", "sess-abandoned"), pastTime, pastTime)
	_ = os.Chtimes(unlinkedFile, pastTime, pastTime)

	// Register tracked file in DB
	task := &domain.Task{
		ID:             "t-tracked",
		SourceFileName: "tracked.mp4",
		SourceFilePath: trackedFile,
		Status:         domain.StatusPending,
		CreatedAt:      time.Now(),
	}
	_ = taskRepo.Create(task)

	handler := NewStorageHandler(taskRepo, uploadDir, outputDir)

	// 1. Test GET /api/v1/system/orphans/scan
	reqScan := httptest.NewRequest(http.MethodGet, "/api/v1/system/orphans/scan", nil)
	rrScan := httptest.NewRecorder()
	handler.HandleScanOrphans(rrScan, reqScan)

	if rrScan.Code != http.StatusOK {
		t.Fatalf("Scan returned status %d: %s", rrScan.Code, rrScan.Body.String())
	}

	var scanRes engine.OrphanScanSummary
	if err := json.Unmarshal(rrScan.Body.Bytes(), &scanRes); err != nil {
		t.Fatalf("Failed to decode scan response: %v", err)
	}

	if scanRes.TotalCount != 2 {
		t.Fatalf("Expected 2 orphan items, got %d: %+v", scanRes.TotalCount, scanRes.Items)
	}

	// 2. Test POST /api/v1/system/orphans/clean
	cleanPayload := CleanOrphansRequest{}
	body, _ := json.Marshal(cleanPayload)
	reqClean := httptest.NewRequest(http.MethodPost, "/api/v1/system/orphans/clean", bytes.NewReader(body))
	rrClean := httptest.NewRecorder()
	handler.HandleCleanOrphans(rrClean, reqClean)

	if rrClean.Code != http.StatusOK {
		t.Fatalf("Clean returned status %d: %s", rrClean.Code, rrClean.Body.String())
	}

	var cleanRes map[string]interface{}
	_ = json.Unmarshal(rrClean.Body.Bytes(), &cleanRes)
	if count, ok := cleanRes["deleted_count"].(float64); !ok || count != 2 {
		t.Errorf("Expected deleted_count 2, got %v", cleanRes["deleted_count"])
	}

	// Verify orphans are deleted but tracked file is kept
	if _, err := os.Stat(unlinkedFile); !os.IsNotExist(err) {
		t.Errorf("unlinkedFile should be deleted")
	}
	if _, err := os.Stat(chunkFile); !os.IsNotExist(err) {
		t.Errorf("chunkFile should be deleted")
	}
	if _, err := os.Stat(trackedFile); os.IsNotExist(err) {
		t.Errorf("trackedFile should still exist")
	}
}
