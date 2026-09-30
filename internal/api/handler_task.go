package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"slimbox/internal/domain"
	"slimbox/internal/engine"
	"slimbox/internal/repository"
	"slimbox/internal/scheduler"
)

type TaskHandler struct {
	taskRepo     *repository.TaskRepository
	profileRepo  *repository.ProfileRepository
	queue        *scheduler.SerialQueue
	lifecycle    *scheduler.LifecycleManager
	outputDir    string
}

func NewTaskHandler(
	taskRepo *repository.TaskRepository,
	profileRepo *repository.ProfileRepository,
	queue *scheduler.SerialQueue,
	lifecycle *scheduler.LifecycleManager,
	outputDir string,
) *TaskHandler {
	_ = os.MkdirAll(outputDir, 0755)
	return &TaskHandler{
		taskRepo:    taskRepo,
		profileRepo: profileRepo,
		queue:       queue,
		lifecycle:   lifecycle,
		outputDir:   outputDir,
	}
}

// HandleListTasks returns paginated tasks, optionally filtered by status.
func (h *TaskHandler) HandleListTasks(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	tasks, total, err := h.taskRepo.List(status, limit, offset)
	if err != nil {
		WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Enrich actively running task with in-memory live progress
	active := h.queue.GetActiveTask()
	if active != nil {
		for i, t := range tasks {
			if t.ID == active.ID {
				tasks[i].Progress = active.Progress
				break
			}
		}
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"tasks":  tasks,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// HandleGetTask returns a single task's details.
func (h *TaskHandler) HandleGetTask(w http.ResponseWriter, r *http.Request) {
	id := extractIDFromPath(r.URL.Path, "/api/v1/tasks/")
	task, err := h.taskRepo.GetByID(id)
	if err != nil || task == nil {
		WriteJSONError(w, http.StatusNotFound, "Task not found")
		return
	}

	active := h.queue.GetActiveTask()
	if active != nil && active.ID == task.ID {
		task.Progress = active.Progress
	}

	WriteJSON(w, http.StatusOK, task)
}

// StartTaskPayload is the JSON payload sent by UI or CLI to queue a task
type StartTaskPayload struct {
	ProfileName  string                 `json:"profile_name"` // e.g. "720p", "1080p", "custom"
	CustomParams *domain.TranscodeParams `json:"custom_params,omitempty"`
}

// HandleStartTask assigns compression parameters and places the task into the execution queue.
func (h *TaskHandler) HandleStartTask(w http.ResponseWriter, r *http.Request) {
	id := extractIDFromPath(r.URL.Path, "/api/v1/tasks/")
	// strip trailing "/start"
	id = strings.TrimSuffix(id, "/start")

	task, err := h.taskRepo.GetByID(id)
	if err != nil || task == nil {
		WriteJSONError(w, http.StatusNotFound, "Task not found")
		return
	}

	if task.Status != domain.StatusPending && task.Status != domain.StatusFailed && task.Status != domain.StatusAborted {
		WriteJSONError(w, http.StatusBadRequest, fmt.Sprintf("Task cannot be started in status %s", task.Status))
		return
	}

	var payload StartTaskPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		WriteJSONError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	var params domain.TranscodeParams
	if payload.CustomParams != nil && payload.ProfileName == "custom" {
		params = *payload.CustomParams
		params.ProfileName = "custom"
	} else if payload.ProfileName != "" {
		p, err := h.profileRepo.GetEffectiveParams(payload.ProfileName)
		if err != nil {
			WriteJSONError(w, http.StatusBadRequest, fmt.Sprintf("Unknown profile %s", payload.ProfileName))
			return
		}
		params = *p
		// If custom parameters were supplied on top of profile (e.g. override video codec to h264)
		if payload.CustomParams != nil {
			if payload.CustomParams.VideoCodec != "" {
				params.VideoCodec = payload.CustomParams.VideoCodec
			}
			if payload.CustomParams.CRF > 0 {
				params.CRF = payload.CustomParams.CRF
			}
			if payload.CustomParams.Preset != "" {
				params.Preset = payload.CustomParams.Preset
			}
			if payload.CustomParams.AudioBitrate != "" {
				params.AudioBitrate = payload.CustomParams.AudioBitrate
			}
			if payload.CustomParams.AudioCodec != "" {
				params.AudioCodec = payload.CustomParams.AudioCodec
			}
			if payload.CustomParams.AudioTrackPolicy != "" {
				params.AudioTrackPolicy = payload.CustomParams.AudioTrackPolicy
			}
			if payload.CustomParams.SubtitlePolicy != "" {
				params.SubtitlePolicy = payload.CustomParams.SubtitlePolicy
			}
			if payload.CustomParams.FastStart != nil {
				params.FastStart = payload.CustomParams.FastStart
			}
			if payload.CustomParams.KeyframeInterval > 0 {
				params.KeyframeInterval = payload.CustomParams.KeyframeInterval
			}
			if payload.CustomParams.TargetResolution != "" {
				params.TargetResolution = payload.CustomParams.TargetResolution
			}
		}
	} else {
		// Default to standard 720p profile for one-click startup from task card
		p, err := h.profileRepo.GetEffectiveParams("720p")
		if err != nil {
			WriteJSONError(w, http.StatusBadRequest, "Profile selection is required")
			return
		}
		params = *p
	}
	params.VideoCodec = engine.SanitizeVideoCodec(params.VideoCodec)

	// Prepare output filename using universal container decision (MP4 with FastStart preferred)
	outputFileName := engine.GenerateOutputFileName(task.SourceFileName, params.ProfileName, task.ID, task.MediaInfo, params.SubtitlePolicy)
	task.OutputFileName = outputFileName
	task.OutputFilePath = filepath.Join(h.outputDir, outputFileName)
	task.Params = params
	task.Status = domain.StatusQueued
	task.ErrorMsg = ""
	task.Progress = domain.TaskProgress{}

	if err := h.taskRepo.Update(task); err != nil {
		WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to queue task: %v", err))
		return
	}

	// Wake up the queue worker immediately
	h.queue.NotifyNewTask()

	WriteJSON(w, http.StatusOK, task)
}

// HandleAbortTask aborts an actively running task or cancels a queued task (ADR-0008).
func (h *TaskHandler) HandleAbortTask(w http.ResponseWriter, r *http.Request) {
	id := extractIDFromPath(r.URL.Path, "/api/v1/tasks/")
	id = strings.TrimSuffix(id, "/abort")

	actualID := id
	if id == "active" {
		actualID = h.queue.GetActiveTaskID()
	}

	if err := h.queue.AbortTask(id); err != nil {
		WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	var task *domain.Task
	if actualID != "" {
		task, _ = h.taskRepo.GetByID(actualID)
	}
	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Task aborted",
		"task":    task,
	})
}

// HandleRetryTask re-queues an aborted or failed task to run again.
func (h *TaskHandler) HandleRetryTask(w http.ResponseWriter, r *http.Request) {
	id := extractIDFromPath(r.URL.Path, "/api/v1/tasks/")
	id = strings.TrimSuffix(id, "/retry")

	task, err := h.taskRepo.GetByID(id)
	if err != nil || task == nil {
		WriteJSONError(w, http.StatusNotFound, "Task not found")
		return
	}

	if task.Status != domain.StatusAborted && task.Status != domain.StatusFailed {
		WriteJSONError(w, http.StatusBadRequest, fmt.Sprintf("Cannot retry task in status '%s'", task.Status))
		return
	}

	// Verify source file still exists
	if _, statErr := os.Stat(task.SourceFilePath); statErr != nil {
		WriteJSONError(w, http.StatusBadRequest, "源文件已不存在，无法重跑，请重新上传")
		return
	}

	// Clean partial output file if any
	engine.CleanupPartialFile(task.OutputFilePath)

	task.Status = domain.StatusQueued
	task.Priority = 0
	task.CreatedAt = time.Now()
	task.ErrorMsg = ""
	task.Progress = domain.TaskProgress{}
	task.StartedAt = nil
	task.CompletedAt = nil

	if err := h.taskRepo.Update(task); err != nil {
		WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to update task: %v", err))
		return
	}

	h.queue.NotifyNewTask()

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Task re-queued successfully",
		"task":    task,
	})
}

type AdjustPriorityPayload struct {
	Direction string `json:"direction"` // "up", "down", "top"
}

// HandleAdjustPriority reorders queued tasks by adjusting their priority.
func (h *TaskHandler) HandleAdjustPriority(w http.ResponseWriter, r *http.Request) {
	id := extractIDFromPath(r.URL.Path, "/api/v1/tasks/")
	id = strings.TrimSuffix(id, "/priority")

	var payload AdjustPriorityPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		WriteJSONError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	queued, err := h.taskRepo.GetQueuedTasks()
	if err != nil {
		WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	idx := -1
	for i, t := range queued {
		if t.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		WriteJSONError(w, http.StatusNotFound, "Task not found in queue")
		return
	}

	curTask := queued[idx]

	switch payload.Direction {
	case "top":
		maxPrio := 0
		for _, t := range queued {
			if t.Priority > maxPrio {
				maxPrio = t.Priority
			}
		}
		curTask.Priority = maxPrio + 1
		_ = h.taskRepo.UpdatePriority(curTask.ID, curTask.Priority)

	case "up":
		if idx > 0 {
			prevTask := queued[idx-1]
			if curTask.Priority <= prevTask.Priority {
				curTask.Priority = prevTask.Priority + 1
				_ = h.taskRepo.UpdatePriority(curTask.ID, curTask.Priority)
			} else {
				curTask.Priority, prevTask.Priority = prevTask.Priority, curTask.Priority
				_ = h.taskRepo.UpdatePriority(curTask.ID, curTask.Priority)
				_ = h.taskRepo.UpdatePriority(prevTask.ID, prevTask.Priority)
			}
		}

	case "down":
		if idx < len(queued)-1 {
			nextTask := queued[idx+1]
			if curTask.Priority >= nextTask.Priority {
				curTask.Priority = nextTask.Priority - 1
				_ = h.taskRepo.UpdatePriority(curTask.ID, curTask.Priority)
			} else {
				curTask.Priority, nextTask.Priority = nextTask.Priority, curTask.Priority
				_ = h.taskRepo.UpdatePriority(curTask.ID, curTask.Priority)
				_ = h.taskRepo.UpdatePriority(nextTask.ID, nextTask.Priority)
			}
		}
	case "bottom":
		minPrio := 0
		for _, t := range queued {
			if t.Priority < minPrio {
				minPrio = t.Priority
			}
		}
		curTask.Priority = minPrio - 1
		_ = h.taskRepo.UpdatePriority(curTask.ID, curTask.Priority)

	default:
		WriteJSONError(w, http.StatusBadRequest, "Invalid direction: must be 'up', 'down', 'top', or 'bottom'")
		return
	}

	h.queue.NotifyNewTask()

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"task_id":  curTask.ID,
		"priority": curTask.Priority,
	})
}

// HandleDeleteTask deletes a task and associated files.
func (h *TaskHandler) HandleDeleteTask(w http.ResponseWriter, r *http.Request) {
	id := extractIDFromPath(r.URL.Path, "/api/v1/tasks/")

	task, err := h.taskRepo.GetByID(id)
	if err != nil || task == nil {
		WriteJSONError(w, http.StatusNotFound, "Task not found")
		return
	}

	if task.Status == domain.StatusTranscoding {
		_ = h.queue.AbortTask(id)
	}

	_ = engine.RemoveFile(task.SourceFilePath)
	_ = engine.RemoveFile(task.OutputFilePath)
	engine.CleanupPartialFile(task.OutputFilePath)

	if err := h.taskRepo.Delete(id); err != nil {
		WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Task and associated files deleted",
	})
}

// HandleDownload streams the compressed output file to the client.
func (h *TaskHandler) HandleDownload(w http.ResponseWriter, r *http.Request) {
	id := extractIDFromPath(r.URL.Path, "/api/v1/tasks/")
	id = strings.TrimSuffix(id, "/download")

	task, err := h.taskRepo.GetByID(id)
	if err != nil || task == nil {
		http.NotFound(w, r)
		return
	}

	if task.Status != domain.StatusCompleted || task.OutputFilePath == "" {
		WriteJSONError(w, http.StatusBadRequest, "Task has not completed or output file is unavailable")
		return
	}

	file, err := os.Open(task.OutputFilePath)
	if err != nil {
		WriteJSONError(w, http.StatusNotFound, "Output file not found on disk")
		return
	}
	defer file.Close()

	fi, err := file.Stat()
	if err != nil {
		WriteJSONError(w, http.StatusInternalServerError, "Failed to read file info")
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", task.OutputFileName))
	w.Header().Set("Content-Type", "application/octet-stream")

	http.ServeContent(w, r, task.OutputFileName, fi.ModTime(), file)

	// Trigger lifecycle hook after download completes (ADR-0004)
	go h.lifecycle.HandleDownloadCompleted(task)
}

func extractIDFromPath(path, prefix string) string {
	rest := strings.TrimPrefix(path, prefix)
	parts := strings.Split(rest, "/")
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}
