package repository

import (
	"path/filepath"
	"testing"
	"time"

	"slimbox/internal/domain"
)

func TestTaskRepository_CRUDAndQueue(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_slimbox.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize test DB: %v", err)
	}
	defer db.Close()

	repo := NewTaskRepository(db)

	task1 := &domain.Task{
		ID:             "task-001",
		SourceFileName: "Movie1.mkv",
		SourceFilePath: "/data/uploads/movie1.mkv",
		SourceFileSize: 1024 * 1024 * 100, // 100MB
		Status:         domain.StatusPending,
		CreatedAt:      time.Now().Add(-10 * time.Minute),
	}
	if err := repo.Create(task1); err != nil {
		t.Fatalf("Failed to create task1: %v", err)
	}

	task2 := &domain.Task{
		ID:             "task-002",
		SourceFileName: "Movie2.mkv",
		SourceFilePath: "/data/uploads/movie2.mkv",
		SourceFileSize: 1024 * 1024 * 200,
		Status:         domain.StatusQueued,
		CreatedAt:      time.Now().Add(-5 * time.Minute),
	}
	if err := repo.Create(task2); err != nil {
		t.Fatalf("Failed to create task2: %v", err)
	}

	// Test GetNextQueued (ADR-0002 serial FIFO)
	queued, err := repo.GetNextQueued()
	if err != nil {
		t.Fatalf("Failed to get queued task: %v", err)
	}
	if queued == nil || queued.ID != "task-002" {
		t.Fatalf("Expected task-002 to be next in queue, got %+v", queued)
	}

	// Test UpdateProgress
	prog := domain.TaskProgress{
		Percent:    45.5,
		CurrentFPS: 22.0,
		Speed:      "1.5x",
	}
	if err := repo.UpdateProgress(task2.ID, prog); err != nil {
		t.Fatalf("Failed to update progress: %v", err)
	}

	updated, err := repo.GetByID(task2.ID)
	if err != nil {
		t.Fatalf("Failed to get task2: %v", err)
	}
	if updated.Progress.Percent != 45.5 {
		t.Errorf("Expected progress 45.5%%, got %.1f%%", updated.Progress.Percent)
	}

	// Test List
	tasks, total, err := repo.List("", 10, 0)
	if err != nil {
		t.Fatalf("Failed to list tasks: %v", err)
	}
	if total != 2 || len(tasks) != 2 {
		t.Errorf("Expected 2 total tasks, got total=%d len=%d", total, len(tasks))
	}
}

func TestProfileRepository_CustomAndReset(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_profiles.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize test DB: %v", err)
	}
	defer db.Close()

	repo := NewProfileRepository(db)

	// Get initial 720p factory preset
	params, err := repo.GetEffectiveParams("720p")
	if err != nil {
		t.Fatalf("Failed to get 720p params: %v", err)
	}
	if params.CRF != 24 || params.VideoCodec != "libx265" {
		t.Errorf("Expected factory default 720p H.265 CRF 24, got codec=%s crf=%d", params.VideoCodec, params.CRF)
	}

	// Save custom user preset override (ADR-0007)
	custom := *params
	custom.CRF = 26
	custom.Preset = "veryfast"
	if err := repo.SaveCustom("720p", custom); err != nil {
		t.Fatalf("Failed to save custom profile: %v", err)
	}

	effective, err := repo.GetEffectiveParams("720p")
	if err != nil {
		t.Fatalf("Failed to get effective params: %v", err)
	}
	if effective.CRF != 26 || effective.Preset != "veryfast" {
		t.Errorf("Expected customized params CRF 26, got crf=%d preset=%s", effective.CRF, effective.Preset)
	}

	// Reset to factory default
	if err := repo.Reset("720p"); err != nil {
		t.Fatalf("Failed to reset profile: %v", err)
	}
	reverted, _ := repo.GetEffectiveParams("720p")
	if reverted.CRF != 24 {
		t.Errorf("Expected reverted CRF 24, got %d", reverted.CRF)
	}
}
