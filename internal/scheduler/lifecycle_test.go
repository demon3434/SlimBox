package scheduler

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"slimbox/internal/repository"
)

func TestLifecycleManager_TempUploadCleanup(t *testing.T) {
	tempRoot := t.TempDir()
	uploadDir := filepath.Join(tempRoot, "uploads")
	dbPath := filepath.Join(tempRoot, "test_lc.db")
	_ = os.MkdirAll(filepath.Join(uploadDir, "temp", "old-session"), 0755)

	db, err := repository.NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to init DB: %v", err)
	}
	defer db.Close()

	taskRepo := repository.NewTaskRepository(db)
	settingsRepo := repository.NewSettingsRepository(db)

	partFile := filepath.Join(uploadDir, "temp", "old-session", "0.part")
	_ = os.WriteFile(partFile, []byte("data"), 0644)

	pastTime := time.Now().Add(-30 * time.Hour)
	_ = os.Chtimes(partFile, pastTime, pastTime)
	_ = os.Chtimes(filepath.Join(uploadDir, "temp", "old-session"), pastTime, pastTime)

	lifecycle := NewLifecycleManager(taskRepo, settingsRepo, uploadDir)
	lifecycle.runCleanupCycle()

	if _, err := os.Stat(filepath.Join(uploadDir, "temp", "old-session")); !os.IsNotExist(err) {
		t.Errorf("Expected old-session to be evicted by lifecycle runCleanupCycle")
	}
}
