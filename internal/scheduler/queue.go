package scheduler

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"slimbox/internal/domain"
	"slimbox/internal/engine"
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

// AbortTask cancels a running or queued task immediately (ADR-0008).
func (q *SerialQueue) AbortTask(taskID string) error {
	q.mu.Lock()
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
		return fmt.Errorf("task %s not found", taskID)
	}

	if task.Status == domain.StatusQueued || task.Status == domain.StatusPending {
		return q.taskRepo.UpdateStatus(taskID, domain.StatusAborted, "Cancelled by user before processing")
	}

	return fmt.Errorf("cannot abort task %s with status %s", taskID, task.Status)
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

	q.mu.Lock()
	q.activeTaskID = task.ID
	q.activeCancel = cancel
	q.activeTaskObj = task
	q.mu.Unlock()

	defer func() {
		q.mu.Lock()
		q.activeTaskID = ""
		q.activeCancel = nil
		q.activeTaskObj = nil
		q.mu.Unlock()
	}()

	// Mark task as transcoding
	if err := q.taskRepo.UpdateStatus(task.ID, domain.StatusTranscoding, ""); err != nil {
		log.Printf("[Queue] Failed to update task status to transcoding: %v", err)
		return
	}

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
	now := time.Now()
	task.CompletedAt = &now
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
