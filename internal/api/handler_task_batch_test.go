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
	"slimbox/internal/scheduler"
)

func setupTestBatchHandler(t *testing.T) (*TaskHandler, *repository.TaskRepository, string) {
	tempDir := t.TempDir()
	db, err := repository.NewDB(filepath.Join(tempDir, "test_batch.db"))
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	taskRepo := repository.NewTaskRepository(db)
	profileRepo := repository.NewProfileRepository(db)
	settingsRepo := repository.NewSettingsRepository(db)
	transcoder := engine.NewTranscoder()
	queue := scheduler.NewSerialQueue(taskRepo, settingsRepo, transcoder)
	lifecycle := scheduler.NewLifecycleManager(taskRepo, settingsRepo)
	handler := NewTaskHandler(taskRepo, profileRepo, queue, lifecycle, tempDir)

	return handler, taskRepo, tempDir
}

func TestTaskHandler_BatchEndpoints(t *testing.T) {
	handler, taskRepo, tempDir := setupTestBatchHandler(t)

	// Create test files
	file1 := filepath.Join(tempDir, "1.mp4")
	file2 := filepath.Join(tempDir, "2.mp4")
	_ = os.WriteFile(file1, []byte("data1"), 0644)
	_ = os.WriteFile(file2, []byte("data2"), 0644)

	// Setup tasks
	t1 := &domain.Task{ID: "t-pending-1", SourceFileName: "1.mp4", SourceFilePath: file1, Status: domain.StatusPending, CreatedAt: time.Now()}
	t2 := &domain.Task{ID: "t-pending-2", SourceFileName: "2.mp4", SourceFilePath: file2, Status: domain.StatusPending, CreatedAt: time.Now()}
	t3 := &domain.Task{ID: "t-failed-1", SourceFileName: "1.mp4", SourceFilePath: file1, Status: domain.StatusFailed, CreatedAt: time.Now()}
	_ = taskRepo.Create(t1)
	_ = taskRepo.Create(t2)
	_ = taskRepo.Create(t3)

	// 1. Test BatchStart
	startBody, _ := json.Marshal(BatchIDsPayload{TaskIDs: []string{"t-pending-1", "t-pending-2"}})
	reqStart := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/batch-start", bytes.NewReader(startBody))
	rrStart := httptest.NewRecorder()
	handler.HandleBatchStart(rrStart, reqStart)

	if rrStart.Code != http.StatusOK {
		t.Fatalf("BatchStart failed with status %d: %s", rrStart.Code, rrStart.Body.String())
	}
	t1Updated, _ := taskRepo.GetByID("t-pending-1")
	if t1Updated.Status != domain.StatusQueued {
		t.Fatalf("Expected t-pending-1 to be queued, got %s", t1Updated.Status)
	}

	// 2. Test BatchReorder
	reorderBody, _ := json.Marshal(ReorderPayload{OrderedIDs: []string{"t-pending-2", "t-pending-1"}})
	reqReorder := httptest.NewRequest(http.MethodPut, "/api/v1/tasks/reorder", bytes.NewReader(reorderBody))
	rrReorder := httptest.NewRecorder()
	handler.HandleReorderTasks(rrReorder, reqReorder)

	if rrReorder.Code != http.StatusOK {
		t.Fatalf("Reorder failed with status %d: %s", rrReorder.Code, rrReorder.Body.String())
	}
	p2, _ := taskRepo.GetByID("t-pending-2")
	p1, _ := taskRepo.GetByID("t-pending-1")
	if p2.Priority <= p1.Priority {
		t.Fatalf("Expected p2 (%d) > p1 (%d)", p2.Priority, p1.Priority)
	}

	// 3. Test BatchRetry
	retryBody, _ := json.Marshal(BatchIDsPayload{TaskIDs: []string{"t-failed-1"}})
	reqRetry := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/batch-retry", bytes.NewReader(retryBody))
	rrRetry := httptest.NewRecorder()
	handler.HandleBatchRetry(rrRetry, reqRetry)

	if rrRetry.Code != http.StatusOK {
		t.Fatalf("BatchRetry failed with status %d: %s", rrRetry.Code, rrRetry.Body.String())
	}
	t3Updated, _ := taskRepo.GetByID("t-failed-1")
	if t3Updated.Status != domain.StatusQueued {
		t.Fatalf("Expected t-failed-1 to be queued, got %s", t3Updated.Status)
	}

	// 4. Test BatchDelete
	delBody, _ := json.Marshal(BatchIDsPayload{TaskIDs: []string{"t-pending-1", "t-failed-1"}})
	reqDel := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/batch-delete", bytes.NewReader(delBody))
	rrDel := httptest.NewRecorder()
	handler.HandleBatchDelete(rrDel, reqDel)

	if rrDel.Code != http.StatusOK {
		t.Fatalf("BatchDelete failed with status %d: %s", rrDel.Code, rrDel.Body.String())
	}
	left, _ := taskRepo.GetByIDs([]string{"t-pending-1", "t-pending-2", "t-failed-1"})
	if len(left) != 1 || left[0].ID != "t-pending-2" {
		t.Fatalf("Expected only t-pending-2 to remain, got %d tasks", len(left))
	}
}

func TestTaskHandler_BatchStartWithParams(t *testing.T) {
	handler, taskRepo, tempDir := setupTestBatchHandler(t)

	f1 := filepath.Join(tempDir, "p1.mp4")
	f2 := filepath.Join(tempDir, "p2.mp4")
	_ = os.WriteFile(f1, []byte("p1"), 0644)
	_ = os.WriteFile(f2, []byte("p2"), 0644)

	t1 := &domain.Task{ID: "t-p1", SourceFileName: "p1.mp4", SourceFilePath: f1, Status: domain.StatusPending, CreatedAt: time.Now()}
	t2 := &domain.Task{ID: "t-p2", SourceFileName: "p2.mp4", SourceFilePath: f2, Status: domain.StatusPending, CreatedAt: time.Now()}
	_ = taskRepo.Create(t1)
	_ = taskRepo.Create(t2)

	// 1. Batch start with preset profile "1080p"
	bodyPreset, _ := json.Marshal(BatchStartPayload{
		TaskIDs:     []string{"t-p1", "t-p2"},
		ProfileName: "1080p",
	})
	reqPreset := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/batch-start", bytes.NewReader(bodyPreset))
	rrPreset := httptest.NewRecorder()
	handler.HandleBatchStart(rrPreset, reqPreset)

	if rrPreset.Code != http.StatusOK {
		t.Fatalf("BatchStart with preset failed: %s", rrPreset.Body.String())
	}

	t1Updated, _ := taskRepo.GetByID("t-p1")
	t2Updated, _ := taskRepo.GetByID("t-p2")
	if t1Updated.Status != domain.StatusQueued || t1Updated.Params.ProfileName != "1080p" {
		t.Fatalf("Expected t-p1 to be queued with 1080p, got status=%s profile=%s", t1Updated.Status, t1Updated.Params.ProfileName)
	}
	if t2Updated.Status != domain.StatusQueued || t2Updated.Params.ProfileName != "1080p" {
		t.Fatalf("Expected t-p2 to be queued with 1080p, got status=%s profile=%s", t2Updated.Status, t2Updated.Params.ProfileName)
	}

	// 2. Batch start with custom parameters
	f3 := filepath.Join(tempDir, "p3.mp4")
	_ = os.WriteFile(f3, []byte("p3"), 0644)
	t3 := &domain.Task{ID: "t-p3", SourceFileName: "p3.mp4", SourceFilePath: f3, Status: domain.StatusPending, CreatedAt: time.Now()}
	_ = taskRepo.Create(t3)

	bodyCustom, _ := json.Marshal(BatchStartPayload{
		TaskIDs:     []string{"t-p3"},
		ProfileName: "custom",
		CustomParams: &domain.TranscodeParams{
			VideoCodec: "libx265",
			CRF:        21,
			Preset:     "fast",
		},
	})
	reqCustom := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/batch-start", bytes.NewReader(bodyCustom))
	rrCustom := httptest.NewRecorder()
	handler.HandleBatchStart(rrCustom, reqCustom)

	if rrCustom.Code != http.StatusOK {
		t.Fatalf("BatchStart with custom failed: %s", rrCustom.Body.String())
	}
	t3Updated, _ := taskRepo.GetByID("t-p3")
	if t3Updated.Status != domain.StatusQueued || t3Updated.Params.ProfileName != "custom" || t3Updated.Params.CRF != 21 {
		t.Fatalf("Expected t-p3 to have custom CRF 21, got profile=%s CRF=%d", t3Updated.Params.ProfileName, t3Updated.Params.CRF)
	}
}
