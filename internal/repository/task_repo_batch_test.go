package repository

import (
	"path/filepath"
	"testing"
	"time"

	"slimbox/internal/domain"
)

func TestTaskRepository_BatchOperations(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_batch.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize test DB: %v", err)
	}
	defer db.Close()

	repo := NewTaskRepository(db)

	tasks := []*domain.Task{
		{ID: "t-1", SourceFileName: "1.mp4", Status: domain.StatusPending, Priority: 0, CreatedAt: time.Now()},
		{ID: "t-2", SourceFileName: "2.mp4", Status: domain.StatusQueued, Priority: 1, CreatedAt: time.Now()},
		{ID: "t-3", SourceFileName: "3.mp4", Status: domain.StatusCompleted, Priority: 0, CreatedAt: time.Now()},
	}

	for _, task := range tasks {
		if err := repo.Create(task); err != nil {
			t.Fatalf("Failed to create task %s: %v", task.ID, err)
		}
	}

	// 1. Test GetByIDs
	found, err := repo.GetByIDs([]string{"t-1", "t-3"})
	if err != nil {
		t.Fatalf("GetByIDs failed: %v", err)
	}
	if len(found) != 2 {
		t.Fatalf("Expected 2 tasks, got %d", len(found))
	}

	// 2. Test BatchReorder
	if err := repo.BatchReorder([]string{"t-1", "t-2"}); err != nil {
		t.Fatalf("BatchReorder failed: %v", err)
	}

	reordered1, _ := repo.GetByID("t-1")
	reordered2, _ := repo.GetByID("t-2")
	if reordered1.Priority <= reordered2.Priority {
		t.Fatalf("Expected t-1 priority (%d) > t-2 priority (%d)", reordered1.Priority, reordered2.Priority)
	}

	// 3. Test BatchDelete
	if err := repo.BatchDelete([]string{"t-1", "t-2"}); err != nil {
		t.Fatalf("BatchDelete failed: %v", err)
	}

	remaining, _ := repo.GetByIDs([]string{"t-1", "t-2", "t-3"})
	if len(remaining) != 1 || remaining[0].ID != "t-3" {
		t.Fatalf("Expected only t-3 to remain, got %+v", remaining)
	}
}
