package api

import (
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

func TestTaskHandler_Retry(t *testing.T) {
	tempDir := t.TempDir()
	db, err := repository.NewDB(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer db.Close()

	taskRepo := repository.NewTaskRepository(db)
	profileRepo := repository.NewProfileRepository(db)
	settingsRepo := repository.NewSettingsRepository(db)
	transcoder := engine.NewTranscoder()
	queue := scheduler.NewSerialQueue(taskRepo, settingsRepo, transcoder)
	lifecycle := scheduler.NewLifecycleManager(taskRepo, settingsRepo)
	handler := NewTaskHandler(taskRepo, profileRepo, queue, lifecycle, tempDir)

	// Create a dummy source file
	sourcePath := filepath.Join(tempDir, "input.mp4")
	if err := os.WriteFile(sourcePath, []byte("fake video content"), 0644); err != nil {
		t.Fatalf("failed to write dummy source: %v", err)
	}

	task := &domain.Task{
		ID:             "task-retry-test-1",
		SourceFileName: "input.mp4",
		SourceFilePath: sourcePath,
		SourceFileSize: 18,
		Status:         domain.StatusFailed,
		ErrorMsg:       "Simulated error",
	}
	if err := taskRepo.Create(task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	// 1. Send Retry request
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-retry-test-1/retry", nil)
	rr := httptest.NewRecorder()
	handler.HandleRetryTask(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}

	// Check status is updated to queued
	updated, err := taskRepo.GetByID("task-retry-test-1")
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}
	if updated.Status != domain.StatusQueued {
		t.Errorf("expected status queued, got %s", updated.Status)
	}
	if updated.ErrorMsg != "" {
		t.Errorf("expected empty error_msg, got %s", updated.ErrorMsg)
	}
}

func TestHandleDownload(t *testing.T) {
	tempDir := t.TempDir()
	db, err := repository.NewDB(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer db.Close()

	taskRepo := repository.NewTaskRepository(db)
	profileRepo := repository.NewProfileRepository(db)
	settingsRepo := repository.NewSettingsRepository(db)
	transcoder := engine.NewTranscoder()
	queue := scheduler.NewSerialQueue(taskRepo, settingsRepo, transcoder)
	lifecycle := scheduler.NewLifecycleManager(taskRepo, settingsRepo)
	handler := NewTaskHandler(taskRepo, profileRepo, queue, lifecycle, tempDir)

	// Create dummy completed output file
	content := []byte("0123456789abcdefgh") // 18 bytes
	outputPath := filepath.Join(tempDir, "output.mp4")
	if err := os.WriteFile(outputPath, content, 0644); err != nil {
		t.Fatalf("failed to write dummy output: %v", err)
	}

	task := &domain.Task{
		ID:             "task-dl-1",
		SourceFileName: "input.mp4",
		OutputFileName: "output.mp4",
		OutputFilePath: outputPath,
		OutputFileSize: int64(len(content)),
		Status:         domain.StatusCompleted,
	}
	if err := taskRepo.Create(task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	// 1. Full download without Range header
	reqFull := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/task-dl-1/download", nil)
	rrFull := httptest.NewRecorder()
	handler.HandleDownload(rrFull, reqFull)

	if rrFull.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rrFull.Code)
	}
	if rrFull.Header().Get("Accept-Ranges") != "bytes" {
		t.Errorf("expected Accept-Ranges: bytes, got %q", rrFull.Header().Get("Accept-Ranges"))
	}
	if rrFull.Body.String() != string(content) {
		t.Errorf("expected full body %q, got %q", string(content), rrFull.Body.String())
	}

	// 2. Partial content with Range header (e.g. bytes=0-4 -> 5 bytes: "01234")
	reqRange := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/task-dl-1/download", nil)
	reqRange.Header.Set("Range", "bytes=0-4")
	rrRange := httptest.NewRecorder()
	handler.HandleDownload(rrRange, reqRange)

	if rrRange.Code != http.StatusPartialContent {
		t.Fatalf("expected 206 Partial Content, got %d", rrRange.Code)
	}
	if rrRange.Body.String() != "01234" {
		t.Errorf("expected body '01234', got %q", rrRange.Body.String())
	}
	contentRange := rrRange.Header().Get("Content-Range")
	if contentRange != "bytes 0-4/18" {
		t.Errorf("expected Content-Range 'bytes 0-4/18', got %q", contentRange)
	}
}

func TestTaskHandler_AbortActiveAlias(t *testing.T) {
	tempDir := t.TempDir()
	db, err := repository.NewDB(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer db.Close()

	taskRepo := repository.NewTaskRepository(db)
	profileRepo := repository.NewProfileRepository(db)
	settingsRepo := repository.NewSettingsRepository(db)
	transcoder := engine.NewTranscoder()
	queue := scheduler.NewSerialQueue(taskRepo, settingsRepo, transcoder)
	lifecycle := scheduler.NewLifecycleManager(taskRepo, settingsRepo)
	handler := NewTaskHandler(taskRepo, profileRepo, queue, lifecycle, tempDir)

	// Test aborting /active when no task is running -> 400
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/active/abort", nil)
	rr := httptest.NewRecorder()
	handler.HandleAbortTask(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when no task active, got %d", rr.Code)
	}
}

func TestTaskHandler_CompletedTaskTimestamps(t *testing.T) {
	tempDir := t.TempDir()
	db, err := repository.NewDB(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer db.Close()

	taskRepo := repository.NewTaskRepository(db)
	profileRepo := repository.NewProfileRepository(db)
	settingsRepo := repository.NewSettingsRepository(db)
	transcoder := engine.NewTranscoder()
	queue := scheduler.NewSerialQueue(taskRepo, settingsRepo, transcoder)
	lifecycle := scheduler.NewLifecycleManager(taskRepo, settingsRepo)
	handler := NewTaskHandler(taskRepo, profileRepo, queue, lifecycle, tempDir)

	startTime := time.Now().Add(-10 * time.Minute)
	completeTime := time.Now()
	task := &domain.Task{
		ID:             "task-done-timestamps",
		SourceFileName: "test.mp4",
		Status:         domain.StatusCompleted,
		StartedAt:      &startTime,
		CompletedAt:    &completeTime,
		MediaInfo: &domain.MediaInfo{
			DurationSeconds: 1200,
		},
	}
	if err := taskRepo.Create(task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/task-done-timestamps", nil)
	rr := httptest.NewRecorder()
	handler.HandleGetTask(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var res domain.Task
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to parse JSON response: %v", err)
	}
	if res.StartedAt == nil || res.CompletedAt == nil {
		t.Fatalf("expected non-nil StartedAt and CompletedAt, got StartedAt: %v, CompletedAt: %v", res.StartedAt, res.CompletedAt)
	}
}


