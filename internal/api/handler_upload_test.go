package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"slimbox/internal/domain"
	"slimbox/internal/repository"
)

func TestUploadHandler_ChunkWorkflow(t *testing.T) {
	tempDir := t.TempDir()
	db, err := repository.NewDB(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer db.Close()

	taskRepo := repository.NewTaskRepository(db)
	uploadDir := filepath.Join(tempDir, "uploads")
	handler := NewUploadHandler(taskRepo, uploadDir)

	uploadID := "upload-test-session-1"

	// 1. Upload Chunk 0
	chunk0Data := []byte("chunk-zero-payload-")
	body0 := &bytes.Buffer{}
	writer0 := multipart.NewWriter(body0)
	_ = writer0.WriteField("upload_id", uploadID)
	_ = writer0.WriteField("chunk_index", "0")
	_ = writer0.WriteField("total_chunks", "2")
	part0, _ := writer0.CreateFormFile("chunk", "blob")
	_, _ = part0.Write(chunk0Data)
	_ = writer0.Close()

	req0 := httptest.NewRequest(http.MethodPost, "/api/v1/upload/chunk", body0)
	req0.Header.Set("Content-Type", writer0.FormDataContentType())
	rr0 := httptest.NewRecorder()
	handler.HandleUploadChunk(rr0, req0)

	if rr0.Code != http.StatusOK {
		t.Fatalf("expected chunk 0 to return 200, got %d: %s", rr0.Code, rr0.Body.String())
	}

	// 2. Upload Chunk 1
	chunk1Data := []byte("chunk-one-payload")
	body1 := &bytes.Buffer{}
	writer1 := multipart.NewWriter(body1)
	_ = writer1.WriteField("upload_id", uploadID)
	_ = writer1.WriteField("chunk_index", "1")
	_ = writer1.WriteField("total_chunks", "2")
	part1, _ := writer1.CreateFormFile("chunk", "blob")
	_, _ = part1.Write(chunk1Data)
	_ = writer1.Close()

	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/upload/chunk", body1)
	req1.Header.Set("Content-Type", writer1.FormDataContentType())
	rr1 := httptest.NewRecorder()
	handler.HandleUploadChunk(rr1, req1)

	if rr1.Code != http.StatusOK {
		t.Fatalf("expected chunk 1 to return 200, got %d: %s", rr1.Code, rr1.Body.String())
	}

	// 3. Query Upload Status
	reqStatus := httptest.NewRequest(http.MethodGet, "/api/v1/upload/status?upload_id="+uploadID, nil)
	rrStatus := httptest.NewRecorder()
	handler.HandleUploadStatus(rrStatus, reqStatus)

	if rrStatus.Code != http.StatusOK {
		t.Fatalf("expected status query to return 200, got %d", rrStatus.Code)
	}

	var statusResp struct {
		UploadID       string `json:"upload_id"`
		UploadedChunks []int  `json:"uploaded_chunks"`
	}
	if err := json.Unmarshal(rrStatus.Body.Bytes(), &statusResp); err != nil {
		t.Fatalf("failed to decode status response: %v", err)
	}
	if len(statusResp.UploadedChunks) != 2 {
		t.Errorf("expected 2 uploaded chunks, got %d", len(statusResp.UploadedChunks))
	}

	// 4. Complete Upload
	completeReq := CompleteUploadRequest{
		UploadID:    uploadID,
		Filename:    "my_test_video.mp4",
		TotalChunks: 2,
	}
	completeJSON, _ := json.Marshal(completeReq)
	reqComplete := httptest.NewRequest(http.MethodPost, "/api/v1/upload/complete", bytes.NewReader(completeJSON))
	reqComplete.Header.Set("Content-Type", "application/json")
	rrComplete := httptest.NewRecorder()
	handler.HandleUploadComplete(rrComplete, reqComplete)

	if rrComplete.Code != http.StatusCreated {
		t.Fatalf("expected complete to return 201 Created, got %d: %s", rrComplete.Code, rrComplete.Body.String())
	}

	var task domain.Task
	if err := json.Unmarshal(rrComplete.Body.Bytes(), &task); err != nil {
		t.Fatalf("failed to decode task response: %v", err)
	}

	expectedSize := int64(len(chunk0Data) + len(chunk1Data))
	if task.SourceFileSize != expectedSize {
		t.Errorf("expected task file size %d, got %d", expectedSize, task.SourceFileSize)
	}

	// Verify merged file content on disk
	mergedContent, err := os.ReadFile(task.SourceFilePath)
	if err != nil {
		t.Fatalf("failed to read merged file: %v", err)
	}
	expectedContent := string(chunk0Data) + string(chunk1Data)
	if string(mergedContent) != expectedContent {
		t.Errorf("expected merged content %q, got %q", expectedContent, string(mergedContent))
	}

	// Verify temp directory was removed
	tempChunkDir := filepath.Join(uploadDir, "temp", uploadID)
	if _, err := os.Stat(tempChunkDir); !os.IsNotExist(err) {
		t.Errorf("expected temp chunk directory to be purged, but it still exists")
	}
}

func TestUploadHandler_DirectStreamUpload(t *testing.T) {
	tempDir := t.TempDir()
	db, err := repository.NewDB(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer db.Close()

	taskRepo := repository.NewTaskRepository(db)
	uploadDir := filepath.Join(tempDir, "uploads")
	handler := NewUploadHandler(taskRepo, uploadDir)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "direct_video.mp4")
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	fileData := []byte("direct-stream-payload-12345")
	_, _ = part.Write(fileData)
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()
	handler.HandleUpload(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", rr.Code, rr.Body.String())
	}

	var task domain.Task
	if err := json.Unmarshal(rr.Body.Bytes(), &task); err != nil {
		t.Fatalf("failed to unmarshal task: %v", err)
	}

	if task.SourceFileName != "direct_video.mp4" {
		t.Errorf("expected direct_video.mp4, got %s", task.SourceFileName)
	}
	if _, err := os.Stat(task.SourceFilePath); err != nil {
		t.Errorf("expected file to exist at %s, got: %v", task.SourceFilePath, err)
	}
}

