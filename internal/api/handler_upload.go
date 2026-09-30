package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"slimbox/internal/domain"
	"slimbox/internal/engine"
	"slimbox/internal/repository"
)

type UploadHandler struct {
	taskRepo  *repository.TaskRepository
	uploadDir string
}

func NewUploadHandler(taskRepo *repository.TaskRepository, uploadDir string) *UploadHandler {
	_ = os.MkdirAll(uploadDir, 0755)
	return &UploadHandler{
		taskRepo:  taskRepo,
		uploadDir: uploadDir,
	}
}

func (h *UploadHandler) HandleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// 32GB maximum upload memory footprint (buffered to temporary disk files)
	mr, err := r.MultipartReader()
	if err != nil {
		WriteJSONError(w, http.StatusBadRequest, fmt.Sprintf("Failed to read multipart request: %v", err))
		return
	}

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			WriteJSONError(w, http.StatusBadRequest, fmt.Sprintf("Error reading multipart part: %v", err))
			return
		}

		if part.FormName() == "file" {
			originalName := filepath.Base(part.FileName())
			if originalName == "" || originalName == "." {
				originalName = fmt.Sprintf("video_%d.mp4", time.Now().Unix())
			}

			taskID := uuid.New().String()
			safeFileName := fmt.Sprintf("%s_%s", taskID[:8], sanitizeFileName(originalName))
			destPath := filepath.Join(h.uploadDir, safeFileName)

			uploadSuccess := false
			defer func() {
				if !uploadSuccess && destPath != "" {
					_ = os.Remove(destPath)
				}
			}()

			outFile, err := os.Create(destPath)
			if err != nil {
				WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to create destination file: %v", err))
				return
			}

			written, err := io.Copy(outFile, part)
			_ = outFile.Close()
			if err != nil {
				WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to save uploaded file: %v", err))
				return
			}

			log.Printf("[Upload] Received file %s (%d bytes). Probing media metadata...", originalName, written)

			// Probe media immediately
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			mediaInfo, probeErr := engine.ProbeMedia(ctx, destPath)
			cancel()

			if probeErr != nil {
				log.Printf("[Upload] Warning: ffprobe failed for %s: %v", destPath, probeErr)
				// Create minimal fallback info
				mediaInfo = &domain.MediaInfo{
					FileSizeBytes: written,
				}
			}

			task := &domain.Task{
				ID:             taskID,
				SourceFileName: originalName,
				SourceFilePath: destPath,
				SourceFileSize: written,
				Status:         domain.StatusPending,
				MediaInfo:      mediaInfo,
				CreatedAt:      time.Now(),
			}

			if err := h.taskRepo.Create(task); err != nil {
				WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to save task: %v", err))
				return
			}

			uploadSuccess = true
			WriteJSON(w, http.StatusCreated, task)
			return
		}
	}

	WriteJSONError(w, http.StatusBadRequest, "No file provided in 'file' field")
}

func sanitizeFileName(name string) string {
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	return name
}

func sanitizeUploadID(id string) string {
	var clean strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			clean.WriteRune(r)
		}
	}
	return clean.String()
}

// HandleUploadChunk receives an individual chunk part and stores it in a temp directory.
func (h *UploadHandler) HandleUploadChunk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		WriteJSONError(w, http.StatusBadRequest, fmt.Sprintf("Failed to parse form: %v", err))
		return
	}

	uploadID := sanitizeUploadID(r.FormValue("upload_id"))
	if uploadID == "" {
		WriteJSONError(w, http.StatusBadRequest, "Missing or invalid upload_id")
		return
	}

	chunkIndexStr := r.FormValue("chunk_index")
	chunkIndex, err := strconv.Atoi(chunkIndexStr)
	if err != nil || chunkIndex < 0 {
		WriteJSONError(w, http.StatusBadRequest, "Invalid chunk_index")
		return
	}

	file, _, err := r.FormFile("chunk")
	if err != nil {
		file, _, err = r.FormFile("file")
		if err != nil {
			WriteJSONError(w, http.StatusBadRequest, "Missing chunk file in form")
			return
		}
	}
	defer file.Close()

	tempDir := filepath.Join(h.uploadDir, "temp", uploadID)
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to create chunk temp dir: %v", err))
		return
	}

	chunkPath := filepath.Join(tempDir, fmt.Sprintf("%d.part", chunkIndex))
	outFile, err := os.Create(chunkPath)
	if err != nil {
		WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to create chunk file: %v", err))
		return
	}

	_, err = io.Copy(outFile, file)
	_ = outFile.Close()
	if err != nil {
		_ = os.Remove(chunkPath)
		WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to write chunk: %v", err))
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status":      "ok",
		"upload_id":   uploadID,
		"chunk_index": chunkIndex,
	})
}

// HandleUploadStatus returns the indexes of chunks already uploaded for this session.
func (h *UploadHandler) HandleUploadStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	uploadID := sanitizeUploadID(r.URL.Query().Get("upload_id"))
	if uploadID == "" {
		WriteJSONError(w, http.StatusBadRequest, "Missing upload_id parameter")
		return
	}

	tempDir := filepath.Join(h.uploadDir, "temp", uploadID)
	entries, err := os.ReadDir(tempDir)
	uploaded := make([]int, 0)
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".part") {
				idxStr := strings.TrimSuffix(e.Name(), ".part")
				if idx, err := strconv.Atoi(idxStr); err == nil {
					uploaded = append(uploaded, idx)
				}
			}
		}
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"upload_id":       uploadID,
		"uploaded_chunks": uploaded,
	})
}

// CompleteUploadRequest payload for completing chunk upload.
type CompleteUploadRequest struct {
	UploadID    string `json:"upload_id"`
	Filename    string `json:"filename"`
	TotalChunks int    `json:"total_chunks"`
}

// HandleUploadComplete sequentially merges all chunks into the final video file and probes metadata.
func (h *UploadHandler) HandleUploadComplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req CompleteUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	uploadID := sanitizeUploadID(req.UploadID)
	if uploadID == "" || req.TotalChunks <= 0 {
		WriteJSONError(w, http.StatusBadRequest, "Missing upload_id or total_chunks")
		return
	}

	originalName := filepath.Base(req.Filename)
	if originalName == "" || originalName == "." {
		originalName = fmt.Sprintf("video_%d.mp4", time.Now().Unix())
	}

	tempDir := filepath.Join(h.uploadDir, "temp", uploadID)

	// Validate that all chunks from 0 to TotalChunks-1 exist
	for i := 0; i < req.TotalChunks; i++ {
		partPath := filepath.Join(tempDir, fmt.Sprintf("%d.part", i))
		if _, err := os.Stat(partPath); err != nil {
			WriteJSONError(w, http.StatusBadRequest, fmt.Sprintf("Missing chunk %d", i))
			return
		}
	}

	taskID := uuid.New().String()
	safeFileName := fmt.Sprintf("%s_%s", taskID[:8], sanitizeFileName(originalName))
	destPath := filepath.Join(h.uploadDir, safeFileName)

	outFile, err := os.Create(destPath)
	if err != nil {
		WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to create destination file: %v", err))
		return
	}

	var totalWritten int64
	for i := 0; i < req.TotalChunks; i++ {
		partPath := filepath.Join(tempDir, fmt.Sprintf("%d.part", i))
		partFile, err := os.Open(partPath)
		if err != nil {
			_ = outFile.Close()
			_ = os.Remove(destPath)
			WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to read chunk %d: %v", i, err))
			return
		}
		written, copyErr := io.Copy(outFile, partFile)
		_ = partFile.Close()
		if copyErr != nil {
			_ = outFile.Close()
			_ = os.Remove(destPath)
			WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to append chunk %d: %v", i, copyErr))
			return
		}
		totalWritten += written
	}
	_ = outFile.Close()
	_ = os.RemoveAll(tempDir)

	log.Printf("[Upload] Merged %d chunks for %s (%d bytes). Probing media metadata...", req.TotalChunks, originalName, totalWritten)

	// Probe media
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	mediaInfo, probeErr := engine.ProbeMedia(ctx, destPath)
	cancel()

	if probeErr != nil {
		log.Printf("[Upload] Warning: ffprobe failed for %s: %v", destPath, probeErr)
		mediaInfo = &domain.MediaInfo{
			FileSizeBytes: totalWritten,
		}
	}

	task := &domain.Task{
		ID:             taskID,
		SourceFileName: originalName,
		SourceFilePath: destPath,
		SourceFileSize: totalWritten,
		Status:         domain.StatusPending,
		MediaInfo:      mediaInfo,
		CreatedAt:      time.Now(),
	}

	if err := h.taskRepo.Create(task); err != nil {
		_ = os.Remove(destPath)
		WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to save task: %v", err))
		return
	}

	WriteJSON(w, http.StatusCreated, task)
}

