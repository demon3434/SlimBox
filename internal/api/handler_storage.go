package api

import (
	"encoding/json"
	"net/http"
	"time"

	"slimbox/internal/engine"
	"slimbox/internal/repository"
)

type StorageHandler struct {
	taskRepo  *repository.TaskRepository
	uploadDir string
	outputDir string
}

func NewStorageHandler(taskRepo *repository.TaskRepository, uploadDir, outputDir string) *StorageHandler {
	return &StorageHandler{
		taskRepo:  taskRepo,
		uploadDir: uploadDir,
		outputDir: outputDir,
	}
}

// HandleScanOrphans handles GET /api/v1/system/orphans/scan
func (h *StorageHandler) HandleScanOrphans(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	trackedPaths, err := h.taskRepo.GetAllTrackedFilePaths()
	if err != nil {
		WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Grace periods: 1 hour for temp chunk dirs, 5 minutes for uploads/outputs
	summary, err := engine.ScanOrphanArtifacts(h.uploadDir, h.outputDir, trackedPaths, 1*time.Hour, 5*time.Minute)
	if err != nil {
		WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, summary)
}

// CleanOrphansRequest payload
type CleanOrphansRequest struct {
	Categories []string `json:"categories,omitempty"` // optional filter: "temp_chunk", "unlinked_upload", "unlinked_output", "stale_partial"
}

// HandleCleanOrphans handles POST /api/v1/system/orphans/clean
func (h *StorageHandler) HandleCleanOrphans(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req CleanOrphansRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	trackedPaths, err := h.taskRepo.GetAllTrackedFilePaths()
	if err != nil {
		WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	summary, err := engine.ScanOrphanArtifacts(h.uploadDir, h.outputDir, trackedPaths, 1*time.Hour, 5*time.Minute)
	if err != nil {
		WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var itemsToPurge []engine.OrphanItem
	if len(req.Categories) > 0 {
		allowed := make(map[string]bool)
		for _, cat := range req.Categories {
			allowed[cat] = true
		}
		for _, it := range summary.Items {
			if allowed[it.Category] {
				itemsToPurge = append(itemsToPurge, it)
			}
		}
	} else {
		itemsToPurge = summary.Items
	}

	res, err := engine.PurgeOrphanArtifacts(itemsToPurge)
	if err != nil {
		WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":       true,
		"deleted_count": res.DeletedCount,
		"freed_bytes":   res.FreedBytes,
	})
}
