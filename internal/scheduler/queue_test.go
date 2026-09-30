package scheduler

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"slimbox/internal/domain"
	"slimbox/internal/engine"
	"slimbox/internal/repository"
)

func TestSerialQueue_ProcessTaskStartedAt(t *testing.T) {
	tempDir := t.TempDir()
	db, err := repository.NewDB(filepath.Join(tempDir, "test_queue.db"))
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer db.Close()

	taskRepo := repository.NewTaskRepository(db)
	settingsRepo := repository.NewSettingsRepository(db)
	transcoder := engine.NewTranscoder()
	queue := NewSerialQueue(taskRepo, settingsRepo, transcoder)

	dummySource := filepath.Join(tempDir, "input.mp4")
	_ = os.WriteFile(dummySource, []byte("fake video"), 0644)

	task := &domain.Task{
		ID:             "queue-task-1",
		SourceFileName: "input.mp4",
		SourceFilePath: dummySource,
		SourceFileSize: 10,
		Status:         domain.StatusQueued,
		CreatedAt:      time.Now(),
	}
	if err := taskRepo.Create(task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	// Run processNext (will attempt transcoding, fail gracefully on invalid video, and update status)
	queue.processNext()

	// Verify the task in database recorded started_at
	persisted, err := taskRepo.GetByID(task.ID)
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}
	if persisted.StartedAt == nil {
		t.Fatalf("expected non-nil StartedAt after processNext, got nil")
	}

	// Simulate successful completion update on the in-memory task
	completedTime := time.Now()
	persisted.Status = domain.StatusCompleted
	persisted.CompletedAt = &completedTime
	if err := taskRepo.Update(persisted); err != nil {
		t.Fatalf("failed to update task: %v", err)
	}

	completedTask, err := taskRepo.GetByID(task.ID)
	if err != nil {
		t.Fatalf("failed to fetch completed task: %v", err)
	}
	if completedTask.StartedAt == nil {
		t.Fatalf("expected StartedAt to be preserved after completion update, got nil")
	}
	if completedTask.CompletedAt == nil {
		t.Fatalf("expected CompletedAt to be non-nil")
	}
}

func TestSerialQueue_ZombieTaskRecoveryAndAbort(t *testing.T) {
	tempDir := t.TempDir()
	db, err := repository.NewDB(filepath.Join(tempDir, "test_zombie.db"))
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer db.Close()

	taskRepo := repository.NewTaskRepository(db)
	settingsRepo := repository.NewSettingsRepository(db)
	transcoder := engine.NewTranscoder()
	queue := NewSerialQueue(taskRepo, settingsRepo, transcoder)

	partFile := filepath.Join(tempDir, "out.mp4.part")
	_ = os.WriteFile(partFile, []byte("partial output"), 0644)

	zombieTask1 := &domain.Task{
		ID:             "zombie-1",
		SourceFileName: "zombie1.mp4",
		OutputFilePath: filepath.Join(tempDir, "out.mp4"),
		Status:         domain.StatusTranscoding,
		CreatedAt:      time.Now(),
	}
	_ = taskRepo.Create(zombieTask1)

	// 1. Test explicit AbortTask on non-active transcoding task
	if err := queue.AbortTask("zombie-1"); err != nil {
		t.Fatalf("expected AbortTask to succeed on orphaned transcoding task, got: %v", err)
	}
	res1, _ := taskRepo.GetByID("zombie-1")
	if res1.Status != domain.StatusAborted {
		t.Errorf("expected status aborted, got: %s", res1.Status)
	}
	if _, err := os.Stat(partFile); !os.IsNotExist(err) {
		t.Errorf("expected partial file to be cleaned up, but it still exists")
	}

	// 2. Test recoverZombieTasks on startup
	partFile2 := filepath.Join(tempDir, "out2.mp4.part")
	_ = os.WriteFile(partFile2, []byte("partial output 2"), 0644)

	// zombieTask2 has no source file -> should abort
	zombieTask2 := &domain.Task{
		ID:             "zombie-2",
		SourceFileName: "zombie2.mp4",
		OutputFilePath: filepath.Join(tempDir, "out2.mp4"),
		Status:         domain.StatusTranscoding,
		CreatedAt:      time.Now(),
	}
	_ = taskRepo.Create(zombieTask2)

	// zombieTask3 has an existing source file -> should automatically re-queue
	sourceFile3 := filepath.Join(tempDir, "src3.mp4")
	_ = os.WriteFile(sourceFile3, []byte("valid video source"), 0644)
	partFile3 := filepath.Join(tempDir, "out3.mp4.part")
	_ = os.WriteFile(partFile3, []byte("partial artifact 3"), 0644)

	zombieTask3 := &domain.Task{
		ID:             "zombie-3",
		SourceFileName: "zombie3.mp4",
		SourceFilePath: sourceFile3,
		OutputFilePath: filepath.Join(tempDir, "out3.mp4"),
		Status:         domain.StatusTranscoding,
		CreatedAt:      time.Now(),
	}
	_ = taskRepo.Create(zombieTask3)

	queue.recoverZombieTasks()

	res2, _ := taskRepo.GetByID("zombie-2")
	if res2.Status != domain.StatusAborted {
		t.Errorf("expected status aborted after recoverZombieTasks for missing source, got: %s", res2.Status)
	}
	if _, err := os.Stat(partFile2); !os.IsNotExist(err) {
		t.Errorf("expected partial file 2 to be cleaned up")
	}

	res3, _ := taskRepo.GetByID("zombie-3")
	if res3.Status != domain.StatusQueued {
		t.Errorf("expected status queued (auto-recovered) after recoverZombieTasks for intact source, got: %s", res3.Status)
	}
	if _, err := os.Stat(partFile3); !os.IsNotExist(err) {
		t.Errorf("expected partial file 3 to be cleaned up during auto-requeue")
	}
}

func TestSerialQueue_ExternalStorageDisconnection(t *testing.T) {
	tempDir := t.TempDir()
	db, err := repository.NewDB(filepath.Join(tempDir, "test_storage.db"))
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer db.Close()

	taskRepo := repository.NewTaskRepository(db)
	settingsRepo := repository.NewSettingsRepository(db)
	transcoder := engine.NewTranscoder()
	queue := NewSerialQueue(taskRepo, settingsRepo, transcoder)

	// Task referencing a non-existent external volume path
	task := &domain.Task{
		ID:             "ext-vol-task",
		SourceFileName: "missing.mp4",
		SourceFilePath: filepath.Join(tempDir, "non_existent_volume", "missing.mp4"),
		OutputFilePath: filepath.Join(tempDir, "non_existent_volume", "out.mp4"),
		Status:         domain.StatusQueued,
		CreatedAt:      time.Now(),
	}
	if err := taskRepo.Create(task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	queue.processNext()

	updated, err := taskRepo.GetByID("ext-vol-task")
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}
	if updated.Status != domain.StatusFailed {
		t.Errorf("expected task to fail gracefully when storage volume is disconnected, got status: %s", updated.Status)
	}
	if updated.ErrorMsg == "" {
		t.Errorf("expected error message describing disconnected volume")
	}
}


