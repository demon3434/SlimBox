package scheduler

import (
	"log"
	"time"

	"slimbox/internal/domain"
	"slimbox/internal/engine"
	"slimbox/internal/repository"
)

type LifecycleManager struct {
	taskRepo     *repository.TaskRepository
	settingsRepo *repository.SettingsRepository
	uploadDir    string
	stopChan     chan struct{}
}

func NewLifecycleManager(
	taskRepo *repository.TaskRepository,
	settingsRepo *repository.SettingsRepository,
	uploadDirs ...string,
) *LifecycleManager {
	var uDir string
	if len(uploadDirs) > 0 {
		uDir = uploadDirs[0]
	}
	return &LifecycleManager{
		taskRepo:     taskRepo,
		settingsRepo: settingsRepo,
		uploadDir:    uDir,
		stopChan:     make(chan struct{}),
	}
}

// Start launches the periodic retention cleanup worker.
func (m *LifecycleManager) Start() {
	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-m.stopChan:
				return
			case <-ticker.C:
				m.runCleanupCycle()
			}
		}
	}()
}

// Stop shuts down the lifecycle manager.
func (m *LifecycleManager) Stop() {
	close(m.stopChan)
}

func (m *LifecycleManager) runCleanupCycle() {
	// 1. Evict abandoned chunk upload sessions older than 24 hours
	if m.uploadDir != "" {
		count, bytes := engine.CleanOrphanTempUploads(m.uploadDir, 24*time.Hour)
		if count > 0 {
			log.Printf("[Lifecycle] Periodic sweep cleaned %d abandoned chunk upload sessions (%d bytes)", count, bytes)
		}
	}

	// 2. Retention policy cleanup for completed tasks
	settings, err := m.settingsRepo.GetStorageSettings()
	if err != nil || settings.RetentionHours <= 0 {
		return
	}

	expiredTasks, err := m.taskRepo.GetExpiredCompleted(settings.RetentionHours)
	if err != nil {
		log.Printf("[Lifecycle] Error fetching expired tasks: %v", err)
		return
	}

	for _, task := range expiredTasks {
		log.Printf("[Lifecycle] Task %s exceeded retention of %d hours. Purging files...", task.ID, settings.RetentionHours)
		if task.OutputFilePath != "" {
			_ = engine.RemoveFile(task.OutputFilePath)
		}
		if task.SourceFilePath != "" {
			_ = engine.RemoveFile(task.SourceFilePath)
		}
		_ = m.taskRepo.Delete(task.ID)
	}
}

// HandleDownloadCompleted is triggered when a client successfully downloads a completed video.
func (m *LifecycleManager) HandleDownloadCompleted(task *domain.Task) {
	_ = m.taskRepo.IncrementDownloadCount(task.ID)
}

