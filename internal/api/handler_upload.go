package api

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
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

			outFile, err := os.Create(destPath)
			if err != nil {
				WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to create destination file: %v", err))
				return
			}

			written, err := io.Copy(outFile, part)
			_ = outFile.Close()
			if err != nil {
				_ = os.Remove(destPath)
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
				_ = os.Remove(destPath)
				WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to save task: %v", err))
				return
			}

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
