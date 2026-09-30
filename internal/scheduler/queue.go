package scheduler

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"slimbox/internal/domain"
	"slimbox/internal/engine"
	"slimbox/internal/power"
	"slimbox/internal/repository"
)

// SerialQueue manages the strictly serialized FIFO transcoding pipeline (ADR-0002).
// Ensures exactly one FFmpeg transcode process runs at any time, protecting ARM SBCs.
type SerialQueue struct {
	taskRepo     *repository.TaskRepository
	settingsRepo *repository.SettingsRepository
	transcoder   *engine.Transcoder

	wakeChan chan struct{}
	stopChan chan struct{}

	mu            sync.Mutex
	activeTaskID  string
	activeCancel  context.CancelFunc
	activeTaskObj *domain.Task
}

func NewSerialQueue(
	taskRepo *repository.TaskRepository,
	settingsRepo *repository.SettingsRepository,
	transcoder *engine.Transcoder,
) *SerialQueue {
	return &SerialQueue{
		taskRepo:     taskRepo,
		settingsRepo: settingsRepo,
		transcoder:   transcoder,
		wakeChan:     make(chan struct{}, 1),
		stopChan:     make(chan struct{}),
	}
}

// Start launches the background worker loop.
func (q *SerialQueue) Start() {
	q.recoverZombieTasks()
	go q.workerLoop()
}

// Stop terminates the scheduler worker.
func (q *SerialQueue) Stop() {
	close(q.stopChan)
}

// NotifyNewTask wakes up the worker loop when a task is submitted to the queue.
func (q *SerialQueue) NotifyNewTask() {
	select {
	case q.wakeChan <- struct{}{}:
	default:
	}
}

// GetActiveTask returns the currently running task, if any.
func (q *SerialQueue) GetActiveTask() *domain.Task {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.activeTaskObj
}

// GetActiveTaskID returns the ID of the currently running task, if any.
func (q *SerialQueue) GetActiveTaskID() string {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.activeTaskID
}

// AbortTask cancels a running or queued task immediately (ADR-0008).
func (q *SerialQueue) AbortTask(taskID string) error {
	q.mu.Lock()
	if taskID == "active" {
		if q.activeTaskID != "" && q.activeCancel != nil {
			log.Printf("[Queue] Aborting actively running task (via alias) %s", q.activeTaskID)
			q.activeCancel()
			q.mu.Unlock()
			return nil
		}
		q.mu.Unlock()
		return fmt.Errorf("当前没有正在执行的任务")
	}

	if q.activeTaskID == taskID && q.activeCancel != nil {
		log.Printf("[Queue] Aborting actively running task %s", taskID)
		q.activeCancel()
		q.mu.Unlock()
		return nil
	}
	q.mu.Unlock()

	// If not running, check if it's queued in DB
	task, err := q.taskRepo.GetByID(taskID)
	if err != nil {
		return err
	}
	if task == nil {
		return fmt.Errorf("未找到任务 %s", taskID)
	}

	if task.Status == domain.StatusQueued || task.Status == domain.StatusPending || task.Status == domain.StatusPaused {
		return q.taskRepo.UpdateStatus(taskID, domain.StatusAborted, "Cancelled by user before processing")
	}

	if task.Status == domain.StatusTranscoding {
		log.Printf("[Queue] Aborting orphaned transcoding task %s from database", taskID)
		engine.CleanupPartialFile(task.OutputFilePath)
		return q.taskRepo.UpdateStatus(taskID, domain.StatusAborted, "Cancelled by user (interrupted task)")
	}

	if task.Status == domain.StatusCompleted {
		return fmt.Errorf("该任务已转码完成，无需终止")
	}
	if task.Status == domain.StatusAborted {
		return fmt.Errorf("该任务已被终止")
	}
	if task.Status == domain.StatusFailed {
		return fmt.Errorf("该任务已失败，无需终止")
	}

	return fmt.Errorf("无法终止状态为 %s 的任务", task.Status)
}

func (q *SerialQueue) recoverZombieTasks() {
	tasks, _, err := q.taskRepo.List(string(domain.StatusTranscoding), 100, 0)
	if err != nil {
		log.Printf("[Queue] Failed to query zombie tasks on startup: %v", err)
		return
	}
	requeuedCount := 0
	for _, t := range tasks {
		// Clean up partial .part file from the interrupted run
		engine.CleanupPartialFile(t.OutputFilePath)

		// Check if source file is intact; if so, automatically re-queue for seamless recovery
		if t.SourceFilePath != "" {
			if _, statErr := os.Stat(t.SourceFilePath); statErr == nil {
				log.Printf("[Queue] Interrupted task %s source file intact. Automatically re-queuing for transcode!", t.ID)
				t.Status = domain.StatusQueued
				t.StartedAt = nil
				t.CompletedAt = nil
				t.Progress = domain.TaskProgress{}
				t.ErrorMsg = ""
				if err := q.taskRepo.Update(t); err == nil {
					requeuedCount++
					continue
				}
			}
		}

		log.Printf("[Queue] Interrupted task %s source file missing. Resetting to aborted.", t.ID)
		_ = q.taskRepo.UpdateStatus(t.ID, domain.StatusAborted, "服务重启中断，且源文件已不存在")
	}

	if requeuedCount > 0 {
		log.Printf("[Queue] Successfully recovered and re-queued %d interrupted tasks on startup.", requeuedCount)
		q.NotifyNewTask()
	}
}

func (q *SerialQueue) workerLoop() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-q.stopChan:
			return
		case <-q.wakeChan:
			q.processNext()
		case <-ticker.C:
			q.processNext()
		}
	}
}

func (q *SerialQueue) processNext() {
	// Look for next queued task in FIFO order
	task, err := q.taskRepo.GetNextQueued()
	if err != nil {
		log.Printf("[Queue] Error retrieving next queued task: %v", err)
		return
	}
	if task == nil {
		return // No tasks waiting
	}

	ctx, cancel := context.WithCancel(context.Background())

	now := time.Now()
	task.StartedAt = &now
	task.Status = domain.StatusTranscoding

	q.mu.Lock()
	q.activeTaskID = task.ID
	q.activeCancel = cancel
	q.activeTaskObj = task
	q.mu.Unlock()

	defer func() {
		power.AllowSleep()
		q.mu.Lock()
		q.activeTaskID = ""
		q.activeCancel = nil
		q.activeTaskObj = nil
		q.mu.Unlock()
	}()

	// Verify source file presence and external storage volume accessibility
	if _, statErr := os.Stat(task.SourceFilePath); statErr != nil {
		log.Printf("[Queue] Task %s source file not accessible: %v", task.ID, statErr)
		_ = q.taskRepo.UpdateStatus(task.ID, domain.StatusFailed, fmt.Sprintf("源文件不存在或存储卷已断开: %v", statErr))
		return
	}

	if task.OutputFilePath != "" {
		outDir := filepath.Dir(task.OutputFilePath)
		if mkErr := os.MkdirAll(outDir, 0755); mkErr != nil {
			log.Printf("[Queue] Task %s output volume not accessible: %v", task.ID, mkErr)
			_ = q.taskRepo.UpdateStatus(task.ID, domain.StatusFailed, fmt.Sprintf("输出存储卷不可访问或不可写: %v", mkErr))
			return
		}
	}

	// Mark task as transcoding
	if err := q.taskRepo.UpdateStatus(task.ID, domain.StatusTranscoding, ""); err != nil {
		log.Printf("[Queue] Failed to update task status to transcoding: %v", err)
		return
	}

	power.PreventSleep(fmt.Sprintf("Transcode task %s", task.ID))

	lastProgressUpdate := time.Now()
	err = q.transcoder.Execute(ctx, task, func(prog domain.TaskProgress) {
		q.mu.Lock()
		if q.activeTaskObj != nil {
			q.activeTaskObj.Progress = prog
		}
		q.mu.Unlock()

		// Persist progress to DB periodically (every 1.5 seconds) to avoid high disk write on SD/USB
		if time.Since(lastProgressUpdate) >= 1500*time.Millisecond {
			lastProgressUpdate = time.Now()
			_ = q.taskRepo.UpdateProgress(task.ID, prog)
		}
	})

	if ctx.Err() != nil {
		// Task was canceled at runtime by user
		log.Printf("[Queue] Task %s was aborted by user", task.ID)
		_ = q.taskRepo.UpdateStatus(task.ID, domain.StatusAborted, "Cancelled by user")
		return
	}

	if err != nil {
		// ADR-0011: Task failed. Error logged, partial artifact cleaned, queue advances immediately to next!
		log.Printf("[Queue] Task %s failed: %v", task.ID, err)
		_ = q.taskRepo.UpdateStatus(task.ID, domain.StatusFailed, err.Error())
		return
	}

	// Task completed successfully
	task.Status = domain.StatusCompleted
	completedTime := time.Now()
	task.CompletedAt = &completedTime
	task.Progress.Percent = 100.0
	_ = q.taskRepo.Update(task)

	// Apply storage lifecycle policy (ADR-0004)
	settings, _ := q.settingsRepo.GetStorageSettings()
	if settings.DeleteSourceAfterTranscode && task.SourceFilePath != "" {
		log.Printf("[Lifecycle] Auto-deleting source file for task %s: %s", task.ID, task.SourceFilePath)
		_ = engine.RemoveFile(task.SourceFilePath)
	}

	// Wake immediately to check for next task in queue
	q.NotifyNewTask()
}
