package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"slimbox/internal/domain"
	"slimbox/internal/engine"
)

type BatchIDsPayload struct {
	TaskIDs []string `json:"task_ids"`
}

type BatchStartPayload struct {
	TaskIDs      []string                `json:"task_ids"`
	ProfileName  string                  `json:"profile_name,omitempty"`
	CustomParams *domain.TranscodeParams `json:"custom_params,omitempty"`
}

type ReorderPayload struct {
	OrderedIDs []string `json:"ordered_ids"`
}

func (h *TaskHandler) resolveTranscodeParams(profileName string, customParams *domain.TranscodeParams) (domain.TranscodeParams, error) {
	if customParams != nil && profileName == "custom" {
		params := *customParams
		params.ProfileName = "custom"
		params.VideoCodec = engine.SanitizeVideoCodec(params.VideoCodec)
		return params, nil
	}
	if profileName != "" {
		p, err := h.profileRepo.GetEffectiveParams(profileName)
		if err != nil {
			return domain.TranscodeParams{}, fmt.Errorf("unknown profile %s", profileName)
		}
		params := *p
		if customParams != nil {
			if customParams.VideoCodec != "" {
				params.VideoCodec = customParams.VideoCodec
			}
			if customParams.CRF > 0 {
				params.CRF = customParams.CRF
			}
			if customParams.Preset != "" {
				params.Preset = customParams.Preset
			}
			if customParams.AudioBitrate != "" {
				params.AudioBitrate = customParams.AudioBitrate
			}
			if customParams.AudioCodec != "" {
				params.AudioCodec = customParams.AudioCodec
			}
			if customParams.AudioTrackPolicy != "" {
				params.AudioTrackPolicy = customParams.AudioTrackPolicy
			}
			if customParams.SubtitlePolicy != "" {
				params.SubtitlePolicy = customParams.SubtitlePolicy
			}
			if customParams.FastStart != nil {
				params.FastStart = customParams.FastStart
			}
			if customParams.KeyframeInterval > 0 {
				params.KeyframeInterval = customParams.KeyframeInterval
			}
			if customParams.TargetResolution != "" {
				params.TargetResolution = customParams.TargetResolution
			}
		}
		params.VideoCodec = engine.SanitizeVideoCodec(params.VideoCodec)
		return params, nil
	}
	p, err := h.profileRepo.GetEffectiveParams("720p")
	if err != nil {
		return domain.TranscodeParams{
			ProfileName: "720p",
			VideoCodec:  engine.SanitizeVideoCodec("libx265"),
			CRF:         28,
			Preset:      "medium",
		}, nil
	}
	params := *p
	params.VideoCodec = engine.SanitizeVideoCodec(params.VideoCodec)
	return params, nil
}

// HandleBatchDelete handles atomic batch deletion of tasks and associated files.
func (h *TaskHandler) HandleBatchDelete(w http.ResponseWriter, r *http.Request) {
	var payload BatchIDsPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		WriteJSONError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	if len(payload.TaskIDs) == 0 {
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"success":       true,
			"deleted_count": 0,
		})
		return
	}

	tasks, err := h.taskRepo.GetByIDs(payload.TaskIDs)
	if err != nil {
		WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	for _, task := range tasks {
		if task.Status == domain.StatusTranscoding {
			_ = h.queue.AbortTask(task.ID)
		}
		_ = engine.RemoveFile(task.SourceFilePath)
		_ = engine.RemoveFile(task.OutputFilePath)
		engine.CleanupPartialFile(task.OutputFilePath)
	}

	if err := h.taskRepo.BatchDelete(payload.TaskIDs); err != nil {
		WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":       true,
		"deleted_count": len(tasks),
	})
}

// HandleBatchStart batch launches pending tasks into the queued transcode schedule.
func (h *TaskHandler) HandleBatchStart(w http.ResponseWriter, r *http.Request) {
	var payload BatchStartPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		WriteJSONError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	if len(payload.TaskIDs) == 0 {
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"success":       true,
			"started_count": 0,
		})
		return
	}

	tasks, err := h.taskRepo.GetByIDs(payload.TaskIDs)
	if err != nil {
		WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var batchParams *domain.TranscodeParams
	if payload.ProfileName != "" || payload.CustomParams != nil {
		resolved, err := h.resolveTranscodeParams(payload.ProfileName, payload.CustomParams)
		if err != nil {
			WriteJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		batchParams = &resolved
	}

	startedCount := 0
	for _, task := range tasks {
		if task.Status != domain.StatusPending && task.Status != domain.StatusFailed && task.Status != domain.StatusAborted {
			continue
		}

		params := task.Params
		if batchParams != nil {
			params = *batchParams
		} else if params.ProfileName == "" || params.ProfileName == "pending" {
			p, err := h.profileRepo.GetEffectiveParams("720p")
			if err == nil && p != nil {
				params = *p
			} else {
				params = domain.TranscodeParams{
					ProfileName: "720p",
					VideoCodec:  "hevc",
					CRF:         28,
					Preset:      "medium",
				}
			}
		}

		outName := engine.GenerateOutputFileName(task.SourceFileName, params.ProfileName, task.ID, task.MediaInfo, params.SubtitlePolicy)
		task.OutputFileName = outName
		task.OutputFilePath = filepath.Join(h.outputDir, outName)
		task.Params = params
		task.Status = domain.StatusQueued
		task.ErrorMsg = ""
		task.Progress = domain.TaskProgress{}

		if err := h.taskRepo.Update(task); err == nil {
			startedCount++
		}
	}

	if startedCount > 0 {
		h.queue.NotifyNewTask()
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":       true,
		"started_count": startedCount,
	})
}

// HandleBatchRetry batch re-queues aborted or failed tasks.
func (h *TaskHandler) HandleBatchRetry(w http.ResponseWriter, r *http.Request) {
	var payload BatchIDsPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		WriteJSONError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	if len(payload.TaskIDs) == 0 {
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"success":       true,
			"retried_count": 0,
		})
		return
	}

	tasks, err := h.taskRepo.GetByIDs(payload.TaskIDs)
	if err != nil {
		WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	retriedCount := 0
	for _, task := range tasks {
		if task.Status != domain.StatusAborted && task.Status != domain.StatusFailed {
			continue
		}

		// Verify source file still exists
		if _, statErr := os.Stat(task.SourceFilePath); statErr != nil {
			continue
		}

		engine.CleanupPartialFile(task.OutputFilePath)

		task.Status = domain.StatusQueued
		task.Priority = 0
		task.CreatedAt = time.Now()
		task.ErrorMsg = ""
		task.Progress = domain.TaskProgress{}
		task.StartedAt = nil
		task.CompletedAt = nil

		if err := h.taskRepo.Update(task); err == nil {
			retriedCount++
		}
	}

	if retriedCount > 0 {
		h.queue.NotifyNewTask()
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":       true,
		"retried_count": retriedCount,
	})
}

// HandleReorderTasks updates queue priorities in bulk based on an ordered array of IDs.
func (h *TaskHandler) HandleReorderTasks(w http.ResponseWriter, r *http.Request) {
	var payload ReorderPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		WriteJSONError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	if err := h.taskRepo.BatchReorder(payload.OrderedIDs); err != nil {
		WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"count":   len(payload.OrderedIDs),
	})
}

// HandlePauseTask pauses a single queued task in the waiting queue.
func (h *TaskHandler) HandlePauseTask(w http.ResponseWriter, r *http.Request) {
	id := extractIDFromPath(r.URL.Path, "/api/v1/tasks/")
	id = strings.TrimSuffix(id, "/pause")

	task, err := h.taskRepo.GetByID(id)
	if err != nil || task == nil {
		WriteJSONError(w, http.StatusNotFound, "任务未找到")
		return
	}

	if task.Status != domain.StatusQueued {
		WriteJSONError(w, http.StatusBadRequest, fmt.Sprintf("只有排队中的任务可以暂停 (当前状态: %s)", task.Status))
		return
	}

	if err := h.taskRepo.UpdateStatus(id, domain.StatusPaused, ""); err != nil {
		WriteJSONError(w, http.StatusInternalServerError, "暂停任务失败: "+err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Task paused",
	})
}

// HandleResumeTask resumes a paused task back into the waiting queue.
func (h *TaskHandler) HandleResumeTask(w http.ResponseWriter, r *http.Request) {
	id := extractIDFromPath(r.URL.Path, "/api/v1/tasks/")
	id = strings.TrimSuffix(id, "/resume")

	task, err := h.taskRepo.GetByID(id)
	if err != nil || task == nil {
		WriteJSONError(w, http.StatusNotFound, "任务未找到")
		return
	}

	if task.Status != domain.StatusPaused {
		WriteJSONError(w, http.StatusBadRequest, fmt.Sprintf("只有已暂停的任务可以恢复排队 (当前状态: %s)", task.Status))
		return
	}

	if err := h.taskRepo.UpdateStatus(id, domain.StatusQueued, ""); err != nil {
		WriteJSONError(w, http.StatusInternalServerError, "恢复任务失败: "+err.Error())
		return
	}

	h.queue.NotifyNewTask()

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Task resumed to queue",
	})
}

// HandleBatchPause pauses multiple queued tasks in bulk.
func (h *TaskHandler) HandleBatchPause(w http.ResponseWriter, r *http.Request) {
	var payload BatchIDsPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		WriteJSONError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	pausedCount := 0
	for _, id := range payload.TaskIDs {
		task, err := h.taskRepo.GetByID(id)
		if err != nil || task == nil {
			continue
		}
		if task.Status == domain.StatusQueued {
			if err := h.taskRepo.UpdateStatus(id, domain.StatusPaused, ""); err == nil {
				pausedCount++
			}
		}
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":      true,
		"paused_count": pausedCount,
	})
}

// HandleBatchResume resumes multiple paused tasks in bulk back to queued status.
func (h *TaskHandler) HandleBatchResume(w http.ResponseWriter, r *http.Request) {
	var payload BatchIDsPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		WriteJSONError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	resumedCount := 0
	for _, id := range payload.TaskIDs {
		task, err := h.taskRepo.GetByID(id)
		if err != nil || task == nil {
			continue
		}
		if task.Status == domain.StatusPaused {
			if err := h.taskRepo.UpdateStatus(id, domain.StatusQueued, ""); err == nil {
				resumedCount++
			}
		}
	}

	if resumedCount > 0 {
		h.queue.NotifyNewTask()
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":       true,
		"resumed_count": resumedCount,
	})
}

